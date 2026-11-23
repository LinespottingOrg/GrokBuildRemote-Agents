package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/grok"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session"
)

func TestPhonePayloadsOmitHidden(t *testing.T) {
	dir := t.TempDir()
	vis, err := session.OpenVisibility(filepath.Join(dir, "roster-visibility.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := vis.Set("admin-shell", true, "Admin", "bash"); err != nil {
		t.Fatal(err)
	}
	rt := &agentRuntime{
		vis:     vis,
		scanner: session.NewScanner(nil, session.NewRegistry(), nil),
	}
	rt.scanner.Registry.Upsert(&session.Session{ID: "admin-shell", Title: "Admin", Shell: "bash", CWD: "/tmp/admin"})
	rt.scanner.Registry.Upsert(&session.Session{ID: "proj-app", Title: "Proj", Shell: "zsh", CWD: "/tmp/proj"})

	phone := rt.phoneSessionPayloads()
	if len(phone) != 1 {
		t.Fatalf("phone roster=%v", phone)
	}
	id, _ := phone[0]["session_id"].(string)
	if id != "proj-app" {
		t.Fatalf("phone saw %s", id)
	}

	local := rt.listSessionPayloads()
	var sawHidden bool
	for _, row := range local {
		rid, _ := row["session_id"].(string)
		if rid == "admin-shell" {
			sawHidden = true
			if row["hidden"] != true {
				t.Fatalf("local row missing hidden flag: %#v", row)
			}
		}
	}
	if !sawHidden {
		t.Fatal("local list dropped the hidden session")
	}
}

func TestPhoneInjectHiddenDoesNotCallHybrid(t *testing.T) {
	vis, err := session.OpenVisibility(filepath.Join(t.TempDir(), "v.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := vis.Set("admin-shell", true, "Admin", "bash"); err != nil {
		t.Fatal(err)
	}
	rt := &agentRuntime{vis: vis}
	env, err := grok.NewEnvelope(grok.TypeInject, "phone", "admin-shell", "cmd-1", grok.InjectPayload{
		Mode:   "text",
		Text:   "echo hidden",
		Submit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.handle(context.Background(), "mb", env); err != nil {
		t.Fatal(err)
	}
}

func TestPhoneControlResultHiddenDoesNotCapture(t *testing.T) {
	vis, err := session.OpenVisibility(filepath.Join(t.TempDir(), "v.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := vis.Set("admin-shell", true, "Admin", "bash"); err != nil {
		t.Fatal(err)
	}
	rt := &agentRuntime{vis: vis}
	env, err := grok.NewEnvelope(grok.TypeControl, "phone", "admin-shell", "cmd-2", grok.ControlPayload{
		Action:    "result",
		SessionID: "admin-shell",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.handle(context.Background(), "mb", env); err != nil {
		t.Fatal(err)
	}
}

func TestVisibleInjectStillReachesHybridNilPanicGuard(t *testing.T) {
	// A visible session must not be treated as hidden. We stop before Inject
	// by using an empty text payload, which still enters the inject path.
	vis := session.EmptyVisibility()
	rt := &agentRuntime{vis: vis, hybrid: nil}
	env, err := grok.NewEnvelope(grok.TypeInject, "phone", "proj-app", "cmd-3", grok.InjectPayload{
		Mode: "text",
		Text: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() != nil {
			t.Fatal("visible session was refused by panicking on a nil hybrid before the hide check")
		}
	}()
	// Empty text still calls hybrid.Inject. nil hybrid panics, which would
	// mean we got past the hide check — that is what we want for a visible id.
	// Catch it so the test process stays up, and treat the panic as success
	// only when the id is not hidden.
	func() {
		defer func() { _ = recover() }()
		_ = rt.handle(context.Background(), "mb", env)
		t.Fatal("expected nil hybrid to panic after the hide check allowed the session")
	}()
}
