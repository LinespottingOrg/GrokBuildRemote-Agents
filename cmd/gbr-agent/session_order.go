package main

import (
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session"
)

// DefaultFeedbackMaxSessions is the fan-out cap for periodic phone feedback.
// 0.5.0 used 6, which hid Grok Build on busy Windows desktops (many conhost
// windows). 32 covers a typical multi-window machine without flooding the relay.
const DefaultFeedbackMaxSessions = 32

// feedbackMaxSessions returns the feedback fan-out cap.
// Override with GBR_FEEDBACK_MAX_SESSIONS (1–128). 0 or unset → default.
func feedbackMaxSessions() int {
	raw := strings.TrimSpace(os.Getenv("GBR_FEEDBACK_MAX_SESSIONS"))
	if raw == "" {
		return DefaultFeedbackMaxSessions
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return DefaultFeedbackMaxSessions
	}
	if n > 128 {
		return 128
	}
	return n
}

func isGrokSessionKey(s string) bool {
	lt := strings.ToLower(s)
	return strings.Contains(lt, "grok")
}

func isAgentSessionKey(s string) bool {
	lt := strings.ToLower(s)
	return lt == "admin" || strings.Contains(lt, "gbr-agent") || strings.Contains(lt, "managed")
}

// sessionPriority: 0 = Grok Build (must appear first on the phone),
// 1 = the agent shell, 2 = other terminals.
func sessionPriority(id, title, shell string) int {
	if isGrokSessionKey(id) || isGrokSessionKey(title) || isGrokSessionKey(shell) {
		return 0
	}
	if isAgentSessionKey(id) || isAgentSessionKey(title) {
		return 1
	}
	return 2
}

// prioritizeSessionIDs stable-sorts ids (Grok first) then applies max.
// max <= 0 means no cap.
func prioritizeSessionIDs(ids []string, max int) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi := sessionPriority(out[i], "", "")
		pj := sessionPriority(out[j], "", "")
		if pi != pj {
			return pi < pj
		}
		return false
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func sortWindowsGrokFirst(wins []inject.TerminalWindow) {
	sort.SliceStable(wins, func(i, j int) bool {
		gi := isGrokSessionKey(string(wins[i].Kind)) || isGrokSessionKey(wins[i].Title)
		gj := isGrokSessionKey(string(wins[j].Kind)) || isGrokSessionKey(wins[j].Title)
		if gi != gj {
			return gi
		}
		return wins[i].Title < wins[j].Title
	})
}

func windowsToCandidates(wins []inject.TerminalWindow) (out []session.Candidate, grokN, otherN int) {
	out = make([]session.Candidate, 0, len(wins))
	for _, w := range wins {
		kind := string(w.Kind)
		if kind == "" {
			kind = "window"
		}
		// UNIQUE per HWND — never share agent cwd (that collapsed all terminals
		// into one session_id and hid Grok Build on multi-window Windows desktops).
		prefer := kind + "-" + hexHWND(w.HWND)
		if isGrokSessionKey(kind) || isGrokSessionKey(w.Title) {
			prefer = "grok-build-" + hexHWND(w.HWND)
			grokN++
		} else {
			otherN++
		}
		title := w.Title
		if title == "" {
			title = kind
		}
		out = append(out, session.Candidate{
			CWD:      "gbr-ui-" + kind + "-" + itoaPID(w.PID) + "-" + hexHWND(w.HWND),
			Shell:    kind,
			PID:      int(w.PID),
			HWND:     w.HWND,
			Title:    title,
			PreferID: prefer,
		})
	}
	return out, grokN, otherN
}

func hexHWND(h uintptr) string {
	return strings.ToLower(strconv.FormatUint(uint64(h), 16))
}

func itoaPID(pid uint32) string {
	return strconv.FormatUint(uint64(pid), 10)
}
