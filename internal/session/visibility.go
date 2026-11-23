package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Visibility persists which session ids are hidden from the phone roster.
// Missing file means every session is visible, so existing pairings stay unchanged.
// Default path: $HOME/.gbr/roster-visibility.json
type Visibility struct {
	path string

	mu     sync.RWMutex
	hidden map[string]hiddenMeta
}

type hiddenMeta struct {
	Title string `json:"title,omitempty"`
	Shell string `json:"shell,omitempty"`
}

type visibilityFile struct {
	Hidden map[string]hiddenMeta `json:"hidden"`
}

// HiddenSession is one persisted hide, including sessions that are not open right now.
type HiddenSession struct {
	SessionID string
	Title     string
	Shell     string
}

// DefaultVisibilityPath returns the roster-visibility file under the user home.
func DefaultVisibilityPath() string {
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
	return filepath.Join(home, ".gbr", "roster-visibility.json")
}

// EmptyVisibility is an in-memory all-visible store. Set is a no-op.
func EmptyVisibility() *Visibility {
	return &Visibility{hidden: map[string]hiddenMeta{}}
}

// OpenVisibility loads path. An empty path uses DefaultVisibilityPath.
// A missing file is an empty (all-visible) store.
func OpenVisibility(path string) (*Visibility, error) {
	if path == "" {
		path = DefaultVisibilityPath()
	}
	v := &Visibility{
		path:   path,
		hidden: map[string]hiddenMeta{},
	}
	if err := v.Load(); err != nil {
		return nil, err
	}
	return v, nil
}

// Path returns the on-disk file, or empty for a memory-only store.
func (v *Visibility) Path() string {
	if v == nil {
		return ""
	}
	return v.path
}

// Load reads the file. A missing file leaves the store all-visible.
func (v *Visibility) Load() error {
	if v == nil {
		return errors.New("session: nil visibility")
	}
	if v.path == "" {
		v.mu.Lock()
		if v.hidden == nil {
			v.hidden = map[string]hiddenMeta{}
		}
		v.mu.Unlock()
		return nil
	}
	data, err := os.ReadFile(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			v.mu.Lock()
			v.hidden = map[string]hiddenMeta{}
			v.mu.Unlock()
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		v.mu.Lock()
		v.hidden = map[string]hiddenMeta{}
		v.mu.Unlock()
		return nil
	}
	var f visibilityFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("session: parse %s: %w", v.path, err)
	}
	m := make(map[string]hiddenMeta, len(f.Hidden))
	for id, meta := range f.Hidden {
		id = strings.TrimSpace(id)
		if !ValidSessionID(id) {
			continue
		}
		meta.Title = strings.TrimSpace(meta.Title)
		meta.Shell = strings.TrimSpace(meta.Shell)
		m[id] = meta
	}
	v.mu.Lock()
	v.hidden = m
	v.mu.Unlock()
	return nil
}

// Hidden reports whether id is hidden from the phone. Unknown ids are visible.
func (v *Visibility) Hidden(id string) bool {
	if v == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	_, ok := v.hidden[id]
	return ok
}

// Set records or clears a hide and writes the file.
// Hiding persists across agent restart for the same session id.
func (v *Visibility) Set(id string, hidden bool, title, shell string) error {
	if v == nil {
		return errors.New("session: nil visibility")
	}
	id = strings.TrimSpace(id)
	if !ValidSessionID(id) {
		return fmt.Errorf("session: invalid session_id %q", id)
	}
	v.mu.Lock()
	if v.hidden == nil {
		v.hidden = map[string]hiddenMeta{}
	}
	if hidden {
		meta := v.hidden[id]
		if strings.TrimSpace(title) != "" {
			meta.Title = strings.TrimSpace(title)
		}
		if strings.TrimSpace(shell) != "" {
			meta.Shell = strings.TrimSpace(shell)
		}
		v.hidden[id] = meta
	} else {
		delete(v.hidden, id)
	}
	v.mu.Unlock()
	return v.Save()
}

// List returns persisted hides. Order is not significant.
func (v *Visibility) List() []HiddenSession {
	if v == nil {
		return nil
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]HiddenSession, 0, len(v.hidden))
	for id, meta := range v.hidden {
		out = append(out, HiddenSession{SessionID: id, Title: meta.Title, Shell: meta.Shell})
	}
	return out
}

// Save writes the file atomically. A memory-only store succeeds without a file.
func (v *Visibility) Save() error {
	if v == nil {
		return errors.New("session: nil visibility")
	}
	if v.path == "" {
		return nil
	}
	v.mu.RLock()
	f := visibilityFile{Hidden: make(map[string]hiddenMeta, len(v.hidden))}
	for id, meta := range v.hidden {
		f.Hidden[id] = meta
	}
	path := v.path
	v.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("session: mkdir %s: %w", filepath.Dir(path), err)
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
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("session: rename visibility: %w", err2)
		}
	}
	return nil
}
