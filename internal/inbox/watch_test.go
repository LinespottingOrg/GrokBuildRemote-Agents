package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchSessionExactAndPrefix(t *testing.T) {
	ss := []Session{{ID: "a", Title: "GBR claw compat"}, {ID: "b", Title: "other"}}
	if got := matchSession("GBR claw compat", ss); got != "a" {
		t.Fatalf("exact got %q", got)
	}
	ss[0].Title = "GBR claw compat - grok"
	if got := matchSession("GBR claw compat", ss); got != "a" {
		t.Fatalf("prefix got %q", got)
	}
	if got := matchSession("nope", ss); got != "" {
		t.Fatalf("miss got %q", got)
	}
}

func TestIsAgentReport(t *testing.T) {
	if !isAgentReport("bound. slug=x") {
		t.Fatal("bound")
	}
	if !isAgentReport("result: watcher up") {
		t.Fatal("result")
	}
	if isAgentReport("PRIO 1 no-paste. Inbox comment IS the prompt.") {
		t.Fatal("boss order should inject")
	}
}

func TestTickSeedsThenInjects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GBR_INBOX_SEEN", filepath.Join(dir, "seen.json"))

	list1 := []Issue{{Number: 75, Title: "GBR claw compat", Body: "body"}}
	comments1 := []Comment{{ID: "c1", Body: "old boss"}}
	gh := fakeGH(t, list1, comments1)
	w := New("", "", gh)
	acts, err := w.Tick([]Session{{ID: "sess-1", Title: "GBR claw compat"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 0 {
		t.Fatalf("seed should not inject, got %#v", acts)
	}

	comments2 := []Comment{
		{ID: "c1", Body: "old boss"},
		{ID: "c2", Body: "bound. slug=x\nresult: starting"},
		{ID: "c3", Body: "NEW ORDER from BOSS"},
	}
	w.GH = fakeGH(t, list1, comments2)
	acts, err = w.Tick([]Session{{ID: "sess-1", Title: "GBR claw compat"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Kind != "inject" || acts[0].SessionID != "sess-1" {
		t.Fatalf("want inject sess-1, got %#v", acts)
	}
	if acts[0].Text != "NEW ORDER from BOSS" {
		t.Fatalf("text %q", acts[0].Text)
	}
}

func TestTickSpawnWhenNoWindow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GBR_INBOX_SEEN", filepath.Join(dir, "seen.json"))
	list := []Issue{{Number: 75, Title: "GBR claw compat", Body: "issue body here"}}
	w := New("", "", fakeGH(t, list, []Comment{{ID: "c1", Body: "old"}}))
	_, _ = w.Tick(nil) // seed
	w.GH = fakeGH(t, list, []Comment{{ID: "c1", Body: "old"}, {ID: "c9", Body: "later"}})
	acts, err := w.Tick(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Kind != "spawn" {
		t.Fatalf("want spawn, got %#v", acts)
	}
	if !strings.Contains(acts[0].Text, "issue body here") {
		t.Fatalf("spawn should inject issue body, got %q", acts[0].Text)
	}
}

func fakeGH(t *testing.T, issues []Issue, comments []Comment) GH {
	t.Helper()
	return func(args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "issue list") {
			return json.Marshal(issues)
		}
		if strings.Contains(joined, "issue view") {
			title := ""
			body := ""
			if len(issues) > 0 {
				title = issues[0].Title
				body = issues[0].Body
			}
			return json.Marshal(map[string]any{
				"title":    title,
				"body":     body,
				"comments": comments,
			})
		}
		t.Fatalf("unexpected gh %v", args)
		return nil, nil
	}
}

func TestSeenPathEnv(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	t.Setenv("GBR_INBOX_SEEN", p)
	got, err := seenPath()
	if err != nil || got != p {
		t.Fatalf("got %q err %v", got, err)
	}
	_ = os.WriteFile(p, []byte(`{"issues":{}}`), 0o600)
}
