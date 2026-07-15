//go:build windows

package windows

import (
	"errors"
	"testing"
	"time"
)

func TestInjector_RefuseEmptySession(t *testing.T) {
	inj := New()
	defer inj.Close()

	if err := inj.Inject("", Request{Text: "hi"}); !errors.Is(err, ErrEmptySession) {
		t.Fatalf("want ErrEmptySession, got %v", err)
	}
	if err := inj.Bind("", Window{HWND: 1}); !errors.Is(err, ErrEmptySession) {
		t.Fatalf("bind: want ErrEmptySession, got %v", err)
	}
	if _, err := inj.Capture(""); !errors.Is(err, ErrEmptySession) {
		t.Fatalf("capture: want ErrEmptySession, got %v", err)
	}
}

func TestInjector_RefuseEmptyText(t *testing.T) {
	inj := New()
	defer inj.Close()
	// Bind a fake hwnd so we get past resolve only if text validated first.
	if err := inj.Inject("s1", Request{}); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("want ErrEmptyText, got %v", err)
	}
}

func TestInjector_RateLimit(t *testing.T) {
	inj := NewWithLimits(50*time.Millisecond, 100, time.Second)
	defer inj.Close()
	// Force rate limit without needing a real window: bind then first inject will
	// fail on focus, but rate limiter runs before resolve... actually order is
	// validate → rate limit → resolve. First call consumes a token even if later
	// steps fail? Looking at Inject: rate limit is before resolve. Good.
	// Use a bound fake hwnd — focus will fail, but rate limit still records.
	_ = inj.Bind("s1", Window{HWND: 1, PID: 1})
	_ = inj.Inject("s1", Request{Text: "a"}) // may fail focus/send
	if err := inj.Inject("s1", Request{Text: "b"}); !errors.Is(err, ErrRateLimited) {
		// If first inject failed before rate limit... it shouldn't.
		// Rate limit is checked before resolve; first call always Allow's.
		t.Fatalf("want ErrRateLimited on second inject, got %v", err)
	}
}

func TestEffectiveText(t *testing.T) {
	if got := effectiveText(Request{Text: "x", Submit: true}); got != "x\n" {
		t.Fatalf("got %q", got)
	}
	if got := effectiveText(Request{Submit: true}); got != "\n" {
		t.Fatalf("got %q", got)
	}
}

func TestDiscover_Smoke(t *testing.T) {
	// Best-effort: must not panic; may return empty on headless CI.
	wins, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	t.Logf("discovered %d terminal-like window(s)", len(wins))
	for i, w := range wins {
		if i >= 5 {
			break
		}
		t.Logf("  hwnd=%v pid=%d kind=%s title=%q exe=%s class=%s",
			w.HWND, w.PID, w.Kind, w.Title, w.ExeName, w.ClassName)
	}
}
