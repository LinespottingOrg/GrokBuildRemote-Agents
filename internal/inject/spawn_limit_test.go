package inject

import (
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestSpawnGuard_ThreeThenBlock(t *testing.T) {
	g := newSpawnGuard()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }

	for i := 1; i <= MaxAutoOpen; i++ {
		if err := g.allow(); err != nil {
			t.Fatalf("attempt %d should be allowed: %v", i, err)
		}
		g.record(1000 + i)
	}
	if err := g.allow(); !errors.Is(err, ErrSpawnLimit) {
		t.Fatalf("4th spawn must be ErrSpawnLimit, got %v", err)
	}
	attempts, live := g.used()
	if attempts != 3 || live != 3 {
		t.Fatalf("used attempts=%d live=%d want 3/3", attempts, live)
	}

	now = now.Add(AutoOpenWindow + time.Second)
	if err := g.allow(); err != nil {
		t.Fatalf("after window, spawn should be allowed again: %v", err)
	}
}

func TestSpawnGuard_EmptySessionAttachesWithoutCounting(t *testing.T) {
	ui := &grokWinUI{}
	h := NewHybrid(ui, NewManager(nil))
	res, err := h.OpenOrAttach(OpenRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Attached || res.Opened {
		t.Fatalf("empty open must attach existing grok, got opened=%v attached=%v note=%q", res.Opened, res.Attached, res.Note)
	}
	if res.HWND != 42 || res.PID != 99 {
		t.Fatalf("bound hwnd/pid = %d/%d", res.HWND, res.PID)
	}
	if n, live := h.spawns.used(); n != 0 || live != 0 {
		t.Fatalf("attach must not consume spawn budget, attempts=%d live=%d", n, live)
	}
	if ui.binds != 1 {
		t.Fatalf("expected 1 bind, got %d", ui.binds)
	}
}

func TestHybrid_SpawnCapAttachesInsteadOfFourthWindow(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("CREATE_NEW_CONSOLE spawn cap is Windows-only")
	}
	ui := &grokWinUI{}
	h := NewHybrid(ui, NewManager(nil))
	h.spawns.record(11)
	h.spawns.record(12)
	h.spawns.record(13)

	res, err := h.OpenOrAttach(OpenRequest{SessionID: "gbr-open-new1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Attached {
		t.Fatalf("over cap must attach, not spawn: %+v", res)
	}
	if res.Note == "" {
		t.Fatal("expected cap note")
	}
}

func TestHybrid_SpawnCapErrorsWhenNothingToAttach(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("CREATE_NEW_CONSOLE spawn cap is Windows-only")
	}
	ui := &stubUI{} // Discover returns a generic Terminal, not grok
	h := NewHybrid(ui, NewManager(nil))
	h.spawns.record(11)
	h.spawns.record(12)
	h.spawns.record(13)

	_, err := h.OpenOrAttach(OpenRequest{SessionID: "gbr-open-new2"})
	if !errors.Is(err, ErrSpawnLimit) {
		t.Fatalf("want ErrSpawnLimit, got %v", err)
	}
}

type grokWinUI struct {
	binds int
}

func (g *grokWinUI) Discover() ([]TerminalWindow, error) {
	return []TerminalWindow{{
		HWND:    42,
		PID:     99,
		Title:   "Grok Build",
		ExeName: "grok.exe",
		Kind:    KindGrokBuild,
	}}, nil
}
func (g *grokWinUI) Inject(string, InjectRequest) error { return nil }
func (g *grokWinUI) Capture(string) (CaptureResult, error) {
	return CaptureResult{Method: "ui"}, nil
}
func (g *grokWinUI) Bind(string, TerminalWindow) error { g.binds++; return nil }
func (g *grokWinUI) Unbind(string)                     {}
func (g *grokWinUI) Close() error                      { return nil }
