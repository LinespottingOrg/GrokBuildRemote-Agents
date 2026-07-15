//go:build linux

package linux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ListWindows returns terminal-like windows via xdotool search (best-effort).
func ListWindows(ctx context.Context, classHints []string) ([]WinInfo, error) {
	if err := ensureXdotool(ctx); err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	var out []WinInfo

	hints := classHints
	if len(hints) == 0 {
		hints = defaultTerminalClasses()
	}
	for _, class := range hints {
		cids, err := searchWindowIDs(ctx, "--class", class)
		if err != nil {
			continue
		}
		for _, id := range cids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, inspectWindow(ctx, id))
		}
	}

	for _, token := range []string{"Terminal", "terminal", "tmux", "zsh", "bash", "fish"} {
		ids, err := searchWindowIDs(ctx, "--name", token)
		if err != nil {
			continue
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			info := inspectWindow(ctx, id)
			if isTerminalLike(info, hints) {
				out = append(out, info)
			}
		}
	}

	return out, nil
}

func defaultTerminalClasses() []string {
	return []string{
		"gnome-terminal", "Gnome-terminal",
		"konsole", "Konsole",
		"xfce4-terminal", "Xfce4-terminal",
		"kitty", "Kitty",
		"alacritty", "Alacritty",
		"wezterm", "org.wezfurlong.wezterm",
		"xterm", "XTerm",
		"terminator", "Terminator",
		"tilix", "Tilix",
		"mate-terminal", "Mate-terminal",
		"foot", "Foot",
		"urxvt", "URxvt",
		"st", "St",
	}
}

func searchWindowIDs(ctx context.Context, flag, value string) ([]string, error) {
	args := []string{"search", "--onlyvisible", flag, value}
	out, err := runXdotool(ctx, args...)
	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			return nil, nil
		}
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		id := strings.TrimSpace(line)
		if id == "" {
			continue
		}
		if _, err := strconv.ParseUint(id, 0, 64); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func inspectWindow(ctx context.Context, id string) WinInfo {
	info := WinInfo{ID: id}
	if name, err := runXdotool(ctx, "getwindowname", id); err == nil {
		info.Name = name
	}
	if class, err := runXdotool(ctx, "getwindowclassname", id); err == nil {
		info.Class = class
	}
	if pidStr, err := runXdotool(ctx, "getwindowpid", id); err == nil {
		if p, err := strconv.ParseUint(pidStr, 10, 32); err == nil {
			info.PID = uint32(p)
		}
	}
	return info
}

func isTerminalLike(w WinInfo, classHints []string) bool {
	class := strings.ToLower(w.Class)
	name := strings.ToLower(w.Name)
	for _, h := range classHints {
		if h == "" {
			continue
		}
		hl := strings.ToLower(h)
		if class != "" && (class == hl || strings.Contains(class, hl)) {
			return true
		}
	}
	for _, tok := range []string{"terminal", "konsole", "kitty", "alacritty", "wezterm", "xterm", "tmux", "shell"} {
		if strings.Contains(name, tok) || strings.Contains(class, tok) {
			return true
		}
	}
	return false
}

func matchWindow(wins []WinInfo, sessionID string) (WinInfo, bool) {
	if sessionID == "" {
		return WinInfo{}, false
	}
	want := strings.ToLower(sessionID)
	var sub *WinInfo
	for i := range wins {
		w := wins[i]
		name := strings.ToLower(w.Name)
		if name == want {
			return w, true
		}
		if sub == nil && strings.Contains(name, want) {
			cp := w
			sub = &cp
		}
	}
	if sub != nil {
		return *sub, true
	}
	return WinInfo{}, false
}

// SearchBySession uses xdotool search --name sessionID (fast path).
func SearchBySession(ctx context.Context, sessionID string) ([]WinInfo, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("empty sessionID")
	}
	ids, err := searchWindowIDs(ctx, "--name", sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]WinInfo, 0, len(ids))
	for _, id := range ids {
		out = append(out, inspectWindow(ctx, id))
	}
	return out, nil
}

func winToWindow(w WinInfo) Window {
	var hwnd uintptr
	if id, err := strconv.ParseUint(w.ID, 0, 64); err == nil {
		hwnd = uintptr(id)
	}
	return Window{
		HWND:      hwnd,
		PID:       w.PID,
		Title:     w.Name,
		ClassName: w.Class,
		ExeName:   w.Class,
		ID:        w.ID,
	}
}

func windowIDFromHWND(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(hwnd), 10)
}
