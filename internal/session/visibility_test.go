package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVisibilityDefaultIsAllVisible(t *testing.T) {
	v, err := OpenVisibility(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Hidden("admin-shell") {
		t.Fatal("missing file must leave every session visible")
	}
	if got := v.List(); len(got) != 0 {
		t.Fatalf("list=%v", got)
	}
}

func TestVisibilityPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roster-visibility.json")
	v, err := OpenVisibility(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set("admin-shell", true, "Administrator", "bash"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set("proj-app", false, "", ""); err != nil {
		t.Fatal(err)
	}

	again, err := OpenVisibility(path)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Hidden("admin-shell") {
		t.Fatal("hide did not survive reopen")
	}
	if again.Hidden("proj-app") {
		t.Fatal("visible session was stored as hidden")
	}
	listed := again.List()
	if len(listed) != 1 || listed[0].SessionID != "admin-shell" || listed[0].Title != "Administrator" {
		t.Fatalf("list=%+v", listed)
	}

	if err := again.Set("admin-shell", false, "", ""); err != nil {
		t.Fatal(err)
	}
	third, err := OpenVisibility(path)
	if err != nil {
		t.Fatal(err)
	}
	if third.Hidden("admin-shell") {
		t.Fatal("unhide did not survive reopen")
	}
}

func TestVisibilityRejectsBadID(t *testing.T) {
	v, err := OpenVisibility(filepath.Join(t.TempDir(), "v.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set("../admin", true, "", ""); err == nil {
		t.Fatal("path-like id must be rejected")
	}
	if err := v.Set("a", true, "", ""); err == nil {
		t.Fatal("too-short id must be rejected")
	}
}

func TestVisibilityIgnoresCorruptAndKeepsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roster-visibility.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenVisibility(path); err == nil {
		t.Fatal("corrupt file must error so the caller can keep all-visible without wiping it")
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != "{" {
		t.Fatalf("corrupt file was rewritten: %q", buf)
	}
}

func TestEmptyVisibilitySetDoesNotPanic(t *testing.T) {
	v := EmptyVisibility()
	if err := v.Set("admin-shell", true, "Admin", "zsh"); err != nil {
		t.Fatal(err)
	}
	if !v.Hidden("admin-shell") {
		t.Fatal("memory store should remember the hide")
	}
}
