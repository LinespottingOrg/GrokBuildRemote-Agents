package session

import (
	"runtime"
	"sync"
	"time"
)

// Session is a tracked terminal/session observation.
type Session struct {
	ID        string    `json:"session_id"`
	CWD       string    `json:"cwd"`
	Shell     string    `json:"shell"`
	PID       int       `json:"pid"`  // process id placeholder (0 if unknown)
	HWND      uintptr   `json:"hwnd"` // window handle placeholder (0 if unknown)
	Title     string    `json:"title"`
	GitRemote string    `json:"git_remote,omitempty"`
	LastSeen  time.Time `json:"last_seen"`
	OS        string    `json:"os"`
	Source    ResolveSource `json:"source,omitempty"`
}

// Clone returns a shallow copy safe for concurrent readers.
func (s *Session) Clone() *Session {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

// Candidate is a raw terminal observation from the platform/inject layer.
type Candidate struct {
	CWD    string
	Shell  string
	PID    int
	HWND   uintptr
	Title  string
}

// Registry holds the live set of sessions, keyed by session_id and cwd.
type Registry struct {
	mu    sync.RWMutex
	byID  map[string]*Session
	byCWD map[string]*Session
}

// NewRegistry creates an empty session registry.
func NewRegistry() *Registry {
	return &Registry{
		byID:  make(map[string]*Session),
		byCWD: make(map[string]*Session),
	}
}

// Upsert inserts or updates a session. Returns the stored pointer and whether it was new by ID.
func (r *Registry) Upsert(s *Session) (stored *Session, isNew bool) {
	if r == nil || s == nil || s.ID == "" {
		return nil, false
	}
	s.CWD = NormalizeCWD(s.CWD)
	if s.OS == "" {
		s.OS = runtime.GOOS
	}
	if s.LastSeen.IsZero() {
		s.LastSeen = time.Now().UTC()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byID[s.ID]; ok {
		// same id — update fields
		existing.CWD = s.CWD
		existing.Shell = s.Shell
		existing.PID = s.PID
		existing.HWND = s.HWND
		if s.Title != "" {
			existing.Title = s.Title
		}
		existing.GitRemote = s.GitRemote
		existing.LastSeen = s.LastSeen
		existing.OS = s.OS
		existing.Source = s.Source
		r.byCWD[existing.CWD] = existing
		return existing, false
	}

	// cwd collision with different id: drop old cwd index
	if old, ok := r.byCWD[s.CWD]; ok && old.ID != s.ID {
		delete(r.byID, old.ID)
	}

	cp := s.Clone()
	r.byID[cp.ID] = cp
	if cp.CWD != "" {
		r.byCWD[cp.CWD] = cp
	}
	return cp, true
}

// Get returns a clone of the session by id.
func (r *Registry) Get(id string) (*Session, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	return s.Clone(), true
}

// GetByCWD returns a clone of the session for cwd.
func (r *Registry) GetByCWD(cwd string) (*Session, bool) {
	if r == nil {
		return nil, false
	}
	cwd = NormalizeCWD(cwd)
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byCWD[cwd]
	if !ok {
		return nil, false
	}
	return s.Clone(), true
}

// List returns clones of all sessions (order not guaranteed).
func (r *Registry) List() []*Session {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Session, 0, len(r.byID))
	for _, s := range r.byID {
		out = append(out, s.Clone())
	}
	return out
}

// Len returns the number of tracked sessions.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}

// Remove deletes a session by id.
func (r *Registry) Remove(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return false
	}
	delete(r.byID, id)
	if s.CWD != "" {
		if cur, ok := r.byCWD[s.CWD]; ok && cur.ID == id {
			delete(r.byCWD, s.CWD)
		}
	}
	return true
}

// RemoveStale drops sessions not seen within maxAge. Returns removed clones.
func (r *Registry) RemoveStale(maxAge time.Duration) []*Session {
	if r == nil || maxAge <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	r.mu.Lock()
	defer r.mu.Unlock()
	var removed []*Session
	for id, s := range r.byID {
		if s.LastSeen.Before(cutoff) {
			removed = append(removed, s.Clone())
			delete(r.byID, id)
			if s.CWD != "" {
				if cur, ok := r.byCWD[s.CWD]; ok && cur.ID == id {
					delete(r.byCWD, s.CWD)
				}
			}
		}
	}
	return removed
}
