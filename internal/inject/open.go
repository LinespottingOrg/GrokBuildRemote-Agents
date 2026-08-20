package inject

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// OpenRequest starts or attaches a Grok Build / shell session for the chain.
type OpenRequest struct {
	SessionID string
	CWD       string
	Resume    string // grok --resume <id>
	Command   string // grok | shell  (default grok)
	Title     string
	Holder    string
}

// OpenResult is what Bot API / MCP return after open/attach.
type OpenResult struct {
	SessionID string  `json:"session_id"`
	Opened    bool    `json:"opened"`
	Attached  bool    `json:"attached"`
	Method    string  `json:"method"`
	Command   string  `json:"command,omitempty"`
	CWD       string  `json:"cwd,omitempty"`
	Resume    string  `json:"resume,omitempty"`
	PID       int     `json:"pid,omitempty"`
	HWND      uintptr `json:"hwnd,omitempty"`
	Note      string  `json:"note,omitempty"`
}

// LookGrok returns the grok CLI path if installed.
func LookGrok() (string, error) {
	for _, name := range []string{"grok", "grok.exe"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("grok CLI not on PATH — install Grok Build, or open an existing session")
}

func sanitizeOpenID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 63 {
		s = s[:63]
	}
	if len(s) < 2 {
		return ""
	}
	if s[0] < 'a' && (s[0] < '0' || s[0] > '9') {
		return ""
	}
	if s == "session" {
		return ""
	}
	return s
}

func newOpenSessionID(resume string) string {
	if r := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(resume), "-", "")); len(r) >= 8 {
		id := sanitizeOpenID("grok-" + r[:8])
		if id != "" {
			return id
		}
	}
	raw := uuid.NewString()
	raw = strings.ReplaceAll(raw, "-", "")
	return "gbr-open-" + raw[:8]
}

func resolveOpenCWD(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd != "" {
		if abs, err := filepath.Abs(cwd); err == nil {
			return abs
		}
		return cwd
	}
	if w, err := os.Getwd(); err == nil && w != "" {
		return w
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// OpenOrAttach starts grok (or a shell) in a managed session, or attaches
// an already-running one with the same session_id.
func (m *Manager) OpenOrAttach(req OpenRequest) (OpenResult, error) {
	if m == nil {
		return OpenResult{}, fmt.Errorf("open: no session manager")
	}
	sid := sanitizeOpenID(req.SessionID)
	if existing := m.Get(sid); existing != nil && !existing.IsClosed() {
		return OpenResult{
			SessionID: sid,
			Attached:  true,
			Method:    "pty",
			CWD:       existing.Cwd,
			Command:   existing.Shell,
			Note:      "already managed",
		}, nil
	}
	if sid == "" {
		sid = newOpenSessionID(req.Resume)
	}
	cwd := resolveOpenCWD(req.CWD)
	cmdName := strings.ToLower(strings.TrimSpace(req.Command))
	wantGrok := cmdName == "" || cmdName == "grok" || cmdName == "grok-build"
	if wantGrok {
		bin, err := LookGrok()
		if err != nil && (cmdName == "grok" || cmdName == "grok-build") {
			return OpenResult{}, err
		}
		if err == nil {
			args := []string{}
			resume := strings.TrimSpace(req.Resume)
			if resume != "" {
				args = []string{"--resume", resume}
			}
			s, err := m.EnsureCmdTTY(sid, cwd, bin, args)
			if err != nil {
				return OpenResult{}, err
			}
			note := "spawned grok"
			if resume != "" {
				note = "spawned grok --resume"
			}
			kind := "ConPTY"
			if s.conpty == nil {
				kind = "pipes"
			}
			return OpenResult{
				SessionID: sid,
				Opened:    true,
				Method:    "pty",
				Command:   "grok",
				CWD:       cwd,
				Resume:    resume,
				PID:       s.PID(),
				Note:      note + " (" + kind + "; TUI stdin attached)",
			}, nil
		}
	}
	s, err := m.Ensure(sid, cwd)
	if err != nil {
		return OpenResult{}, err
	}
	return OpenResult{
		SessionID: sid,
		Opened:    true,
		Method:    "pty",
		Command:   "shell",
		CWD:       cwd,
		PID:       s.PID(),
		Note:      "grok CLI not on PATH — opened a managed shell. Inject `grok` or install Grok Build.",
	}, nil
}
