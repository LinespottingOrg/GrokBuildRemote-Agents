# Windows service — Grok Build Remote

**Product:** Grok Build Remote  
**Binary (CLI name unchanged):** `gbr-agent.exe` at `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe`  
**Owner:** LinespottingOrg (private source; free end-user binaries)

**Display name (issue #55):** Users must see **Grok Build Remote** (or **Grok Build Remote Agent**), not bare `gbr`. WinSW `<name>` is `Grok Build Remote Agent`. Internal ids (`GrokBuildRemoteAgent` legacy interactive, `GrokBuildRemoteAgentService` this PR’s S4U task) are **not** the human name.

**Supported NI install:** [`scripts/windows/`](../../scripts/windows/README.md) (`install-service.ps1` / `uninstall-service.ps1`).

**Refuse:** commit `6f451ac` (labelled 0.6.3, 2026-08-24 wmic-flash only — **no** PR #40 halt / ack-on-fail). Not installable. Rebuild from `origin/main` after #40 (`f7bd6c1`) and later (#55/#61). Never `.aiprojects\gbr\agents\dist`. Never InteractiveToken.

---

## Goals (PC1 / no-popup)

1. **Non-interactive** runner — **Interactive-only / `InteractiveToken` is forbidden**
2. **One** `gbr-agent` process
3. Default **`GBR_INJECT_HALT=1`** and args **`-log=info run -inject-halt`** (David clears halt for live inject)
4. **`GBR_NO_AUTO_OPEN=1`** so Open cannot `CREATE_NEW_CONSOLE`
5. Logs → **`C:\pc-build\gbr-agent-out\`** (`GBR_LOG_DIR`)
6. Keep Agents **PR #40** ack-on-fail / single `command_id` (do not regress inject loop fixes)
7. After NI lands: **disable** legacy `\GrokBuildRemoteAgent` — **do not delete** without David yes

Admin Session 0 cannot inject into interactive desktops reliably. That is acceptable while inject is halted.

`gbr-agent service install` (Go) historically registered **InteractiveToken**. Do **not** use it on PC1. This folder is the supported path.

---

## Recommended: `scripts/windows/install-service.ps1`

```powershell
# Elevated PowerShell — registers; does not start unless -Start
cd <repo>\scripts\windows
.\install-service.ps1
```

Prefer **WinSW** when `gbr-agent-service.exe` is beside the LocalAppData binary; otherwise an **S4U + Highest** scheduled task named `\GrokBuildRemoteAgentService`.

Default: **no `-Start`**. David yes to start. Do not run this from unattended docs.

See [scripts/windows/README.md](../../scripts/windows/README.md).

---

## WinSW (Windows Service Wrapper)

[WinSW](https://github.com/winsw/winsw) wraps the executable as a Windows service with restart policies and logging.

### Layout (user-local — matches PC1)

```
%LOCALAPPDATA%\GrokBuildRemote\
  gbr-agent.exe          # halt-capable binary (NOT 6f451ac, NOT dist)
  gbr-agent-service.exe  # renamed WinSW executable
  gbr-agent.xml          # sample here; install-service.ps1 rewrites
```

Sample XML sets:

- `GBR_INJECT_HALT=1`
- `GBR_NO_AUTO_OPEN=1`
- `GBR_LOG_DIR=C:\pc-build\gbr-agent-out`
- arguments: `-log=info run -inject-halt` (`-inject-halt` is a **run** flag)

### Manual WinSW commands

```powershell
cd "$env:LOCALAPPDATA\GrokBuildRemote"
.\gbr-agent-service.exe install
# start only when intentionally bringing the agent up:
.\gbr-agent-service.exe start
.\gbr-agent-service.exe status
.\gbr-agent-service.exe stop
.\gbr-agent-service.exe uninstall
```

---

## Forbidden: Interactive-only Task Scheduler

Do **not** register tasks with:

- “Run only when user is logged on” / **Interactive only**
- XML `LogonType` = `InteractiveToken`
- Wrong binary under `.aiprojects\gbr\agents\dist\gbr-agent.exe`
- commit `6f451ac`

Legacy task on PC1: `\GrokBuildRemoteAgent` (interactive). After NI install, **disable** it:

```powershell
schtasks /Change /TN GrokBuildRemoteAgent /DISABLE
```

**Do not delete** without David yes:

```powershell
# FORBIDDEN unless David explicitly approves
# schtasks /Delete /TN GrokBuildRemoteAgent /F
```

---

## Alternative (documented): S4U + Highest AtLogon

Used automatically by `install-service.ps1` when WinSW is absent.

| Setting | Value |
| --- | --- |
| Task name (id) | `GrokBuildRemoteAgentService` |
| Human label | **Grok Build Remote** / WinSW **Grok Build Remote Agent** |
| LogonType | `S4U` (non-interactive) |
| RunLevel | `HighestAvailable` |
| Instances | `IgnoreNew` |
| Exec | `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe -log=info run -inject-halt` |
| Env (User) | `GBR_INJECT_HALT=1`, `GBR_NO_AUTO_OPEN=1`, `GBR_LOG_DIR=C:\pc-build\gbr-agent-out` |

Requires elevated PowerShell to register.

---

## Configuration

| Item | Location |
| --- | --- |
| xAI / Grok API key | `%USERPROFILE%\.grok\config.json` |
| Device / pairing | Agent-managed under `%LOCALAPPDATA%\GrokBuildRemote\` |
| Inject halt | User env `GBR_INJECT_HALT=1` + `-inject-halt` |
| No auto-open | User env `GBR_NO_AUTO_OPEN=1` |
| Logs | `C:\pc-build\gbr-agent-out\` |
| Protocol | `gbr/1` envelopes over Grok API |

Never commit API keys. Do not place secrets in WinSW XML committed to git.

---

## Uninstall

```powershell
cd <repo>\scripts\windows
.\uninstall-service.ps1
# David yes only:
# .\uninstall-service.ps1 -DeleteInteractiveTask
```

Or WinSW manually:

```powershell
cd "$env:LOCALAPPDATA\GrokBuildRemote"
.\gbr-agent-service.exe stop
.\gbr-agent-service.exe uninstall
```

---

## Microsoft Store (future)

Store-packaged builds will use MSIX and OS-managed lifecycle. WinSW / S4U remains the sideload path for halt + log directory until Store packaging is finalized.
