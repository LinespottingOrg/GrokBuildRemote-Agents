package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SeenStore persists processed command_id fingerprints so agent restarts
// do not re-execute old injects from the relay queue.
type SeenStore struct {
	mu   sync.Mutex
	path string
	// id -> unix timestamp
	ids map[string]int64
	// max retained entries
	max int
}

type seenFile struct {
	IDs map[string]int64 `json:"ids"`
}

// OpenSeen loads ~/.gbr/seen.json (or empty).
func OpenSeen() (*SeenStore, error) {
	dir, err := deviceDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "seen.json")
	s := &SeenStore{
		path: path,
		ids:  make(map[string]int64),
		max:  4096,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var f seenFile
	if err := json.Unmarshal(data, &f); err != nil {
		return s, nil // corrupt → start fresh
	}
	if f.IDs != nil {
		s.ids = f.IDs
	}
	return s, nil
}

// Has reports whether fp was already processed.
func (s *SeenStore) Has(fp string) bool {
	if s == nil || fp == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.ids[fp]
	return ok
}

// Add records fp and persists (best-effort).
func (s *SeenStore) Add(fp string) {
	if s == nil || fp == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids[fp] = time.Now().Unix()
	if len(s.ids) > s.max {
		// drop oldest half
		type kv struct {
			k string
			t int64
		}
		all := make([]kv, 0, len(s.ids))
		for k, t := range s.ids {
			all = append(all, kv{k, t})
		}
		// simple partial clear: keep newest max/2 by deleting random older
		// (avoid sort import cost: two-pass min threshold)
		var sum int64
		for _, e := range all {
			sum += e.t
		}
		avg := sum / int64(len(all))
		for k, t := range s.ids {
			if t < avg {
				delete(s.ids, k)
			}
		}
	}
	_ = s.saveLocked()
}

func (s *SeenStore) saveLocked() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(seenFile{IDs: s.ids}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Len returns number of tracked fingerprints.
func (s *SeenStore) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ids)
}
