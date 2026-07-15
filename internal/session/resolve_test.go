package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePriority_GrokSessionFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, GrokSessionFile), []byte("global-edition\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// rename map would say otherwise — file wins
	renames := map[string]string{NormalizeCWD(dir): "from-rename"}
	id, src, err := ResolveSessionID(dir, renames)
	if err != nil {
		t.Fatal(err)
	}
	if id != "global-edition" || src != SourceFile {
		t.Fatalf("got id=%q src=%q want global-edition / %s", id, src, SourceFile)
	}
}

func TestResolvePriority_RenameMap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// no .grok-session
	cwd := NormalizeCWD(dir)
	renames := map[string]string{cwd: "my-custom-session"}
	id, src, err := ResolveSessionID(dir, renames)
	if err != nil {
		t.Fatal(err)
	}
	if id != "my-custom-session" || src != SourceRename {
		t.Fatalf("got id=%q src=%q", id, src)
	}
}

func TestResolvePriority_FallbackFolder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "MyProject")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id, src, err := ResolveSessionID(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceFallback {
		t.Fatalf("src=%q", src)
	}
	if id != "myproject" {
		t.Fatalf("id=%q want myproject", id)
	}
}

func TestResolvePriority_FallbackWithGit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "agents")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	id, src, err := ResolveSessionID(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceFallback {
		t.Fatalf("src=%q", src)
	}
	want := "agents-grokbuildremote-agents"
	if id != want {
		t.Fatalf("id=%q want %q", id, want)
	}
}

func TestResolve_RejectsTraversalInGrokSession(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, GrokSessionFile), []byte("../etc/passwd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, src, err := ResolveSessionID(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	// falls through to fallback
	if src != SourceFallback {
		t.Fatalf("expected fallback after bad file, got %s id=%s", src, id)
	}
}

func TestResolve_GrokSessionCommentsAndBlank(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "# comment\n\n  cool-session  \n"
	if err := os.WriteFile(filepath.Join(dir, GrokSessionFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	id, src, err := ResolveSessionID(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if id != "cool-session" || src != SourceFile {
		t.Fatalf("got %q %q", id, src)
	}
}

func TestFallbackSessionID_NoGit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "Hello_World")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	got := FallbackSessionID(dir)
	if got != "hello-world" {
		t.Fatalf("got %q", got)
	}
}
