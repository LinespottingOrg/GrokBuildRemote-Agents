package inject

import (
	"errors"
	"sync"
	"time"
)

// MaxAutoOpen is the hard cap on agent-spawned Grok consoles.
// Phone/Bot API retries of POST /v1/sessions/open used to CREATE_NEW_CONSOLE
// forever and lock the PC under a pile of grok.exe / conhost popups.
const MaxAutoOpen = 3

// AutoOpenWindow is the rolling window for the spawn cap.
const AutoOpenWindow = 10 * time.Minute

// ErrSpawnLimit is returned when auto-open has already spawned MaxAutoOpen
// consoles in AutoOpenWindow (or that many are still counted live).
var ErrSpawnLimit = errors.New("open: auto-spawn limit 3 reached — attach an existing Grok Build session instead of opening another window")

type spawnGuard struct {
	mu       sync.Mutex
	attempts []time.Time
	live     map[int]time.Time // pid → spawned at
	now      func() time.Time
}

func newSpawnGuard() *spawnGuard {
	return &spawnGuard{
		live: make(map[int]time.Time),
		now:  time.Now,
	}
}

func (g *spawnGuard) clock() time.Time {
	if g == nil || g.now == nil {
		return time.Now()
	}
	return g.now()
}

func (g *spawnGuard) pruneLocked(now time.Time) {
	cutoff := now.Add(-AutoOpenWindow)
	kept := g.attempts[:0]
	for _, t := range g.attempts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	g.attempts = kept
	for pid, t := range g.live {
		if !t.After(cutoff) {
			delete(g.live, pid)
		}
	}
}

func (g *spawnGuard) allow() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.clock()
	g.pruneLocked(now)
	if len(g.attempts) >= MaxAutoOpen || len(g.live) >= MaxAutoOpen {
		return ErrSpawnLimit
	}
	return nil
}

func (g *spawnGuard) record(pid int) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.clock()
	g.pruneLocked(now)
	g.attempts = append(g.attempts, now)
	if pid > 0 {
		if g.live == nil {
			g.live = make(map[int]time.Time)
		}
		g.live[pid] = now
	}
}

func (g *spawnGuard) used() (attempts, live int) {
	if g == nil {
		return 0, 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pruneLocked(g.clock())
	return len(g.attempts), len(g.live)
}
