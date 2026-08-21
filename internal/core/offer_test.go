package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteLoadFleetOffer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("GBR_FLEET_OFFER_DIR", "")

	written, err := WriteFleetOffer(FleetOffer{
		Name: "Mac", MailboxID: "gbr-testmb", MailboxKey: "super-secret-key", OS: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("expected at least one offer path")
	}
	local := filepath.Join(dir, ".gbr", OfferDirName, "mac.json")
	foundLocal := false
	for _, p := range written {
		if p == local {
			foundLocal = true
		}
	}
	if !foundLocal {
		t.Fatalf("missing local offer %s in %v", local, written)
	}
	st, err := os.Stat(local)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("perm %o want 0600", st.Mode().Perm())
	}

	got, path, err := LoadFleetOffer("mac")
	if err != nil {
		t.Fatal(err)
	}
	if path != local {
		t.Fatalf("path %s want %s", path, local)
	}
	if got.MailboxID != "gbr-testmb" || got.MailboxKey != "super-secret-key" || got.OS != "darwin" {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadFleetOfferMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("GBR_FLEET_OFFER_DIR", filepath.Join(dir, "empty-offers"))
	_, _, err := LoadFleetOffer("mac")
	if err == nil {
		t.Fatal("expected missing offer")
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatal("error must not contain a key")
	}
}

func TestFleetOfferEnvDirWins(t *testing.T) {
	dir := t.TempDir()
	extra := filepath.Join(dir, "extra")
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("USERPROFILE", filepath.Join(dir, "home"))
	t.Setenv("GBR_FLEET_OFFER_DIR", extra)
	if err := os.MkdirAll(extra, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteFleetOffer(FleetOffer{Name: "mac", MailboxID: "gbr-x", MailboxKey: "k", OS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	got, path, err := LoadFleetOffer("mac")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != extra {
		t.Fatalf("want env dir, got %s", path)
	}
	if got.MailboxID != "gbr-x" {
		t.Fatalf("got %+v", got)
	}
}
