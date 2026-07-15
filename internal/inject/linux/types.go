//go:build linux

package linux

// Window is a discovered X11 window (cycle-free local type).
type Window struct {
	HWND      uintptr // X window id
	PID       uint32
	Title     string
	ClassName string
	ExeName   string
	ID        string // decimal string for xdotool
}

// WinInfo is an alias-friendly discovery record.
type WinInfo struct {
	ID    string
	Name  string
	Class string
	PID   uint32
}
