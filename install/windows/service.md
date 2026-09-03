# Windows service — gbr-agent

**Product:** Grok Build Remote  
**Binary:** `gbr-agent.exe` at `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe` (must include PR #40 halt; **not** `6f451ac`)  
**Owner:** LinespottingOrg (private source; free end-user binaries)

**Supported NI install:** [`scripts/windows/`](../../scripts/windows/README.md) (`install-service.ps1` / `uninstall-service.ps1`).

## Goals (PC1 / no-popup)

1. **Non-interactive** runner — **Interactive-only / `InteractiveToken` is forbidden**
2. **One** `gbr-agent` process
3. Default **`GBR_INJECT_HALT=1`** (David clears halt for live inject)
4. Logs → **`C:\pc-build\gbr-agent-out\`** (`GBR_LOG_DIR`)
5. Keep Agents **PR #40** ack-on-fail / single `command_id` (do not regress inject loop fixes)
6. After NI lands: **disable** legacy `\GrokBuildRemoteAgent` — **do not delete** without David yes

Admin Session 0 services cannot inject into interactive desktops reliably. That is acceptable while inject is halted. For a David-gated live inject trial, prefer a user-session design that still avoids Interactive-only focus theft — and keep halt off only with explicit approval.

---

## Recommended: `scripts/windows/install-service.ps1`

```powershell
# Elevated PowerShell — registers; does not start unless -Start
cd <repo>\scripts\windows
.\install-service.ps1
```

Prefer **WinSW** when `gbr-agent-service.exe` is beside the LocalAppData binary; otherwise an **S4U + Highest** scheduled task named `\GrokBuildRemoteAgentService`.

See [scripts/windows/README.md](../../scripts/windows/README.md) for install, uninstall, halt, and log path.

---

## WinSW (Windows Service Wrapper)

[WinSW](https://github.com/winsw/winsw) wraps the executable as a Windows service with restart policies and logging.

### Layout (user-local — matches PC1)

```
%LOCALAPPDATA%\GrokBuildRemote\
  gbr-agent.exe          # agent binary 0.6.3+ (REQUIRED path)
  gbr-agent-service.exe  # renamed WinSW executable
  gbr-agent.xml          # WinSW config (sample in this folder; install script rewrites)
```

Sample XML in this folder sets:

- `GBR_INJECT_HALT=1`
- `GBR_LOG_DIR=C:\pc-build\gbr-agent-out`
- arguments: `-log=info run -inject-halt` (`-inject-halt` is a **run** flag; before `run` it is unknown-command)

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

Legacy task name on PC1: `\GrokBuildRemoteAgent` (interactive). After NI install, **disable** it:

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

Used automatically by `install-service.ps1` when WinSW is absent. Summary:

| Setting | Value |
| --- | --- |
| Task name | `GrokBuildRemoteAgentService` |
| LogonType | `S4U` (non-interactive) |
| RunLevel | `HighestAvailable` |
| Instances | `IgnoreNew` |
| Exec | `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe -log=info run -inject-halt` |
| Env (User) | `GBR_INJECT_HALT=1`, `GBR_LOG_DIR=C:\pc-build\gbr-agent-out` |

Requires elevated PowerShell to register.

---

## Configuration

| Item | Location |
| --- | --- |
| xAI / Grok API key | `%USERPROFILE%\.grok\config.json` |
| Device / pairing | Agent-managed under `%LOCALAPPDATA%\GrokBuildRemote\` |
| Inject halt | User env `GBR_INJECT_HALT=1` + `-inject-halt` |
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

Store-packaged builds will use MSIX and OS-managed lifecycle. WinSW / S4U remains the sideload path for full control of halt + log directory until Store packaging is finalized.
