package doctor

import (
	"strings"
	"testing"
)

func TestNetworkDocMentionsNoInbound(t *testing.T) {
	if !strings.Contains(NetworkDoc, "NEVER open ports") {
		t.Fatal("NetworkDoc must state no phone↔PC ports")
	}
	if !strings.Contains(NetworkDoc, "443") {
		t.Fatal("NetworkDoc must mention 443")
	}
	if !strings.Contains(NetworkDoc, "gbr-relay") {
		t.Fatal("NetworkDoc must name default relay host")
	}
}

func TestRunNetworkLive(t *testing.T) {
	// Live check against production relay — skip if offline CI wants speed:
	// always run; if blocked, surface FAIL for operators.
	results := RunNetwork("")
	if len(results) < 4 {
		t.Fatalf("expected several checks, got %d", len(results))
	}
	var hasModel, hasPorts bool
	for _, r := range results {
		if r.Name == "model" {
			hasModel = true
			if !r.OK {
				t.Error("model check should always pass")
			}
		}
		if r.Name == "ports" {
			hasPorts = true
			if !strings.Contains(r.Detail, "inbound: none") {
				t.Errorf("ports detail: %s", r.Detail)
			}
		}
	}
	if !hasModel || !hasPorts {
		t.Fatal("missing model/ports checks")
	}
	out := FormatNetwork(results)
	if !strings.Contains(out, "netcheck") {
		t.Fatal(out)
	}
	t.Log("\n" + out)
}
