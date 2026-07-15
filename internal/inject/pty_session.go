package inject

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// ManagedSession is a shell process owned by the agent, keyed by session_id.
//
// This is the reliability fallback when UI automation (SendInput / AppleScript /
// xdotool) cannot discover or focus a real terminal window. Day-1 capture is
// also far more reliable here than AttachConsole/ReadConsoleOutput against a
// foreign Windows Terminal / ConPTY host.
//
// Notes:
//   - Uses stdin/stdout/stderr pipes (not a full pseudo-TTY). Full-screen TUI
//     apps (vim, htop) may misbehave; CLI tools and shells work well.
//   - On Windows the default shell is pwsh (fallback powershell, then cmd).
//   - On Unix the default shell is bash (fallback sh).
//   - Empty session_id is refused. Writes are rate-limited via RateLimiter.
type ManagedSession struct {
	SessionID string
	Shell     string
	Args      []string
	Cwd       string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	mu     sync.Mutex
	closed bool
	// Ring of recent output for Capture().
	buf       []byte
	bufMax    int
	outDone   chan struct{}
	startOnce sync.Once
	startErr  error
}

// Manager owns many ManagedSessions and applies shared safety policy.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*ManagedSession
	limiter  *RateLimiter
	// Shell override; empty → platform default.
	Shell string
	Args  []string
	// Output ring size per session (bytes). Default 256 KiB.
	BufMax int
}

// NewManager creates an empty session manager.
func NewManager(limiter *RateLimiter) *Manager {
	if limiter == nil {
		limiter = NewRateLimiter(0, 0, 0)
	}
	return &Manager{
		sessions: make(map[string]*ManagedSession),
		limiter:  limiter,
		BufMax:   256 * 1024,
	}
}

// DefaultShell returns a platform-appropriate interactive shell argv.
func DefaultShell() (string, []string) {
	switch runtime.GOOS {
	case "windows":
		// Prefer PowerShell 7, then Windows PowerShell, then cmd.
		for _, cand := range []struct {
			path string
			args []string
		}{
			{"pwsh.exe", []string{"-NoLogo", "-NoExit"}},
			{"powershell.exe", []string{"-NoLogo", "-NoExit"}},
			{"cmd.exe", []string{"/K"}},
		} {
			if p, err := exec.LookPath(cand.path); err == nil {
				return p, cand.args
			}
		}
		return "cmd.exe", []string{"/K"}
	default:
		if p, err := exec.LookPath("bash"); err == nil {
			return p, []string{"-i"}
		}
		return "sh", []string{"-i"}
	}
}

// Get returns an existing managed session, or nil.
func (m *Manager) Get(sessionID string) *ManagedSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[sessionID]
}

// Ensure starts (or returns) a managed shell for sessionID.
func (m *Manager) Ensure(sessionID, cwd string) (*ManagedSession, error) {
	if sessionID == "" {
		return nil, ErrEmptySession
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok && !s.IsClosed() {
		return s, nil
	}

	shell := m.Shell
	args := m.Args
	if shell == "" {
		shell, args = DefaultShell()
	}
	bufMax := m.BufMax
	if bufMax <= 0 {
		bufMax = 256 * 1024
	}

	s := &ManagedSession{
		SessionID: sessionID,
		Shell:     shell,
		Args:      append([]string(nil), args...),
		Cwd:       cwd,
		bufMax:    bufMax,
		outDone:   make(chan struct{}),
	}
	if err := s.Start(); err != nil {
		return nil, err
	}
	m.sessions[sessionID] = s
	return s, nil
}

// Inject writes text into the managed session (rate-limited).
// Creates the session on demand when missing.
func (m *Manager) Inject(sessionID string, req InjectRequest) error {
	if err := ValidateRequest(sessionID, req); err != nil {
		return err
	}
	if err := m.limiter.Allow(sessionID); err != nil {
		return err
	}
	s, err := m.Ensure(sessionID, "")
	if err != nil {
		return err
	}
	return s.Write(EffectiveText(req))
}

// Capture returns the recent output ring for sessionID.
func (m *Manager) Capture(sessionID string) (CaptureResult, error) {
	if sessionID == "" {
		return CaptureResult{}, ErrEmptySession
	}
	s := m.Get(sessionID)
	if s == nil {
		return CaptureResult{
			Partial: true,
			Method:  "pty",
			Note:    "no managed session for id",
		}, ErrNotFound
	}
	text := s.Snapshot()
	return CaptureResult{
		Text:    text,
		Partial: false,
		Method:  "pty",
		Note:    "pipe-backed managed shell (not a full PTY); suitable for CLI output",
	}, nil
}

// CloseSession stops one managed session.
func (m *Manager) CloseSession(sessionID string) error {
	m.mu.Lock()
	s := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	m.limiter.Reset(sessionID)
	if s == nil {
		return nil
	}
	return s.Close()
}

// Close stops every managed session.
func (m *Manager) Close() error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	var first error
	for _, id := range ids {
		if err := m.CloseSession(id); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// List returns active managed session IDs.
func (m *Manager) List() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		out = append(out, id)
	}
	return out
}

// Start launches the shell process and begins output collection.
func (s *ManagedSession) Start() error {
	s.startOnce.Do(func() {
		s.startErr = s.start()
	})
	return s.startErr
}

func (s *ManagedSession) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}

	cmd := exec.Command(s.Shell, s.Args...)
	if s.Cwd != "" {
		cmd.Dir = s.Cwd
	}
	// Inherit a minimal environment; mark interactive-ish.
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("inject/pty: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("inject/pty: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("inject/pty: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return fmt.Errorf("inject/pty: start %s: %w", s.Shell, err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.stdout = stdout
	s.stderr = stderr

	go s.collect(stdout, stderr)
	return nil
}

func (s *ManagedSession) collect(stdout, stderr io.Reader) {
	defer close(s.outDone)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.pump(stdout)
	}()
	go func() {
		defer wg.Done()
		s.pump(stderr)
	}()
	wg.Wait()
	// Reap process.
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Wait()
	}
}

func (s *ManagedSession) pump(r io.Reader) {
	br := bufio.NewReader(r)
	tmp := make([]byte, 4096)
	for {
		n, err := br.Read(tmp)
		if n > 0 {
			s.appendBuf(tmp[:n])
		}
		if err != nil {
			return
		}
	}
}

func (s *ManagedSession) appendBuf(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	if len(s.buf) > s.bufMax {
		// Keep the tail.
		excess := len(s.buf) - s.bufMax
		s.buf = append([]byte(nil), s.buf[excess:]...)
	}
}

// Write sends raw bytes to the shell stdin.
func (s *ManagedSession) Write(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.stdin == nil {
		return ErrSessionClosed
	}
	_, err := io.WriteString(s.stdin, text)
	return err
}

// Snapshot returns a copy of the output ring.
func (s *ManagedSession) Snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.buf)
}

// IsClosed reports whether Close was called.
func (s *ManagedSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Close terminates the shell process.
// Process reaping is owned by the collect goroutine (single cmd.Wait).
func (s *ManagedSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	stdin := s.stdin
	cmd := s.cmd
	s.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		// Ask nicely, then kill if collectors do not finish.
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-s.outDone:
			return nil
		case <-time.After(1500 * time.Millisecond):
			_ = cmd.Process.Kill()
		}
	}
	select {
	case <-s.outDone:
	case <-time.After(500 * time.Millisecond):
	}
	return nil
}
