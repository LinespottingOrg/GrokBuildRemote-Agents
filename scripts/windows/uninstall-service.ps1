#Requires -Version 5.1
<#
.SYNOPSIS
  Uninstall the non-interactive gbr-agent WinSW service and/or S4U task.

.DESCRIPTION
  Removes \GrokBuildRemoteAgentService (S4U task) and/or WinSW service id gbr-agent.
  Does NOT start or stop unrelated processes beyond the service/task itself.
  Does NOT delete the legacy interactive task \GrokBuildRemoteAgent unless
  -DeleteInteractiveTask is passed (requires David yes).

.PARAMETER DeleteInteractiveTask
  Also unregister legacy \GrokBuildRemoteAgent. Default: off.
  Only use after David explicitly approves deleting the old interactive task.

.PARAMETER ClearHaltEnv
  Remove User-scope GBR_INJECT_HALT (leaves GBR_LOG_DIR unless -ClearLogEnv).

.PARAMETER ClearLogEnv
  Remove User-scope GBR_LOG_DIR.

.NOTES
  No secrets. Does not drive approval UI.
#>
[CmdletBinding()]
param(
  [switch]$DeleteInteractiveTask,
  [switch]$ClearHaltEnv,
  [switch]$ClearLogEnv
)

$ErrorActionPreference = "Stop"
$TaskServiceName = "GrokBuildRemoteAgentService"
$LegacyInteractiveTask = "GrokBuildRemoteAgent"
$InstallDir = Join-Path $env:LOCALAPPDATA "GrokBuildRemote"

Write-Host "Grok Build Remote — non-interactive Windows uninstall" -ForegroundColor Green

# WinSW
$winsw = Join-Path $InstallDir "gbr-agent-service.exe"
if (Test-Path -LiteralPath $winsw) {
  Write-Host "Stopping/uninstalling WinSW service..."
  & $winsw stop 2>$null | Out-Null
  & $winsw uninstall 2>$null | Out-Null
  Write-Host "WinSW service removed (if it was installed)." -ForegroundColor Cyan
} else {
  Write-Host "No WinSW exe at $winsw — skip WinSW uninstall." -ForegroundColor DarkGray
}

# Non-interactive task
$null = schtasks.exe /Query /TN $TaskServiceName 2>&1
if ($LASTEXITCODE -eq 0) {
  schtasks.exe /End /TN $TaskServiceName 2>$null | Out-Null
  schtasks.exe /Delete /TN $TaskServiceName /F | Out-Null
  Write-Host "Deleted task \$TaskServiceName." -ForegroundColor Cyan
} else {
  Write-Host "Task \$TaskServiceName not present." -ForegroundColor DarkGray
}

# Legacy interactive — disable stays; delete only with explicit switch
$null = schtasks.exe /Query /TN $LegacyInteractiveTask 2>&1
if ($LASTEXITCODE -eq 0) {
  if ($DeleteInteractiveTask) {
    schtasks.exe /End /TN $LegacyInteractiveTask 2>$null | Out-Null
    schtasks.exe /Delete /TN $LegacyInteractiveTask /F | Out-Null
    Write-Host "Deleted legacy interactive task \$LegacyInteractiveTask (-DeleteInteractiveTask)." -ForegroundColor Yellow
  } else {
    Write-Host "Left legacy task \$LegacyInteractiveTask in place (pass -DeleteInteractiveTask only with David yes)." -ForegroundColor Yellow
  }
}

if ($ClearHaltEnv) {
  [Environment]::SetEnvironmentVariable("GBR_INJECT_HALT", $null, "User")
  Write-Host "Cleared User env GBR_INJECT_HALT." -ForegroundColor Cyan
}
if ($ClearLogEnv) {
  [Environment]::SetEnvironmentVariable("GBR_LOG_DIR", $null, "User")
  Write-Host "Cleared User env GBR_LOG_DIR." -ForegroundColor Cyan
}

Write-Host "Done. Binary under $InstallDir was not removed." -ForegroundColor Green
