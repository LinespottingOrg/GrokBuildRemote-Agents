// Package inbox polls GitHub issues (boss-steer) and turns new comments
// into Grok Build injects so BOSS does not paste email into the TUI.
package inbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultRepo  = "LinespottingOrg/grok-build-inbox"
	DefaultLabel = "boss-steer"
	seenFileName = "inbox-seen.json"
)

// GH runs `gh` with args and returns stdout. Tests replace this.
type GH func(args ...string) ([]byte, error)

func DefaultGH(args ...string) ([]byte, error) {
	bin, err := exec.LookPath("gh")
	if err != nil {
		return nil, fmt.Errorf("gh not on PATH — install GitHub CLI")
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, trim(string(out), 400))
	}
	return out, nil
}

type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
}

type Session struct {
	ID    string
	Title string
}

type Action struct {
	Kind      string // inject | spawn
	Issue     int
	Title     string
	SessionID string
	Text      string
	CommentID string
}

type Watcher struct {
	Repo  string
	Label string
	GH    GH

	mu   sync.Mutex
	seen map[int]map[string]bool
	path string
}

func New(repo, label string, gh GH) *Watcher {
	if repo == "" {
		repo = DefaultRepo
	}
	if label == "" {
		label = DefaultLabel
	}
	if gh == nil {
		gh = DefaultGH
	}
	w := &Watcher{Repo: repo, Label: label, GH: gh, seen: map[int]map[string]bool{}}
	_ = w.load()
	return w
}

func (w *Watcher) Tick(sessions []Session) ([]Action, error) {
	raw, err := w.GH("issue", "list", "--repo", w.Repo, "--label", w.Label, "--state", "open", "--limit", "50", "--json", "number,title,body,updatedAt")
	if err != nil {
		return nil, err
	}
	var issues []Issue
	if err := json.Unmarshal(raw, &issues); err != nil {
		return nil, fmt.Errorf("parse issue list: %w", err)
	}
	var out []Action
	for _, iss := range issues {
		acts, err := w.tickIssue(iss, sessions)
		if err != nil {
			return out, err
		}
		out = append(out, acts...)
	}
	return out, nil
}

func (w *Watcher) tickIssue(iss Issue, sessions []Session) ([]Action, error) {
	raw, err := w.GH("issue", "view", fmt.Sprintf("%d", iss.Number), "--repo", w.Repo, "--comments", "--json", "title,body,comments")
	if err != nil {
		return nil, err
	}
	var view struct {
		Title    string    `json:"title"`
		Body     string    `json:"body"`
		Comments []Comment `json:"comments"`
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		return nil, fmt.Errorf("parse issue %d: %w", iss.Number, err)
	}
	title := strings.TrimSpace(view.Title)
	if title == "" {
		title = strings.TrimSpace(iss.Title)
	}

	w.mu.Lock()
	if w.seen[iss.Number] == nil {
		w.seen[iss.Number] = map[string]bool{}
	}
	seeded := len(w.seen[iss.Number]) == 0
	if seeded {
		for _, c := range view.Comments {
			if c.ID != "" {
				w.seen[iss.Number][c.ID] = true
			}
		}
		w.mu.Unlock()
		_ = w.save()
		return nil, nil
	}
	w.mu.Unlock()

	var newest *Comment
	for i := range view.Comments {
		c := &view.Comments[i]
		if isAgentReport(c.Body) {
			w.mark(iss.Number, c.ID)
			continue
		}
		if w.already(iss.Number, c.ID) {
			continue
		}
		newest = c
	}
	if newest == nil {
		return nil, nil
	}

	sid := matchSession(title, sessions)
	if sid != "" {
		w.mark(iss.Number, newest.ID)
		_ = w.save()
		return []Action{{
			Kind:      "inject",
			Issue:     iss.Number,
			Title:     title,
			SessionID: sid,
			Text:      strings.TrimSpace(newest.Body),
			CommentID: newest.ID,
		}}, nil
	}

	w.mark(iss.Number, newest.ID)
	_ = w.save()
	body := strings.TrimSpace(view.Body)
	if body == "" {
		body = strings.TrimSpace(newest.Body)
	}
	return []Action{{
		Kind:      "spawn",
		Issue:     iss.Number,
		Title:     title,
		Text:      body,
		CommentID: newest.ID,
	}}, nil
}

func matchSession(title string, sessions []Session) string {
	want := normTitle(title)
	if want == "" {
		return ""
	}
	for _, s := range sessions {
		got := normTitle(s.Title)
		if got == want {
			return s.ID
		}
		// Grok window titles often look like "GBR claw compat - grok"
		if strings.HasPrefix(got, want+" -") || strings.HasPrefix(got, want+" —") {
			return s.ID
		}
	}
	return ""
}

func normTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.ToLower(s)
}

func isAgentReport(body string) bool {
	b := strings.TrimSpace(body)
	if b == "" {
		return true
	}
	low := strings.ToLower(b)
	if strings.HasPrefix(low, "bound.") {
		return true
	}
	first := strings.SplitN(low, "\n", 2)[0]
	return strings.HasPrefix(first, "result:") || strings.HasPrefix(first, "result ")
}

func (w *Watcher) already(n int, id string) bool {
	if id == "" {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seen[n][id]
}

func (w *Watcher) mark(n int, id string) {
	if id == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.seen[n] == nil {
		w.seen[n] = map[string]bool{}
	}
	w.seen[n][id] = true
}

func (w *Watcher) load() error {
	p, err := seenPath()
	if err != nil {
		return err
	}
	w.path = p
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var file struct {
		Issues map[string][]string `json:"issues"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seen = map[int]map[string]bool{}
	for k, ids := range file.Issues {
		var n int
		fmt.Sscanf(k, "%d", &n)
		if n == 0 {
			continue
		}
		w.seen[n] = map[string]bool{}
		for _, id := range ids {
			w.seen[n][id] = true
		}
	}
	return nil
}

func (w *Watcher) save() error {
	p, err := seenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	w.mu.Lock()
	file := struct {
		Issues map[string][]string `json:"issues"`
	}{Issues: map[string][]string{}}
	for n, ids := range w.seen {
		key := fmt.Sprintf("%d", n)
		for id := range ids {
			file.Issues[key] = append(file.Issues[key], id)
		}
	}
	w.mu.Unlock()
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}

func seenPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("GBR_INBOX_SEEN")); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gbr", seenFileName), nil
}

// StripLeadingRename removes a first-line /rename or /title so the job body
// is never submitted as a slash command paste.
func StripLeadingRename(text string) string {
	s := strings.TrimSpace(text)
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return s
	}
	first := strings.TrimSpace(lines[0])
	low := strings.ToLower(first)
	if strings.HasPrefix(low, "/rename ") || low == "/rename" || strings.HasPrefix(low, "/title ") || low == "/title" {
		return strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	return s
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
