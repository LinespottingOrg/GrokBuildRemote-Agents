package inject

import (
	"strings"
	"time"
)

// clawReadyWait is how long a claw open waits for a TTY prompt.
// Tests set it to 0.
var clawReadyWait = 6 * time.Second

// ClawHolder reports OpenClaw, Hermes, and NemoClaw.
// Those clients use the managed-TTY protocol on every OS (ConPTY on Windows,
// the Mac pipe/PTY path elsewhere). They do not SendInput into a foreground console.
func ClawHolder(h string) bool {
	return ClawHolderName(h) != ""
}

// ClawHolderName returns hermes, openclaw, or nemoclaw. Empty if h is not a claw client.
func ClawHolderName(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.ReplaceAll(h, "_", "-")
	h = strings.Join(strings.Fields(h), "-")
	switch h {
	case "hermes":
		return "hermes"
	case "openclaw", "open-claw":
		return "openclaw"
	case "nemoclaw", "nemo-claw":
		return "nemoclaw"
	default:
		return ""
	}
}
