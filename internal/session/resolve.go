package session

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ResolveSource indicates which priority tier produced the session_id.
type ResolveSource string

const (
	// SourceFile is .grok-session first line.
	SourceFile ResolveSource = "grok-session"
	// SourceRename is the explicit rename map (sessions.json).
	SourceRename ResolveSource = "rename"
	// SourceFallback is folder name (+ optional git repo) slug.
	SourceFallback ResolveSource = "fallback"
)

// GrokSessionFile is the per-cwd session identity file (first line = session_id).
const GrokSessionFile = ".grok-session"

// ResolveSessionID applies protocol naming priority for a working directory.
//
//  1. .grok-session first line (must be valid or slugifiable to valid)
//  2. renames[normalized cwd]
//  3. {folder} or {folder}-{git-repo} slugified
//
// renames may be nil. cwd is cleaned; empty cwd returns ("session", SourceFallback, nil).
func ResolveSessionID(cwd string, renames map[string]string) (id string, src ResolveSource, err error) {
	cwd = NormalizeCWD(cwd)

	// 1. .grok-session
	if line, ok, rerr := readGrokSessionLine(cwd); rerr != nil {
		return "", "", rerr
	} else if ok {
		if ValidSessionID(line) {
			return line, SourceFile, nil
		}
		if slug := Slugify(line); ValidSessionID(slug) && line != "" {
			return slug, SourceFile, nil
		}
		// invalid empty/unusable file content → fall through
	}

	// 2. explicit rename map
	if renames != nil {
		if v, ok := lookupRename(renames, cwd); ok && ValidSessionID(v) {
			return v, SourceRename, nil
		}
		// if stored value is non-slug, slugify once
		if v, ok := lookupRename(renames, cwd); ok {
			if slug := Slugify(v); ValidSessionID(slug) {
				return slug, SourceRename, nil
			}
		}
	}

	// 3. fallback folder (+ git repo name)
	return FallbackSessionID(cwd), SourceFallback, nil
}

// FallbackSessionID builds the slug from folder name and optional git remote repo.
func FallbackSessionID(cwd string) string {
	cwd = NormalizeCWD(cwd)
	folder := filepath.Base(cwd)
	if folder == "" || folder == "." || folder == string(filepath.Separator) {
		folder = "session"
	}

	repo := GitRemoteRepoName(cwd)
	var raw string
	if repo != "" && !strings.EqualFold(repo, folder) {
		raw = folder + "-" + repo
	} else {
		raw = folder
	}
	return Slugify(raw)
}

// NormalizeCWD cleans and absolutizes when possible.
func NormalizeCWD(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	cwd = filepath.Clean(cwd)
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	return cwd
}

func readGrokSessionLine(cwd string) (line string, ok bool, err error) {
	if cwd == "" {
		return "", false, nil
	}
	p := filepath.Join(cwd, GrokSessionFile)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// first non-empty logical line
	for sc.Scan() {
		line = strings.TrimSpace(sc.Text())
		// strip UTF-8 BOM if present
		line = strings.TrimPrefix(line, "\ufeff")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// reject path traversal / separators in raw claim
		if strings.Contains(line, "..") || strings.ContainsAny(line, `/\:`) {
			return "", false, nil
		}
		return line, true, nil
	}
	if err := sc.Err(); err != nil {
		return "", false, err
	}
	return "", false, nil
}

func lookupRename(renames map[string]string, cwd string) (string, bool) {
	if v, ok := renames[cwd]; ok {
		return v, true
	}
	// try alternate slash forms (Windows)
	alt := cwd
	if strings.Contains(cwd, `\`) {
		alt = strings.ReplaceAll(cwd, `\`, `/`)
	} else if strings.Contains(cwd, `/`) {
		alt = strings.ReplaceAll(cwd, `/`, `\`)
	}
	if alt != cwd {
		if v, ok := renames[alt]; ok {
			return v, true
		}
	}
	// case-insensitive path match (Windows-friendly)
	lower := strings.ToLower(cwd)
	for k, v := range renames {
		if strings.ToLower(NormalizeCWD(k)) == lower {
			return v, true
		}
	}
	return "", false
}
