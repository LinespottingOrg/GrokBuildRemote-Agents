package session

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// DefaultScanInterval is the protocol-recommended scan period.
const DefaultScanInterval = 5 * time.Second

// DiscoverFunc returns currently observed terminal candidates each scan tick.
// Core/inject supplies the OS-specific implementation. Nil means only manual
// Track calls and re-resolution of already-registered CWDs.
type DiscoverFunc func(ctx context.Context) ([]Candidate, error)

// ScanResult is the delta from a single scan pass.
type ScanResult struct {
	Added   []*Session
	Updated []*Session
	Removed []*Session
	All     []*Session
}

// Scanner periodically discovers candidates, resolves session IDs, and updates the Registry.
type Scanner struct {
	Interval time.Duration
	Store    *Store
	Registry *Registry
	Discover DiscoverFunc

	// StaleAfter removes sessions not rediscovered within this duration.
	// Zero disables stale removal (default: 3 * Interval, applied at Run).
	StaleAfter time.Duration

	// OnScan is invoked after each successful scan (optional).
	OnScan func(ScanResult)

	mu       sync.Mutex
	manual   map[string]Candidate // cwd → candidate (always re-scanned)
	lastTick time.Time
}

// NewScanner constructs a Scanner. store and reg may be non-nil; nil store skips renames,
// nil reg allocates a new Registry.
func NewScanner(store *Store, reg *Registry, discover DiscoverFunc) *Scanner {
	if reg == nil {
		reg = NewRegistry()
	}
	return &Scanner{
		Interval: DefaultScanInterval,
		Store:    store,
		Registry: reg,
		Discover: discover,
		manual:   make(map[string]Candidate),
	}
}

// Track forces a candidate into the scan set (e.g. self-registered agent cwd).
func (sc *Scanner) Track(c Candidate) {
	if sc == nil {
		return
	}
	c.CWD = NormalizeCWD(c.CWD)
	if c.CWD == "" {
		return
	}
	sc.mu.Lock()
	if sc.manual == nil {
		sc.manual = make(map[string]Candidate)
	}
	sc.manual[c.CWD] = c
	sc.mu.Unlock()
}

// Untrack removes a manually tracked cwd.
func (sc *Scanner) Untrack(cwd string) {
	if sc == nil {
		return
	}
	cwd = NormalizeCWD(cwd)
	sc.mu.Lock()
	delete(sc.manual, cwd)
	sc.mu.Unlock()
}

// Rename is the agent-side "gbr rename <id>" handler: persists map and re-resolves cwd.
func (sc *Scanner) Rename(cwd, sessionID string) (*Session, error) {
	if sc == nil {
		return nil, errNilScanner
	}
	cwd = NormalizeCWD(cwd)
	if sc.Store == nil {
		// ephemeral rename without disk
		st, err := OpenStore("") // still try default path
		if err != nil {
			return nil, err
		}
		sc.Store = st
	}
	if err := sc.Store.Rename(cwd, sessionID); err != nil {
		return nil, err
	}
	// drop old registry entry for this cwd if id changed
	if old, ok := sc.Registry.GetByCWD(cwd); ok {
		sc.Registry.Remove(old.ID)
	}
	c := Candidate{CWD: cwd, Shell: defaultShell()}
	sc.mu.Lock()
	if m, ok := sc.manual[cwd]; ok {
		c = m
	}
	sc.mu.Unlock()
	sess, err := BuildSession(c, sc.renames())
	if err != nil {
		return nil, err
	}
	stored, _ := sc.Registry.Upsert(sess)
	return stored, nil
}

// Run blocks, scanning every Interval until ctx is done.
func (sc *Scanner) Run(ctx context.Context) error {
	if sc == nil {
		return errNilScanner
	}
	interval := sc.Interval
	if interval <= 0 {
		interval = DefaultScanInterval
	}
	if sc.StaleAfter == 0 {
		sc.StaleAfter = 3 * interval
	}
	// initial immediate scan
	if _, err := sc.ScanOnce(ctx); err != nil && ctx.Err() == nil {
		// non-fatal for discover errors; still loop
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			_, _ = sc.ScanOnce(ctx)
		}
	}
}

// ScanOnce performs one discovery + resolve + registry update pass.
func (sc *Scanner) ScanOnce(ctx context.Context) (ScanResult, error) {
	var res ScanResult
	if sc == nil {
		return res, errNilScanner
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}

	renames := sc.renames()
	seenCWD := make(map[string]struct{})
	candidates := make([]Candidate, 0, 8)

	// manual tracks
	sc.mu.Lock()
	for _, c := range sc.manual {
		candidates = append(candidates, c)
	}
	sc.mu.Unlock()

	// platform discover — only live candidates refresh LastSeen
	if sc.Discover != nil {
		found, err := sc.Discover(ctx)
		if err != nil {
			// still process manuals below
			if len(candidates) == 0 {
				res.All = sc.Registry.List()
				return res, err
			}
		} else {
			candidates = append(candidates, found...)
		}
	}

	var added, updated []*Session
	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		c.CWD = NormalizeCWD(c.CWD)
		if c.CWD == "" {
			continue
		}
		if _, dup := seenCWD[c.CWD]; dup {
			continue
		}
		seenCWD[c.CWD] = struct{}{}

		sess, err := BuildSession(c, renames)
		if err != nil {
			continue
		}
		// if registry had different id for same cwd, remove old
		if old, ok := sc.Registry.GetByCWD(sess.CWD); ok && old.ID != sess.ID {
			sc.Registry.Remove(old.ID)
			res.Removed = append(res.Removed, old)
		}
		stored, isNew := sc.Registry.Upsert(sess)
		if isNew {
			added = append(added, stored.Clone())
		} else {
			updated = append(updated, stored.Clone())
		}
	}

	// sessions not rediscovered keep prior LastSeen → dropped after StaleAfter
	if sc.StaleAfter > 0 {
		stale := sc.Registry.RemoveStale(sc.StaleAfter)
		res.Removed = append(res.Removed, stale...)
	}

	res.Added = added
	res.Updated = updated
	res.All = sc.Registry.List()
	sc.mu.Lock()
	sc.lastTick = time.Now().UTC()
	sc.mu.Unlock()

	if sc.OnScan != nil {
		sc.OnScan(res)
	}
	return res, nil
}

// LastTick returns when ScanOnce last completed.
func (sc *Scanner) LastTick() time.Time {
	if sc == nil {
		return time.Time{}
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.lastTick
}

// Registers returns register envelopes for all current sessions.
func (sc *Scanner) Registers(deviceID string) []RegisterMessage {
	if sc == nil || sc.Registry == nil {
		return nil
	}
	list := sc.Registry.List()
	out := make([]RegisterMessage, 0, len(list))
	for _, s := range list {
		out = append(out, s.ToRegister(deviceID))
	}
	return out
}

func (sc *Scanner) renames() map[string]string {
	if sc == nil || sc.Store == nil {
		return nil
	}
	return sc.Store.Snapshot()
}

var errNilScanner = errString("session: nil scanner")

type errString string

func (e errString) Error() string { return string(e) }

func defaultShell() string {
	switch runtime.GOOS {
	case "windows":
		return "pwsh"
	case "darwin":
		return "zsh"
	default:
		return "bash"
	}
}

func hostOS() string {
	return runtime.GOOS
}
