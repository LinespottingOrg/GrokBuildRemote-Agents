package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OfferDirName is the local offer folder under ~/.gbr (gitignored on workstations).
const OfferDirName = "offers"

// FleetOffer is written by `gbr-agent pair-as-mailbox` on a remote so the hub
// can `fleet add -name NAME -os OS` without pasting the mailbox key.
// Treat mailbox_key like a password. Never log or print it.
type FleetOffer struct {
	Name       string `json:"name"`
	MailboxID  string `json:"mailbox_id"`
	MailboxKey string `json:"mailbox_key"`
	OS         string `json:"os,omitempty"`
	WrittenAt  string `json:"written_at,omitempty"`
}

// FleetOfferSearchDirs is the ordered list of directories the hub reads and
// the remote writes. First writable path under ~/.gbr is always included.
func FleetOfferSearchDirs() []string {
	var dirs []string
	if extra := strings.TrimSpace(os.Getenv("GBR_FLEET_OFFER_DIR")); extra != "" {
		dirs = append(dirs, extra)
	}
	if d, err := deviceDir(); err == nil {
		dirs = append(dirs, filepath.Join(d, OfferDirName))
	}
	home, _ := os.UserHomeDir()
	up := strings.TrimSpace(os.Getenv("USERPROFILE"))
	roots := []string{home, up}
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		dirs = append(dirs, filepath.Join(root, "Dropbox", "aiprojekt", "APPAR", "Grok Build Remote", "_ops", "fleet-offers"))
	}
	return uniqueExistingPrefer(dirs)
}

func uniqueExistingPrefer(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range in {
		d = filepath.Clean(d)
		if d == "." || d == string(filepath.Separator) || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func offerFileName(name string) string {
	id := normalizeFleetID(name)
	if id == "" {
		id = "device"
	}
	return id + ".json"
}

// WriteFleetOffer persists the offer at 0600 into every search dir that can be
// created. Returns the paths that were written (not the contents).
func WriteFleetOffer(o FleetOffer) ([]string, error) {
	o.Name = normalizeFleetID(firstNonEmpty(o.Name, "device"))
	o.MailboxID = strings.TrimSpace(o.MailboxID)
	o.MailboxKey = strings.TrimSpace(o.MailboxKey)
	o.OS = strings.TrimSpace(o.OS)
	if o.Name == "" || o.MailboxID == "" || o.MailboxKey == "" {
		return nil, fmt.Errorf("offer needs name, mailbox_id, and mailbox_key")
	}
	if o.WrittenAt == "" {
		o.WrittenAt = time.Now().UTC().Format(time.RFC3339)
	}
	raw, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')

	var written []string
	var lastErr error
	for _, dir := range FleetOfferSearchDirs() {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			lastErr = err
			continue
		}
		path := filepath.Join(dir, offerFileName(o.Name))
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, raw, 0o600); err != nil {
			lastErr = err
			continue
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			lastErr = err
			continue
		}
		written = append(written, path)
	}
	if len(written) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("could not write fleet offer")
	}
	return written, nil
}

// LoadFleetOffer finds the first offer file for name. The key stays in the
// returned struct — callers must not print it.
func LoadFleetOffer(name string) (FleetOffer, string, error) {
	want := offerFileName(name)
	var lastErr error
	for _, dir := range FleetOfferSearchDirs() {
		path := filepath.Join(dir, want)
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				lastErr = err
			}
			continue
		}
		var o FleetOffer
		if err := json.Unmarshal(data, &o); err != nil {
			lastErr = fmt.Errorf("parse offer %s: %w", filepath.Base(path), err)
			continue
		}
		o.Name = normalizeFleetID(firstNonEmpty(o.Name, name))
		o.MailboxID = strings.TrimSpace(o.MailboxID)
		o.MailboxKey = strings.TrimSpace(o.MailboxKey)
		if o.MailboxID == "" || o.MailboxKey == "" {
			lastErr = fmt.Errorf("offer %s incomplete", filepath.Base(path))
			continue
		}
		return o, path, nil
	}
	if lastErr != nil {
		return FleetOffer{}, "", lastErr
	}
	return FleetOffer{}, "", fmt.Errorf("no fleet offer for %q (run pair-as-mailbox on that machine)", normalizeFleetID(name))
}
