package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFleetUpsertGetRemove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	f, err := LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Get("local"); !ok {
		t.Fatal("local should always resolve")
	}
	if err := f.Upsert(FleetDevice{ID: "Studio Linux", MailboxID: "gbr-abc", MailboxKey: "k"}); err != nil {
		t.Fatal(err)
	}
	got, ok := f.Get("studio-linux")
	if !ok || got.MailboxID != "gbr-abc" || got.Kind != "relay" {
		t.Fatalf("got %+v", got)
	}
	pub := f.PublicDevices()
	if len(pub) != 1 || pub[0]["has_key"] != true {
		t.Fatalf("public %+v", pub)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gbr", FleetFileName)); err != nil {
		t.Fatal(err)
	}
	if err := f.Remove("studio-linux"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Get("studio-linux"); ok {
		t.Fatal("removed device still present")
	}
}

func TestFleetRejectsLocalID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	f, _ := LoadFleet()
	if err := f.Upsert(FleetDevice{ID: "local", MailboxID: "gbr-x", MailboxKey: "k"}); err == nil {
		t.Fatal("expected reject local")
	}
}

func TestFleetResolveClassAndNoSilentFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("GBR_DEVICE_CLASS", "mac-mini")
	f, err := LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Upsert(FleetDevice{ID: "studio-linux", MailboxID: "gbr-abc", MailboxKey: "k", OS: "linux", Class: "linux"}); err != nil {
		t.Fatal(err)
	}

	local, err := f.Resolve("")
	if err != nil || local.ID != "local" || local.Class != ClassMacMini {
		t.Fatalf("empty → local mac mini, got %+v err=%v", local, err)
	}

	linux, err := f.Resolve("linux")
	if err != nil || linux.ID != "studio-linux" {
		t.Fatalf("class linux → studio-linux, got %+v err=%v", linux, err)
	}

	mini, err := f.Resolve("mac-mini")
	if err != nil || mini.ID != "local" {
		t.Fatalf("class mac-mini → local, got %+v err=%v", mini, err)
	}

	_, err = f.Resolve("typo-box")
	if err == nil || !strings.Contains(err.Error(), "unknown_device") {
		t.Fatalf("unknown must not fall back to local: %v", err)
	}

	_, err = f.Resolve("phone")
	if err != ErrSpectatorDevice {
		t.Fatalf("phone must be spectator, got %v", err)
	}
}

func TestFleetResolveAmbiguousClass(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("GBR_DEVICE_CLASS", "laptop")
	f, _ := LoadFleet()
	_ = f.Upsert(FleetDevice{ID: "other-laptop", MailboxID: "gbr-y", MailboxKey: "k", Class: ClassLaptop, OS: "darwin"})
	_, err := f.Resolve("laptop")
	if err == nil || !strings.Contains(err.Error(), "ambiguous_device") {
		t.Fatalf("two laptops must be ambiguous, got %v", err)
	}
	got, err := f.Resolve("other-laptop")
	if err != nil || got.ID != "other-laptop" {
		t.Fatalf("id still works: %+v %v", got, err)
	}
}


