package session

import (
	"testing"
)

func TestParseGrokCommand(t *testing.T) {
	cases := []struct {
		cmd    string
		is     bool
		resume string
	}{
		{"grok --resume 40a22abc-1111-2222-3333-444444444444", true, "40a22abc-1111-2222-3333-444444444444"},
		{"/usr/local/bin/grok --resume=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"grok", true, ""},
		{"gbr-agent run", false, ""},
		{"grep grok --resume foo", false, ""},
		{"bash", false, ""},
		{"ProjectA - grok — agent · clang", false, ""},
	}
	for _, c := range cases {
		is, r := ParseGrokCommand(c.cmd)
		if is != c.is || r != c.resume {
			t.Errorf("%q → (%v,%q) want (%v,%q)", c.cmd, is, r, c.is, c.resume)
		}
	}
}

func TestLooksLikeGrokWindow(t *testing.T) {
	if !LooksLikeGrokWindow("ProjectA - grok — agent · clang", "Terminal", "unknown") {
		t.Fatal("macOS 26.5 title must classify as grok")
	}
	if LooksLikeGrokWindow("bash", "Terminal", "unknown") {
		t.Fatal("plain terminal is not grok")
	}
}

func TestSuggestGrokSessionID(t *testing.T) {
	id := SuggestGrokSessionID("40a22abc-1111-2222-3333-444444444444", 0x40a22, 0)
	if id != "grok-40a22abc" {
		t.Fatalf("got %q", id)
	}
	if !ValidSessionID(id) {
		t.Fatal("must be a valid session id")
	}
	id2 := SuggestGrokSessionID("", 0x40a22, 0)
	if id2 != "grok-build-40a22" {
		t.Fatalf("hwnd fallback %q", id2)
	}
}

func TestParsePSTable(t *testing.T) {
	raw := `
  12   1 /sbin/launchd
  99  12 /bin/bash
 20924  99 grok --resume 40a22abc-1111-2222-3333-444444444444
  50  12 grep grok
`
	got := ParsePSTable(raw)
	if len(got) != 1 || got[0].PID != 20924 || got[0].ResumeID == "" {
		t.Fatalf("%+v", got)
	}
}

func TestMatchGrokProcessAncestor(t *testing.T) {
	procs := []GrokProc{{PID: 20924, PPID: 99, ResumeID: "abc", Cmd: "grok --resume abc"}}
	if _, ok := MatchGrokProcess(99, procs); !ok {
		t.Fatal("terminal pid 99 should match child grok")
	}
	if _, ok := MatchGrokProcess(20924, procs); !ok {
		t.Fatal("direct pid")
	}
	if _, ok := MatchGrokProcess(1, procs); ok {
		t.Fatal("unrelated pid")
	}
}
