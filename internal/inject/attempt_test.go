package inject

import (
	"errors"
	"testing"
	"time"
)

func TestAttemptGuard_ReplaySameCommandID(t *testing.T) {
	g := newAttemptGuard()
	req := InjectRequest{Text: "do the thing", Submit: true, CommandID: "cmd-loop"}
	if err := g.Admit("sess-a", req.CommandID); err != nil {
		t.Fatal(err)
	}
	err := g.Admit("sess-a", req.CommandID)
	if !errors.Is(err, ErrInjectReplay) {
		t.Fatalf("second admit must be ErrInjectReplay, got %v", err)
	}
}

func TestAttemptGuard_EmptyCommandIDNotReplay(t *testing.T) {
	g := newAttemptGuard()
	if err := g.Admit("sess-a", ""); err != nil {
		t.Fatal(err)
	}
	if err := g.Admit("sess-a", ""); err != nil {
		t.Fatalf("empty command_id must not trip replay: %v", err)
	}
}

func TestAttemptGuard_Halt(t *testing.T) {
	t.Setenv("GBR_INJECT_HALT", "1")
	g := newAttemptGuard()
	if err := g.Admit("sess-a", "cmd-1"); !errors.Is(err, ErrInjectHalted) {
		t.Fatalf("halt must refuse, got %v", err)
	}
}

func TestAttemptGuard_MaxZeroHalts(t *testing.T) {
	t.Setenv("GBR_INJECT_MAX", "0")
	g := newAttemptGuard()
	if err := g.Admit("sess-a", "cmd-1"); !errors.Is(err, ErrInjectHalted) {
		t.Fatalf("MAX=0 must halt, got %v", err)
	}
}

func TestAttemptGuard_SessionBudget(t *testing.T) {
	t.Setenv("GBR_INJECT_MAX", "2")
	g := newAttemptGuard()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }

	if err := g.Admit("sess-a", "c1"); err != nil {
		t.Fatal(err)
	}
	if err := g.Admit("sess-a", "c2"); err != nil {
		t.Fatal(err)
	}
	if err := g.Admit("sess-a", "c3"); !errors.Is(err, ErrInjectBudget) {
		t.Fatalf("3rd inject must be ErrInjectBudget, got %v", err)
	}
	// Other session still allowed.
	if err := g.Admit("sess-b", "c4"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(DefaultInjectWindow + time.Second)
	if err := g.Admit("sess-a", "c5"); err != nil {
		t.Fatalf("after window, budget must reset: %v", err)
	}
}

func TestAttemptGuard_UnsetMaxUnlimitedDistinctIDs(t *testing.T) {
	t.Setenv("GBR_INJECT_MAX", "")
	g := newAttemptGuard()
	for i := 0; i < 12; i++ {
		if err := g.Admit("sess-a", ""); err != nil {
			t.Fatalf("unset max must not cap empty-id injects: %v", err)
		}
	}
}
