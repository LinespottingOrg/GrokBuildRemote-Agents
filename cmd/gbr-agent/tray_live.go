//go:build darwin || linux

package main

import (
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/gogpu/systray"
)

func trayRunsOnMain() bool {
	if trayDisabled() {
		return false
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		slog.Info("tray skipped; no DISPLAY or WAYLAND_DISPLAY", "settings", settingsURL(0))
		return false
	}
	return true
}

func runTrayBlocking(port int, agent func()) {
	tray := systray.New().
		SetAppName("Grok Build Remote").
		SetIcon(trayIconPNG).
		SetTooltip("Grok Build Remote settings").
		SetMenu(trayMenu(port)).
		OnClick(func() { openSettings(port) }).
		Show()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if agent != nil {
			agent()
		}
		tray.Remove()
	}()
	if err := tray.Run(); err != nil {
		slog.Warn("tray stopped", "err", err, "settings", settingsURL(port))
	}
	<-done
}

func trayMenu(port int) *systray.Menu {
	menu := systray.NewMenu()
	menu.Add("Settings…", func() { openSettings(port) })
	menu.AddSeparator()
	menu.Add("Quit", func() {
		p, err := os.FindProcess(os.Getpid())
		if err == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
	})
	return menu
}

func openSettings(port int) {
	u := settingsURL(port)
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
