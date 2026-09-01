package inject

import (
	"errors"
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

type failUI struct{ stubUI }

func (f *failUI) Discover() ([]TerminalWindow, error) {
	return []TerminalWindow{{
		Title: "++ Felanmälan.org",
		Kind:  KindGrokBuild,
		HWND:  1,
	}}, nil
}

func (f *failUI) Inject(string, InjectRequest) error {
	return fmtNotFound("gbr-open-missing")
}

func fmtNotFound(id string) error {
	return errors.Join(ErrNotFound, errors.New(`session "`+id+`" (bind a window or use managed PTY)`))
}

func TestHybrid_WindowNotFoundIsError(t *testing.T) {
	pty := NewManager(NewRateLimiter(time.Millisecond, 50, time.Second))
	defer pty.Close()
	h := NewHybrid(&failUI{}, pty)
	err := h.Inject("gbr-open-missing", InjectRequest{Text: "GBR_FEEDBACK_OK", Submit: true})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if got := pty.List(); len(got) != 0 {
		t.Fatalf("window-not-found must not spawn a dummy PTY shell: %v", got)
	}
}

func TestHybrid_BindWindowBeatsPTY(t *testing.T) {
	pty := NewManager(NewRateLimiter(time.Millisecond, 50, time.Second))
	defer pty.Close()
	if _, err := pty.Ensure("grok-build-abc", ""); err != nil {
		t.Fatal(err)
	}
	ui := &stubUI{}
	h := NewHybrid(ui, pty)
	if err := h.Bind("grok-build-abc", TerminalWindow{HWND: 48234499, Title: "Grok Build"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Inject("grok-build-abc", InjectRequest{Text: "echo gbr-ok", Submit: true}); err != nil {
		t.Fatal(err)
	}
	if ui.n != 1 {
		t.Fatalf("bound X11/HWND session must UI-inject (type path), got %d UI calls", ui.n)
	}
}

func TestHybrid_WindowSessionUsesUI(t *testing.T) {
	pty := NewManager(NewRateLimiter(time.Millisecond, 50, time.Second))
	defer pty.Close()
	ui := &stubUI{}
	h := NewHybrid(ui, pty)
	h.rememberWindow("gbr-open-win", 1, 1)
	if err := h.Inject("gbr-open-win", InjectRequest{Text: "hi", Submit: true}); err != nil {
		t.Fatal(err)
	}
	if ui.n != 1 {
		t.Fatalf("window session must UI-inject, got %d calls", ui.n)
	}
	if got := pty.List(); len(got) != 0 {
		t.Fatalf("window session must not create a PTY: %v", got)
	}
}

func TestHybrid_ReplaySameCommandID(t *testing.T) {
	pty := NewManager(NewRateLimiter(time.Millisecond, 50, time.Second))
	defer pty.Close()
	ui := &stubUI{}
	h := NewHybrid(ui, pty)
	req := InjectRequest{Text: "do the thing", Submit: true, CommandID: "cmd-loop"}
	if err := h.Inject("gbr-open-a", req); err != nil {
		t.Fatal(err)
	}
	if err := h.Inject("gbr-open-a", req); !errors.Is(err, ErrInjectReplay) {
		t.Fatalf("same command_id must die, not re-type: %v", err)
	}
	if ui.n != 1 {
		t.Fatalf("replay must not SendInput again, got %d UI injects", ui.n)
	}
}

type splashUI struct{ stubUI }

func (s *splashUI) Capture(string) (CaptureResult, error) {
	return CaptureResult{
		Text:   "Grok Build 1.0.5\nGrok 4.6 is here\nNew worktree    Resume    Changelog\n>",
		Method: "ui",
	}, nil
}

func TestHybrid_SplashRefusesInject(t *testing.T) {
	h := NewHybrid(&splashUI{}, NewManager(nil))
	err := h.Inject("gbr-open-splash", InjectRequest{Text: "fix tests", Submit: true, CommandID: "c-splash"})
	if !errors.Is(err, ErrSplash) {
		t.Fatalf("want ErrSplash, got %v", err)
	}
	if h.attempts.Seen("c-splash") {
		t.Fatal("splash must not consume command_id — caller may retry after ready")
	}
}

func TestHybrid_HaltRefusesInject(t *testing.T) {
	t.Setenv("GBR_INJECT_HALT", "1")
	h := NewHybrid(&stubUI{}, NewManager(nil))
	if err := h.Inject("gbr-open-h", InjectRequest{Text: "hi", Submit: true, CommandID: "c-h"}); !errors.Is(err, ErrInjectHalted) {
		t.Fatalf("halt must refuse, got %v", err)
	}
}

func TestPickInjectTargetSkipsProtected(t *testing.T) {
	wins := []TerminalWindow{
		{Title: "++ Felanmälan.org", Kind: KindGrokBuild, HWND: 1},
		{Title: "++ QA PC ANdroid", Kind: KindGrokBuild, HWND: 3},
		{Title: "gbr-open-abc", Kind: KindGrokBuild, HWND: 2},
	}
	got := pickInjectTarget(wins, "gbr-open-abc")
	if got.HWND != 2 {
		t.Fatalf("got hwnd=%v title=%q", got.HWND, got.Title)
	}
	onlyProtected := pickInjectTarget(wins[:2], "gbr-open-abc")
	if onlyProtected.HWND != 0 {
		t.Fatalf("protected-only set must not be a success path: %+v", onlyProtected)
	}
}
