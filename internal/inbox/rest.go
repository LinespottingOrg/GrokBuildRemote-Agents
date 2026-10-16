package inbox

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/proc"
)

// DefaultGH answers the same gh-shaped JSON the Watcher expects, but fetches it
// from the GitHub REST API over net/http instead of shelling out to `gh`. This
// removes the per-tick gh.exe child process that flashed a WindowsTerminal /
// OpenConsole window on Windows. It reuses the token `gh` already stores; the
// token is never logged.
//
// Supported arg shapes (produced by the Watcher):
//
//	issue list --repo R --label L --state open --limit 50 --json number,title,body,updatedAt
//	issue view N --repo R --comments --json title,body,comments
func DefaultGH(args ...string) ([]byte, error) {
	repo := argValue(args, "--repo")
	if repo == "" {
		return nil, fmt.Errorf("inbox: --repo missing in gh args")
	}
	switch {
	case len(args) >= 2 && args[0] == "issue" && args[1] == "list":
		return restListIssues(repo, argValue(args, "--label"))
	case len(args) >= 3 && args[0] == "issue" && args[1] == "view":
		n, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil {
			return nil, fmt.Errorf("inbox: bad issue number %q: %w", args[2], err)
		}
		return restViewIssue(repo, n)
	}
	return nil, fmt.Errorf("inbox: unsupported gh args %v", args)
}

func argValue(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

// apiBase is the REST base. GITHUB_API_URL allows GitHub Enterprise; default is
// the public API.
func apiBase() string {
	v := strings.TrimSpace(os.Getenv("GITHUB_API_URL"))
	if v == "" {
		return "https://api.github.com"
	}
	return strings.TrimRight(v, "/")
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func restGet(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, apiBase()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "gbr-agent-inbox")
	if tok := ghToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never include the Authorization header; GitHub error bodies do not
		// echo the token.
		return fmt.Errorf("github GET %s: %s: %s", path, resp.Status, trim(string(body), 300))
	}
	return json.Unmarshal(body, out)
}

type restIssue struct {
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	UpdatedAt   time.Time       `json:"updated_at"`
	PullRequest json.RawMessage `json:"pull_request"`
}

type restComment struct {
	NodeID    string    `json:"node_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
}

func restListIssues(repo, label string) ([]byte, error) {
	path := fmt.Sprintf("/repos/%s/issues?state=open&per_page=50", repo)
	if strings.TrimSpace(label) != "" {
		path += "&labels=" + urlQueryEscape(label)
	}
	var raw []restIssue
	if err := restGet(path, &raw); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(raw))
	for _, it := range raw {
		if len(it.PullRequest) > 0 { // /issues also returns PRs; skip them
			continue
		}
		out = append(out, Issue{Number: it.Number, Title: it.Title, Body: it.Body, UpdatedAt: it.UpdatedAt})
	}
	return json.Marshal(out)
}

func restViewIssue(repo string, n int) ([]byte, error) {
	var iss restIssue
	if err := restGet(fmt.Sprintf("/repos/%s/issues/%d", repo, n), &iss); err != nil {
		return nil, err
	}
	var rc []restComment
	if err := restGet(fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=100", repo, n), &rc); err != nil {
		return nil, err
	}
	comments := make([]Comment, 0, len(rc))
	for _, c := range rc {
		cm := Comment{ID: c.NodeID, Body: c.Body, CreatedAt: c.CreatedAt}
		cm.Author.Login = c.User.Login
		comments = append(comments, cm)
	}
	view := struct {
		Title    string    `json:"title"`
		Body     string    `json:"body"`
		Comments []Comment `json:"comments"`
	}{Title: iss.Title, Body: iss.Body, Comments: comments}
	return json.Marshal(view)
}

func urlQueryEscape(s string) string {
	// labels are simple (boss-steer); escape spaces and a few reserved chars.
	r := strings.NewReplacer(" ", "%20", "&", "%26", "#", "%23", "?", "%3F", "+", "%2B")
	return r.Replace(s)
}

// ghToken resolves the GitHub token the same way gh does, without ever logging
// it: GH_TOKEN / GITHUB_TOKEN env first, then the oauth_token for github.com in
// gh's hosts.yml, and finally `gh auth token` (covers the common case where gh
// keeps the token in the OS keyring/Credential Manager instead of hosts.yml).
// The CLI result is cached with a TTL so we spawn gh rarely, and that one child
// is created hidden (no console window flash).
func ghToken() string {
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	for _, p := range ghHostsPaths() {
		if tok := parseOAuthToken(p); tok != "" {
			return tok
		}
	}
	return ghCLIToken()
}

const ghTokenTTL = 30 * time.Minute

var (
	ghTokMu  sync.Mutex
	ghTokVal string
	ghTokAt  time.Time
)

// ghCLIToken returns `gh auth token`, cached for ghTokenTTL. gh reads the token
// from the OS keyring when hosts.yml has none. The value is never logged, and
// the gh child is spawned with no visible window.
func ghCLIToken() string {
	ghTokMu.Lock()
	defer ghTokMu.Unlock()
	if ghTokVal != "" && time.Since(ghTokAt) < ghTokenTTL {
		return ghTokVal
	}
	bin := ghBinary()
	if bin == "" {
		return ghTokVal // may be stale but better than nothing
	}
	cmd := proc.Command(bin, "auth", "token")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return ghTokVal
	}
	tok := strings.TrimSpace(string(out))
	if tok != "" {
		ghTokVal = tok
		ghTokAt = time.Now()
	}
	return ghTokVal
}

// ghBinary finds the gh executable: PATH first, then common install locations
// (Windows "Program Files", winget links) so the agent's PATH at logon does not
// have to include it.
func ghBinary() string {
	if p, err := exec.LookPath("gh"); err == nil {
		return p
	}
	var cands []string
	if runtime.GOOS == "windows" {
		pf := os.Getenv("ProgramFiles")
		pf86 := os.Getenv("ProgramFiles(x86)")
		la := os.Getenv("LOCALAPPDATA")
		for _, c := range []string{
			filepath.Join(pf, "gh.exe"),
			filepath.Join(pf, "GitHub CLI", "gh.exe"),
			filepath.Join(pf86, "GitHub CLI", "gh.exe"),
			filepath.Join(la, "Programs", "GitHub CLI", "gh.exe"),
			filepath.Join(la, "Microsoft", "WinGet", "Links", "gh.exe"),
		} {
			if c != "" {
				cands = append(cands, c)
			}
		}
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func ghHostsPaths() []string {
	var out []string
	if d := strings.TrimSpace(os.Getenv("GH_CONFIG_DIR")); d != "" {
		out = append(out, filepath.Join(d, "hosts.yml"))
	}
	if runtime.GOOS == "windows" {
		if ad := strings.TrimSpace(os.Getenv("AppData")); ad != "" {
			out = append(out, filepath.Join(ad, "GitHub CLI", "hosts.yml"))
		}
	}
	if x := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); x != "" {
		out = append(out, filepath.Join(x, "gh", "hosts.yml"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, filepath.Join(home, ".config", "gh", "hosts.yml"))
	}
	return out
}

// parseOAuthToken does a minimal YAML scan for:
//
//	github.com:
//	    oauth_token: ghp_xxx
//
// It avoids a YAML dependency and never logs the value.
func parseOAuthToken(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	inGitHub := false
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// top-level host key: no leading whitespace, ends with ':'
		if line == strings.TrimLeft(line, " \t") && strings.HasSuffix(trimmed, ":") {
			host := strings.TrimSuffix(trimmed, ":")
			inGitHub = strings.EqualFold(host, "github.com")
			continue
		}
		if inGitHub && strings.HasPrefix(trimmed, "oauth_token:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "oauth_token:"))
			val = strings.Trim(val, "\"'")
			return val
		}
	}
	return ""
}
