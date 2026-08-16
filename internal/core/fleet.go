package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// FleetFileName lives next to device.json.
const FleetFileName = "fleet.json"

const maxFleetDevices = 32

var fleetIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// FleetDevice is one PC a Grok bot can drive from a single hub instance.
type FleetDevice struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Kind      string `json:"kind"` // local | relay
	MailboxID string `json:"mailbox_id,omitempty"`
	MailboxKey string `json:"mailbox_key,omitempty"`
	OS        string `json:"os,omitempty"`
	AddedAt   string `json:"added_at,omitempty"`
}

// Fleet is the durable list of remotes plus the implicit local hub.
type Fleet struct {
	Devices []FleetDevice `json:"devices"`
	Updated string        `json:"updated,omitempty"`

	mu   sync.Mutex `json:"-"`
	path string     `json:"-"`
}

func fleetPath() (string, error) {
	dir, err := deviceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FleetFileName), nil
}

// LoadFleet reads ~/.gbr/fleet.json (empty list if missing).
func LoadFleet() (*Fleet, error) {
	path, err := fleetPath()
	if err != nil {
		return nil, err
	}
	f := &Fleet{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("parse fleet %s: %w", path, err)
	}
	f.path = path
	return f, nil
}

func (f *Fleet) saveLocked() error {
	if f.path == "" {
		p, err := fleetPath()
		if err != nil {
			return err
		}
		f.path = p
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	f.Updated = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, raw, 0o600)
}

// PublicDevices hides mailbox keys (has_key instead).
func (f *Fleet) PublicDevices() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, 0, len(f.Devices))
	for _, d := range f.Devices {
		out = append(out, publicDevice(d))
	}
	return out
}

func publicDevice(d FleetDevice) map[string]any {
	return map[string]any{
		"id":         d.ID,
		"name":       d.Name,
		"kind":       d.Kind,
		"mailbox_id": d.MailboxID,
		"os":         d.OS,
		"has_key":    strings.TrimSpace(d.MailboxKey) != "",
		"added_at":   d.AddedAt,
	}
}

// Get returns a copy of the named device (local aliases: "", local, this, hub).
func (f *Fleet) Get(id string) (FleetDevice, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getLocked(id)
}

func (f *Fleet) getLocked(id string) (FleetDevice, bool) {
	id = normalizeFleetID(id)
	if isLocalFleetID(id) {
		return FleetDevice{ID: "local", Kind: "local", Name: "this PC"}, true
	}
	for _, d := range f.Devices {
		if d.ID == id || strings.EqualFold(d.Name, id) {
			return d, true
		}
	}
	return FleetDevice{}, false
}

// Upsert adds or replaces a remote device.
func (f *Fleet) Upsert(d FleetDevice) error {
	id := normalizeFleetID(firstNonEmpty(d.ID, d.Name))
	if !fleetIDRe.MatchString(id) {
		return fmt.Errorf("bad device id %q (use lowercase slug)", d.ID)
	}
	if isLocalFleetID(id) {
		return fmt.Errorf("id %q is reserved for this PC", id)
	}
	d.ID = id
	if d.Kind == "" {
		d.Kind = "relay"
	}
	if strings.TrimSpace(d.MailboxID) == "" || strings.TrimSpace(d.MailboxKey) == "" {
		return fmt.Errorf("mailbox_id and mailbox_key required")
	}
	if d.AddedAt == "" {
		d.AddedAt = time.Now().UTC().Format(time.RFC3339)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	found := false
	for i := range f.Devices {
		if f.Devices[i].ID == id {
			if d.MailboxKey == "" {
				d.MailboxKey = f.Devices[i].MailboxKey
			}
			if d.AddedAt == "" {
				d.AddedAt = f.Devices[i].AddedAt
			}
			f.Devices[i] = d
			found = true
			break
		}
	}
	if !found {
		if len(f.Devices) >= maxFleetDevices {
			return fmt.Errorf("fleet full (%d)", maxFleetDevices)
		}
		f.Devices = append(f.Devices, d)
	}
	return f.saveLocked()
}

// Remove drops a remote device.
func (f *Fleet) Remove(id string) error {
	id = normalizeFleetID(id)
	if isLocalFleetID(id) {
		return fmt.Errorf("cannot remove local")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.Devices[:0]
	found := false
	for _, d := range f.Devices {
		if d.ID == id {
			found = true
			continue
		}
		kept = append(kept, d)
	}
	if !found {
		return fmt.Errorf("device %q not found", id)
	}
	f.Devices = kept
	return f.saveLocked()
}

func normalizeFleetID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, "_", "-")
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

func isLocalFleetID(id string) bool {
	switch normalizeFleetID(id) {
	case "", "local", "this", "hub", "self", "here":
		return true
	default:
		return false
	}
}
