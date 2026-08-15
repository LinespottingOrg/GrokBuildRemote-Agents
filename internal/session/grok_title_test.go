package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveDisplayTitle_LabelWins(t *testing.T) {
	labels := map[string]string{"grok-build-1": "Phone Grok"}
	got := ResolveDisplayTitle("grok-build-1", "Grok 4.6 - grok", "grok-build", labels)
	if got != "Phone Grok" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveDisplayTitle_GrokPinnedFromDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GROK_HOME", home)
	dir := filepath.Join(home, "sessions", "cwd", "sid")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{
  "title": "Pinned From Slash Rename",
  "generated_title": "auto mash",
  "last_active_at": "2026-08-15T19:00:00Z"
}`
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	grokTitleMu.Lock()
	grokTitleAt = time.Time{}
	grokTitleCached = ""
	grokTitleMu.Unlock()

	got := ResolveDisplayTitle("grok-build-abc", "Grok 4.6 - grok", "grok-build", nil)
	if got != "Pinned From Slash Rename" {
		t.Fatalf("got %q", got)
	}
}

func TestLiveLooksHuman(t *testing.T) {
	if liveLooksHuman("Grok 4.6 - grok") {
		t.Fatal("wt mash should not look human")
	}
	if !liveLooksHuman("Phone Grok") {
		t.Fatal("short rename should look human")
	}
}


