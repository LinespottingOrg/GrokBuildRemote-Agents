package inject

import (
	"strings"
	"testing"
	"time"
)

// stubUI records Inject calls and never fails, so a wrong Hybrid path
// would "succeed" without writing the PTY.
type stubUI struct{ n int }

func (s *stubUI) Discover() ([]TerminalWindow, error) {
	return []TerminalWindow{{Title: "Terminal", Kind: KindUnknown}}, nil
}
func (s *stubUI) Inject(string, InjectRequest) error { s.n++; return nil }
func (s *stubUI) Capture(string) (CaptureResult, error) {
	return CaptureResult{Method: "ui"}, nil
}
func (s *stubUI) Bind(string, TerminalWindow) error { return nil }
func (s *stubUI) Unbind(string)                     {}
func (s *stubUI) Close() error                      { return nil }

func TestHybrid_ManagedSessionSkipsUI(t *testing.T) {
	pty := NewManager(NewRateLimiter(time.Millisecond, 50, time.Second))
	defer pty.Close()
	if _, err := pty.Ensure("gbr-open-test", ""); err != nil {
		t.Fatal(err)
	}
	ui := &stubUI{}
	h := NewHybrid(ui, pty)
	if err := h.Inject("gbr-open-test", InjectRequest{Text: "echo gbr-e2e-ok", Submit: true}); err != nil {
		t.Fatal(err)
	}
	if ui.n != 0 {
		t.Fatalf("UI inject called %d times; managed session must stay on PTY", ui.n)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cap, err := h.Capture("gbr-open-test")
		if err == nil && strings.Contains(cap.Text, "gbr-e2e-ok") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	cap, _ := h.Capture("gbr-open-test")
	t.Fatalf("PTY never showed echo; snapshot=%q", cap.Text)
}
