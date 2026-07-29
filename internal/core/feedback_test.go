package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNearestFeedbackInterval(t *testing.T) {
	cases := map[int]int{
		0:    10,
		5:    5,
		7:    5,
		9:    10,
		12:   10,
		45:   60,
		90:   60,
		500:  600,
		2000: 600, // closer to 600 than 3600
		3000: 3600,
	}
	for in, want := range cases {
		if got := NearestFeedbackInterval(in); got != want {
			t.Fatalf("NearestFeedbackInterval(%d)=%d want %d", in, got, want)
		}
	}
}

func TestFeedbackSaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	InvalidateFeedbackCache()

	cfg := FeedbackConfig{Enabled: true, IntervalSec: 60, Expand: true}
	if err := SaveFeedback(cfg); err != nil {
		t.Fatal(err)
	}
	InvalidateFeedbackCache()
	got := LoadFeedback()
	if !got.Enabled || got.IntervalSec != 60 || !got.Expand {
		t.Fatalf("loaded %+v", got)
	}
	path, _ := FeedbackPath()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	// ensure under temp home
	if filepath.Dir(path) != filepath.Join(dir, ".gbr") {
		t.Fatalf("path %s not under temp .gbr", path)
	}
}

func TestMaxFeedbackChunk(t *testing.T) {
	c := FeedbackConfig{Expand: false}
	if c.MaxFeedbackChunk() >= 12*1024 {
		t.Fatal("minimal should be smaller than expand")
	}
	c.Expand = true
	if c.MaxFeedbackChunk() < 8*1024 {
		t.Fatal("expand should allow larger tail")
	}
}
