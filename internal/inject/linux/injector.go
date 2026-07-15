//go:build linux

package linux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Injector types into X11 terminal windows via xdotool.
// Methods match the core inject contract without importing package inject.
type Injector struct {
	mu    sync.Mutex
	bound map[string]Window

	ClassHints            []string
	TypeDelayMs           int
	FocusPause            time.Duration
	OpTimeout             time.Duration
	AllowClipboardCapture bool

	MinInterval time.Duration
	MaxBurst    int
	BurstWindow time.Duration
	rl          *rateLimiter
}

// New returns a linux Injector with defaults.
func New() *Injector {
	inj := &Injector{
		bound:       make(map[string]Window),
		TypeDelayMs: 1,
		FocusPause:  50 * time.Millisecond,
		OpTimeout:   8 * time.Second,
		MinInterval: 150 * time.Millisecond,
		MaxBurst:    8,
		BurstWindow: time.Second,
	}
	inj.rl = newRateLimiter(inj.MinInterval, inj.MaxBurst, inj.BurstWindow)
	return inj
}

func (inj *Injector) ctx() (context.Context, context.CancelFunc) {
	d := inj.OpTimeout
	if d <= 0 {
		d = 8 * time.Second
	}
	return context.WithTimeout(context.Background(), d)
}

// Discover lists terminal-like X11 windows.
func (inj *Injector) Discover() ([]Window, error) {
	ctx, cancel := inj.ctx()
	defer cancel()
	if err := inj.checkEnv(ctx); err != nil {
		return nil, err
	}
	wins, err := ListWindows(ctx, inj.ClassHints)
	if err != nil {
		return nil, fmt.Errorf("linux discover: %w", err)
	}
	out := make([]Window, 0, len(wins))
	for _, w := range wins {
		out = append(out, winToWindow(w))
	}
	return out, nil
}

// Bind associates sessionID with a window.
func (inj *Injector) Bind(sessionID string, win Window) error {
	if sessionID == "" {
		return fmt.Errorf("linux inject: empty session_id")
	}
	if win.HWND == 0 && win.PID == 0 && strings.TrimSpace(win.Title) == "" && win.ID == "" {
		return fmt.Errorf("linux inject: bind requires HWND, PID, Title, or ID")
	}
	inj.mu.Lock()
	defer inj.mu.Unlock()
	if inj.bound == nil {
		inj.bound = make(map[string]Window)
	}
	inj.bound[sessionID] = win
	return nil
}

// Unbind drops the session association.
func (inj *Injector) Unbind(sessionID string) {
	inj.mu.Lock()
	delete(inj.bound, sessionID)
	inj.mu.Unlock()
	if inj.rl != nil {
		inj.rl.reset(sessionID)
	}
}

// Inject types text into the session window. If submit is true, sends Return after.
func (inj *Injector) Inject(ctx context.Context, sessionID, text string, submit bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if sessionID == "" {
		return fmt.Errorf("linux inject: empty session_id")
	}
	if text == "" && !submit {
		return fmt.Errorf("linux inject: empty text")
	}
	if inj.rl != nil {
		if err := inj.rl.allow(sessionID); err != nil {
			return err
		}
	}

	opCtx, cancel := inj.ctx()
	defer cancel()
	ctx, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	go func() {
		select {
		case <-opCtx.Done():
			cancel2()
		case <-ctx.Done():
		}
	}()

	if err := inj.checkEnv(ctx); err != nil {
		return err
	}

	win, err := inj.resolveWindow(ctx, sessionID)
	if err != nil {
		return err
	}
	if win.ID == "" {
		return fmt.Errorf("linux inject: no X window id for session %q", sessionID)
	}

	if _, err := runXdotool(ctx, "windowactivate", "--sync", win.ID); err != nil {
		_ = err
	}
	if inj.FocusPause > 0 {
		if err := sleepCtx(ctx, inj.FocusPause); err != nil {
			return err
		}
	}

	if text != "" {
		parts := strings.Split(text, "\n")
		for i, part := range parts {
			part = strings.TrimSuffix(part, "\r")
			if part != "" {
				args := []string{"type", "--window", win.ID}
				if inj.TypeDelayMs > 0 {
					args = append(args, "--delay", fmt.Sprintf("%d", inj.TypeDelayMs))
				}
				args = append(args, "--", part)
				if _, err := runXdotool(ctx, args...); err != nil {
					return fmt.Errorf("linux inject: type into window %s (%q): %w", win.ID, win.Title, err)
				}
			}
			if i < len(parts)-1 {
				if _, err := runXdotool(ctx, "key", "--window", win.ID, "Return"); err != nil {
					return fmt.Errorf("linux inject: newline Return: %w", err)
				}
			}
		}
	}

	if submit {
		if _, err := runXdotool(ctx, "key", "--window", win.ID, "Return"); err != nil {
			return fmt.Errorf("linux inject: submit Return: %w", err)
		}
	}
	return nil
}

// Capture best-effort; refuses unless AllowClipboardCapture is set.
func (inj *Injector) Capture(ctx context.Context, sessionID string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if sessionID == "" {
		return "", fmt.Errorf("linux capture: empty session_id")
	}

	opCtx, cancel := inj.ctx()
	defer cancel()
	ctx, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	go func() {
		select {
		case <-opCtx.Done():
			cancel2()
		case <-ctx.Done():
		}
	}()

	if err := inj.checkEnv(ctx); err != nil {
		return "", err
	}

	win, err := inj.resolveWindow(ctx, sessionID)
	if err != nil {
		return "", err
	}

	if !inj.AllowClipboardCapture {
		return "", fmt.Errorf("linux capture: scrollback not available via X11 for window %s (%q); enable AllowClipboardCapture or use PTY", win.ID, win.Title)
	}
	return captureViaClipboard(ctx, win.ID)
}

// Close releases bound state.
func (inj *Injector) Close() error {
	inj.mu.Lock()
	inj.bound = make(map[string]Window)
	inj.mu.Unlock()
	return nil
}

func (inj *Injector) checkEnv(ctx context.Context) error {
	if sessionIsWayland() && !displayEnvOK() {
		return fmt.Errorf("linux inject: Wayland session without DISPLAY — xdotool requires X11/XWayland (see package docs)")
	}
	if !displayEnvOK() {
		return fmt.Errorf("linux inject: DISPLAY is unset — cannot talk to X11")
	}
	return ensureXdotool(ctx)
}

func (inj *Injector) boundWindow(sessionID string) (Window, bool) {
	inj.mu.Lock()
	defer inj.mu.Unlock()
	w, ok := inj.bound[sessionID]
	return w, ok
}

func (inj *Injector) resolveWindow(ctx context.Context, sessionID string) (Window, error) {
	if w, ok := inj.boundWindow(sessionID); ok {
		if w.ID == "" {
			w.ID = windowIDFromHWND(w.HWND)
		}
		if w.ID != "" {
			return w, nil
		}
		if w.Title != "" {
			if direct, err := SearchBySession(ctx, w.Title); err == nil && len(direct) > 0 {
				return winToWindow(direct[0]), nil
			}
		}
	}

	if direct, err := SearchBySession(ctx, sessionID); err == nil && len(direct) == 1 {
		ww := winToWindow(direct[0])
		_ = inj.Bind(sessionID, ww)
		return ww, nil
	} else if err == nil && len(direct) > 1 {
		if w, ok := matchWindow(direct, sessionID); ok && strings.EqualFold(w.Name, sessionID) {
			ww := winToWindow(w)
			_ = inj.Bind(sessionID, ww)
			return ww, nil
		}
		return Window{}, fmt.Errorf("linux inject: session %q matches %d windows; bind explicitly", sessionID, len(direct))
	}

	wins, err := ListWindows(ctx, inj.ClassHints)
	if err != nil {
		return Window{}, fmt.Errorf("linux inject: list windows: %w", err)
	}
	if w, ok := matchWindow(wins, sessionID); ok {
		ww := winToWindow(w)
		_ = inj.Bind(sessionID, ww)
		return ww, nil
	}

	if sessionIsWayland() {
		return Window{}, fmt.Errorf("linux inject: no X11 window matching session %q (Wayland active — target may not be on XWayland; xdotool cannot inject into pure Wayland surfaces)", sessionID)
	}
	return Window{}, fmt.Errorf("linux inject: no window matching session %q (%d terminal-like windows scanned)", sessionID, len(wins))
}

func captureViaClipboard(ctx context.Context, windowID string) (string, error) {
	if _, err := runXdotool(ctx, "windowactivate", "--sync", windowID); err != nil {
		return "", err
	}
	if _, err := runXdotool(ctx, "key", "--window", windowID, "ctrl+shift+a"); err != nil {
		return "", fmt.Errorf("select all: %w", err)
	}
	if err := sleepCtx(ctx, 30*time.Millisecond); err != nil {
		return "", err
	}
	if _, err := runXdotool(ctx, "key", "--window", windowID, "ctrl+shift+c"); err != nil {
		return "", fmt.Errorf("copy: %w", err)
	}
	if err := sleepCtx(ctx, 50*time.Millisecond); err != nil {
		return "", err
	}

	out, err := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-o").Output()
	if err != nil {
		out, err = exec.CommandContext(ctx, "xsel", "--clipboard", "--output").Output()
		if err != nil {
			return "", fmt.Errorf("clipboard read failed (install xclip or xsel): %w", err)
		}
	}
	return string(out), nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// --- internal rate limiter ---

type rateLimiter struct {
	mu          sync.Mutex
	minInterval time.Duration
	maxBurst    int
	burstWindow time.Duration
	state       map[string]*rlState
}

type rlState struct {
	last       time.Time
	timestamps []time.Time
}

func newRateLimiter(minInterval time.Duration, maxBurst int, burstWindow time.Duration) *rateLimiter {
	if minInterval <= 0 {
		minInterval = 150 * time.Millisecond
	}
	if maxBurst <= 0 {
		maxBurst = 8
	}
	if burstWindow <= 0 {
		burstWindow = time.Second
	}
	return &rateLimiter{
		minInterval: minInterval,
		maxBurst:    maxBurst,
		burstWindow: burstWindow,
		state:       make(map[string]*rlState),
	}
}

func (r *rateLimiter) allow(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("linux inject: empty session_id")
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
		return fmt.Errorf("linux inject: rate limited (min interval %s) for session %q", r.minInterval, sessionID)
	}
	cut := now.Add(-r.burstWindow)
	kept := st.timestamps[:0]
	for _, t := range st.timestamps {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	st.timestamps = kept
	if len(st.timestamps) >= r.maxBurst {
		return fmt.Errorf("linux inject: rate limited (max %d per %s) for session %q", r.maxBurst, r.burstWindow, sessionID)
	}
	st.last = now
	st.timestamps = append(st.timestamps, now)
	return nil
}

func (r *rateLimiter) reset(sessionID string) {
	r.mu.Lock()
	delete(r.state, sessionID)
	r.mu.Unlock()
}
