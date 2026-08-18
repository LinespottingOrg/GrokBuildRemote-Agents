package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Session leases stop two clients (Grok bot + Claude Cowork) typing into the
// same window at once. This is NOT the single-agent PID lock in lock.go.

const (
	LeaseFileName     = "leases.json"
	DefaultLeaseTTL   = 15 * time.Minute
	MaxLeaseTTL       = 2 * time.Hour
	MinLeaseTTL       = 30 * time.Second
	DefaultLeaseHolder = "bot"
)

var (
	ErrLeaseHeld    = errors.New("session already leased")
	ErrLeaseMissing = errors.New("no lease")
	ErrLeaseOwner   = errors.New("lease held by another client")
)

// SessionLease is one client's exclusive claim on a session_id.
type SessionLease struct {
	SessionID string    `json:"session_id"`
	Holder    string    `json:"holder"`
	Token     string    `json:"token"`
	Goal      string    `json:"goal,omitempty"`
	Acquired  time.Time `json:"acquired"`
	Expires   time.Time `json:"expires"`
}

// PublicLease is the wire shape (token omitted unless the caller holds it).
func (l SessionLease) Public(revealToken bool) map[string]any {
	m := map[string]any{
		"session_id": l.SessionID,
		"holder":     l.Holder,
		"acquired":   l.Acquired.UTC().Format(time.RFC3339),
		"expires":    l.Expires.UTC().Format(time.RFC3339),
		"ttl_s":      int(time.Until(l.Expires).Seconds()),
	}
	if l.Goal != "" {
		m["goal"] = l.Goal
	}
	if revealToken && l.Token != "" {
		m["token"] = l.Token
	}
	return m
}

type leaseFile struct {
	Leases  map[string]SessionLease `json:"leases"`
	Updated string                  `json:"updated,omitempty"`
}

var (
	leaseMu sync.Mutex
)

func leasePath() (string, error) {
	dir, err := deviceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, LeaseFileName), nil
}

func loadLeasesLocked() (map[string]SessionLease, error) {
	path, err := leasePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]SessionLease{}, nil
		}
		return nil, err
	}
	var f leaseFile
	if err := json.Unmarshal(b, &f); err != nil || f.Leases == nil {
		return map[string]SessionLease{}, nil
	}
	now := time.Now()
	out := make(map[string]SessionLease, len(f.Leases))
	for id, l := range f.Leases {
		if l.Expires.After(now) && strings.TrimSpace(id) != "" {
			out[id] = l
		}
	}
	return out, nil
}

func saveLeasesLocked(m map[string]SessionLease) error {
	path, err := leasePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f := leaseFile{Leases: m, Updated: time.Now().UTC().Format(time.RFC3339Nano)}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func normalizeHolder(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.ReplaceAll(h, "_", "-")
	h = strings.Join(strings.Fields(h), "-")
	switch h {
	case "", "bot", "gbr-bot", "grok", "x":
		return "grok-bot"
	case "claude", "coworker", "cowork", "claude-code", "mcp":
		return "claude-coworker"
	case "phone", "mobile", "ios", "android":
		return "phone"
	}
	if len(h) > 64 {
		h = h[:64]
	}
	return h
}

func clampLeaseTTL(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultLeaseTTL
	}
	if d < MinLeaseTTL {
		return MinLeaseTTL
	}
	if d > MaxLeaseTTL {
		return MaxLeaseTTL
	}
	return d
}

func newLeaseToken() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(b[:])
}

// AcquireLease claims sessionID for holder. Same holder refreshes. Other
// holder fails unless steal is true.
func AcquireLease(sessionID, holder, goal string, ttl time.Duration, steal bool) (SessionLease, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || sessionID == "session" {
		return SessionLease{}, fmt.Errorf("lease: empty or pseudo session_id")
	}
	holder = normalizeHolder(holder)
	ttl = clampLeaseTTL(ttl)

	leaseMu.Lock()
	defer leaseMu.Unlock()
	m, err := loadLeasesLocked()
	if err != nil {
		return SessionLease{}, err
	}
	if cur, ok := m[sessionID]; ok {
		if cur.Holder != holder && !steal {
			return cur, fmt.Errorf("%w by %s until %s", ErrLeaseHeld, cur.Holder, cur.Expires.UTC().Format(time.RFC3339))
		}
		cur.Holder = holder
		cur.Expires = time.Now().Add(ttl)
		if goal != "" {
			cur.Goal = goal
		}
		if cur.Token == "" {
			cur.Token = newLeaseToken()
		}
		m[sessionID] = cur
		if err := saveLeasesLocked(m); err != nil {
			return SessionLease{}, err
		}
		return cur, nil
	}
	l := SessionLease{
		SessionID: sessionID,
		Holder:    holder,
		Token:     newLeaseToken(),
		Goal:      strings.TrimSpace(goal),
		Acquired:  time.Now().UTC(),
		Expires:   time.Now().UTC().Add(ttl),
	}
	m[sessionID] = l
	if err := saveLeasesLocked(m); err != nil {
		return SessionLease{}, err
	}
	return l, nil
}

// ReleaseLease drops a claim. Holder must match unless force.
func ReleaseLease(sessionID, holder string, force bool) error {
	sessionID = strings.TrimSpace(sessionID)
	holder = normalizeHolder(holder)
	leaseMu.Lock()
	defer leaseMu.Unlock()
	m, err := loadLeasesLocked()
	if err != nil {
		return err
	}
	cur, ok := m[sessionID]
	if !ok {
		return ErrLeaseMissing
	}
	if !force && holder != "" && cur.Holder != holder {
		return fmt.Errorf("%w (%s)", ErrLeaseOwner, cur.Holder)
	}
	delete(m, sessionID)
	return saveLeasesLocked(m)
}

// GetLease returns a live lease, if any.
func GetLease(sessionID string) (SessionLease, bool) {
	leaseMu.Lock()
	defer leaseMu.Unlock()
	m, err := loadLeasesLocked()
	if err != nil {
		return SessionLease{}, false
	}
	l, ok := m[strings.TrimSpace(sessionID)]
	return l, ok
}

// ListLeases returns every unexpired lease.
func ListLeases() []SessionLease {
	leaseMu.Lock()
	defer leaseMu.Unlock()
	m, err := loadLeasesLocked()
	if err != nil {
		return nil
	}
	out := make([]SessionLease, 0, len(m))
	for _, l := range m {
		out = append(out, l)
	}
	return out
}

// LeaseConflictHint is a short line for Bot API / MCP errors.
func LeaseConflictHint(l SessionLease) string {
	return fmt.Sprintf("session %s is leased by %s until %s — wait, pass steal=true, or pick another session",
		l.SessionID, l.Holder, l.Expires.UTC().Format(time.RFC3339))
}
