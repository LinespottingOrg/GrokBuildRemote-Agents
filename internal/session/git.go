package session

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// GitRemoteURL returns the origin (or first) remote URL for a git working tree at cwd.
// Pure filesystem read of .git/config — no exec. Returns "" if not a git repo or no remote.
func GitRemoteURL(cwd string) string {
	cfgPath, ok := gitConfigPath(cwd)
	if !ok {
		return ""
	}
	return parseRemoteURL(cfgPath)
}

// GitRemoteRepoName extracts the repository name slug fragment from the git remote URL.
// Examples:
//
//	https://github.com/Org/MyRepo.git  → "MyRepo"
//	git@github.com:Org/MyRepo.git      → "MyRepo"
//
// Returns "" if unavailable.
func GitRemoteRepoName(cwd string) string {
	url := GitRemoteURL(cwd)
	if url == "" {
		return ""
	}
	return repoNameFromRemote(url)
}

// GitRemoteDisplay returns a short org/repo style string when possible, else the repo name.
func GitRemoteDisplay(cwd string) string {
	url := GitRemoteURL(cwd)
	if url == "" {
		return ""
	}
	return displayFromRemote(url)
}

func gitConfigPath(cwd string) (string, bool) {
	if cwd == "" {
		return "", false
	}
	gitPath := filepath.Join(cwd, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		// Walk up a few levels for nested dirs (limited, deterministic).
		parent := filepath.Dir(cwd)
		if parent == cwd || parent == "" || parent == string(filepath.Separator) {
			return "", false
		}
		// Only one-level walk here; Resolve callers usually pass project root.
		// Deeper walk is intentional for subfolder shells:
		return gitConfigPathWalk(cwd, 6)
	}
	if info.IsDir() {
		p := filepath.Join(gitPath, "config")
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		return "", false
	}
	// .git file (worktree / submodule): gitdir: <path>
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(strings.ToLower(line), prefix) {
		return "", false
	}
	gitdir := strings.TrimSpace(line[len(prefix):])
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(cwd, gitdir)
	}
	p := filepath.Join(gitdir, "config")
	// Common worktree: config may live in main git dir parent.
	if _, err := os.Stat(p); err != nil {
		// try parent of gitdir (main .git)
		alt := filepath.Join(filepath.Dir(gitdir), "config")
		if _, err2 := os.Stat(alt); err2 == nil {
			return alt, true
		}
		return "", false
	}
	return p, true
}

func gitConfigPathWalk(start string, maxUp int) (string, bool) {
	dir := start
	for i := 0; i <= maxUp; i++ {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				p := filepath.Join(gitPath, "config")
				if _, err := os.Stat(p); err == nil {
					return p, true
				}
			} else {
				// re-enter via file form at this dir
				if p, ok := gitConfigPathAt(dir); ok {
					return p, true
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func gitConfigPathAt(cwd string) (string, bool) {
	gitPath := filepath.Join(cwd, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		p := filepath.Join(gitPath, "config")
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		return "", false
	}
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(strings.ToLower(line), prefix) {
		return "", false
	}
	gitdir := strings.TrimSpace(line[len(prefix):])
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(cwd, gitdir)
	}
	p := filepath.Join(gitdir, "config")
	if _, err := os.Stat(p); err == nil {
		return p, true
	}
	alt := filepath.Join(filepath.Dir(gitdir), "config")
	if _, err := os.Stat(alt); err == nil {
		return alt, true
	}
	return "", false
}

func parseRemoteURL(cfgPath string) string {
	f, err := os.Open(cfgPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var (
		inOrigin   bool
		inAnyRemote bool
		originURL  string
		firstURL   string
		section    string
	)

	sc := bufio.NewScanner(f)
	// git configs can be large but remotes are near the top; 1MB cap via Scanner default is fine
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			// [remote "origin"]
			inOrigin = section == `remote "origin"`
			inAnyRemote = strings.HasPrefix(section, `remote "`)
			continue
		}
		if !inOrigin && !inAnyRemote {
			continue
		}
		key, val, ok := splitConfigKV(line)
		if !ok || key != "url" {
			continue
		}
		if inOrigin && originURL == "" {
			originURL = val
		}
		if inAnyRemote && firstURL == "" {
			firstURL = val
		}
	}
	if originURL != "" {
		return originURL
	}
	return firstURL
}

func splitConfigKV(line string) (key, val string, ok bool) {
	// key = value  OR  key=value
	i := strings.IndexByte(line, '=')
	if i < 0 {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(line[:i]))
	val = strings.TrimSpace(line[i+1:])
	// strip optional quotes
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
	}
	return key, val, true
}

func repoNameFromRemote(url string) string {
	path := remotePath(url)
	if path == "" {
		return ""
	}
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	return strings.TrimSpace(path)
}

func displayFromRemote(url string) string {
	return remotePath(url)
}

// remotePath extracts org/repo (or deeper) path from a git remote URL.
func remotePath(url string) string {
	u := strings.TrimSpace(url)
	u = strings.TrimSuffix(u, "/")
	u = strings.TrimSuffix(u, ".git")
	if u == "" {
		return ""
	}

	var path string
	// Prefer scheme://host/path (https, ssh://, git://)
	if j := strings.Index(u, "://"); j >= 0 {
		rest := u[j+3:]
		if k := strings.IndexByte(rest, '/'); k >= 0 {
			path = rest[k+1:]
		}
	} else if i := strings.Index(u, ":"); i > 0 {
		// SCP-like: git@host:org/repo
		path = u[i+1:]
	} else {
		path = u
	}
	return strings.Trim(path, "/")
}
