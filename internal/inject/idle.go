package inject

import (
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Idle detection for the bot/MCP feedback loop. We never wait for a full
// Grok TUI scrollback — a prompt at the tail, or N seconds of no new bytes,
// is enough to harvest an excerpt and let the caller judge.

const (
	DefaultIdleQuiet = 2500 * time.Millisecond
	DefaultIdleWait  = 60 * time.Second
	MinIdleQuiet     = 400 * time.Millisecond
	MaxIdleWait      = 180 * time.Second
)

// Common interactive prompts (bash / zsh / fish / pwsh) plus a few Grok TUI
// tail markers. Keep these conservative — a false "still running" is safer
// than harvesting mid-stream.
var promptLine = regexp.MustCompile(`(?i)` +
	`(?:` +
	`(?:^|\n)\s*(?:bash|zsh|sh|fish|ksh)[-0-9.]*\$\s*$` + // bash-3.2$
	`|` +
	`(?:^|\n)\s*(?:\S+@\S+[:\s]\S+\s*[$#%>]|[$#%>])\s*$` + // user@host:dir$
	`|` +
	`(?:^|\n)\s*(?:PS\s+[A-Z]:\\[^\n]*>)\s*$` + // PS C:\>
	`|` +
	`(?:^|\n)\s*(?:❯|➜|→)\s*$` +
	`)`)

var grokIdleTail = regexp.MustCompile(`(?i)` +
	`(?:` +
	`what do you want` +
	`|what next` +
	`|waiting for (?:input|prompt)` +
	`|(?:^|\n)\s*(?:ready)\s*$` +
	`|(?:^|\n)\s*[│┃]\s*>\s*$` +
	`)`)

// LooksLikeGrokSplash reports the Grok Build welcome chrome (version banner,
// "Grok 4.x is here", New worktree / Resume / Changelog). That screen is
// static for many seconds before the TUI accepts input — it is NOT idle.
func LooksLikeGrokSplash(text string) bool {
	t := strings.ToLower(stripANSI(text))
	if strings.TrimSpace(t) == "" {
		return false
	}
	hits := 0
	if strings.Contains(t, "grok 4.6 is here") || strings.Contains(t, "grok 4.5 is here") ||
		(strings.Contains(t, "grok 4") && strings.Contains(t, "is here")) {
		hits += 2
	}
	if strings.Contains(t, "new worktree") {
		hits++
	}
	if strings.Contains(t, "changelog") {
		hits++
	}
	if strings.Contains(t, "grok build") && (strings.Contains(t, "1.0") || strings.Contains(t, "resume")) {
		hits++
	}
	return hits >= 2
}

// LooksLikePrompt reports whether the capture tail looks like an idle prompt.
func LooksLikePrompt(text string) bool {
	if LooksLikeGrokSplash(text) {
		return false
	}
	text = stripANSI(text)
	text = strings.TrimRightFunc(text, unicode.IsSpace)
	if text == "" {
		return false
	}
	// Only the last ~800 bytes decide — a long log ending in a prompt is idle.
	if len(text) > 800 {
		text = text[len(text)-800:]
	}
	return promptLine.MatchString(text) || grokIdleTail.MatchString(text)
}

// IdleReason is why WaitIdle stopped.
const (
	IdleReasonPrompt  = "prompt"
	IdleReasonQuiet   = "quiet"
	IdleReasonTimeout = "timeout"
	IdleReasonError   = "error"
)

// IdleResult is the structured peek both Bot API and MCP return.
type IdleResult struct {
	Idle        bool   `json:"idle"`
	Reason      string `json:"reason"`
	State       string `json:"state"` // idle | busy | timeout | error
	Excerpt     string `json:"excerpt"`
	ExcerptBytes int   `json:"excerpt_bytes"`
	Prompt      bool   `json:"prompt"`
	WaitedMS    int    `json:"waited_ms"`
	QuietMS     int    `json:"quiet_ms,omitempty"`
	Method      string `json:"method,omitempty"`
	Changed     bool   `json:"changed"`
}

// ExcerptTail keeps the most recent bytes (Grok TUI / shell output).
func ExcerptTail(text string, max int) string {
	if max <= 0 {
		max = 4000
	}
	text = stripANSI(text)
	if len(text) <= max {
		return text
	}
	return text[len(text)-max:]
}

func stripANSI(s string) string {
	if s == "" || !strings.ContainsRune(s, '\x1b') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				j++
				if (c >= '@' && c <= '~') || c == 'm' {
					break
				}
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// ClampIdleWait bounds a caller-supplied wait.
func ClampIdleWait(wait, quiet time.Duration) (time.Duration, time.Duration) {
	if wait <= 0 {
		wait = DefaultIdleWait
	}
	if wait > MaxIdleWait {
		wait = MaxIdleWait
	}
	if quiet <= 0 {
		quiet = DefaultIdleQuiet
	}
	if quiet < MinIdleQuiet {
		quiet = MinIdleQuiet
	}
	if quiet > wait {
		quiet = wait
	}
	return wait, quiet
}

// CaptureFn is a session snapshot. Empty text is allowed (UI often has none).
type CaptureFn func() (text, method string, err error)

// WaitIdle polls capture until prompt, quiet period, or timeout.
func WaitIdle(capture CaptureFn, wait, quiet time.Duration) IdleResult {
	wait, quiet = ClampIdleWait(wait, quiet)
	start := time.Now()
	deadline := start.Add(wait)
	var last, method string
	lastChange := start
	poll := 250 * time.Millisecond
	if quiet < poll*2 {
		poll = quiet / 2
		if poll < 80*time.Millisecond {
			poll = 80 * time.Millisecond
		}
	}

	for {
		text, meth, err := capture()
		if meth != "" {
			method = meth
		}
		if err != nil && strings.TrimSpace(text) == "" {
			return IdleResult{
				Reason:  IdleReasonError,
				State:   "error",
				Excerpt: err.Error(),
				WaitedMS: int(time.Since(start).Milliseconds()),
				Method:  method,
			}
		}
		if text != last {
			if last != "" || strings.TrimSpace(text) != "" {
				lastChange = time.Now()
			}
			last = text
		}
		prompt := LooksLikePrompt(text)
		splash := LooksLikeGrokSplash(text)
		quietFor := time.Since(lastChange)
		if prompt && !splash {
			return finishIdle(true, IdleReasonPrompt, text, method, start, quietFor, last != "")
		}
		if quietFor >= quiet && time.Since(start) >= quiet {
			// Quiet with some text → idle. Quiet with empty capture is NOT
			// success (typical Grok UI) — keep waiting until timeout.
			// The welcome splash is also NOT idle: it sits still for seconds
			// before Grok is ready to take a prompt.
			if strings.TrimSpace(text) != "" && !splash {
				return finishIdle(true, IdleReasonQuiet, text, method, start, quietFor, true)
			}
		}
		if time.Now().After(deadline) {
			reason := IdleReasonTimeout
			state := "timeout"
			idle := false
			if prompt && !splash {
				reason = IdleReasonPrompt
				state = "idle"
				idle = true
			} else if strings.TrimSpace(text) != "" && quietFor >= quiet && !splash {
				reason = IdleReasonQuiet
				state = "idle"
				idle = true
			} else if strings.TrimSpace(text) != "" {
				state = "busy"
				if splash {
					reason = "splash"
				}
			}
			ex := ExcerptTail(text, 4000)
			return IdleResult{
				Idle:         idle,
				Reason:       reason,
				State:        state,
				Excerpt:      ex,
				ExcerptBytes: len(ex),
				Prompt:       prompt,
				WaitedMS:     int(time.Since(start).Milliseconds()),
				QuietMS:      int(quietFor.Milliseconds()),
				Method:       method,
				Changed:      strings.TrimSpace(text) != "",
			}
		}
		time.Sleep(poll)
	}
}

func finishIdle(idle bool, reason, text, method string, start time.Time, quietFor time.Duration, changed bool) IdleResult {
	state := "busy"
	if idle {
		state = "idle"
	}
	ex := ExcerptTail(text, 4000)
	return IdleResult{
		Idle:         idle,
		Reason:       reason,
		State:        state,
		Excerpt:      ex,
		ExcerptBytes: len(ex),
		Prompt:       reason == IdleReasonPrompt,
		WaitedMS:     int(time.Since(start).Milliseconds()),
		QuietMS:      int(quietFor.Milliseconds()),
		Method:       method,
		Changed:      changed,
	}
}

// PeekIdle is a single-shot classify (no wait).
func PeekIdle(text, method string) IdleResult {
	ex := ExcerptTail(text, 4000)
	splash := LooksLikeGrokSplash(text)
	prompt := LooksLikePrompt(text) && !splash
	idle := prompt && strings.TrimSpace(text) != ""
	state := "busy"
	reason := ""
	if strings.TrimSpace(text) == "" {
		state = "busy"
		reason = "empty"
	} else if splash {
		state = "busy"
		reason = "splash"
	} else if prompt {
		state = "idle"
		reason = IdleReasonPrompt
	} else {
		reason = "output"
	}
	return IdleResult{
		Idle:         idle,
		Reason:       reason,
		State:        state,
		Excerpt:      ex,
		ExcerptBytes: len(ex),
		Prompt:       prompt,
		Method:       method,
		Changed:      strings.TrimSpace(text) != "",
	}
}
