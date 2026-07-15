package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DeviceFileName is the short-path device identity file under ~/.gbr/.
const DeviceFileName = "device.json"

// Device is the durable PC identity for GBR protocol (device_id UUID v4).
type Device struct {
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// MailboxConversationID is Mode B shared conversation id (set after pair).
	MailboxConversationID string `json:"mailbox_conversation_id,omitempty"`

	mu   sync.Mutex `json:"-"`
	path string     `json:"-"`
}

// deviceDir returns %USERPROFILE%\.gbr (short path) or $HOME/.gbr.
func deviceDir() (string, error) {
	if up := os.Getenv("USERPROFILE"); up != "" {
		return filepath.Join(up, ".gbr"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home for .gbr: %w", err)
	}
	return filepath.Join(home, ".gbr"), nil
}

// DevicePath returns the absolute path to device.json.
func DevicePath() (string, error) {
	dir, err := deviceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DeviceFileName), nil
}

// LoadOrCreateDevice loads device.json or creates a new UUID-backed identity.
func LoadOrCreateDevice() (*Device, error) {
	path, err := DevicePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err == nil {
		var d Device
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("parse device file %s: %w", path, err)
		}
		if d.DeviceID == "" {
			return nil, fmt.Errorf("device file %s missing device_id", path)
		}
		if _, err := uuid.Parse(d.DeviceID); err != nil {
			return nil, fmt.Errorf("device file %s invalid device_id: %w", path, err)
		}
		d.path = path
		return &d, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read device file %s: %w", path, err)
	}

	now := time.Now().UTC()
	name, _ := os.Hostname()
	d := &Device{
		DeviceID:   uuid.NewString(),
		DeviceName: name,
		CreatedAt:  now,
		UpdatedAt:  now,
		path:       path,
	}
	if err := d.Save(); err != nil {
		return nil, err
	}
	return d, nil
}

// Save persists device identity to disk (0600, directory 0700).
func (d *Device) Save() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	path := d.path
	if path == "" {
		var err error
		path, err = DevicePath()
		if err != nil {
			return err
		}
		d.path = path
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create device dir: %w", err)
	}

	d.UpdatedAt = time.Now().UTC()
	// Encode without mutex fields (json:"-")
	type wire Device
	raw, err := json.MarshalIndent((*wire)(d), "", "  ")
	if err != nil {
		return fmt.Errorf("encode device: %w", err)
	}
	raw = append(raw, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write device temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace device file: %w", err)
	}
	return nil
}

// SetMailboxConversationID updates Mode B conversation id and persists.
func (d *Device) SetMailboxConversationID(id string) error {
	d.mu.Lock()
	d.MailboxConversationID = id
	d.mu.Unlock()
	return d.Save()
}

// SetDeviceName renames the human-readable device label and persists.
func (d *Device) SetDeviceName(name string) error {
	d.mu.Lock()
	d.DeviceName = name
	d.mu.Unlock()
	return d.Save()
}

// Path returns the on-disk path for this device record.
func (d *Device) Path() string {
	return d.path
}
