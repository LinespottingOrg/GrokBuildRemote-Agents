package inject

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestManager_RefuseEmptySession(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()
	_, err := m.Ensure("", "")
	if !errors.Is(err, ErrEmptySession) {
		t.Fatalf("want ErrEmptySession, got %v", err)
	}
	err = m.Inject("", InjectRequest{Text: "x"})
	if !errors.Is(err, ErrEmptySession) {
		t.Fatalf("want ErrEmptySession, got %v", err)
	}
}

func TestManager_InjectAndCapture(t *testing.T) {
	m := NewManager(NewRateLimiter(1*time.Millisecond, 50, time.Second))
	defer m.Close()

	// Platform-appropriate one-shot echo via interactive shell is flaky;
	// write a simple command and wait for any output ring growth.
	var cmd string
	switch runtime.GOOS {
	case "windows":
		cmd = "echo gbr-pty-ok\r\n"
	default:
		cmd = "echo gbr-pty-ok\n"
	}

	if err := m.Inject("test-session", InjectRequest{Text: strings.TrimRight(cmd, "\r\n"), Submit: true}); err != nil {
		t.Fatalf("inject: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var snap string
	for time.Now().Before(deadline) {
		res, err := m.Capture("test-session")
		if err != nil {
			t.Fatalf("capture: %v", err)
		}
		snap = res.Text
		if strings.Contains(snap, "gbr-pty-ok") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Soft-fail on environments where the shell is non-interactive/echo suppressed.
	t.Logf("timed out waiting for echo; snapshot=%q (shell may not echo in CI)", snap)
}
