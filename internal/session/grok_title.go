package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type grokSummaryFile struct {
	Title          string `json:"title"`
	GeneratedTitle string `json:"generated_title"`
	SessionSummary string `json:"session_summary"`
	LastActiveAt   string `json:"last_active_at"`
	UpdatedAt      string `json:"updated_at"`
	Info           struct {
		Title string `json:"title"`
	} `json:"info"`
}

var (
	grokTitleMu     sync.Mutex
	grokTitleCached string
	grokTitleAt     time.Time
)

// ResolveDisplayTitle picks the name the phone should show.
// Priority: stored label → pinned Grok /rename title → short live title →
// Grok generated_title → live window title → shell.
func ResolveDisplayTitle(id, live, shell string, labels map[string]string) string {
	id = strings.TrimSpace(id)
	live = strings.TrimSpace(live)
	shell = strings.TrimSpace(shell)
	if labels != nil {
		if t := strings.TrimSpace(labels[id]); t != "" {
			return t
		}
	}
	grokish := looksLikeGrok(id) || looksLikeGrok(shell) || looksLikeGrok(live)
	if grokish {
		pinned, generated := latestGrokSessionTitles()
		if pinned != "" {
			return pinned
		}
		if liveLooksHuman(live) {
			return live
		}
		if generated != "" {
			return generated
		}
	}
	// Reject sticky Windows mash (conhost / C:\Program Files\…) so a later
	// /rename or gbr-agent rename is what the phone sees.
	if liveLooksHuman(live) {
		return live
	}
	if shell != "" {
		return shell
	}
	if live != "" {
		return live
	}
	return id
}

func looksLikeGrok(s string) bool {
	return strings.Contains(strings.ToLower(s), "grok")
}

func liveLooksHuman(live string) bool {
	if live == "" {
		return false
	}
	lt := strings.ToLower(live)
	switch lt {
	case "conhost", "grok-build", "grok", "window", "windows-terminal",
		"cmd", "pwsh", "powershell", "c: program", "c:program":
		return false
	}
	if strings.Contains(lt, " - grok") {
		return false
	}
	if strings.HasPrefix(lt, "grok 4") {
		return false
	}
	if strings.HasPrefix(lt, "c:") || strings.Contains(lt, "program files") ||
		strings.Contains(lt, `windows\system32`) || strings.Contains(lt, "windows/system32") {
		return false
	}
	if len(live) > 80 {
		return false
	}
	return true
}

func latestGrokSessionTitles() (pinned, generated string) {
	grokTitleMu.Lock()
	defer grokTitleMu.Unlock()
	if time.Since(grokTitleAt) < 5*time.Second && grokTitleCached != "" {
		// cached is "pinned\x00generated"
		parts := strings.SplitN(grokTitleCached, "\x00", 2)
		pinned = parts[0]
		if len(parts) > 1 {
			generated = parts[1]
		}
		return pinned, generated
	}
	pinned, generated = scanLatestGrokSummary()
	grokTitleCached = pinned + "\x00" + generated
	grokTitleAt = time.Now()
	return pinned, generated
}

func scanLatestGrokSummary() (pinned, generated string) {
	home := strings.TrimSpace(os.Getenv("GROK_HOME"))
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(h, ".grok")
		}
	}
	if home == "" {
		return "", ""
	}
	root := filepath.Join(home, "sessions")
	var best time.Time
	var bestPinned, bestGen string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "summary.json" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var sum grokSummaryFile
		if json.Unmarshal(raw, &sum) != nil {
			return nil
		}
		when := parseGrokTime(sum.LastActiveAt)
		if when.IsZero() {
			when = parseGrokTime(sum.UpdatedAt)
		}
		if !best.IsZero() && !when.After(best) {
			return nil
		}
		pin := firstNonEmptyTrim(sum.Title, sum.Info.Title)
		gen := firstNonEmptyTrim(sum.GeneratedTitle, sum.SessionSummary)
		best = when
		bestPinned = pin
		bestGen = gen
		return nil
	})
	return bestPinned, bestGen
}

func parseGrokTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

func firstNonEmptyTrim(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}
