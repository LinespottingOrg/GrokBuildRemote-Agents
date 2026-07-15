package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRenamePersistAndLookup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cwd := NormalizeCWD(filepath.Join(dir, "proj"))
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := st.Rename(cwd, "Global Edition"); err != nil {
		t.Fatal(err)
	}
	id, ok := st.Lookup(cwd)
	if !ok || id != "global-edition" {
		t.Fatalf("lookup got %q ok=%v", id, ok)
	}

	// reload from disk
	st2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	id2, ok2 := st2.Lookup(cwd)
	if !ok2 || id2 != "global-edition" {
		t.Fatalf("reload got %q ok=%v", id2, ok2)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f sessionsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Renames) != 1 {
		t.Fatalf("renames=%v", f.Renames)
	}
}

func TestStoreUnrename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cwd := NormalizeCWD(dir)
	if err := st.Rename(cwd, "abc-session"); err != nil {
		t.Fatal(err)
	}
	if err := st.Unrename(cwd); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Lookup(cwd); ok {
		t.Fatal("expected miss after unrename")
	}
}

func TestStoreRejectsPathSessionID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Rename(dir, "foo/bar"); err == nil {
		t.Fatal("expected error for path-like session id")
	}
}

func TestDefaultStorePath(t *testing.T) {
	t.Parallel()
	p := DefaultStorePath()
	if p == "" {
		t.Fatal("empty path")
	}
	if filepath.Base(p) != "sessions.json" {
		t.Fatalf("base=%s", filepath.Base(p))
	}
	if filepath.Base(filepath.Dir(p)) != ".gbr" {
		t.Fatalf("dir=%s", filepath.Dir(p))
	}
}

func TestOpenStoreMissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nope", "sessions.json")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Snapshot()) != 0 {
		t.Fatal("expected empty")
	}
}
