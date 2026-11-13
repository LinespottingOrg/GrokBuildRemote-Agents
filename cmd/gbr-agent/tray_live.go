//go:build linux || (darwin && cgo)

package main

import (
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"fyne.io/systray"
)

func trayRunsOnMain() bool {
	if trayDisabled() {
		return false
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		slog.Info("tray skipped; no DISPLAY or WAYLAND_DISPLAY", "settings", "http://127.0.0.1:8788/settings")
		return false
	}
	return true
}

func runTrayBlocking(port int, agent func()) {
	systray.Run(func() {
		installTrayMenu(port)
		go func() {
			if agent != nil {
				agent()
			}
			systray.Quit()
		}()
	}, func() {})
}

func installTrayMenu(port int) {
	systray.SetIcon(trayIconPNG)
	systray.SetTitle("Grok Build Remote")
	systray.SetTooltip("Grok Build Remote settings")
	mSettings := systray.AddMenuItem("Settings…", "Roster visibility and agent settings")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop gbr-agent")
	systray.SetOnTapped(func() { openSettings(port) })
	go func() {
		for {
			select {
			case <-mSettings.ClickedCh:
				openSettings(port)
			case <-mQuit.ClickedCh:
				if p, err := os.FindProcess(os.Getpid()); err == nil {
					_ = p.Signal(syscall.SIGTERM)
				}
				return
			}
		}
	}()
}

func openSettings(port int) {
	if port <= 0 {
		port = defaultBotPort
	}
	u := "http://127.0.0.1:" + itoaPort(port) + "/settings"
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "linux":
		cmd = exec.Command("xdg-open", u)
	default:
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("open settings", "url", u, "err", err)
	}
}

func itoaPort(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
