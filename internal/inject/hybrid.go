package inject

import (
	"log/slog"
	"strings"
)

// Hybrid tries platform UI inject first, then managed shell (PTY pipes).
// This is the production default for Day-1 reliability.
type Hybrid struct {
	UI  Injector
	PTY *Manager
	log *slog.Logger
}

// NewHybrid builds a hybrid injector. ui may be nil (PTY-only).
func NewHybrid(ui Injector, pty *Manager) *Hybrid {
	if pty == nil {
		pty = NewManager(nil)
	}
	return &Hybrid{
		UI:  ui,
		PTY: pty,
		log: slog.Default(),
	}
}

func (h *Hybrid) Discover() ([]TerminalWindow, error) {
	if h.UI != nil {
		return h.UI.Discover()
	}
	return nil, nil
}

func (h *Hybrid) Bind(sessionID string, win TerminalWindow) error {
	if h.UI != nil {
		return h.UI.Bind(sessionID, win)
	}
	return nil
}

func (h *Hybrid) Unbind(sessionID string) {
	if h.UI != nil {
		h.UI.Unbind(sessionID)
	}
}

func (h *Hybrid) Inject(sessionID string, req InjectRequest) error {
	if err := ValidateRequest(sessionID, req); err != nil {
		return err
	}
	if h.UI != nil {
		// Prefer already-bound session. Re-discover only if inject will need a window.
		// Prefer Grok Build / matching title over the agent's own PowerShell host.
		if wins, err := h.UI.Discover(); err == nil && len(wins) > 0 {
			chosen := pickInjectTarget(wins, sessionID)
			_ = h.UI.Bind(sessionID, chosen)
		}
		if err := h.UI.Inject(sessionID, req); err == nil {
			return nil
		} else {
			h.log.Debug("ui inject failed; falling back to managed shell", "session", sessionID, "err", err)
		}
	}
	return h.PTY.Inject(sessionID, req)
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
func pickInjectTarget(wins []TerminalWindow, sessionID string) TerminalWindow {
	if len(wins) == 0 {
		return TerminalWindow{}
	}
	score := func(w TerminalWindow) int {
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
	best := wins[0]
	bestS := score(best)
	for _, w := range wins[1:] {
		if sc := score(w); sc > bestS {
			best, bestS = w, sc
		}
	}
	return best
}

func (h *Hybrid) Capture(sessionID string) (CaptureResult, error) {
	// Prefer managed shell output (reliable).
	if h.PTY != nil {
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
	return first
}

// ManagedIDs returns PTY-backed session ids.
func (h *Hybrid) ManagedIDs() []string {
	if h.PTY == nil {
		return nil
	}
	return h.PTY.List()
}
