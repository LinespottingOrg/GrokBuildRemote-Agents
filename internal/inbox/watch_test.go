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

func bossComment(id, body string) Comment {
	return comment(id, DefaultAuthor, body)
}

func comment(id, login, body string) Comment {
	c := Comment{ID: id, Body: body}
	c.Author.Login = login
	return c
}

func TestTickSeedsThenInjects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GBR_INBOX_SEEN", filepath.Join(dir, "seen.json"))

	list1 := []Issue{{Number: 75, Title: "GBR claw compat", Body: "body"}}
	comments1 := []Comment{bossComment("c1", "old boss")}
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
		bossComment("c1", "old boss"),
		bossComment("c2", "bound. slug=x\nresult: starting"),
		bossComment("c3", "NEW ORDER from BOSS"),
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
	w := New("", "", fakeGH(t, list, []Comment{bossComment("c1", "old")}))
	_, _ = w.Tick(nil) // seed
	w.GH = fakeGH(t, list, []Comment{bossComment("c1", "old"), bossComment("c9", "later")})
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

func TestTickSkipsOtherAuthors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GBR_INBOX_SEEN", filepath.Join(dir, "seen.json"))
	list := []Issue{{Number: 75, Title: "GBR claw compat", Body: "body"}}
	w := New("", "", fakeGH(t, list, []Comment{bossComment("c1", "old")}))
	_, _ = w.Tick([]Session{{ID: "sess-1", Title: "GBR claw compat"}})

	w.GH = fakeGH(t, list, []Comment{
		bossComment("c1", "old"),
		comment("c-stranger", "random-user", "steal this window"),
	})
	acts, err := w.Tick([]Session{{ID: "sess-1", Title: "GBR claw compat"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 0 {
		t.Fatalf("non-allowlisted author must not inject/spawn, got %#v", acts)
	}

	w.GH = fakeGH(t, list, []Comment{
		bossComment("c1", "old"),
		comment("c-stranger", "random-user", "steal this window"),
		bossComment("c-boss", "real order"),
	})
	acts, err = w.Tick([]Session{{ID: "sess-1", Title: "GBR claw compat"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Kind != "inject" || acts[0].Text != "real order" {
		t.Fatalf("want BOSS inject after skipping stranger, got %#v", acts)
	}
}

func TestTickEmptyAuthorSkipped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GBR_INBOX_SEEN", filepath.Join(dir, "seen.json"))
	list := []Issue{{Number: 75, Title: "GBR claw compat", Body: "body"}}
	w := New("", "", fakeGH(t, list, []Comment{bossComment("c1", "old")}))
	_, _ = w.Tick(nil)
	w.GH = fakeGH(t, list, []Comment{
		bossComment("c1", "old"),
		comment("c-anon", "", "no login"),
	})
	acts, err := w.Tick(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 0 {
		t.Fatalf("empty author must not spawn, got %#v", acts)
	}
}

func TestTickAuthorsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GBR_INBOX_SEEN", filepath.Join(dir, "seen.json"))
	t.Setenv("GBR_INBOX_AUTHORS", "alice, bob")
	list := []Issue{{Number: 75, Title: "GBR claw compat", Body: "body"}}
	w := New("", "", fakeGH(t, list, []Comment{comment("c1", "alice", "old")}))
	_, _ = w.Tick([]Session{{ID: "sess-1", Title: "GBR claw compat"}})
	w.GH = fakeGH(t, list, []Comment{
		comment("c1", "alice", "old"),
		comment("c2", "LinespottingPrivate", "default author no longer enough"),
		comment("c3", "Bob", "from env allowlist"),
	})
	acts, err := w.Tick([]Session{{ID: "sess-1", Title: "GBR claw compat"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Text != "from env allowlist" {
		t.Fatalf("want bob inject, got %#v", acts)
	}
}

func TestAuthorAllowedDefault(t *testing.T) {
	t.Setenv("GBR_INBOX_AUTHORS", "")
	if !authorAllowed("LinespottingPrivate") || !authorAllowed("linespottingprivate") {
		t.Fatal("default BOSS login must match case-insensitive")
	}
	if authorAllowed("octocat") || authorAllowed("") {
		t.Fatal("strangers and empty must be denied")
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

func TestStripLeadingRename(t *testing.T) {
	got := StripLeadingRename("/rename GBR claw compat\nreal job")
	if got != "real job" {
		t.Fatalf("got %q", got)
	}
	if StripLeadingRename("/title Foo\nbar") != "bar" {
		t.Fatal("title alias")
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
