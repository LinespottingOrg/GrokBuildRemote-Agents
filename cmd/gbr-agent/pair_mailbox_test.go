package main

import (
	"strings"
	"testing"
)

func TestFormatPairAsMailboxReportOmitsSecrets(t *testing.T) {
	out := formatPairAsMailboxReport(pairAsMailboxReport{
		Name: "mac", OS: "darwin", Reused: true, KeyLen: 64,
		Relay: "https://gbr-relay.ekobrott.workers.dev", RelayOK: "ok", OfferFiles: 2,
	})
	if !strings.Contains(out, "paired as mailbox") {
		t.Fatalf("missing headline: %s", out)
	}
	if !strings.Contains(out, "fleet add -name mac -os darwin") {
		t.Fatalf("missing hub one-liner: %s", out)
	}
	if strings.Contains(out, "gbr://") || strings.Contains(out, "code (generated") || strings.Contains(out, "code=") {
		t.Fatalf("must not print pair codes: %s", out)
	}
	if strings.Contains(out, "mailbox=gbr-") || strings.Contains(out, "mailbox_id") {
		t.Fatalf("must not print mailbox id: %s", out)
	}
}

func TestCoreNormalizeFleetName(t *testing.T) {
	if got := coreNormalizeFleetName(" Mac Mini "); got != "mac-mini" {
		t.Fatalf("got %q", got)
	}
}
