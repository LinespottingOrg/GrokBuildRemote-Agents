//go:build darwin

package darwin

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Injector types into macOS Terminal.app / iTerm2 via osascript keystroke.
// Methods match the core inject contract without importing package inject
// (adapters in package inject bridge types and rate limits).
type Injector struct {
	mu    sync.Mutex
	bound map[string]Window

	// PreferApp forces "Terminal" or "iTerm" when non-empty.
	PreferApp string
	// ActivatePause is delay after activate before keystrokes (default 80ms).
	ActivatePause time.Duration
	// OpTimeout bounds each osascript invocation (default 8s).
	OpTimeout time.Duration

	// MinInterval / MaxBurst / BurstWindow configure an internal rate limiter.
	MinInterval time.Duration
	MaxBurst    int
	BurstWindow time.Duration
	rl          *rateLimiter
}

// New returns a darwin Injector with defaults.
func New() *Injector {
	inj := &Injector{
		bound:         make(map[string]Window),
		ActivatePause: 80 * time.Millisecond,
		OpTimeout:     8 * time.Second,
		MinInterval:   150 * time.Millisecond,
		MaxBurst:      8,
		BurstWindow:   time.Second,
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

// Discover lists Terminal/iTerm tabs best-effort.
func (inj *Injector) Discover() ([]Window, error) {
	ctx, cancel := inj.ctx()
	defer cancel()
	tabs, err := ListTabs(ctx)
	if err != nil {
		return nil, fmt.Errorf("darwin discover: %w", err)
	}
	out := make([]Window, 0, len(tabs))
	for _, t := range tabs {
		out = append(out, tabToWindow(t))
	}
	return out, nil
}

// Bind associates sessionID with a discovered window/tab.
func (inj *Injector) Bind(sessionID string, win Window) error {
	if sessionID == "" {
		return fmt.Errorf("darwin inject: empty session_id")
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

// Inject types text into the session terminal. If submit is true, sends Return after.
// This is the primary platform entrypoint (ctx-aware).
func (inj *Injector) Inject(ctx context.Context, sessionID, text string, submit bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if sessionID == "" {
		return fmt.Errorf("darwin inject: empty session_id")
	}
	if text == "" && !submit {
		return fmt.Errorf("darwin inject: empty text")
	}
	if inj.rl != nil {
		if err := inj.rl.allow(sessionID); err != nil {
			return err
		}
	}

	// Merge timeout from injector if parent ctx has none.
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

	tab, app, err := inj.resolveTab(ctx, sessionID)
	if err != nil {
		return err
	}

	if err := activateApp(ctx, appName(app)); err != nil {
		return fmt.Errorf("darwin inject: focus/activate %s: %w (check Privacy → Automation)", app, err)
	}
	pause := inj.ActivatePause
	if pause <= 0 {
		pause = 80 * time.Millisecond
	}
	if err := sleepCtx(ctx, pause); err != nil {
		return err
	}
	if tab.WindowID != "" && tab.Index > 0 {
		if err := selectTab(ctx, tab); err != nil {
			_ = err
		} else if err := sleepCtx(ctx, pause); err != nil {
			return err
		}
	}

	if err := typeText(ctx, text); err != nil {
		return fmt.Errorf("darwin inject: type: %w (check Privacy → Accessibility)", err)
	}
	if submit {
		if err := keyReturn(ctx); err != nil {
			return fmt.Errorf("darwin inject: submit: %w", err)
		}
	}
	return nil
}

// Capture returns best-effort tab history/contents for sessionID.
func (inj *Injector) Capture(ctx context.Context, sessionID string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if sessionID == "" {
		return "", fmt.Errorf("darwin capture: empty session_id")
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

	tab, _, err := inj.resolveTab(ctx, sessionID)
	if err != nil {
		return "", err
	}

	switch tab.App {
	case "iTerm":
		return captureITerm(ctx, tab)
	default:
		return captureTerminal(ctx, tab)
	}
}

// Close releases bound state.
func (inj *Injector) Close() error {
	inj.mu.Lock()
	inj.bound = make(map[string]Window)
	inj.mu.Unlock()
	return nil
}

func (inj *Injector) boundWindow(sessionID string) (Window, bool) {
	inj.mu.Lock()
	defer inj.mu.Unlock()
	w, ok := inj.bound[sessionID]
	return w, ok
}

func (inj *Injector) resolveTab(ctx context.Context, sessionID string) (TabInfo, string, error) {
	if w, ok := inj.boundWindow(sessionID); ok {
		return tabFromWindow(w), firstNonEmpty(w.App, inj.PreferApp, "Terminal"), nil
	}

	tabs, err := ListTabs(ctx)
	if err != nil {
		return TabInfo{}, "", fmt.Errorf("darwin inject: list tabs: %w", err)
	}
	if tab, ok := matchTab(tabs, sessionID); ok {
		_ = inj.Bind(sessionID, tabToWindow(tab))
		return tab, tab.App, nil
	}
	return TabInfo{}, "", fmt.Errorf("darwin inject: no tab matching session %q (bind a tab or use managed PTY; check Accessibility)", sessionID)
}

func tabFromWindow(w Window) TabInfo {
	app := w.App
	if app == "" {
		switch {
		case strings.EqualFold(w.ExeName, "iTerm2"), strings.EqualFold(w.ClassName, "iTerm"):
			app = "iTerm"
		default:
			app = "Terminal"
		}
	}
	wi, ti := decodeTabRef(w.HWND)
	if w.WindowID != "" {
		// prefer stored
	} else if wi > 0 {
		w.WindowID = fmt.Sprintf("%d", wi)
	}
	if w.TabIndex > 0 {
		ti = w.TabIndex
	}
	title := w.Title
	if i := strings.LastIndex(title, " ["); i >= 0 && strings.HasSuffix(title, "]") {
		title = title[:i]
	}
	return TabInfo{
		App:      app,
		WindowID: w.WindowID,
		Index:    ti,
		Title:    title,
		TTY:      w.TTY,
		PID:      w.PID,
	}
}

func appName(app string) string {
	switch strings.ToLower(app) {
	case "iterm", "iterm2":
		return "iTerm"
	default:
		return "Terminal"
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func selectTab(ctx context.Context, tab TabInfo) error {
	switch tab.App {
	case "Terminal":
		if tab.WindowID == "" || tab.Index <= 0 {
			return nil
		}
		script := fmt.Sprintf(`
tell application "Terminal"
	activate
	set w to window %s
	set selected of tab %d of w to true
	set frontmost of w to true
end tell
`, tab.WindowID, tab.Index)
		_, err := runOSAscript(ctx, script)
		return err
	case "iTerm":
		if tab.WindowID == "" || tab.Index <= 0 {
			return nil
		}
		script := fmt.Sprintf(`
tell application "iTerm"
	activate
	tell window %s
		select tab %d
	end tell
end tell
`, tab.WindowID, tab.Index)
		_, err := runOSAscript(ctx, script)
		return err
	default:
		return nil
	}
}

func typeText(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			if keystrokeSafe(line) {
				if err := keystroke(ctx, line); err != nil {
					return err
				}
			} else {
				if err := pasteViaClipboard(ctx, line); err != nil {
					return err
				}
			}
		}
		if i < len(lines)-1 {
			if err := keyReturn(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func keystroke(ctx context.Context, s string) error {
	script := fmt.Sprintf(`
tell application "System Events"
	keystroke "%s"
end tell
`, escapeAS(s))
	_, err := runOSAscript(ctx, script)
	return err
}

func keyReturn(ctx context.Context) error {
	const script = `
tell application "System Events"
	key code 36
end tell
`
	_, err := runOSAscript(ctx, script)
	return err
}

func pasteViaClipboard(ctx context.Context, s string) error {
	prev, _ := runOSAscript(ctx, `try
	the clipboard as text
on error
	return ""
end try`)
	if _, err := runOSAscript(ctx, fmt.Sprintf(`set the clipboard to "%s"`, escapeAS(s))); err != nil {
		return fmt.Errorf("clipboard set: %w", err)
	}
	const paste = `
tell application "System Events"
	keystroke "v" using command down
end tell
`
	if _, err := runOSAscript(ctx, paste); err != nil {
		return fmt.Errorf("clipboard paste: %w", err)
	}
	if prev != "" {
		_, _ = runOSAscript(ctx, fmt.Sprintf(`set the clipboard to "%s"`, escapeAS(prev)))
	}
	return nil
}

func captureTerminal(ctx context.Context, tab TabInfo) (string, error) {
	var script string
	if tab.WindowID != "" && tab.Index > 0 {
		script = fmt.Sprintf(`
tell application "Terminal"
	set t to tab %d of window %s
	try
		return history of t as text
	on error errMsg
		return "error:" & errMsg
	end try
end tell
`, tab.Index, tab.WindowID)
	} else {
		script = `
tell application "Terminal"
	try
		return history of selected tab of front window as text
	on error errMsg
		return "error:" & errMsg
	end try
end tell
`
	}
	out, err := runOSAscript(ctx, script)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(out, "error:") {
		return "", fmt.Errorf("Terminal: %s", strings.TrimPrefix(out, "error:"))
	}
	return out, nil
}

func captureITerm(ctx context.Context, tab TabInfo) (string, error) {
	var script string
	if tab.WindowID != "" && tab.Index > 0 {
		script = fmt.Sprintf(`
tell application "iTerm"
	try
		tell window %s
			tell tab %d
				return contents of current session
			end tell
		end tell
	on error errMsg
		return "error:" & errMsg
	end try
end tell
`, tab.WindowID, tab.Index)
	} else {
		script = `
tell application "iTerm"
	try
		return contents of current session of current window
	on error errMsg
		return "error:" & errMsg
	end try
end tell
`
	}
	out, err := runOSAscript(ctx, script)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(out, "error:") {
		return "", fmt.Errorf("iTerm: %s", strings.TrimPrefix(out, "error:"))
	}
	return out, nil
}

// --- internal rate limiter (mirrors inject.RateLimiter, cycle-free) ---

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
		return fmt.Errorf("darwin inject: empty session_id")
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
		return fmt.Errorf("darwin inject: rate limited (min interval %s) for session %q", r.minInterval, sessionID)
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
		return fmt.Errorf("darwin inject: rate limited (max %d per %s) for session %q", r.maxBurst, r.burstWindow, sessionID)
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
