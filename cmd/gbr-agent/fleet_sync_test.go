package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/core"
)

func TestFleetSyncBodyIncludesClassHostnameImpl(t *testing.T) {
	d := core.FleetDevice{
		ID: "studio-linux", Name: "studio", MailboxID: "gbr-abc", MailboxKey: "k",
		OS: "linux", Class: "linux", Hostname: "studio.local", Impl: "gbr",
	}
	body := fleetSyncBody(d)
	for _, k := range []string{"id", "name", "mailbox_id", "mailbox_key", "os", "class", "hostname", "impl"} {
		if _, ok := body[k]; !ok {
			t.Fatalf("POST /devices body missing %s: %+v", k, body)
		}
	}
	if body["class"] != "linux" || body["hostname"] != "studio.local" || body["impl"] != "gbr" {
		t.Fatalf("class/hostname/impl: %+v", body)
	}
	if body["id"] != "studio-linux" || body["os"] != "linux" {
		t.Fatalf("id/os: %+v", body)
	}
}

func TestFleetSyncBodyDefaultImpl(t *testing.T) {
	body := fleetSyncBody(core.FleetDevice{ID: "mac", Class: "mac_mini"})
	if body["impl"] != "gbr" {
		t.Fatalf("empty impl should default to gbr, got %+v", body)
	}
	if _, ok := body["class"]; !ok || body["class"] != "mac_mini" {
		t.Fatalf("class must be present: %+v", body)
	}
	if _, ok := body["hostname"]; !ok {
		t.Fatalf("hostname key must be present even when empty: %+v", body)
	}
}

func TestPrintStatusFleetPrintsRemoteClass(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	f, err := core.LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Upsert(core.FleetDevice{
		ID: "studio-linux", Name: "studio", MailboxID: "gbr-abc", MailboxKey: "k",
		OS: "linux", Class: "linux", Hostname: "studio.local", Impl: "gbr",
	}); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	printStatusFleet("gbr-hub", true)
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	_ = r.Close()

	found := false
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "studio-linux") && strings.Contains(line, "class=linux") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("remote class missing:\n%s", out)
	}
}

func TestFleetSyncBodyJSONRoundTrip(t *testing.T) {
	body := fleetSyncBody(core.FleetDevice{
		ID: "mac", Name: "mac", MailboxID: "gbr-x", MailboxKey: "k",
		OS: "darwin", Class: "mac_mini", Hostname: "mac.local",
	})
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"class":"mac_mini"`, `"hostname":"mac.local"`, `"impl":"gbr"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("json missing %s: %s", want, s)
		}
	}
}
