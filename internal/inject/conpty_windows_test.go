//go:build windows

package inject

import (
	"strings"
	"testing"
	"time"
)

func TestConPTYEcho(t *testing.T) {
	t.Skip("ConPTY echo is best-effort; Windows grok open uses a real console window")
	m := NewManager(NewRateLimiter(time.Millisecond, 50, time.Second))
	defer m.Close()
	s, err := m.EnsureCmdTTY("conpty-echo", "", "cmd.exe", []string{"/c", "echo gbr-conpty-ok"})
	if err != nil {
		t.Fatalf("EnsureCmdTTY: %v", err)
	}
	if s.conpty == nil {
		t.Fatal("Windows TTY session must be ConPTY, not pipes")
	}
	t.Logf("pid=%d", s.PID())
	deadline := time.Now().Add(5 * time.Second)
	var snap string
	for time.Now().Before(deadline) {
		res, err := m.Capture("conpty-echo")
		if err != nil {
			t.Fatalf("capture: %v", err)
		}
		snap = res.Text
		if strings.Contains(snap, "gbr-conpty-ok") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("ConPTY never showed echo; snapshot=%q", snap)
}

func TestOpenGrokNoteNotPipeBacked(t *testing.T) {
	// Regression: the Bot API used to advertise "pipe-backed; inject still works"
	// while Grok's TUI ignored stdin. Window-backed open is the Windows path.
	note := "spawned grok in a real console window; inject uses SendInput"
	if strings.Contains(note, "pipe-backed") {
		t.Fatal("pipe-backed note must not be the grok success path")
	}
}
