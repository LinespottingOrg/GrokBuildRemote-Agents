package core

import (
	"context"
	"testing"
	"time"
)

func TestWatchdogQuality(t *testing.T) {
	w := NewWatchdog()
	w.probe = func(context.Context, int, string) (bool, string) { return false, "not listening" }

	snap := w.Snapshot(true)
	if snap.Quality != HealthStale {
		t.Fatalf("no roster yet want stale, got %s", snap.Quality)
	}

	w.TouchRoster()
	w.TouchRelay(true)
	snap = w.Snapshot(true)
	if snap.Quality != HealthOK {
		t.Fatalf("fresh roster want ok, got %s", snap.Quality)
	}
	if snap.Class == "" {
		t.Fatal("class must be set")
	}

	w.mu.Lock()
	w.lastRoster = time.Now().Add(-3 * time.Minute)
	w.mu.Unlock()
	snap = w.Snapshot(true)
	if snap.Quality != HealthZombie {
		t.Fatalf("old roster want zombie, got %s", snap.Quality)
	}
}

func TestWatchdogCompanionsUseProbe(t *testing.T) {
	w := NewWatchdog()
	w.probe = func(_ context.Context, port int, _ string) (bool, string) {
		return port == 2421, "stub"
	}
	snap := w.Snapshot(false)
	if len(snap.Companions) != 3 {
		t.Fatalf("want 3 companions, got %d", len(snap.Companions))
	}
	foundUp := false
	for _, c := range snap.Companions {
		if c.ID == "amnibro" && c.Online && c.Quality == HealthOK {
			foundUp = true
		}
		if c.ID == "farina" && c.Online {
			t.Fatal("farina should be down in stub")
		}
		if c.Repo == "" || c.Port == 0 {
			t.Fatalf("incomplete companion %+v", c)
		}
	}
	if !foundUp {
		t.Fatalf("amnibro should be up: %+v", snap.Companions)
	}
}
