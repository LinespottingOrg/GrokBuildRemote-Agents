package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func withLeaseHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".gbr"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeHolder(t *testing.T) {
	if got := normalizeHolder("claude"); got != "claude-coworker" {
		t.Fatalf("claude → %q", got)
	}
	if got := normalizeHolder("Grok Bot"); got != "grok-bot" {
		t.Fatalf("Grok Bot → %q", got)
	}
	if got := normalizeHolder(""); got != "grok-bot" {
		t.Fatalf("empty → %q", got)
	}
}

func TestAcquireAndConflict(t *testing.T) {
	withLeaseHome(t)
	l, err := AcquireLease("proj-a", "grok-bot", "fix tests", time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	if l.Token == "" || l.Holder != "grok-bot" {
		t.Fatalf("lease: %+v", l)
	}
	_, err = AcquireLease("proj-a", "claude-coworker", "", time.Minute, false)
	if !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("want ErrLeaseHeld, got %v", err)
	}
	// same holder refreshes
	l2, err := AcquireLease("proj-a", "grok-bot", "again", time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	if l2.Token != l.Token {
		t.Fatal("refresh must keep token")
	}
	if l2.Goal != "again" {
		t.Fatalf("goal=%q", l2.Goal)
	}
	// steal
	l3, err := AcquireLease("proj-a", "claude", "take over", time.Minute, true)
	if err != nil {
		t.Fatal(err)
	}
	if l3.Holder != "claude-coworker" {
		t.Fatalf("holder=%q", l3.Holder)
	}
}

func TestReleaseLeaseOwner(t *testing.T) {
	withLeaseHome(t)
	if _, err := AcquireLease("s1", "grok-bot", "", time.Minute, false); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseLease("s1", "claude-coworker", false); !errors.Is(err, ErrLeaseOwner) {
		t.Fatalf("want owner err, got %v", err)
	}
	if err := ReleaseLease("s1", "grok-bot", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := GetLease("s1"); ok {
		t.Fatal("lease should be gone")
	}
}

func TestLeaseExpiryDropped(t *testing.T) {
	withLeaseHome(t)
	l, err := AcquireLease("old", "phone", "", MinLeaseTTL, false)
	if err != nil {
		t.Fatal(err)
	}
	l.Expires = time.Now().Add(-time.Second)
	leaseMu.Lock()
	m, _ := loadLeasesLocked()
	m["old"] = l
	_ = saveLeasesLocked(m)
	leaseMu.Unlock()
	if _, ok := GetLease("old"); ok {
		t.Fatal("expired lease must not be returned")
	}
}

func TestRefusePseudoSession(t *testing.T) {
	withLeaseHome(t)
	if _, err := AcquireLease("session", "grok-bot", "", time.Minute, false); err == nil {
		t.Fatal("pseudo session_id must be refused")
	}
	if _, err := AcquireLease("", "grok-bot", "", time.Minute, false); err == nil {
		t.Fatal("empty session_id must be refused")
	}
}
