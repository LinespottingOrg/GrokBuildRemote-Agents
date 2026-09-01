package inject

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultInjectWindow is the rolling window for GBR_INJECT_MAX.
const DefaultInjectWindow = 2 * time.Minute

// HaltInject reports the operator kill-switch.
// GBR_INJECT_HALT=1 / true / on refuses every inject (Bot API, mailbox, inbox).
func HaltInject() bool {
	v := strings.TrimSpace(os.Getenv("GBR_INJECT_HALT"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "on")
}

// InjectMaxFromEnv is the optional per-session cap.
// Unset = no extra cap (command_id replay is still enforced).
// 0 = halt. Negative values are treated as unset.
func InjectMaxFromEnv() int {
	if HaltInject() {
		return 0
	}
	s := strings.TrimSpace(os.Getenv("GBR_INJECT_MAX"))
	if s == "" {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

// AttemptGuard records injects so a timed-out / failed command_id cannot be
// typed again, and so GBR_INJECT_MAX can bound a session stampede.
type AttemptGuard struct {
	mu         sync.Mutex
	ids        map[string]time.Time
	perSession map[string][]time.Time
	now        func() time.Time
	window     time.Duration
}

func newAttemptGuard() *AttemptGuard {
	return &AttemptGuard{
		ids:        make(map[string]time.Time),
		perSession: make(map[string][]time.Time),
		now:        time.Now,
		window:     DefaultInjectWindow,
	}
}

func (g *AttemptGuard) clock() time.Time {
	if g == nil || g.now == nil {
		return time.Now()
	}
	return g.now()
}

func (g *AttemptGuard) win() time.Duration {
	if g == nil || g.window <= 0 {
		return DefaultInjectWindow
	}
	return g.window
}

// Admit returns nil if this inject may type. It records commandID (when set)
// so a later replay of the same id dies instead of opening another card.
func (g *AttemptGuard) Admit(sessionID, commandID string) error {
	if HaltInject() {
		return ErrInjectHalted
	}
	max := InjectMaxFromEnv()
	if max == 0 {
		return ErrInjectHalted
	}
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.clock()
	if commandID != "" {
		if _, ok := g.ids[commandID]; ok {
			return fmt.Errorf("%w: %s", ErrInjectReplay, commandID)
		}
	}
	if max > 0 && sessionID != "" {
		cutoff := now.Add(-g.win())
		kept := g.perSession[sessionID][:0]
		for _, t := range g.perSession[sessionID] {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		g.perSession[sessionID] = kept
		if len(kept) >= max {
			return fmt.Errorf("%w: %d injects / %s for session %q (GBR_INJECT_MAX)", ErrInjectBudget, max, g.win(), sessionID)
		}
	}
	if commandID != "" {
		if g.ids == nil {
			g.ids = make(map[string]time.Time)
		}
		g.ids[commandID] = now
	}
	if max > 0 && sessionID != "" {
		if g.perSession == nil {
			g.perSession = make(map[string][]time.Time)
		}
		g.perSession[sessionID] = append(g.perSession[sessionID], now)
	}
	return nil
}

// Seen reports whether commandID was already admitted.
func (g *AttemptGuard) Seen(commandID string) bool {
	if g == nil || commandID == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.ids[commandID]
	return ok
}
