# gbr-tray — Windows system-tray app

`gbr-tray` is a GUI-subsystem (no console) supervisor for `gbr-agent` on Windows.
It is the login autostart; it keeps the agent running and gives a tray menu.

Build:

```
go build -ldflags "-H=windowsgui -X main.version=<v>" ./cmd/gbr-tray
```

## Tray menu

| Item | Action |
|------|--------|
| Status | MessageBox with agent health (version, relay quality, phone online, paused) from `GET /health` |
| Open logs | Opens the agent log folder (`GBR_LOG_DIR`) |
| Pause / Resume polling | Toggles `POST /v1/control/pause` / `/resume` — gates the boss-steer inbox watch and the main relay poll; the relay connection and Bot API stay up so the phone still shows the device online |
| Restart agent | Stops `gbr-agent.exe` and relaunches it hidden |
| Quit | Stops the agent and exits |

The tray also supervises: if the agent stops responding on `127.0.0.1:8788` it is
relaunched (~every 15s). The running `grok -p` jobs are never touched — only
`gbr-agent.exe` is stopped/started.

## Autostart (session 1)

On launch (and via `gbr-tray install-autostart`) it writes
`HKCU\Software\Microsoft\Windows\CurrentVersion\Run\GrokBuildRemoteTray` pointing
at itself and removes the legacy `GrokBuildRemoteAgent.lnk` Startup shortcut (which
launched `start-gbr-agent-interactive.cmd`) so the agent is not started twice.
HKCU Run entries execute in the interactive logon session (session 1).

`gbr-tray uninstall-autostart` removes the Run value.

## Child-window behaviour

The agent still spawns real grok consoles (CREATE_NEW_CONSOLE) because SendInput
injection needs a window — that is unchanged. Only the agent's own helper children
(gh/git/etc.) are created hidden; see `internal/proc`.
