// Package inject defines the shared terminal inject/discovery contract used by
// platform backends (windows / darwin / linux) and the managed PTY fallback.
//
// Protocol alignment (gbr/1):
//   - inject payloads carry session_id + command_id + text + submit
//   - empty session_id is always refused
//   - injects are rate-limited per session to protect the foreground UI
//
// Reliability strategy:
//  1. Prefer OS UI automation (SendInput / AppleScript / xdotool) against an
//     already-visible terminal window bound to the session.
//  2. If discovery/focus fails, fall back to a managed shell session (see
//     pty_session.go) owned by the agent process — critical Day-1 path.
package inject

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Protocol / safety errors.
var (
	ErrEmptySession   = errors.New("inject: empty session_id refused")
	ErrEmptyText      = errors.New("inject: empty text refused")
	ErrRateLimited    = errors.New("inject: rate limited")
	ErrNotFound       = errors.New("inject: terminal window not found")
	ErrFocusFailed    = errors.New("inject: failed to focus target window")
	ErrInjectFailed   = errors.New("inject: send input failed")
	ErrCaptureUnavail = errors.New("inject: console capture unavailable")
	ErrSessionClosed  = errors.New("inject: session closed")
	ErrNotSupported   = errors.New("inject: operation not supported on this platform")
)

// Kind classifies a discovered terminal window (best-effort).
type Kind string

const (
	KindWindowsTerminal Kind = "windows-terminal"
	KindConhost         Kind = "conhost"
	KindPowerShell      Kind = "powershell"
	KindCmd             Kind = "cmd"
	KindUnknown         Kind = "unknown"
)

// TerminalWindow is a best-effort snapshot of a visible terminal UI.
// HWND is platform-specific (Windows HWND as uintptr; 0 on other OSes).
type TerminalWindow struct {
	HWND      uintptr
	PID       uint32
	Title     string
	ClassName string
	ExeName   string
	Kind      Kind
}

// InjectRequest is one keyboard inject attempt (maps to gbr/1 inject payload).
type InjectRequest struct {
	SessionID string
	CommandID string // idempotency key from protocol; logged/traced by callers
	Text      string
	Submit    bool // append Enter / newline when true
}

// CaptureResult is a best-effort console buffer snapshot.
type CaptureResult struct {
	Text      string
	Partial   bool   // true when capture is incomplete or heuristic
	Method    string // e.g. "readconsole", "pty", "none"
	Note      string // human-readable limitation note
}

// Injector is the platform inject surface.
// Implementations must be safe for concurrent use from the agent poll loop.
type Injector interface {
	// Discover returns visible terminal-like windows (best-effort).
	Discover() ([]TerminalWindow, error)

	// Inject focuses the window bound to sessionID (if any) and types text.
	// Must refuse empty sessionID and enforce rate limits.
	Inject(sessionID string, req InjectRequest) error

	// Capture reads console output for sessionID when possible.
	// May return ErrCaptureUnavail; callers should use managed PTY instead.
	Capture(sessionID string) (CaptureResult, error)

	// Bind associates a session_id with a discovered window (HWND/PID).
	Bind(sessionID string, win TerminalWindow) error

	// Unbind drops the session→window association.
	Unbind(sessionID string)

	// Close releases OS resources.
	Close() error
}

// Default rate-limit policy: protect interactive UI from stampeding injects.
const (
	DefaultMinInterval = 150 * time.Millisecond
	DefaultMaxBurst    = 8
	DefaultBurstWindow = time.Second
)

// RateLimiter is a per-session token / spacing guard.
type RateLimiter struct {
	mu          sync.Mutex
	minInterval time.Duration
	maxBurst    int
	burstWindow time.Duration
	// session_id → state
	state map[string]*rlState
}

type rlState struct {
	last       time.Time
	timestamps []time.Time
}

// NewRateLimiter builds a limiter. Zero values select defaults.
func NewRateLimiter(minInterval time.Duration, maxBurst int, burstWindow time.Duration) *RateLimiter {
	if minInterval <= 0 {
		minInterval = DefaultMinInterval
	}
	if maxBurst <= 0 {
		maxBurst = DefaultMaxBurst
	}
	if burstWindow <= 0 {
		burstWindow = DefaultBurstWindow
	}
	return &RateLimiter{
		minInterval: minInterval,
		maxBurst:    maxBurst,
		burstWindow: burstWindow,
		state:       make(map[string]*rlState),
	}
}

// Allow returns nil if an inject for sessionID may proceed now.
// Returns ErrEmptySession or ErrRateLimited otherwise.
func (r *RateLimiter) Allow(sessionID string) error {
	if sessionID == "" {
		return ErrEmptySession
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	st := r.state[sessionID]
	if st == nil {
		st = &rlState{}
		r.state[sessionID] = st
	}

	if !st.last.IsZero() && now.Sub(st.last) < r.minInterval {
		return fmt.Errorf("%w: min interval %s for session %q", ErrRateLimited, r.minInterval, sessionID)
	}

	// Drop timestamps outside the burst window.
	cut := now.Add(-r.burstWindow)
	kept := st.timestamps[:0]
	for _, t := range st.timestamps {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	st.timestamps = kept

	if len(st.timestamps) >= r.maxBurst {
		return fmt.Errorf("%w: max %d injects per %s for session %q", ErrRateLimited, r.maxBurst, r.burstWindow, sessionID)
	}

	st.last = now
	st.timestamps = append(st.timestamps, now)
	return nil
}

// Reset clears limiter state for a session (e.g. after Unbind).
func (r *RateLimiter) Reset(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.state, sessionID)
}

// ValidateRequest enforces shared safety rules before any platform inject.
func ValidateRequest(sessionID string, req InjectRequest) error {
	if sessionID == "" {
		return ErrEmptySession
	}
	if req.SessionID != "" && req.SessionID != sessionID {
		return fmt.Errorf("inject: session_id mismatch (%q vs %q)", sessionID, req.SessionID)
	}
	if req.Text == "" && !req.Submit {
		return ErrEmptyText
	}
	return nil
}

// EffectiveText returns the string to type, appending newline when Submit.
func EffectiveText(req InjectRequest) string {
	if req.Submit {
		if req.Text == "" {
			return "\n"
		}
		// Prefer \n; platform backends translate to VK_RETURN where needed.
		if len(req.Text) > 0 && req.Text[len(req.Text)-1] == '\n' {
			return req.Text
		}
		return req.Text + "\n"
	}
	return req.Text
}

// Default returns the platform UI-automation injector (windows/darwin/linux).
// platformInjector is defined in build-tagged files (inject_windows.go,
// default_darwin.go, default_linux.go, inject_stub.go).
// When UI automation cannot resolve a session, fall back to Manager (pty_session.go).
func Default() Injector {
	return platformInjector()
}
