package inject

import (
	"log/slog"
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
		if err := h.UI.Inject(sessionID, req); err == nil {
			return nil
		} else {
			h.log.Debug("ui inject failed; falling back to managed shell", "session", sessionID, "err", err)
		}
	}
	return h.PTY.Inject(sessionID, req)
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
