//go:build windows

package inject

import (
	"errors"
	"fmt"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject/windows"
)

// platformInjector returns the Windows SendInput + EnumWindows backend.
func platformInjector() Injector {
	return &windowsAdapter{inner: windows.New()}
}

// windowsAdapter maps windows.* types onto inject.Injector without an import
// cycle (windows does not import inject).
type windowsAdapter struct {
	inner *windows.Injector
}

func (a *windowsAdapter) Discover() ([]TerminalWindow, error) {
	wins, err := a.inner.Discover()
	if err != nil {
		return nil, mapWinErr(err)
	}
	out := make([]TerminalWindow, len(wins))
	for i, w := range wins {
		out[i] = winToTerminal(w)
	}
	return out, nil
}

func (a *windowsAdapter) Inject(sessionID string, req InjectRequest) error {
	if err := ValidateRequest(sessionID, req); err != nil {
		return err
	}
	return mapWinErr(a.inner.Inject(sessionID, windows.Request{
		SessionID: sessionID,
		CommandID: req.CommandID,
		Text:      req.Text,
		Submit:    req.Submit,
	}))
}

func (a *windowsAdapter) Capture(sessionID string) (CaptureResult, error) {
	if sessionID == "" {
		return CaptureResult{}, ErrEmptySession
	}
	c, err := a.inner.Capture(sessionID)
	res := CaptureResult{
		Text:    c.Text,
		Partial: c.Partial,
		Method:  c.Method,
		Note:    c.Note,
	}
	return res, mapWinErr(err)
}

func (a *windowsAdapter) Bind(sessionID string, win TerminalWindow) error {
	return mapWinErr(a.inner.Bind(sessionID, terminalToWin(win)))
}

func (a *windowsAdapter) Unbind(sessionID string) {
	a.inner.Unbind(sessionID)
}

func (a *windowsAdapter) Close() error {
	return a.inner.Close()
}

func winToTerminal(w windows.Window) TerminalWindow {
	return TerminalWindow{
		HWND:      w.HWND,
		PID:       w.PID,
		Title:     w.Title,
		ClassName: w.ClassName,
		ExeName:   w.ExeName,
		Kind:      Kind(w.Kind),
	}
}

func terminalToWin(w TerminalWindow) windows.Window {
	return windows.Window{
		HWND:      w.HWND,
		PID:       w.PID,
		Title:     w.Title,
		ClassName: w.ClassName,
		ExeName:   w.ExeName,
		Kind:      windows.Kind(w.Kind),
	}
}

func mapWinErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, windows.ErrEmptySession):
		return fmt.Errorf("%w: %v", ErrEmptySession, err)
	case errors.Is(err, windows.ErrEmptyText):
		return fmt.Errorf("%w: %v", ErrEmptyText, err)
	case errors.Is(err, windows.ErrRateLimited):
		return fmt.Errorf("%w: %v", ErrRateLimited, err)
	case errors.Is(err, windows.ErrNotFound):
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	case errors.Is(err, windows.ErrFocusFailed):
		return fmt.Errorf("%w: %v", ErrFocusFailed, err)
	case errors.Is(err, windows.ErrInjectFailed):
		return fmt.Errorf("%w: %v", ErrInjectFailed, err)
	case errors.Is(err, windows.ErrCaptureUnavail):
		return fmt.Errorf("%w: %v", ErrCaptureUnavail, err)
	default:
		return err
	}
}

var _ Injector = (*windowsAdapter)(nil)
