package core

import (
	"os"
	"path/filepath"
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
