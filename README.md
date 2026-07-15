# Grok Build Remote — Agents

**Free** desktop agent for Windows, macOS, and Linux.  
Source is **private company IP** (LinespottingOrg). Binaries are free for end users.

**Product:** Grok Build Remote  
**Binary:** `gbr-agent`  
**Relay:** phone never connects to PC — both use the durable mailbox relay.

## Platforms

| OS | UI inject | Fallback | Autostart |
|----|-----------|----------|-----------|
| **Windows** | SendInput (Windows Terminal / conhost) | Managed `pwsh` | Task Scheduler (user logon) |
| **macOS** | AppleScript Terminal / iTerm2 | Managed `bash` | LaunchAgent |
| **Linux** | xdotool (X11) | Managed `bash` | systemd `--user` |

UI inject is best-effort. **Managed shell always works** (reliability path).

## Install

### Windows (PowerShell)

```powershell
cd path\to\GrokBuildRemote-Agents
.\install\windows\install.ps1
# or copy dist\gbr-agent-windows-amd64.exe to %LOCALAPPDATA%\gbr\gbr-agent.exe
gbr-agent doctor
gbr-agent pair -code YOURCODE
gbr-agent service install
```

### macOS

```bash
chmod +x install/darwin/install.sh
./install/darwin/install.sh
gbr-agent doctor
gbr-agent pair -code YOURCODE
gbr-agent service install
# System Settings → Privacy & Security → Accessibility + Automation
```

### Linux

```bash
chmod +x install/linux/install.sh
./install/linux/install.sh
# optional: sudo apt install xdotool
gbr-agent doctor
gbr-agent pair -code YOURCODE
gbr-agent service install
# optional: loginctl enable-linger $USER
```

### Universal (Unix)

```bash
curl -fsSL https://grok-build-remote.pages.dev/install.sh | bash
# private releases: export GH_TOKEN=...
```

## Commands

```
gbr-agent version
gbr-agent doctor
gbr-agent status
gbr-agent pair -code ABCD1234
gbr-agent rename -name "Grok Build Remote"
gbr-agent rename -session my-project
gbr-agent sessions
gbr-agent run                 # foreground poll loop
gbr-agent service install     # autostart
gbr-agent service status
gbr-agent service uninstall
```

## Build all platforms

```powershell
# Windows host
.\scripts\build-all.ps1
```

```bash
# Unix host
./scripts/build-all.sh
```

Outputs under `dist/`:

- `gbr-agent-windows-amd64.exe`
- `gbr-agent-darwin-amd64` / `darwin-arm64`
- `gbr-agent-linux-amd64` / `linux-arm64`

## Release

Push a tag `v*` → GitHub Actions builds all five binaries and publishes a Release.

```bash
git tag v0.3.0
git push origin v0.3.0
```

## Layout

```
cmd/gbr-agent/          CLI
internal/core/          config, device, seen dedup
internal/relay/         durable mailbox client
internal/session/       discovery + naming
internal/inject/        hybrid UI + managed shell (win/darwin/linux)
internal/service/       Task Scheduler / launchd / systemd
internal/doctor/        readiness checks
install/                OS installers + unit samples
```

## License

Proprietary — LinespottingOrg. Free binary use for end users. See `LICENSE`.
