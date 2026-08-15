package main

import (
	"reflect"
	"testing"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
)

func TestPrioritizeSessionIDs_GrokFirstNoCap(t *testing.T) {
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
	got := prioritizeSessionIDs(in)
	if len(got) != 8 {
		t.Fatalf("len=%d want all 8 (no cap): %v", len(got), got)
	}
	if got[0] != "grok-build-40a22" {
		t.Fatalf("first=%q want grok-build-40a22", got[0])
	}
	if got[1] != "gbr-agent" {
		t.Fatalf("second=%q want gbr-agent", got[1])
	}
}

func TestPrioritizeSessionIDs_DedupeAndEmpty(t *testing.T) {
	got := prioritizeSessionIDs([]string{"", "admin", "admin", "grok-cli"})
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
