package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitRemoteRepoName_HTTPS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGitConfig(t, dir, `https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git`)
	got := GitRemoteRepoName(dir)
	if got != "GrokBuildRemote-Agents" {
		t.Fatalf("got %q", got)
	}
	disp := GitRemoteDisplay(dir)
	if disp != "LinespottingOrg/GrokBuildRemote-Agents" {
		t.Fatalf("display=%q", disp)
	}
}

func TestGitRemoteRepoName_SSH(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGitConfig(t, dir, `git@github.com:Org/MyRepo.git`)
	got := GitRemoteRepoName(dir)
	if got != "MyRepo" {
		t.Fatalf("got %q", got)
	}
}

func TestGitRemote_WalksUp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeGitConfig(t, root, `https://github.com/acme/widgets.git`)
	sub := filepath.Join(root, "pkg", "api")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := GitRemoteRepoName(sub)
	if got != "widgets" {
		t.Fatalf("got %q", got)
	}
}

func TestGitRemote_NoRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if GitRemoteURL(dir) != "" || GitRemoteRepoName(dir) != "" {
		t.Fatal("expected empty")
	}
}

func TestRepoNameFromRemote(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"https://github.com/a/b.git":     "b",
		"https://github.com/a/b":         "b",
		"git@github.com:a/b.git":         "b",
		"ssh://git@github.com/a/b.git":   "b",
		"https://gitlab.com/g/sub/r.git": "r",
	}
	for in, want := range cases {
		if got := repoNameFromRemote(in); got != want {
			t.Errorf("repoNameFromRemote(%q)=%q want %q", in, got, want)
		}
	}
}

func writeGitConfig(t *testing.T, dir, url string) {
	t.Helper()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[remote \"origin\"]\n\turl = " + url + "\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}
