package inject

import (
	"errors"
	"os"
	"strconv"
	"strings"
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

// AutoOpenMax is the effective spawn cap. GBR_NO_AUTO_OPEN=1 or
// GBR_MAX_AUTO_OPEN=0 disable CREATE_NEW_CONSOLE / grok spawn.
func AutoOpenMax() int {
	v := strings.TrimSpace(os.Getenv("GBR_NO_AUTO_OPEN"))
	if v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "on") {
		return 0
	}
	if s := strings.TrimSpace(os.Getenv("GBR_MAX_AUTO_OPEN")); s != "" {
		n, err := strconv.Atoi(s)
		if err == nil && n >= 0 {
			return n
		}
	}
	return MaxAutoOpen
}

func (g *spawnGuard) allow() error {
	max := AutoOpenMax()
	if max == 0 {
		return ErrSpawnLimit
	}
	if g == nil {
		if max == 0 {
			return ErrSpawnLimit
		}
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.clock()
	g.pruneLocked(now)
	if len(g.attempts) >= max || len(g.live) >= max {
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
