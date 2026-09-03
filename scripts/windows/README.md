# Windows — non-interactive gbr-agent install

**Product:** Grok Build Remote  
**Inbox:** [grok-build-inbox#123](https://github.com/LinespottingOrg/grok-build-inbox/issues/123) (GBR Windows service no popup), [#91](https://github.com/LinespottingOrg/grok-build-inbox/issues/91) (popup loop)  
**Related:** Agents PR #40 (ack-on-fail / single `command_id` — do not regress)

Install the agent so it **cannot** raise interactive approval UI on the logged-in desktop by default. Inject stays **halted** until David explicitly clears the halt.

**PC1 fact:** `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe` labelled **0.6.3** `commit=6f451ac` (2026-08-24) is the **wmic flash** fix only. It does **not** include PR #40 (`GBR_INJECT_HALT` / `-inject-halt` / ack-on-fail). `install-service.ps1` **refuses** that SHA. Replace the exe from `origin/main` after #40 (`f7bd6c1`) before install.

**Flag order:** `-inject-halt` is a **`run` subcommand** flag. Correct: `-log=info run -inject-halt`. Wrong: `-log=info -inject-halt run` (unknown-command, process dies).

## Hard rules

| Rule | Detail |
| --- | --- |
| Binary path | `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe` with **PR #40** (`-inject-halt` / `GBR_INJECT_HALT`). **Never** `.aiprojects\gbr\agents\dist\...`. **Refuse** commit `6f451ac` (labelled 0.6.3, 2026-08-24 — wmic flash fix only, no halt). |
| Interactive-only | **Forbidden.** Do not use Task Scheduler “Interactive only” / `InteractiveToken`. |
| Inject | Default `GBR_INJECT_HALT=1` (+ `-inject-halt`). David must clear halt for live inject. |
| Logs | `C:\pc-build\gbr-agent-out\` via `GBR_LOG_DIR` |
| One agent | IgnoreNew / singleton lock — do not spawn duplicates |
| Legacy task | `\GrokBuildRemoteAgent` (interactive) → **disable** after NI install; **do not delete** without David yes |
| Secrets | Never in commits, scripts, XML, or PR bodies |

## What gets installed

**Prefer (real Windows Service):** [WinSW](https://github.com/winsw/winsw) renamed to `gbr-agent-service.exe` beside the binary. Session 0 is fine while inject is halted.

**Fallback (no WinSW):** Scheduled Task `\GrokBuildRemoteAgentService` with:

- `LogonType` = **S4U** (non-interactive — not InteractiveToken)
- `RunLevel` = **HighestAvailable**
- `MultipleInstancesPolicy` = **IgnoreNew**
- `Hidden` = true
- Direct `Exec` of `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe -log=info run -inject-halt`

## Install

Elevated PowerShell (required for S4U task / typical WinSW service install):

```powershell
cd <repo>\scripts\windows
# Registers service/task; does NOT start the agent
.\install-service.ps1

# Optional: start after register (only when David wants the process up)
.\install-service.ps1 -Start
```

Optional flags:

| Flag | Meaning |
| --- | --- |
| `-Start` | Start service/task after install (default: **off**) |
| `-SkipDisableInteractiveTask` | Leave legacy `\GrokBuildRemoteAgent` enabled |
| `-AllowInject` | **David live trial only** — skip `GBR_INJECT_HALT` / `-inject-halt` |
| `-BinaryPath <path>` | Override exe (must still be the LocalAppData 0.6.3+ binary in normal use) |
| `-LogDir <path>` | Override log dir (default `C:\pc-build\gbr-agent-out`) |

WinSW (optional, preferred when available):

1. Download `WinSW-x64.exe`, rename to `gbr-agent-service.exe`
2. Place next to `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe`
3. Run `.\install-service.ps1` (writes `gbr-agent.xml` with halt + log dir)

## Duplicates (one process)

```powershell
cd <repo>\scripts\windows
.\kill-duplicates.ps1          # list only
.\kill-duplicates.ps1 -Kill    # stop extras; keep LocalAppData exe if running
```

Does **not** start the agent. Does **not** clear halt.

## Halt (inject kill-switch)

Default after install:

- User env `GBR_INJECT_HALT=1`
- Process args include `-inject-halt`
- Agent refuses all injects (Bot API / mailbox / inbox) — see `internal/inject/attempt.go`

**Clear halt only with David yes** (live inject trial):

```powershell
# Example — David-approved live trial reinstall
.\install-service.ps1 -AllowInject -Start
# Or manually clear User env GBR_INJECT_HALT, then restart the service/task
```

Keep PR #40 behavior: failed inject must ack/die, not respawn approval cards.

## Logs

| Item | Path |
| --- | --- |
| `GBR_LOG_DIR` | `C:\pc-build\gbr-agent-out\` |
| WinSW roll logs | same directory (when WinSW used) |
| Agent JSONL traces | under `GBR_LOG_DIR` when set (else `~\.gbr\logs`) |

## Uninstall

```powershell
cd <repo>\scripts\windows
.\uninstall-service.ps1
```

| Flag | Meaning |
| --- | --- |
| `-DeleteInteractiveTask` | Also delete legacy `\GrokBuildRemoteAgent` — **David yes only** |
| `-ClearHaltEnv` | Remove User `GBR_INJECT_HALT` |
| `-ClearLogEnv` | Remove User `GBR_LOG_DIR` |

The LocalAppData binary is **not** deleted.

## Disable legacy interactive task (no delete)

`install-service.ps1` disables `\GrokBuildRemoteAgent` after the NI runner lands (unless `-SkipDisableInteractiveTask`).

```powershell
schtasks /Change /TN GrokBuildRemoteAgent /DISABLE
```

Do **not**:

```powershell
schtasks /Delete /TN GrokBuildRemoteAgent /F   # without David yes
```

## Status checks (manual)

```powershell
schtasks /Query /TN GrokBuildRemoteAgentService /V /FO LIST
schtasks /Query /TN GrokBuildRemoteAgent /V /FO LIST
# WinSW:
& "$env:LOCALAPPDATA\GrokBuildRemote\gbr-agent-service.exe" status
Get-Content Env:GBR_INJECT_HALT
Get-Content Env:GBR_LOG_DIR
```

Do not start `gbr-agent` from docs automation; David / PC1 ops decide when to `-Start`. HOLD (inbox #123): do not merge/install/start or clear halt until David picks merge+install.

## Why not `gbr-agent service install` alone?

The built-in Windows installer (`internal/service/service_windows.go`) historically registered an **InteractiveToken** logon task (Interactive-only) and could point at whatever exe was used to install (including a wrong `.aiprojects\...\dist` path). This folder is the supported NI path for PC1 until that Go path is aligned in a later release.
