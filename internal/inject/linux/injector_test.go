//go:build linux

package linux

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTypeWindowArgs_MatchesHandCommand(t *testing.T) {
	args := typeWindowArgs("48234499", "echo gbr-ok")
	if len(args) < 4 {
		t.Fatalf("argv too short: %v", args)
	}
	if args[0] != "type" {
		t.Fatalf("want type, got %q in %v", args[0], args)
	}
	if args[1] != "--window" {
		t.Fatalf("want --window, got %q in %v", args[1], args)
	}
	if args[2] != "48234499" {
		t.Fatalf("want window id, got %q in %v", args[2], args)
	}
	if args[3] != "echo gbr-ok" {
		t.Fatalf("want text, got %q in %v", args[3], args)
	}
	for _, a := range args {
		if a == "--" {
			t.Fatalf("bare -- must not appear (older xdotool types it): %v", args)
		}
	}
}

func TestInject_ExecsTypeWindowHelper(t *testing.T) {
	inj := New()
	inj.FocusPause = 0
	var calls [][]string
	inj.Exec = func(_ context.Context, args ...string) (string, error) {
		cp := append([]string{}, args...)
		calls = append(calls, cp)
		return "", nil
	}
	if err := inj.Bind("grok-build-abc", Window{ID: "48234499", HWND: 48234499, Title: "Grok Build"}); err != nil {
		t.Fatal(err)
	}
	if err := inj.Inject(context.Background(), "grok-build-abc", "echo gbr-ok", true); err != nil {
		t.Fatalf("inject: %v", err)
	}

	var typeCall []string
	for _, c := range calls {
		if len(c) > 0 && c[0] == "type" {
			typeCall = c
			break
		}
	}
	if typeCall == nil {
		t.Fatalf("type helper was not spawned; calls=%v", calls)
	}
	joined := strings.Join(typeCall, " ")
	if !strings.Contains(joined, "type") || !strings.Contains(joined, "--window") || !strings.Contains(joined, "48234499") || !strings.Contains(joined, "echo gbr-ok") {
		t.Fatalf("type argv missing required pieces: %v", typeCall)
	}
}

func TestInject_TypeHelperErrorIsFailure(t *testing.T) {
	inj := New()
	inj.FocusPause = 0
	inj.Exec = func(_ context.Context, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "type" {
			return "", errors.New("helper missing")
		}
		return "", nil
	}
	_ = inj.Bind("s1", Window{ID: "1", HWND: 1})
	err := inj.Inject(context.Background(), "s1", "hello", false)
	if err == nil {
		t.Fatal("type helper failure must not look like success")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Fatalf("want type in error, got %v", err)
	}
}

func TestInject_HWNDOnlyStillTypes(t *testing.T) {
	inj := New()
	inj.FocusPause = 0
	var calls [][]string
	inj.Exec = func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, append([]string{}, args...))
		return "", nil
	}
	// Phone bind often has HWND and no ID string.
	if err := inj.Bind("s1", Window{HWND: 99, Title: "Grok 4.6 - grok"}); err != nil {
		t.Fatal(err)
	}
	if err := inj.Inject(context.Background(), "s1", "hi", false); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range calls {
		if len(c) >= 4 && c[0] == "type" && c[1] == "--window" && c[2] == "99" && c[3] == "hi" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected type --window 99 hi; calls=%v", calls)
	}
}

func TestIsTerminalLike_GrokChrome(t *testing.T) {
	w := WinInfo{Name: "Grok Build", Class: "Google-chrome"}
	if !isTerminalLike(w, defaultTerminalClasses()) {
		t.Fatal("Chrome Grok Build window must be injectable")
	}
	if !looksLikeGrok(w.Name, w.Class) {
		t.Fatal("looksLikeGrok")
	}
	plain := WinInfo{Name: "Inbox - Gmail", Class: "Google-chrome"}
	if isTerminalLike(plain, defaultTerminalClasses()) {
		t.Fatal("plain Chrome tab must not count as a terminal")
	}
}
