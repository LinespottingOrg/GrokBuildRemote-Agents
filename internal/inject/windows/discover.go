//go:build windows

package windows

import (
	"path/filepath"
	"strings"
	"syscall"
)

// Known window class names for terminal hosts.
var terminalClasses = map[string]Kind{
	"CASCADIA_HOSTING_WINDOW_CLASS": KindWindowsTerminal, // Windows Terminal
	"ConsoleWindowClass":            KindConhost,         // classic console / conhost
	"PseudoConsoleWindow":           KindConhost,
}

func kindFromExe(base string) Kind {
	switch strings.ToLower(base) {
	case "windowsterminal.exe":
		return KindWindowsTerminal
	case "conhost.exe":
		return KindConhost
	case "powershell.exe", "pwsh.exe":
		return KindPowerShell
	case "cmd.exe":
		return KindCmd
	default:
		return KindUnknown
	}
}

func isTerminalCandidate(class, exeBase string) (Kind, bool) {
	if k, ok := terminalClasses[class]; ok {
		return k, true
	}
	k := kindFromExe(exeBase)
	if k != KindUnknown {
		return k, true
	}
	return KindUnknown, false
}

// Discover enumerates top-level visible windows and returns terminal-like hosts.
// Best-effort: title/class/exe may be empty under restricted access.
func Discover() ([]Window, error) {
	var out []Window
	seen := make(map[uintptr]struct{})

	_ = enumWindows(func(hwnd syscall.Handle) bool {
		if !isWindowVisible(hwnd) {
			return true
		}
		class := getClassName(hwnd)
		title := getWindowText(hwnd)
		_, knownClass := terminalClasses[class]
		if title == "" && !knownClass {
			return true
		}

		_, pid := getWindowThreadProcessId(hwnd)
		img := openProcessImage(pid)
		base := strings.ToLower(filepath.Base(img))

		kind, ok := isTerminalCandidate(class, base)
		if !ok {
			lt := strings.ToLower(title)
			if strings.Contains(lt, "powershell") || strings.Contains(lt, "windows terminal") ||
				strings.Contains(lt, "command prompt") || strings.HasPrefix(lt, "cmd") {
				kind = kindFromExe(base)
				ok = true
			}
		}
		if !ok {
			return true
		}

		h := uintptr(hwnd)
		if _, dup := seen[h]; dup {
			return true
		}
		seen[h] = struct{}{}

		out = append(out, Window{
			HWND:      h,
			PID:       pid,
			Title:     title,
			ClassName: class,
			ExeName:   base,
			Kind:      kind,
		})
		return true
	})

	return out, nil
}

// FindByTitleSubstring returns windows whose title contains sub (case-insensitive).
func FindByTitleSubstring(sub string) ([]Window, error) {
	all, err := Discover()
	if err != nil {
		return nil, err
	}
	sub = strings.ToLower(sub)
	var out []Window
	for _, w := range all {
		if strings.Contains(strings.ToLower(w.Title), sub) {
			out = append(out, w)
		}
	}
	return out, nil
}

// FindByPID filters discovered terminals by process id.
func FindByPID(pid uint32) ([]Window, error) {
	all, err := Discover()
	if err != nil {
		return nil, err
	}
	var out []Window
	for _, w := range all {
		if w.PID == pid {
			out = append(out, w)
		}
	}
	return out, nil
}
