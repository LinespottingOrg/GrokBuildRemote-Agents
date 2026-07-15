//go:build linux

package inject

import (
	"context"
	"fmt"
	"strings"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject/linux"
)

// platformInjector returns the Linux xdotool (X11) backend (wired by inject.Default).
func platformInjector() Injector {
	return newLinuxAdapter(linux.New())
}

// PlatformName is the GOOS this binary targets for inject.
func PlatformName() string { return "linux" }

type linuxAdapter struct {
	inner *linux.Injector
}

func newLinuxAdapter(inner *linux.Injector) *linuxAdapter {
	return &linuxAdapter{inner: inner}
}

func (a *linuxAdapter) Discover() ([]TerminalWindow, error) {
	wins, err := a.inner.Discover()
	if err != nil {
		return nil, err
	}
	out := make([]TerminalWindow, 0, len(wins))
	for _, w := range wins {
		out = append(out, TerminalWindow{
			HWND:      w.HWND,
			PID:       w.PID,
			Title:     w.Title,
			ClassName: w.ClassName,
			ExeName:   w.ExeName,
			Kind:      KindUnknown,
		})
	}
	return out, nil
}

func (a *linuxAdapter) Inject(sessionID string, req InjectRequest) error {
	if err := ValidateRequest(sessionID, req); err != nil {
		return err
	}
	ctx := context.Background()
	if err := a.inner.Inject(ctx, sessionID, req.Text, req.Submit); err != nil {
		return mapLinuxErr(err)
	}
	return nil
}

func (a *linuxAdapter) Capture(sessionID string) (CaptureResult, error) {
	if sessionID == "" {
		return CaptureResult{}, ErrEmptySession
	}
	body, err := a.inner.Capture(context.Background(), sessionID)
	if err != nil {
		return CaptureResult{
			Partial: true,
			Method:  "none",
			Note:    err.Error(),
		}, mapLinuxErr(err)
	}
	return CaptureResult{
		Text:    body,
		Partial: true,
		Method:  "clipboard",
		Note:    "fragile select-all+copy when enabled",
	}, nil
}

func (a *linuxAdapter) Bind(sessionID string, win TerminalWindow) error {
	id := ""
	if win.HWND != 0 {
		id = fmt.Sprintf("%d", uint64(win.HWND))
	}
	return a.inner.Bind(sessionID, linux.Window{
		HWND:      win.HWND,
		PID:       win.PID,
		Title:     win.Title,
		ClassName: win.ClassName,
		ExeName:   win.ExeName,
		ID:        id,
	})
}

func (a *linuxAdapter) Unbind(sessionID string) { a.inner.Unbind(sessionID) }

func (a *linuxAdapter) Close() error { return a.inner.Close() }

func mapLinuxErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "empty session"):
		return fmt.Errorf("%w: %v", ErrEmptySession, err)
	case strings.Contains(msg, "empty text"):
		return fmt.Errorf("%w: %v", ErrEmptyText, err)
	case strings.Contains(msg, "rate limited"):
		return fmt.Errorf("%w: %v", ErrRateLimited, err)
	case strings.Contains(msg, "no window"), strings.Contains(msg, "no X window"), strings.Contains(msg, "matches"):
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	case strings.Contains(msg, "DISPLAY"), strings.Contains(msg, "Wayland"), strings.Contains(msg, "xdotool not found"):
		return fmt.Errorf("%w: %v", ErrNotSupported, err)
	case strings.Contains(msg, "type"), strings.Contains(msg, "Return"):
		return fmt.Errorf("%w: %v", ErrInjectFailed, err)
	case strings.Contains(msg, "capture"), strings.Contains(msg, "scrollback"), strings.Contains(msg, "clipboard"):
		return fmt.Errorf("%w: %v", ErrCaptureUnavail, err)
	default:
		return err
	}
}

var _ Injector = (*linuxAdapter)(nil)
