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
