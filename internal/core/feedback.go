package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FeedbackFileName stores agent→phone text feedback preferences under ~/.gbr/.
const FeedbackFileName = "feedback.json"

// Allowed feedback poll intervals (seconds). Phone UI mirrors these labels.
var FeedbackIntervals = []int{5, 10, 60, 600, 3600}

// FeedbackConfig controls periodic minimal text feedback from the agent.
// Default is OFF so existing inject/capture behaviour is unchanged until enabled.
type FeedbackConfig struct {
	Enabled     bool `json:"enabled"`
	IntervalSec int  `json:"interval_sec"`
	// Expand asks the agent to send a larger tail (more lines) when true.
	Expand bool `json:"expand"`
	// SessionID optional filter; empty = all registered sessions.
	SessionID string `json:"session_id,omitempty"`
}

// Normalize clamps interval to an allowed value and returns a copy.
func (c FeedbackConfig) Normalize() FeedbackConfig {
	out := c
	out.IntervalSec = NearestFeedbackInterval(c.IntervalSec)
	return out
}

// NearestFeedbackInterval snaps to the closest allowed interval (default 10s).
func NearestFeedbackInterval(sec int) int {
	if sec <= 0 {
		return 10
	}
	best := FeedbackIntervals[0]
	bestDiff := absInt(sec - best)
	for _, v := range FeedbackIntervals[1:] {
		d := absInt(sec - v)
		if d < bestDiff {
			best, bestDiff = v, d
		}
	}
	return best
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// FeedbackIntervalLabel is a short human label for UI/status.
func FeedbackIntervalLabel(sec int) string {
	switch NearestFeedbackInterval(sec) {
	case 5:
		return "5s"
	case 10:
		return "10s"
	case 60:
		return "1m"
	case 600:
		return "10m"
	case 3600:
		return "1h"
	default:
		return fmt.Sprintf("%ds", sec)
	}
}

// MaxFeedbackChunk is the max bytes pushed per periodic sample (minimal text).
func (c FeedbackConfig) MaxFeedbackChunk() int {
	if c.Expand {
		return 12 * 1024
	}
	return 4 * 1024
}

// FeedbackPath returns ~/.gbr/feedback.json.
func FeedbackPath() (string, error) {
	dir, err := deviceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FeedbackFileName), nil
}

var (
	feedbackMu   sync.Mutex
	feedbackMemo *FeedbackConfig
)

// LoadFeedback loads feedback.json or returns defaults (disabled, 10s).
func LoadFeedback() FeedbackConfig {
	feedbackMu.Lock()
	defer feedbackMu.Unlock()
	if feedbackMemo != nil {
		return *feedbackMemo
	}
	cfg := FeedbackConfig{Enabled: false, IntervalSec: 10, Expand: false}
	path, err := FeedbackPath()
	if err != nil {
		feedbackMemo = &cfg
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		feedbackMemo = &cfg
		return cfg
	}
	var f FeedbackConfig
	if err := json.Unmarshal(data, &f); err != nil {
		feedbackMemo = &cfg
		return cfg
	}
	f = f.Normalize()
	feedbackMemo = &f
	return f
}

// SaveFeedback persists and updates the in-memory memo.
func SaveFeedback(c FeedbackConfig) error {
	c = c.Normalize()
	path, err := FeedbackPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	feedbackMu.Lock()
	feedbackMemo = &c
	feedbackMu.Unlock()
	return nil
}

// InvalidateFeedbackCache forces next LoadFeedback to re-read disk.
func InvalidateFeedbackCache() {
	feedbackMu.Lock()
	feedbackMemo = nil
	feedbackMu.Unlock()
}

// FeedbackTickerDuration returns the loop period for the feedback goroutine.
func (c FeedbackConfig) FeedbackTickerDuration() time.Duration {
	return time.Duration(c.Normalize().IntervalSec) * time.Second
}
