//go:build darwin

package inject

import (
	"context"
	"fmt"
	"strings"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject/darwin"
)

// platformInjector returns the macOS AppleScript backend (wired by inject.Default).
func platformInjector() Injector {
	return newDarwinAdapter(darwin.New())
}

// PlatformName is the GOOS this binary targets for inject.
func PlatformName() string { return "darwin" }

type darwinAdapter struct {
	inner *darwin.Injector
}

func newDarwinAdapter(inner *darwin.Injector) *darwinAdapter {
	return &darwinAdapter{inner: inner}
}

func (a *darwinAdapter) Discover() ([]TerminalWindow, error) {
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

func (a *darwinAdapter) Inject(sessionID string, req InjectRequest) error {
	if err := ValidateRequest(sessionID, req); err != nil {
		return err
	}
	ctx := context.Background()
	if err := a.inner.Inject(ctx, sessionID, req.Text, req.Submit); err != nil {
		return mapDarwinErr(err)
	}
	return nil
}

func (a *darwinAdapter) Capture(sessionID string) (CaptureResult, error) {
	if sessionID == "" {
		return CaptureResult{}, ErrEmptySession
	}
	body, err := a.inner.Capture(context.Background(), sessionID)
	if err != nil {
		return CaptureResult{
			Partial: true,
			Method:  "applescript",
			Note:    err.Error(),
		}, mapDarwinErr(err)
	}
	return CaptureResult{
		Text:    body,
		Partial: true,
		Method:  "applescript",
		Note:    "best-effort AppleScript buffer; full scrollback not guaranteed",
	}, nil
}

func (a *darwinAdapter) Bind(sessionID string, win TerminalWindow) error {
	return a.inner.Bind(sessionID, darwin.Window{
		HWND:      win.HWND,
		PID:       win.PID,
		Title:     win.Title,
		ClassName: win.ClassName,
		ExeName:   win.ExeName,
		App:       classToApp(win.ClassName, win.ExeName),
	})
}

func (a *darwinAdapter) Unbind(sessionID string) { a.inner.Unbind(sessionID) }

func (a *darwinAdapter) Close() error { return a.inner.Close() }

func classToApp(class, exe string) string {
	if strings.EqualFold(exe, "iTerm2") || strings.EqualFold(class, "iTerm") || strings.EqualFold(class, "iTerm2") {
		return "iTerm"
	}
	return "Terminal"
}

func mapDarwinErr(err error) error {
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
	case strings.Contains(msg, "no tab matching"), strings.Contains(msg, "list tabs"):
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	case strings.Contains(msg, "focus"), strings.Contains(msg, "activate"):
		return fmt.Errorf("%w: %v", ErrFocusFailed, err)
	case strings.Contains(msg, "Accessibility"), strings.Contains(msg, "type:"):
		return fmt.Errorf("%w: %v", ErrInjectFailed, err)
	case strings.Contains(msg, "capture"), strings.Contains(msg, "history"), strings.Contains(msg, "contents"):
		return fmt.Errorf("%w: %v", ErrCaptureUnavail, err)
	default:
		return err
	}
}

var _ Injector = (*darwinAdapter)(nil)
