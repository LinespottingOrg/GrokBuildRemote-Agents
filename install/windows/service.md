# Windows service — gbr-agent

**Product:** Grok Build Remote  
**Binary:** `gbr-agent.exe`  
**Owner:** LinespottingOrg (private source; free end-user binaries)

The Windows agent should run as a **user-session service** (or login-started process) so it can:

1. Discover terminal sessions (Windows Terminal, ConEmu, etc.)
2. Inject keystrokes via `SendInput` into the focused / target session
3. Long-poll the Grok relay (protocol `gbr/1`) without an open inbound port

Admin services running in Session 0 **cannot** inject into interactive desktops reliably. Prefer **WinSW** under the logged-in user, or Task Scheduler “At log on”.

---

## Recommended: WinSW (Windows Service Wrapper)

[WinSW](https://github.com/winsw/winsw) wraps any executable as a Windows service with restart policies and logging.

### Layout

```
C:\Program Files\GrokBuildRemote\
  gbr-agent.exe          # agent binary (free download)
  gbr-agent-service.exe  # renamed WinSW executable
  gbr-agent.xml          # WinSW config (sample in this folder)
  logs\                  # created by WinSW
```

User-local alternative (no admin):

```
%LOCALAPPDATA%\GrokBuildRemote\
  gbr-agent.exe
  gbr-agent-service.exe
  gbr-agent.xml
  logs\
```

### Install steps

1. Download free `gbr-agent` Windows amd64 binary from the product website (or GitHub Release asset `gbr-agent-windows-amd64.exe`). Rename to `gbr-agent.exe`.
2. Download WinSW (`WinSW-x64.exe`), rename to `gbr-agent-service.exe`, place next to the agent.
3. Copy `gbr-agent.xml` from this repo (`install/windows/gbr-agent.xml`) next to both executables. Edit paths and env if needed.
4. Open an elevated PowerShell **only if** installing under Program Files:

```powershell
cd "C:\Program Files\GrokBuildRemote"
.\gbr-agent-service.exe install
.\gbr-agent-service.exe start
.\gbr-agent-service.exe status
```

5. For user-local install (no elevation), use Task Scheduler instead of a true service (see below), or install WinSW with a user account service where policy allows.

### Useful WinSW commands

```powershell
.\gbr-agent-service.exe stop
.\gbr-agent-service.exe start
.\gbr-agent-service.exe restart
.\gbr-agent-service.exe uninstall
```

Logs: `logs\gbr-agent.out.log` / `logs\gbr-agent.err.log` (paths from XML).

---

## Alternative: Task Scheduler (per-user, recommended for inject)

**PowerShell (run as the interactive user):**

```powershell
$exe = "$env:LOCALAPPDATA\GrokBuildRemote\gbr-agent.exe"
$action  = New-ScheduledTaskAction -Execute $exe -Argument "run"
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName "GrokBuildRemote-Agent" -Action $action -Trigger $trigger -Settings $settings -Description "Grok Build Remote agent (gbr-agent)"
```

Start now:

```powershell
Start-ScheduledTask -TaskName "GrokBuildRemote-Agent"
```

---

## Configuration

| Item | Location |
|------|----------|
| xAI / Grok API key | `%USERPROFILE%\.grok\config.json` |
| Device / pairing | Agent-managed under `%LOCALAPPDATA%\GrokBuildRemote\` (implementation-defined) |
| Protocol | `gbr/1` envelopes over Grok API (no phone↔PC sockets) |

Never commit API keys. Do not place secrets in the WinSW XML committed to git.

---

## Microsoft Store (future)

Store-packaged builds will use MSIX and OS-managed lifecycle. WinSW remains the sideload / website-installer path for full inject capabilities until Store packaging is finalized.

---

## Uninstall

WinSW:

```powershell
cd "C:\Program Files\GrokBuildRemote"   # or your install dir
.\gbr-agent-service.exe stop
.\gbr-agent-service.exe uninstall
```

Task Scheduler:

```powershell
Unregister-ScheduledTask -TaskName "GrokBuildRemote-Agent" -Confirm:$false
```

Remove the install directory and optional `%LOCALAPPDATA%\GrokBuildRemote` state as desired.
