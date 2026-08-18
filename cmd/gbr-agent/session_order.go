package main

import (
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session"
)

func clipAndLog[T any](items []T, where string) []T {
	kept, dropped := session.ClipRoster(items)
	if dropped > 0 {
		slog.Warn("session roster over soft max; dropping extras",
			"have", len(items), "max", session.MaxSessions, "dropped", dropped, "where", where)
	}
	return kept
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

// prioritizeSessionIDs stable-sorts ids (Grok first). Soft max 255 —
// 0.5.0 sliced to 6 and hid Grok Build on busy Windows desktops.
func prioritizeSessionIDs(ids []string) []string {
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
	return clipAndLog(out, "feedback")
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
	procs := session.ScanGrokProcesses()
	out = make([]session.Candidate, 0, len(wins))
	for _, w := range wins {
		kind := string(w.Kind)
		if kind == "" {
			kind = "window"
		}
		gp, fromProc := session.MatchGrokProcess(int(w.PID), procs)
		isGrok := fromProc || session.LooksLikeGrokWindow(w.Title, w.ExeName, kind) ||
			isGrokSessionKey(kind) || isGrokSessionKey(w.Title)
		// UNIQUE per HWND — never share agent cwd (that collapsed all terminals
		// into one session_id and hid Grok Build on multi-window Windows desktops).
		prefer := kind + "-" + hexHWND(w.HWND)
		if isGrok {
			prefer = session.SuggestGrokSessionID(gp.ResumeID, w.HWND, int(w.PID))
			if prefer == "grok-build" || prefer == "" {
				prefer = "grok-build-" + hexHWND(w.HWND)
			}
			kind = string(inject.KindGrokBuild)
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
	// Caller already sorted Grok first; drop extras rather than crash.
	out = clipAndLog(out, "discover")
	return out, grokN, otherN
}

func hexHWND(h uintptr) string {
	return strings.ToLower(strconv.FormatUint(uint64(h), 16))
}

func itoaPID(pid uint32) string {
	return strconv.FormatUint(uint64(pid), 10)
}
