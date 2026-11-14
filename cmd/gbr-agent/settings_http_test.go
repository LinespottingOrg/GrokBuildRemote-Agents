package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session"
)

func TestSettingsRosterHideAndShow(t *testing.T) {
	vis, err := session.OpenVisibility(filepath.Join(t.TempDir(), "roster-visibility.json"))
	if err != nil {
		t.Fatal(err)
	}
	reg := session.NewRegistry()
	reg.Upsert(&session.Session{ID: "admin-shell", Title: "Admin", Shell: "bash", CWD: "/tmp/admin"})
	reg.Upsert(&session.Session{ID: "proj-app", Title: "Proj", Shell: "zsh", CWD: "/tmp/proj"})
	s := &botServer{rt: &agentRuntime{
		vis:     vis,
		scanner: &session.Scanner{Registry: reg},
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1/settings/roster", strings.NewReader(`{"session_id":"admin-shell","hidden":true}`))
	rec := httptest.NewRecorder()
	s.handleRosterSettings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("hide status %d body %s", rec.Code, rec.Body.String())
	}
	if !vis.Hidden("admin-shell") {
		t.Fatal("not hidden")
	}

	page := httptest.NewRecorder()
	s.writeSettingsPage(page)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Hidden terminals") {
		t.Fatalf("page %d", page.Code)
	}

	got := httptest.NewRecorder()
	s.writeRosterSettings(got)
	body := got.Body.String()
	if !strings.Contains(body, `"session_id":"admin-shell"`) || !strings.Contains(body, `"hidden":true`) {
		t.Fatalf("roster %s", body)
	}
	if !strings.Contains(body, `"session_id":"proj-app"`) || !strings.Contains(body, `"hidden":false`) {
		t.Fatalf("visible row missing %s", body)
	}

	phone := s.rt.phoneSessionPayloads()
	if len(phone) != 1 {
		t.Fatalf("phone=%v", phone)
	}

	show := httptest.NewRequest(http.MethodPost, "/v1/settings/roster", strings.NewReader(`{"session_id":"admin-shell","hidden":false}`))
	rec2 := httptest.NewRecorder()
	s.handleRosterSettings(rec2, show)
	if rec2.Code != 200 || vis.Hidden("admin-shell") {
		t.Fatalf("unhide %d hidden=%v", rec2.Code, vis.Hidden("admin-shell"))
	}
}

func TestSettingsAuthorizeWithoutKey(t *testing.T) {
	t.Setenv("GBR_BOT_REQUIRE_KEY", "1")
	s := &botServer{key: "secret"}
	r := httptest.NewRequest(http.MethodGet, "/settings", nil)
	if !s.authorize(r) {
		t.Fatal("loopback settings page must open without a key")
	}
	r2 := httptest.NewRequest(http.MethodPost, "/v1/settings/roster", nil)
	if !s.authorize(r2) {
		t.Fatal("loopback roster save must open without a key")
	}
}
