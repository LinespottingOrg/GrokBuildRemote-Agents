// Package service installs gbr-agent as a user-level background runner.
//
// Windows: Task Scheduler (logon) — preferred so inject can use interactive desktop.
// macOS:   LaunchAgent (~/Library/LaunchAgents).
// Linux:   systemd user unit.
package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Paths holds resolved binary + unit paths for the current OS.
type Paths struct {
	Binary     string // absolute path to gbr-agent
	UnitPath   string // plist / service unit / task name
	DataDir    string // ~/.gbr
	Platform   string
	ExtraNotes string
}

// Resolve finds the running binary and platform install paths.
func Resolve() (Paths, error) {
	bin, err := os.Executable()
	if err != nil {
		return Paths{}, err
	}
	bin, err = filepath.Abs(bin)
	if err != nil {
		return Paths{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	data := filepath.Join(home, ".gbr")
	p := Paths{
		Binary:   bin,
		DataDir:  data,
		Platform: runtime.GOOS,
	}
	switch runtime.GOOS {
	case "windows":
		p.UnitPath = "GrokBuildRemoteAgent" // Task Scheduler task name
		p.ExtraNotes = "User logon task (interactive session for SendInput). Not a Session 0 service."
	case "darwin":
		p.UnitPath = filepath.Join(home, "Library", "LaunchAgents", "com.linespotting.gbr-agent.plist")
		p.ExtraNotes = "LaunchAgent; grant Accessibility + Automation for Terminal/iTerm if using UI inject."
	case "linux":
		p.UnitPath = filepath.Join(home, ".config", "systemd", "user", "gbr-agent.service")
		p.ExtraNotes = "systemd --user; install xdotool for UI inject on X11; managed shell works on Wayland."
	default:
		return p, fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
	return p, nil
}

// Install writes unit files and enables autostart. Implementation is OS-tagged.
func Install() error {
	return installPlatform()
}

// Uninstall removes unit files and disables autostart.
func Uninstall() error {
	return uninstallPlatform()
}

// Status returns a human-readable install state.
func Status() (string, error) {
	return statusPlatform()
}
