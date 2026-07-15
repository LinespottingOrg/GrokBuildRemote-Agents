package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store persists the explicit session rename map.
// Default path: %USERPROFILE%\.gbr\sessions.json (or $HOME/.gbr/sessions.json).
type Store struct {
	path string

	mu      sync.RWMutex
	renames map[string]string // normalized cwd → session_id
}

// sessionsFile is the on-disk JSON shape.
type sessionsFile struct {
	// Renames maps absolute cwd → session_id.
	Renames map[string]string `json:"renames"`
}

// DefaultStorePath returns %USERPROFILE%\.gbr\sessions.json (cross-platform home).
func DefaultStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		home = os.Getenv("HOME")
	}
	if home == "" {
		home = "."
	}
	return filepath.Join(home, ".gbr", "sessions.json")
}

// OpenStore loads (or creates) the rename store at path.
// Empty path uses DefaultStorePath().
func OpenStore(path string) (*Store, error) {
	if path == "" {
		path = DefaultStorePath()
	}
	s := &Store{
		path:    path,
		renames: make(map[string]string),
	}
	if err := s.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Load returns nil for missing file; other errors propagate
		return nil, err
	}
	return s, nil
}

// Path returns the store file path.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Load reads sessions.json from disk. Missing file is not an error (empty map).
func (s *Store) Load() error {
	if s == nil {
		return errors.New("session: nil store")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			s.renames = make(map[string]string)
			s.mu.Unlock()
			return nil
		}
		return err
	}
	if len(data) == 0 {
		s.mu.Lock()
		s.renames = make(map[string]string)
		s.mu.Unlock()
		return nil
	}
	var f sessionsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("session: parse %s: %w", s.path, err)
	}
	m := make(map[string]string, len(f.Renames))
	for k, v := range f.Renames {
		nk := NormalizeCWD(k)
		if nk == "" || v == "" {
			continue
		}
		if !ValidSessionID(v) {
			v = Slugify(v)
		}
		if ValidSessionID(v) {
			m[nk] = v
		}
	}
	s.mu.Lock()
	s.renames = m
	s.mu.Unlock()
	return nil
}

// Save writes the rename map atomically (temp + rename).
func (s *Store) Save() error {
	if s == nil {
		return errors.New("session: nil store")
	}
	s.mu.RLock()
	f := sessionsFile{Renames: make(map[string]string, len(s.renames))}
	for k, v := range s.renames {
		f.Renames[k] = v
	}
	path := s.path
	s.mu.RUnlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("session: mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("session: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Windows: replace existing
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("session: rename store: %w", err2)
		}
	}
	return nil
}

// Rename sets an explicit session_id for cwd and persists.
// sessionID must be a valid slug (or slugifiable).
func (s *Store) Rename(cwd, sessionID string) error {
	if s == nil {
		return errors.New("session: nil store")
	}
	cwd = NormalizeCWD(cwd)
	if cwd == "" {
		return errors.New("session: empty cwd")
	}
	if stringsContainsPathMeta(sessionID) {
		return fmt.Errorf("session: invalid session_id %q", sessionID)
	}
	id := sessionID
	if !ValidSessionID(id) {
		id = Slugify(sessionID)
	}
	if !ValidSessionID(id) {
		return fmt.Errorf("session: session_id %q not valid after slugify", sessionID)
	}
	s.mu.Lock()
	if s.renames == nil {
		s.renames = make(map[string]string)
	}
	s.renames[cwd] = id
	s.mu.Unlock()
	return s.Save()
}

// Unrename removes an explicit rename for cwd and persists.
func (s *Store) Unrename(cwd string) error {
	if s == nil {
		return errors.New("session: nil store")
	}
	cwd = NormalizeCWD(cwd)
	s.mu.Lock()
	delete(s.renames, cwd)
	s.mu.Unlock()
	return s.Save()
}

// Lookup returns the explicit rename for cwd, if any.
func (s *Store) Lookup(cwd string) (string, bool) {
	if s == nil {
		return "", false
	}
	cwd = NormalizeCWD(cwd)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return lookupRename(s.renames, cwd)
}

// Snapshot returns a copy of the rename map (cwd → session_id).
func (s *Store) Snapshot() map[string]string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.renames))
	for k, v := range s.renames {
		out[k] = v
	}
	return out
}

func stringsContainsPathMeta(s string) bool {
	if s == "" {
		return true
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '/', '\\', ':':
			return true
		}
	}
	return len(s) >= 2 && s[0] == '.' && s[1] == '.'
}
