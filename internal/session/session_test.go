package session

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRegistryUpsertGetList(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	s := &Session{
		ID:       "global-edition",
		CWD:      `C:\work\gbr`,
		Shell:    "pwsh",
		PID:      42,
		HWND:     0x100,
		Title:    "Windows Terminal",
		LastSeen: time.Now().UTC(),
		OS:       "windows",
	}
	stored, isNew := r.Upsert(s)
	if !isNew || stored == nil {
		t.Fatalf("isNew=%v stored=%v", isNew, stored)
	}
	if r.Len() != 1 {
		t.Fatalf("len=%d", r.Len())
	}
	got, ok := r.Get("global-edition")
	if !ok || got.PID != 42 {
		t.Fatalf("get=%v ok=%v", got, ok)
	}
	// update
	s2 := got.Clone()
	s2.PID = 99
	_, isNew2 := r.Upsert(s2)
	if isNew2 {
		t.Fatal("expected update not new")
	}
	got2, _ := r.Get("global-edition")
	if got2.PID != 99 {
		t.Fatalf("pid=%d", got2.PID)
	}
	list := r.List()
	if len(list) != 1 {
		t.Fatalf("list=%d", len(list))
	}
}

func TestRegistryRemoveStale(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	old := &Session{ID: "old-session", CWD: "/tmp/a", LastSeen: time.Now().UTC().Add(-time.Hour)}
	fresh := &Session{ID: "new-session", CWD: "/tmp/b", LastSeen: time.Now().UTC()}
	r.Upsert(old)
	r.Upsert(fresh)
	removed := r.RemoveStale(10 * time.Minute)
	if len(removed) != 1 || removed[0].ID != "old-session" {
		t.Fatalf("removed=%v", removed)
	}
	if r.Len() != 1 {
		t.Fatalf("len=%d", r.Len())
	}
}

func TestToRegisterPayload(t *testing.T) {
	t.Parallel()
	s := &Session{
		ID:        "global-edition",
		CWD:       `C:/Users/User/.aiprojects/gbr`,
		Shell:     "pwsh",
		OS:        "windows",
		Title:     "Windows Terminal",
		GitRemote: "LinespottingOrg/GrokBuildRemote-Agents",
	}
	msg := s.ToRegister("11111111-1111-1111-1111-111111111111")
	if msg.Proto != ProtoVersion || msg.Type != RegisterType {
		t.Fatalf("envelope meta: %+v", msg)
	}
	if msg.SessionID != "global-edition" {
		t.Fatalf("session_id=%s", msg.SessionID)
	}
	if msg.Payload.Shell != "pwsh" || msg.Payload.GitRemote == "" {
		t.Fatalf("payload=%+v", msg.Payload)
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["type"] != "register" || m["proto"] != "gbr/1" {
		t.Fatalf("json=%s", raw)
	}
	payload, ok := m["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload type %T", m["payload"])
	}
	if payload["cwd"] == nil || payload["os"] != "windows" {
		t.Fatalf("payload=%v", payload)
	}
}

func TestBuildSession(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := Candidate{CWD: dir, Shell: "pwsh", PID: 7, Title: "t"}
	s, err := BuildSession(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidSessionID(s.ID) {
		t.Fatalf("id=%s", s.ID)
	}
	if s.PID != 7 || s.Shell != "pwsh" {
		t.Fatalf("%+v", s)
	}
	if s.Source != SourceFallback {
		t.Fatalf("source=%s", s.Source)
	}
}
