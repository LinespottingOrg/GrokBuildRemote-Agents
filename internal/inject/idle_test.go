package inject

import (
	"strings"
	"testing"
	"time"
)

func TestLooksLikePrompt(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"bash-3.2$ ", true},
		{"user@host:~/proj$ ", true},
		{"PS C:\\Users\\User> ", true},
		{"still compiling...\n", false},
		{"", false},
		{"error: boom\n", false},
		{"done.\nbash-5.2$ ", true},
		{"❯ ", true},
		{"What do you want to work on?\n", true},
	}
	for _, c := range cases {
		if got := LooksLikePrompt(c.in); got != c.want {
			t.Errorf("LooksLikePrompt(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestExcerptTailKeepsEnd(t *testing.T) {
	s := strings.Repeat("a", 100) + "TAIL"
	got := ExcerptTail(s, 8)
	if !strings.HasSuffix(got, "TAIL") {
		t.Fatalf("got %q", got)
	}
}

func TestWaitIdlePrompt(t *testing.T) {
	n := 0
	res := WaitIdle(func() (string, string, error) {
		n++
		if n < 3 {
			return "running...\n", "pty", nil
		}
		return "running...\nbash-3.2$ ", "pty", nil
	}, 2*time.Second, 400*time.Millisecond)
	if !res.Idle || res.Reason != IdleReasonPrompt {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(res.Excerpt, "bash-3.2$") {
		t.Fatalf("excerpt=%q", res.Excerpt)
	}
}

func TestWaitIdleQuiet(t *testing.T) {
	res := WaitIdle(func() (string, string, error) {
		return "compiled ok\nno prompt here", "pty", nil
	}, 1500*time.Millisecond, 400*time.Millisecond)
	if !res.Idle || res.Reason != IdleReasonQuiet {
		t.Fatalf("%+v", res)
	}
}

func TestWaitIdleEmptyTimesOut(t *testing.T) {
	res := WaitIdle(func() (string, string, error) {
		return "", "ui", nil
	}, 600*time.Millisecond, 400*time.Millisecond)
	if res.Idle {
		t.Fatalf("empty capture must not look idle: %+v", res)
	}
	if res.State != "timeout" && res.State != "busy" {
		t.Fatalf("state=%s", res.State)
	}
}

func TestPeekIdle(t *testing.T) {
	p := PeekIdle("bash-3.2$ ", "pty")
	if !p.Idle || !p.Prompt {
		t.Fatalf("%+v", p)
	}
	b := PeekIdle("working", "pty")
	if b.Idle || b.State != "busy" {
		t.Fatalf("%+v", b)
	}
}

func TestLooksLikeGrokSplash(t *testing.T) {
	splash := "Grok Build 1.0.5\nGrok 4.6 is here\nNew worktree    Resume    Changelog\n>"
	if !LooksLikeGrokSplash(splash) {
		t.Fatal("welcome chrome must look like splash")
	}
	if LooksLikePrompt(splash) {
		t.Fatal("splash must not look like a work prompt")
	}
	if LooksLikeGrokSplash("GBR_FEEDBACK_OK\nstopped as requested.") {
		t.Fatal("a real reply is not splash")
	}
	if LooksLikeGrokSplash("") {
		t.Fatal("empty is not splash")
	}
}

func TestWaitIdleSplashIsNotIdle(t *testing.T) {
	splash := "Grok Build 1.0.5\nGrok 4.6 is here\nNew worktree    Resume    Changelog\n>"
	res := WaitIdle(func() (string, string, error) {
		return splash, "pty", nil
	}, 900*time.Millisecond, 400*time.Millisecond)
	if res.Idle {
		t.Fatalf("splash must not count as idle after quiet: %+v", res)
	}
	if res.Reason == IdleReasonQuiet {
		t.Fatalf("quiet-on-splash is the bug: %+v", res)
	}
	if !strings.Contains(res.Excerpt, "Grok 4.6 is here") {
		t.Fatalf("excerpt should still carry splash for the caller: %q", res.Excerpt)
	}
}

func TestPeekIdleSplashBusy(t *testing.T) {
	splash := "Grok Build 1.0.5\nGrok 4.6 is here\nNew worktree    Resume    Changelog\n>"
	p := PeekIdle(splash, "pty")
	if p.Idle || p.Prompt {
		t.Fatalf("peek splash must be busy: %+v", p)
	}
	if p.Reason != "splash" {
		t.Fatalf("reason=%s", p.Reason)
	}
}
