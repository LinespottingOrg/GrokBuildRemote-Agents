#Requires -Version 5.1
<#
.SYNOPSIS
  PC1 lasting fix: interactive gbr-agent logon auto-start + session watcher + local docs.

.DESCRIPTION
  - Keeps \GrokBuildRemoteAgentService Disabled (session 0 — do not re-enable for discover)
  - Registers interactive logon auto-start for gbr-agent (/IT / Run only when user is logged on)
  - Writes C:\pc-build\gbr-agent-out\SESSION-ISOLATION.md
  - Installs session-watch.ps1, starts minimized, registers logon auto-start
  - Does NOT kill a healthy gbr-agent already in the interactive session owning :8788
  - Does NOT install WinSW / does not touch the WinSW service path (HOLD)

.NOTES
  Prefer fewer UAC prompts. Disabling the scheduled task may need elevation once.
#>
[CmdletBinding()]
param(
  [string]$AgentExe = "$env:LOCALAPPDATA\GrokBuildRemote\gbr-agent.exe",
  [string]$LogDir = 'C:\pc-build\gbr-agent-out',
  [string]$Session0TaskName = '\GrokBuildRemoteAgentService',
  [string]$InteractiveTaskName = '\GrokBuildRemoteAgentInteractive',
  [string]$WatcherTaskName = '\GrokBuildRemoteSessionWatch'
)

$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$WatcherSrc = Join-Path $ScriptDir 'session-watch.ps1'
$WatcherDst = Join-Path $LogDir 'session-watch.ps1'
$DocDst = Join-Path $LogDir 'SESSION-ISOLATION.md'

Write-Host "==> PC1 session-isolation apply"
Write-Host "    AgentExe=$AgentExe"
Write-Host "    LogDir=$LogDir"

# --- 1) Disable session-0 task ---
Write-Host "==> Disable $Session0TaskName (session 0)"
try {
  schtasks /Query /TN $Session0TaskName 2>&1 | Out-Null
  if ($LASTEXITCODE -eq 0) {
    schtasks /Change /TN $Session0TaskName /Disable 2>&1 | Out-Host
    Write-Host "    Disabled (or already disabled)."
  } else {
    Write-Host "    Task not found (ok)."
  }
} catch {
  Write-Warning "Could not disable $Session0TaskName (may need elevation once): $_"
}

# --- 2) Local SESSION-ISOLATION.md ---
Write-Host "==> Write $DocDst"
@'
# Windows session isolation — PC1 (Workstation)

**Date:** 2026-09-20 (Europe/Stockholm)  
**machineId:** 7dc5a2ec-06be-4b4c-a7b5-7b56eb64ce3a

## Symptom

`gbr-agent` discover logged `windows=0 grok_build=0` forever while `grok.exe` (Grok Build CLI) windows were open on the interactive desktop.

## Root cause

- `gbr-agent` ran as scheduled task `\GrokBuildRemoteAgentService` in Windows **session 0**.
- Grok Build CLI windows live in the interactive console **session 1**.
- Session 0 cannot `EnumWindows` the interactive desktop → discover always sees zero windows.

## How to verify

```powershell
Get-Process gbr-agent,grok -ErrorAction SilentlyContinue |
  Format-Table Id,ProcessName,SessionId -AutoSize
curl http://127.0.0.1:8788/v1/status
Get-Content C:\pc-build\gbr-agent-out\agent-$(Get-Date -Format yyyy-MM-dd).jsonl -Tail 80 |
  Select-String 'discover|session_mismatch|session_ok|SESSION-ISOLATION|agent.session'
```

Healthy: `gbr-agent` SessionId == console session (typically 1), port 8788 owned by that PID, discover detail has `grok_build` > 0 when Grok windows exist.

## Fix

1. Keep `\GrokBuildRemoteAgentService` **Disabled** (do not re-enable session-0 / S4U for window discovery).
2. Auto-start `gbr-agent` at user logon **in the interactive session** (Startup shortcut or schtasks `/IT` / "Run only when user is logged on").
3. Never use S4U for an agent that must discover desktop windows.
4. WinSW / service install remains on **HOLD** for this discover path.

### Agent

- Path: `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe` (`C:\Users\User\AppData\Local\GrokBuildRemote\gbr-agent.exe`)
- Args: `-log=info run`
- Env: `GBR_INJECT_HALT=0` `GBR_INBOX_WATCH=1` `GBR_NO_AUTO_OPEN=0` `GBR_LOG_DIR=C:\pc-build\gbr-agent-out`

### Watcher

`C:\pc-build\gbr-agent-out\session-watch.ps1` appends `session_mismatch` / `session_ok` JSON lines to today's `agent-YYYY-MM-DD.jsonl` (hop `agent.session_watch`).

## Log grep keys

`session_mismatch` · `agent.session` · `agent.session_watch` · `grok_build=0` · `SESSION-ISOLATION` · `session_ok`

## Repo docs

https://github.com/LinespottingOrg/GrokBuildRemote-Agents/blob/main/docs/SESSION-ISOLATION.md  
https://github.com/LinespottingOrg/GrokBuildRemote-Agents/blob/main/TROUBLESHOOTING.md
'@ | Set-Content -Path $DocDst -Encoding UTF8

# --- 3) Install watcher script ---
Write-Host "==> Install session-watch.ps1 -> $WatcherDst"
if (Test-Path $WatcherSrc) {
  Copy-Item -Force $WatcherSrc $WatcherDst
} elseif (-not (Test-Path $WatcherDst)) {
  throw "session-watch.ps1 not found at $WatcherSrc and no existing $WatcherDst"
} else {
  Write-Host "    Using existing $WatcherDst"
}

# --- 4) Interactive agent logon task ---
Write-Host "==> Register interactive logon task $InteractiveTaskName"
if (-not (Test-Path $AgentExe)) {
  Write-Warning "Agent exe missing: $AgentExe — task will still be registered."
}

$Launcher = Join-Path $LogDir 'start-gbr-agent-interactive.cmd'
@"
@echo off
set GBR_INJECT_HALT=0
set GBR_INBOX_WATCH=1
set GBR_NO_AUTO_OPEN=0
set GBR_LOG_DIR=$LogDir
cd /d "%LOCALAPPDATA%\GrokBuildRemote"
start "" /MIN "$AgentExe" -log=info run
"@ | Set-Content -Path $Launcher -Encoding ASCII

schtasks /Delete /TN $InteractiveTaskName /F 2>$null | Out-Null
schtasks /Create /TN $InteractiveTaskName /SC ONLOGON /RL LIMITED `
  /TR "`"$Launcher`"" /F 2>&1 | Out-Host
schtasks /Change /TN $InteractiveTaskName /IT 2>&1 | Out-Host
schtasks /Change /TN $InteractiveTaskName /ENABLE 2>&1 | Out-Host

$Startup = [Environment]::GetFolderPath('Startup')
$ShortcutPath = Join-Path $Startup 'GrokBuildRemoteAgent.lnk'
try {
  $w = New-Object -ComObject WScript.Shell
  $sc = $w.CreateShortcut($ShortcutPath)
  $sc.TargetPath = $Launcher
  $sc.WorkingDirectory = $LogDir
  $sc.WindowStyle = 7
  $sc.Description = 'GBR agent interactive session (SESSION-ISOLATION)'
  $sc.Save()
  Write-Host "    Startup shortcut: $ShortcutPath"
} catch {
  Write-Warning "Startup shortcut failed: $_"
}

# --- 5) Watcher logon task + start now ---
Write-Host "==> Register watcher logon task $WatcherTaskName"
$WatcherLauncher = Join-Path $LogDir 'start-session-watch.cmd'
@"
@echo off
set GBR_LOG_DIR=$LogDir
start "" /MIN powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Minimized -File "$WatcherDst"
"@ | Set-Content -Path $WatcherLauncher -Encoding ASCII

schtasks /Delete /TN $WatcherTaskName /F 2>$null | Out-Null
schtasks /Create /TN $WatcherTaskName /SC ONLOGON /RL LIMITED `
  /TR "`"$WatcherLauncher`"" /F 2>&1 | Out-Host
schtasks /Change /TN $WatcherTaskName /IT 2>&1 | Out-Host
schtasks /Change /TN $WatcherTaskName /ENABLE 2>&1 | Out-Host

$WatcherStartup = Join-Path $Startup 'GrokBuildRemoteSessionWatch.lnk'
try {
  $w = New-Object -ComObject WScript.Shell
  $sc = $w.CreateShortcut($WatcherStartup)
  $sc.TargetPath = $WatcherLauncher
  $sc.WorkingDirectory = $LogDir
  $sc.WindowStyle = 7
  $sc.Description = 'GBR session-watch (SESSION-ISOLATION)'
  $sc.Save()
  Write-Host "    Watcher Startup shortcut: $WatcherStartup"
} catch {
  Write-Warning "Watcher Startup shortcut failed: $_"
}

$watchRunning = Get-CimInstance Win32_Process -Filter "Name='powershell.exe'" -ErrorAction SilentlyContinue |
  Where-Object { $_.CommandLine -and $_.CommandLine -like '*session-watch.ps1*' }
if (-not $watchRunning) {
  Write-Host "==> Starting session-watch minimized now"
  Start-Process -FilePath 'powershell.exe' -ArgumentList @(
    '-NoProfile','-ExecutionPolicy','Bypass','-WindowStyle','Minimized','-File', $WatcherDst
  ) -WindowStyle Minimized
} else {
  Write-Host "==> session-watch already running (pid $($watchRunning.ProcessId))"
}

# --- 6) Do not kill healthy agent ---
Write-Host "==> Check healthy gbr-agent (interactive / :8788)"
$agents = @(Get-Process gbr-agent -ErrorAction SilentlyContinue)
$portOwner = $null
try {
  $portOwner = (Get-NetTCPConnection -LocalPort 8788 -State Listen -ErrorAction SilentlyContinue |
    Select-Object -First 1 -ExpandProperty OwningProcess)
} catch {}
$consoleGuess = 1
$ex = Get-Process explorer -ErrorAction SilentlyContinue | Select-Object -First 1
if ($ex) { $consoleGuess = [int]$ex.SessionId }

if ($agents.Count -gt 0) {
  foreach ($a in $agents) {
    Write-Host ("    gbr-agent pid={0} session={1} (console~{2}) port8788_owner={3}" -f `
      $a.Id, $a.SessionId, $consoleGuess, $portOwner)
  }
  $healthy = $agents | Where-Object {
    $_.SessionId -eq $consoleGuess -and ($null -eq $portOwner -or $_.Id -eq $portOwner)
  }
  if ($healthy) {
    Write-Host "    Leaving healthy interactive agent running (no kill)."
  } elseif ($agents | Where-Object { $_.SessionId -eq 0 }) {
    Write-Warning "Agent still in session 0 — stop it manually and start via $Launcher (interactive)."
  }
} else {
  Write-Host "    No gbr-agent running — starting interactive instance now"
  Start-Process -FilePath $Launcher -WindowStyle Minimized
}

Write-Host "==> Done. Verify:"
Write-Host '    Get-Process gbr-agent,grok | ft Id,ProcessName,SessionId'
Write-Host '    curl http://127.0.0.1:8788/v1/status'
Write-Host "    Select-String session_ok,session_mismatch $LogDir\agent-*.jsonl"
