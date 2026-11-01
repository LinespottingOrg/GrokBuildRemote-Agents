package inject

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestClawHolderNames(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hermes", "hermes"},
		{"Hermes", "hermes"},
		{"openclaw", "openclaw"},
		{"OpenClaw", "openclaw"},
		{"open-claw", "openclaw"},
		{"open_claw", "openclaw"},
		{"nemoclaw", "nemoclaw"},
		{"NemoClaw", "nemoclaw"},
		{"nemo-claw", "nemoclaw"},
		{"grok-bot", ""},
		{"phone", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ClawHolderName(tc.in); got != tc.want {
			t.Fatalf("ClawHolderName(%q)=%q want %q", tc.in, got, tc.want)
		}
		if ClawHolder(tc.in) != (tc.want != "") {
			t.Fatalf("ClawHolder(%q) inconsistent with %q", tc.in, tc.want)
		}
	}
}

func quietShell(m *Manager) {
	switch runtime.GOOS {
	case "windows":
		m.Shell = "powershell.exe"
		m.Args = []string{"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", "exit 0"}
	default:
		m.Shell = "true"
	}
}

func TestOpenOrAttach_MissingGrokOpensShellLikeMac(t *testing.T) {
	origLook := lookGrok
	lookGrok = func() (string, error) {
		return "", errors.New("grok CLI not on PATH — install Grok Build, or open an existing session")
	}
	origWait := clawReadyWait
	clawReadyWait = 0
	t.Cleanup(func() {
		lookGrok = origLook
		clawReadyWait = origWait
	})
	t.Setenv("GBR_NO_AUTO_OPEN", "")
	t.Setenv("GBR_MAX_AUTO_OPEN", "")

	for _, holder := range []string{"OpenClaw", "hermes", "grok-bot"} {
		t.Run(holder, func(t *testing.T) {
			ui := &grokWinUI{}
			pty := NewManager(NewRateLimiter(time.Millisecond, 100, time.Second))
			quietShell(pty)
			t.Cleanup(func() { _ = pty.Close() })
			h := NewHybrid(ui, pty)

			res, err := h.OpenOrAttach(OpenRequest{SessionID: "claw-missing-" + holder, Holder: holder})
			if err != nil {
				t.Fatal(err)
			}
			if res.Method != "pty" || res.Command != "shell" || !res.Opened {
				t.Fatalf("method=%q command=%q opened=%v note=%q", res.Method, res.Command, res.Opened, res.Note)
			}
			if ui.binds != 0 {
				t.Fatalf("missing grok must not bind a UI window, binds=%d", ui.binds)
			}
			if !strings.Contains(res.Note, "grok CLI not on PATH") {
				t.Fatalf("note=%q", res.Note)
			}
		})
	}
}

func TestOpenOrAttach_ClawExplicitGrokMissing(t *testing.T) {
	origLook := lookGrok
	lookGrok = func() (string, error) { return "", errors.New("no grok") }
	origWait := clawReadyWait
	clawReadyWait = 0
	t.Cleanup(func() {
		lookGrok = origLook
		clawReadyWait = origWait
	})
	t.Setenv("GBR_NO_AUTO_OPEN", "")

	ui := &grokWinUI{}
	h := NewHybrid(ui, NewManager(nil))
	_, err := h.OpenOrAttach(OpenRequest{SessionID: "claw-explicit-grok", Holder: "hermes", Command: "grok"})
	if err == nil || !strings.Contains(err.Error(), "no grok") {
		t.Fatalf("explicit grok must fail closed, got %v", err)
	}
	if ui.binds != 0 {
		t.Fatalf("failed claw open must not bind a window, binds=%d", ui.binds)
	}
}

func TestOpenOrAttach_ExplicitGrokStaysOnWindowPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("phone/bot grok open stays on the visible console only on Windows")
	}
	origLook := lookGrok
	lookGrok = func() (string, error) { return "", errors.New("no grok") }
	t.Cleanup(func() { lookGrok = origLook })
	t.Setenv("GBR_NO_AUTO_OPEN", "")

	ui := &grokWinUI{}
	h := NewHybrid(ui, NewManager(nil))
	_, err := h.OpenOrAttach(OpenRequest{SessionID: "bot-open-1", Holder: "grok-bot", Command: "grok"})
	if err == nil || !strings.Contains(err.Error(), "no grok") {
		t.Fatalf("explicit grok on Windows must stay on the window path, got %v", err)
	}
	if ui.binds != 0 {
		t.Fatalf("binds=%d", ui.binds)
	}
}
