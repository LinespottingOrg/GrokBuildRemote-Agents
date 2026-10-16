package inbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArgValue(t *testing.T) {
	args := []string{"issue", "list", "--repo", "Org/Repo", "--label", "boss-steer"}
	if got := argValue(args, "--repo"); got != "Org/Repo" {
		t.Fatalf("repo %q", got)
	}
	if got := argValue(args, "--label"); got != "boss-steer" {
		t.Fatalf("label %q", got)
	}
	if got := argValue(args, "--missing"); got != "" {
		t.Fatalf("missing should be empty, got %q", got)
	}
}

func TestParseOAuthTokenHostsYML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hosts.yml")
	body := "github.com:\n" +
		"    oauth_token: ghp_TESTTOKEN123\n" +
		"    user: someone\n" +
		"    git_protocol: https\n" +
		"example.com:\n" +
		"    oauth_token: should_not_win\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := parseOAuthToken(p); got != "ghp_TESTTOKEN123" {
		t.Fatalf("token %q", got)
	}
	if parseOAuthToken(filepath.Join(dir, "nope.yml")) != "" {
		t.Fatal("missing file should return empty")
	}
}

func TestGhTokenPrefersEnv(t *testing.T) {
	t.Setenv("GH_TOKEN", "env_tok")
	t.Setenv("GITHUB_TOKEN", "")
	if got := ghToken(); got != "env_tok" {
		t.Fatalf("env token %q", got)
	}
}

func TestURLQueryEscape(t *testing.T) {
	if got := urlQueryEscape("boss steer"); got != "boss%20steer" {
		t.Fatalf("escape %q", got)
	}
}
