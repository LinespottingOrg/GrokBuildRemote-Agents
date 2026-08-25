package inject

import (
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"
)

type windowSess struct {
	PID  int
	HWND uintptr
}

// Hybrid tries platform UI inject first, then managed shell (PTY pipes).
// This is the production default for Day-1 reliability.
type Hybrid struct {
	UI  Injector
	PTY *Manager
	log *slog.Logger

	mu      sync.Mutex
	windows map[string]windowSess
	spawns  *spawnGuard
}

// NewHybrid builds a hybrid injector. ui may be nil (PTY-only).
func NewHybrid(ui Injector, pty *Manager) *Hybrid {
	if pty == nil {
		pty = NewManager(nil)
	}
	return &Hybrid{
		UI:      ui,
		PTY:     pty,
		log:     slog.Default(),
		windows: make(map[string]windowSess),
		spawns:  newSpawnGuard(),
	}
}

func (h *Hybrid) Discover() ([]TerminalWindow, error) {
	if h.UI != nil {
		return h.UI.Discover()
	}
	return nil, nil
}

func (h *Hybrid) Bind(sessionID string, win TerminalWindow) error {
	// A real HWND means "type into this window". Remember it so Inject
	// cannot fall through to a PTY and log chars=N without spawning type.
	if sessionID != "" && win.HWND != 0 {
		h.rememberWindow(sessionID, int(win.PID), win.HWND)
	}
	if h.UI != nil {
		return h.UI.Bind(sessionID, win)
	}
	return nil
}

func (h *Hybrid) Unbind(sessionID string) {
	if h.UI != nil {
		h.UI.Unbind(sessionID)
	}
	h.mu.Lock()
	delete(h.windows, sessionID)
	h.mu.Unlock()
}

func (h *Hybrid) isWindowSession(sessionID string) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.windows[sessionID]
	return ok
}

func (h *Hybrid) rememberWindow(sessionID string, pid int, hwnd uintptr) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.windows == nil {
		h.windows = make(map[string]windowSess)
	}
	h.windows[sessionID] = windowSess{PID: pid, HWND: hwnd}
}

func (h *Hybrid) Inject(sessionID string, req InjectRequest) error {
	if err := ValidateRequest(sessionID, req); err != nil {
		return err
	}
	// Window-backed grok (visible console): never rediscover — pickInjectTarget
	// would happily steal ++ Felanmälan.org because it also says "Grok Build".
	if h.isWindowSession(sessionID) {
		if h.UI == nil {
			return fmt.Errorf("%w: session %q (window session has no UI injector)", ErrNotFound, sessionID)
		}
		return h.UI.Inject(sessionID, req)
	}
	// A session we spawned (open / managed shell) must stay on the PTY.
	// UI-first was typing into some other Terminal window while Capture
	// kept reading the PTY banner — so `echo gbr-e2e-ok` never came back.
	if h.PTY != nil {
		if s := h.PTY.Get(sessionID); s != nil && !s.IsClosed() {
			return h.PTY.Inject(sessionID, req)
		}
	}
	if h.UI != nil {
		if wins, err := h.UI.Discover(); err == nil && len(wins) > 0 {
			chosen := pickInjectTarget(wins, sessionID)
			if chosen.HWND != 0 && !IsProtectedTitle(chosen.Title) {
				_ = h.UI.Bind(sessionID, chosen)
			}
		}
		if err := h.UI.Inject(sessionID, req); err == nil {
			return nil
		} else {
			h.log.Debug("ui inject failed; not spawning a dummy shell", "session", sessionID, "err", err)
			// Window-not-found is NOT success. Do not Ensure() a pipe shell
			// that reports ok while Grok never sees the keys.
			if h.PTY != nil {
				if s := h.PTY.Get(sessionID); s != nil && !s.IsClosed() {
					return h.PTY.Inject(sessionID, req)
				}
			}
			return err
		}
	}
	if h.PTY != nil {
		return h.PTY.Inject(sessionID, req)
	}
	return fmt.Errorf("%w: session %q", ErrNotFound, sessionID)
}

func containsFold(s, sub string) bool {
	if sub == "" || s == "" {
		return false
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// pickInjectTarget ranks discovered windows for auto-bind:
//  1. Title/exe contains session id
//  2. Grok Build host
//  3. Real terminals (not gbr-agent title)
//  4. First remaining window
//
// Protected operator titles (Felanmälan, QA PC Android) always lose.
func pickInjectTarget(wins []TerminalWindow, sessionID string) TerminalWindow {
	if len(wins) == 0 {
		return TerminalWindow{}
	}
	score := func(w TerminalWindow) int {
		if IsProtectedTitle(w.Title) {
			return -1000
		}
		s := 0
		lt := strings.ToLower(w.Title)
		if sessionID != "" && (containsFold(w.Title, sessionID) || containsFold(w.ExeName, sessionID)) {
			s += 100
		}
		if containsFold(w.Title, "grok build") || containsFold(w.Title, "grok-build") ||
			containsFold(w.ExeName, "grok") || strings.EqualFold(string(w.Kind), "grok-build") {
			s += 50
		}
		if strings.Contains(lt, "gbr-agent") {
			s -= 40
		}
		if w.Kind != "" && w.Kind != "unknown" {
			s += 5
		}
		return s
	}
	best := TerminalWindow{}
	bestS := -999
	for i, w := range wins {
		sc := score(w)
		if i == 0 || sc > bestS {
			best, bestS = w, sc
		}
	}
	if bestS < 0 {
		return TerminalWindow{}
	}
	return best
}

func (h *Hybrid) Capture(sessionID string) (CaptureResult, error) {
	if h.isWindowSession(sessionID) && h.UI != nil {
		return h.UI.Capture(sessionID)
	}
	// Prefer managed shell output (reliable). If a PTY session exists,
	// do not fall through to UI HWND lookup — that is "window not found"
	// for agent-opened ids and is not a success path.
	if h.PTY != nil {
		if s := h.PTY.Get(sessionID); s != nil && !s.IsClosed() {
			return h.PTY.Capture(sessionID)
		}
		if cr, err := h.PTY.Capture(sessionID); err == nil && cr.Text != "" {
			return cr, nil
		}
	}
	if h.UI != nil {
		return h.UI.Capture(sessionID)
	}
	return CaptureResult{Partial: true, Method: "none", Note: "no capture backend"}, ErrCaptureUnavail
}

func (h *Hybrid) Close() error {
	var first error
	if h.PTY != nil {
		if err := h.PTY.Close(); err != nil {
			first = err
		}
	}
	if h.UI != nil {
		if err := h.UI.Close(); err != nil && first == nil {
			first = err
		}
	}
	h.mu.Lock()
	h.windows = make(map[string]windowSess)
	h.mu.Unlock()
	return first
}

// ManagedIDs returns PTY-backed session ids.
func (h *Hybrid) ManagedIDs() []string {
	if h.PTY == nil {
		return nil
	}
	return h.PTY.List()
}

// OpenOrAttach starts grok (or a shell) so inject can actually type.
// On Windows, grok is ConPTY (real TTY) or a visible console window — never a dead pipe.
//
// Anti-spam: empty session_id attaches an already-open Grok window instead of
// spawning. Auto CREATE_NEW_CONSOLE is hard-capped at MaxAutoOpen (3) per
// AutoOpenWindow — a retry loop used to lock the PC under popup consoles.
func (h *Hybrid) OpenOrAttach(req OpenRequest) (OpenResult, error) {
	if h == nil {
		return OpenResult{}, fmt.Errorf("open: no session manager")
	}
	sid := sanitizeOpenID(req.SessionID)
	if sid != "" && h.isWindowSession(sid) {
		h.mu.Lock()
		w := h.windows[sid]
		h.mu.Unlock()
		return OpenResult{
			SessionID: sid,
			Attached:  true,
			Method:    "window",
			PID:       w.PID,
			HWND:      w.HWND,
			Note:      "already managed window",
		}, nil
	}

	cmdName := strings.ToLower(strings.TrimSpace(req.Command))
	wantGrok := cmdName == "" || cmdName == "grok" || cmdName == "grok-build"

	if wantGrok {
		// Phone/Bot diagnose with no session_id: never spawn a second console.
		if sid == "" {
			if att, ok := h.attachExistingGrok(req); ok {
				return att, nil
			}
		}

		if runtime.GOOS == "windows" && h.UI != nil {
			if err := h.spawns.allow(); err != nil {
				if att, ok := h.attachExistingGrok(req); ok {
					att.Note = "spawn cap 3 — attached existing instead of opening another window"
					return att, nil
				}
				h.log.Warn("auto-spawn refused", "limit", MaxAutoOpen, "err", err)
				return OpenResult{}, err
			}
			wres, werr := h.openGrokWindow(req)
			if werr != nil {
				return OpenResult{}, fmt.Errorf("open grok window: %w", werr)
			}
			h.spawns.record(wres.PID)
			n, _ := h.spawns.used()
			h.log.Info("auto-spawn grok window", "pid", wres.PID, "session", wres.SessionID, "used", fmt.Sprintf("%d/%d", n, MaxAutoOpen))
			return wres, nil
		}

		if wres, werr := h.openGrokWindow(req); werr == nil {
			return wres, nil
		} else if h.PTY != nil {
			res, err := h.PTY.OpenOrAttach(req)
			if err == nil {
				h.waitReady(res.SessionID, 6*time.Second)
				return res, nil
			}
			return OpenResult{}, fmt.Errorf("open grok: window: %v; tty: %v", werr, err)
		} else {
			return OpenResult{}, werr
		}
	}
	if h.PTY != nil {
		return h.PTY.OpenOrAttach(req)
	}
	return OpenResult{}, fmt.Errorf("open: no session manager")
}

func isGrokBuildWindow(w TerminalWindow) bool {
	if w.HWND == 0 || IsProtectedTitle(w.Title) {
		return false
	}
	if w.Kind == KindGrokBuild {
		return true
	}
	return containsFold(w.Title, "grok") || containsFold(w.ExeName, "grok")
}

// attachExistingGrok binds an already-visible Grok Build console. No CreateProcess.
func (h *Hybrid) attachExistingGrok(req OpenRequest) (OpenResult, bool) {
	if h == nil || h.UI == nil {
		return OpenResult{}, false
	}
	wins, err := h.UI.Discover()
	if err != nil || len(wins) == 0 {
		return OpenResult{}, false
	}
	var chosen TerminalWindow
	want := sanitizeOpenID(req.SessionID)
	for _, w := range wins {
		if !isGrokBuildWindow(w) {
			continue
		}
		if want != "" && (containsFold(w.Title, want) || containsFold(w.ExeName, want)) {
			chosen = w
			break
		}
		if chosen.HWND == 0 {
			chosen = w
		}
	}
	if chosen.HWND == 0 {
		return OpenResult{}, false
	}
	sid := want
	if sid == "" {
		sid = newOpenSessionID(req.Resume)
	}
	if err := h.UI.Bind(sid, chosen); err != nil {
		return OpenResult{}, false
	}
	h.rememberWindow(sid, int(chosen.PID), chosen.HWND)
	return OpenResult{
		SessionID: sid,
		Attached:  true,
		Method:    "window",
		Command:   "grok",
		CWD:       resolveOpenCWD(req.CWD),
		PID:       int(chosen.PID),
		HWND:      chosen.HWND,
		Note:      "attached existing Grok Build window — did not spawn",
	}, true
}

func (h *Hybrid) waitReady(sessionID string, timeout time.Duration) {
	if sessionID == "" || timeout <= 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cr, err := h.Capture(sessionID)
		if err == nil {
			t := cr.Text
			if LooksLikeGrokSplash(t) || LooksLikePrompt(t) || containsFold(t, "grok") {
				time.Sleep(200 * time.Millisecond)
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
