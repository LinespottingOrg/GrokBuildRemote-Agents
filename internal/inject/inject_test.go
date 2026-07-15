package inject

import (
	"errors"
	"testing"
	"time"
)

func TestValidateRequest_EmptySession(t *testing.T) {
	err := ValidateRequest("", InjectRequest{Text: "hi"})
	if !errors.Is(err, ErrEmptySession) {
		t.Fatalf("want ErrEmptySession, got %v", err)
	}
}

func TestValidateRequest_EmptyText(t *testing.T) {
	err := ValidateRequest("s1", InjectRequest{})
	if !errors.Is(err, ErrEmptyText) {
		t.Fatalf("want ErrEmptyText, got %v", err)
	}
}

func TestValidateRequest_SubmitOnlyOK(t *testing.T) {
	if err := ValidateRequest("s1", InjectRequest{Submit: true}); err != nil {
		t.Fatal(err)
	}
}

func TestEffectiveText_Submit(t *testing.T) {
	got := EffectiveText(InjectRequest{Text: "echo hi", Submit: true})
	if got != "echo hi\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRateLimiter_MinInterval(t *testing.T) {
	rl := NewRateLimiter(50*time.Millisecond, 100, time.Second)
	if err := rl.Allow("a"); err != nil {
		t.Fatal(err)
	}
	if err := rl.Allow("a"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want rate limit, got %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if err := rl.Allow("a"); err != nil {
		t.Fatal(err)
	}
}

func TestRateLimiter_EmptySession(t *testing.T) {
	rl := NewRateLimiter(0, 0, 0)
	if err := rl.Allow(""); !errors.Is(err, ErrEmptySession) {
		t.Fatalf("want empty session, got %v", err)
	}
}

func TestRateLimiter_Burst(t *testing.T) {
	rl := NewRateLimiter(1*time.Millisecond, 3, 500*time.Millisecond)
	for i := 0; i < 3; i++ {
		if err := rl.Allow("b"); err != nil {
			t.Fatalf("inject %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if err := rl.Allow("b"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want burst limit, got %v", err)
	}
}
