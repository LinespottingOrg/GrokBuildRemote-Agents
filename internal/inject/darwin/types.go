//go:build darwin

package darwin

// Window is a best-effort snapshot of a Terminal/iTerm tab (cycle-free local type).
type Window struct {
	// HWND encodes window/tab indices: high 16 bits = window index, low 16 = tab index.
	HWND      uintptr
	PID       uint32
	Title     string
	ClassName string // "Terminal" or "iTerm"
	ExeName   string
	App       string // "Terminal" or "iTerm"
	WindowID  string // AppleScript window index as text
	TabIndex  int    // 1-based
	TTY       string
}

// TabInfo is a best-effort description of an open terminal tab/window.
type TabInfo struct {
	App      string // "Terminal" or "iTerm"
	WindowID string
	Title    string
	TTY      string
	Index    int
	PID      uint32
}
