//go:build darwin

package darwin

import (
	"context"
	"fmt"
	"strings"
)

// ListTabs enumerates open Terminal.app and iTerm2 tabs best-effort.
func ListTabs(ctx context.Context) ([]TabInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var out []TabInfo
	var errs []string

	term, err := listTerminalTabs(ctx)
	if err != nil {
		errs = append(errs, "Terminal: "+err.Error())
	} else {
		out = append(out, term...)
	}

	iterm, err := listITermTabs(ctx)
	if err != nil {
		errs = append(errs, "iTerm: "+err.Error())
	} else {
		out = append(out, iterm...)
	}

	if len(out) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("list tabs: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

func listTerminalTabs(ctx context.Context) ([]TabInfo, error) {
	const script = `
tell application "Terminal"
	if not (exists) then return ""
	set rows to {}
	set wi to 0
	repeat with w in windows
		set wi to wi + 1
		set ti to 0
		repeat with t in tabs of w
			set ti to ti + 1
			set ttl to ""
			try
				set ttl to custom title of t
			end try
			if ttl is "" then
				try
					set ttl to name of w
				end try
			end if
			set ttyn to ""
			try
				set ttyn to tty of t
			end try
			set end of rows to ((wi as text) & tab & (ti as text) & tab & ttl & tab & ttyn)
		end repeat
	end repeat
	set AppleScript's text item delimiters to linefeed
	return rows as text
end tell
`
	raw, err := runOSAscript(ctx, script)
	if err != nil {
		return nil, err
	}
	return parseTabLines(raw, "Terminal"), nil
}

func listITermTabs(ctx context.Context) ([]TabInfo, error) {
	const script = `
tell application "System Events"
	if not (exists process "iTerm2") and not (exists process "iTerm") then return ""
end tell
tell application "iTerm"
	set rows to {}
	set wi to 0
	repeat with w in windows
		set wi to wi + 1
		set ti to 0
		try
			repeat with t in tabs of w
				set ti to ti + 1
				set ttl to ""
				try
					set ttl to name of current session of t
				end try
				if ttl is "" then
					try
						set ttl to name of w
					end try
				end if
				set end of rows to ((wi as text) & tab & (ti as text) & tab & ttl & tab)
			end repeat
		end try
	end repeat
	set AppleScript's text item delimiters to linefeed
	return rows as text
end tell
`
	raw, err := runOSAscript(ctx, script)
	if err != nil {
		return nil, err
	}
	return parseTabLines(raw, "iTerm"), nil
}

func parseTabLines(raw, app string) []TabInfo {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []TabInfo
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		info := TabInfo{App: app}
		if len(parts) > 0 {
			info.WindowID = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &info.Index)
		}
		if len(parts) > 2 {
			info.Title = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			info.TTY = strings.TrimSpace(parts[3])
		}
		out = append(out, info)
	}
	return out
}

func matchTab(tabs []TabInfo, sessionID string) (TabInfo, bool) {
	if sessionID == "" {
		return TabInfo{}, false
	}
	want := strings.ToLower(sessionID)
	var sub *TabInfo
	for i := range tabs {
		t := tabs[i]
		title := strings.ToLower(t.Title)
		if title == want {
			return t, true
		}
		if sub == nil && strings.Contains(title, want) {
			cp := t
			sub = &cp
		}
	}
	if sub != nil {
		return *sub, true
	}
	return TabInfo{}, false
}

func tabToWindow(t TabInfo) Window {
	var wi, ti int
	fmt.Sscanf(t.WindowID, "%d", &wi)
	ti = t.Index
	var hwnd uintptr
	if wi > 0 && ti > 0 {
		hwnd = uintptr((wi << 16) | (ti & 0xffff))
	}
	exe := "Terminal"
	if t.App == "iTerm" {
		exe = "iTerm2"
	}
	title := t.Title
	if t.TTY != "" {
		if title != "" {
			title = title + " [" + t.TTY + "]"
		} else {
			title = t.TTY
		}
	}
	return Window{
		HWND:      hwnd,
		PID:       t.PID,
		Title:     title,
		ClassName: t.App,
		ExeName:   exe,
		App:       t.App,
		WindowID:  t.WindowID,
		TabIndex:  t.Index,
		TTY:       t.TTY,
	}
}

func decodeTabRef(hwnd uintptr) (windowIndex, tabIndex int) {
	if hwnd == 0 {
		return 0, 0
	}
	return int(hwnd >> 16), int(hwnd & 0xffff)
}
