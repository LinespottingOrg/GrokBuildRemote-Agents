//go:build windows

package windows

import "errors"

// Local errors (mapped to inject.* by the parent package adapter).
var (
	ErrEmptySession   = errors.New("windows inject: empty session_id refused")
	ErrEmptyText      = errors.New("windows inject: empty text refused")
	ErrRateLimited    = errors.New("windows inject: rate limited")
	ErrNotFound       = errors.New("windows inject: terminal window not found")
	ErrFocusFailed    = errors.New("windows inject: failed to focus target window")
	ErrInjectFailed   = errors.New("windows inject: send input failed")
	ErrCaptureUnavail = errors.New("windows inject: console capture unavailable")
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

// Window is a best-effort snapshot of a visible terminal UI.
type Window struct {
	HWND      uintptr
	PID       uint32
	Title     string
	ClassName string
	ExeName   string
	Kind      Kind
}

// Request is one keyboard inject attempt.
type Request struct {
	SessionID string
	CommandID string
	Text      string
	Submit    bool
}

// Capture is a best-effort console buffer snapshot.
type Capture struct {
	Text    string
	Partial bool
	Method  string
	Note    string
}
