# Windows: session isolation (discover `grok_build=0`)

**Symptom:** `gbr-agent` discover logs `windows=0 grok_build=0` forever even though `grok.exe` (Grok Build CLI) windows are open on the desktop.

**Root cause:** The agent runs in Windows **session 0** (non-interactive / S4U / service-style scheduled task) while Grok Build windows live on the interactive console **session 1**. Session 0 cannot `EnumWindows` the interactive desktop, so discovery always sees zero windows.

## How to verify

```powershell
Get-Process gbr-agent,grok -ErrorAction SilentlyContinue |
  Format-Table Id,ProcessName,SessionId -AutoSize
```

Compare `SessionId` values. Healthy: both `gbr-agent` and `grok` are in the same interactive session (typically `1`).

```powershell
curl http://127.0.0.1:8788/v1/status
# tail today's agent log for discover:
Get-Content "$env:GBR_LOG_DIR\agent-$(Get-Date -Format yyyy-MM-dd).jsonl" -Tail 80 |
  Select-String 'discover|session_mismatch|session_ok|SESSION-ISOLATION|agent.session'
```

On PC1, `GBR_LOG_DIR` is `C:\pc-build\gbr-agent-out`. Look for discover detail like `windows=N grok_build=N` — `grok_build` must be **> 0** when Grok windows exist.

## Fix

1. **Run `gbr-agent` in the interactive user session** (same SessionId as the console / `grok.exe`).
2. **Disable** any session-0 scheduled task (on PC1: `\GrokBuildRemoteAgentService`). Do **not** re-enable S4U / "Run whether user is logged on or not" for window discovery.
3. Auto-start at logon with one of:
   - Startup-folder shortcut, or
   - `schtasks` with **/IT** / "Run only when user is logged on"
4. Never use S4U / session 0 for an agent that must discover desktop windows.

### Recommended env (PC1)

| Variable | Value |
|----------|--------|
| `GBR_INJECT_HALT` | `0` |
| `GBR_INBOX_WATCH` | `1` |
| `GBR_NO_AUTO_OPEN` | `0` |
| `GBR_LOG_DIR` | `C:\pc-build\gbr-agent-out` |

Agent: `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe` args `-log=info run`.

### Apply script (PC1)

```powershell
cd <repo>\scripts\windows
powershell -NoProfile -ExecutionPolicy Bypass -File .\apply-session-isolation-pc1.ps1
```

That script disables `\GrokBuildRemoteAgentService`, registers interactive logon auto-start for the agent + `session-watch.ps1`, writes `C:\pc-build\gbr-agent-out\SESSION-ISOLATION.md`, and starts the watcher minimized. It does **not** kill a healthy agent already owning port 8788 in session 1.

## Log grep keys

| Key | Meaning |
|-----|---------|
| `session_mismatch` | Watcher detected agent in session 0 or session ≠ console while grok is elsewhere |
| `agent.session` / `agent.session_watch` | Session metadata / watcher hop |
| `grok_build=0` | Discover saw no Grok Build windows (often session isolation) |
| `SESSION-ISOLATION` | Doc / fix marker |
| `session_ok` | Watcher healthy heartbeat (`agent_session` == `console_session`) |

Sample mismatch line:

```json
{"ts":"2026-09-20T20:00:00.000Z","hop":"agent.session_watch","actor":"session-watch","type":"session_mismatch","ok":false,"detail":"agent_session=0 console_session=1 grok_in_other_session=true fix=run_agent_interactive_disable_session0_task"}
```

## WinSW / service install

Windows service / WinSW / S4U installs that force session 0 remain **on HOLD** for machines that need `EnumWindows` discovery. Prefer interactive logon auto-start until a supported interactive-session host exists. See `scripts/windows/README.md` (NI path) vs this doc (discover path).

## Related

- [TROUBLESHOOTING.md](../TROUBLESHOOTING.md#windows-discover-grok_build0--session-isolation) — section **Windows: discover grok_build=0 / session isolation**
- [FAQ.md](../FAQ.md#why-does-discover-show-grok_build0-windows0-on-windows-even-though-grok-build-is-open) — homepage / support Q: *Why does discover show grok_build=0 / windows=0 on Windows even though Grok Build is open?*
- [AGENTS.md](../AGENTS.md)
- `scripts/windows/session-watch.ps1` · `scripts/windows/apply-session-isolation-pc1.ps1`
