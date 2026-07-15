//go:build windows

package windows

import (
	"fmt"
	"sync"
	"syscall"
	"time"
)

// Injector is the Windows UI-automation inject backend.
// Safe for concurrent use; per-session rate limiting is enforced.
//
// NOTE: This package does not import parent inject (avoids import cycles).
// Parent package inject adapts *Injector to inject.Injector via Default().
type Injector struct {
	mu      sync.Mutex
	bound   map[string]Window
	limiter *rateLimiter
}

// New returns a Windows Injector with default rate limits
// (150ms min spacing, 8 injects/sec burst).
func New() *Injector {
	return &Injector{
		bound:   make(map[string]Window),
		limiter: newRateLimiter(150*time.Millisecond, 8, time.Second),
	}
}

// NewWithLimits returns an Injector with custom rate limits.
// Zero values select the same defaults as New.
func NewWithLimits(minInterval time.Duration, maxBurst int, burstWindow time.Duration) *Injector {
	return &Injector{
		bound:   make(map[string]Window),
		limiter: newRateLimiter(minInterval, maxBurst, burstWindow),
	}
}

// Discover lists visible terminal-like windows.
func (inj *Injector) Discover() ([]Window, error) {
	return Discover()
}

// Bind associates sessionID with a discovered window.
func (inj *Injector) Bind(sessionID string, win Window) error {
	if sessionID == "" {
		return ErrEmptySession
	}
	if win.HWND == 0 && win.PID == 0 {
		return fmt.Errorf("windows inject: bind requires HWND or PID")
	}
	inj.mu.Lock()
	defer inj.mu.Unlock()
	inj.bound[sessionID] = win
	return nil
}

// Unbind drops the session→window association.
func (inj *Injector) Unbind(sessionID string) {
	inj.mu.Lock()
	delete(inj.bound, sessionID)
	inj.mu.Unlock()
	inj.limiter.reset(sessionID)
}

func (inj *Injector) boundWindow(sessionID string) (Window, bool) {
	inj.mu.Lock()
	defer inj.mu.Unlock()
	w, ok := inj.bound[sessionID]
	return w, ok
}

// Inject focuses the bound window (or best-effort title match) and types text.
// Refuses empty session_id; enforces rate limits; UTF-16 aware via SendInput.
func (inj *Injector) Inject(sessionID string, req Request) error {
	if sessionID == "" {
		return ErrEmptySession
	}
	if req.SessionID != "" && req.SessionID != sessionID {
		return fmt.Errorf("windows inject: session_id mismatch (%q vs %q)", sessionID, req.SessionID)
	}
	if req.Text == "" && !req.Submit {
		return ErrEmptyText
	}
	if err := inj.limiter.allow(sessionID); err != nil {
		return err
	}

	win, err := inj.resolve(sessionID)
	if err != nil {
		return err
	}
	if win.HWND == 0 {
		return fmt.Errorf("%w: no HWND for session %q", ErrNotFound, sessionID)
	}

	hwnd := syscall.Handle(win.HWND)
	if err := focusWindow(hwnd); err != nil {
		return fmt.Errorf("%w: %v", ErrFocusFailed, err)
	}

	text := effectiveText(req)
	if err := sendUnicodeText(text); err != nil {
		return fmt.Errorf("%w: %v", ErrInjectFailed, err)
	}
	return nil
}

func effectiveText(req Request) string {
	if req.Submit {
		if req.Text == "" {
			return "\n"
		}
		if len(req.Text) > 0 && req.Text[len(req.Text)-1] == '\n' {
			return req.Text
		}
		return req.Text + "\n"
	}
	return req.Text
}

func (inj *Injector) resolve(sessionID string) (Window, error) {
	if w, ok := inj.boundWindow(sessionID); ok {
		return w, nil
	}
	matches, err := FindByTitleSubstring(sessionID)
	if err != nil {
		return Window{}, err
	}
	if len(matches) == 1 {
		_ = inj.Bind(sessionID, matches[0])
		return matches[0], nil
	}
	if len(matches) == 0 {
		return Window{}, fmt.Errorf("%w: session %q (bind a window or use managed PTY)", ErrNotFound, sessionID)
	}
	return Window{}, fmt.Errorf("%w: session %q matches %d windows; bind explicitly", ErrNotFound, sessionID, len(matches))
}

// Capture best-effort reads the console buffer for the bound session.
func (inj *Injector) Capture(sessionID string) (Capture, error) {
	if sessionID == "" {
		return Capture{}, ErrEmptySession
	}
	win, err := inj.resolve(sessionID)
	if err != nil {
		return Capture{
			Partial: true,
			Method:  "none",
			Note:    captureNote,
		}, err
	}
	if win.PID != 0 {
		return CapturePID(win.PID)
	}
	if win.HWND != 0 {
		return CaptureHWND(syscall.Handle(win.HWND))
	}
	return Capture{Partial: true, Method: "none", Note: captureNote}, ErrCaptureUnavail
}

// Close releases bindings.
func (inj *Injector) Close() error {
	inj.mu.Lock()
	inj.bound = make(map[string]Window)
	inj.mu.Unlock()
	return nil
}

// --- internal rate limiter (mirrors inject.RateLimiter; no parent import) ---

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

func (r *rateLimiter) reset(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.state, sessionID)
}
