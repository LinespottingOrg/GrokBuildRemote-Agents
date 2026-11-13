//go:build (!darwin && !linux) || (darwin && !cgo)

package main

// Settings stay on the loopback page. Windows uses cmd/gbr-tray.
// Darwin cross-builds with CGO_ENABLED=0 keep this stub so CI still links.
func trayRunsOnMain() bool { return false }

func runTrayBlocking(_ int, agent func()) {
	if agent != nil {
		agent()
	}
}
