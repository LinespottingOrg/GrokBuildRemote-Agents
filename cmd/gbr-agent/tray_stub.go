//go:build !darwin && !linux

package main

// Settings stay on the loopback page. Windows uses cmd/gbr-tray.
func trayRunsOnMain() bool { return false }

func runTrayBlocking(_ int, agent func()) {
	if agent != nil {
		agent()
	}
}
