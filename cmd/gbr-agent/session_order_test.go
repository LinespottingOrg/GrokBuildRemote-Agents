package main

import (
	"reflect"
	"testing"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
)

func TestPrioritizeSessionIDs_GrokFirstThenCap(t *testing.T) {
	in := []string{
		"conhost-aaa",
		"c-program",
		"usb-removal-causes",
		"gbr-agent",
		"c",
		"conhost-bbb",
		"grok-build-40a22",
		"conhost-ccc",
	}
	got := prioritizeSessionIDs(in, 6)
	if len(got) != 6 {
		t.Fatalf("len=%d want 6: %v", len(got), got)
	}
	if got[0] != "grok-build-40a22" {
		t.Fatalf("first=%q want grok-build-40a22 (Grok must not be dropped by the cap)", got[0])
	}
	if got[1] != "gbr-agent" {
		t.Fatalf("second=%q want gbr-agent", got[1])
	}
}

func TestPrioritizeSessionIDs_RaiseCapKeepsOthers(t *testing.T) {
	in := []string{"a", "b", "c", "d", "e", "f", "g", "grok-build-1"}
	got := prioritizeSessionIDs(in, 32)
	if len(got) != 8 {
		t.Fatalf("len=%d want 8", len(got))
	}
	if got[0] != "grok-build-1" {
		t.Fatalf("first=%q", got[0])
	}
}

func TestPrioritizeSessionIDs_DedupeAndEmpty(t *testing.T) {
	got := prioritizeSessionIDs([]string{"", "admin", "admin", "grok-cli"}, 0)
	want := []string{"grok-cli", "admin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWindowsToCandidates_UniqueGrokID(t *testing.T) {
	wins := []inject.TerminalWindow{
		{HWND: 0x111, PID: 1, Title: "", Kind: inject.KindConhost},
		{HWND: 0x40a22, PID: 20924, Title: "Grok 4.6 - grok", Kind: "grok-build"},
	}
	sortWindowsGrokFirst(wins)
	out, grokN, otherN := windowsToCandidates(wins)
	if grokN != 1 || otherN != 1 {
		t.Fatalf("counts grok=%d other=%d", grokN, otherN)
	}
	if out[0].PreferID != "grok-build-40a22" {
		t.Fatalf("prefer=%q title=%q", out[0].PreferID, out[0].Title)
	}
	if out[0].CWD == out[1].CWD {
		t.Fatal("cwd collapsed")
	}
}

func TestFeedbackMaxSessions_Default(t *testing.T) {
	t.Setenv("GBR_FEEDBACK_MAX_SESSIONS", "")
	if n := feedbackMaxSessions(); n != DefaultFeedbackMaxSessions {
		t.Fatalf("got %d", n)
	}
	t.Setenv("GBR_FEEDBACK_MAX_SESSIONS", "16")
	if n := feedbackMaxSessions(); n != 16 {
		t.Fatalf("got %d", n)
	}
	t.Setenv("GBR_FEEDBACK_MAX_SESSIONS", "999")
	if n := feedbackMaxSessions(); n != 128 {
		t.Fatalf("got %d want clamp 128", n)
	}
}
