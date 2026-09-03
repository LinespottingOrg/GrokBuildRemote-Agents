#Requires -Version 5.1
<#
.SYNOPSIS
  List (default) or stop extra gbr-agent.exe processes. Keep at most one.

.DESCRIPTION
  Inbox #123 / #91 — one agent only. Interactive-only duplicates stack Grok approval cards.

  Default is list-only. Pass -Kill to stop extras (keeps the LocalAppData 0.6.3+ binary
  if it is running; otherwise keeps the oldest remaining PID).

  Does NOT start gbr-agent. Does NOT clear GBR_INJECT_HALT. No secrets.

.PARAMETER Kill
  Stop extra gbr-agent.exe processes after listing. Default: off (list only).
#>
[CmdletBinding(SupportsShouldProcess = $true)]
param(
  [switch]$Kill
)

$ErrorActionPreference = "Stop"
$prefer = Join-Path $env:LOCALAPPDATA "GrokBuildRemote\gbr-agent.exe"

$procs = Get-CimInstance Win32_Process -Filter "Name='gbr-agent.exe'" |
  Sort-Object ProcessId |
  ForEach-Object {
    [pscustomobject]@{
      Pid     = $_.ProcessId
      Path    = $_.ExecutablePath
      Command = $_.CommandLine
      Started = $_.CreationDate
    }
  }

Write-Host ("gbr-agent.exe count: {0}" -f @($procs).Count)
$procs | Format-Table -AutoSize | Out-String | Write-Host

if (@($procs).Count -le 1) {
  Write-Host "No duplicates."
  return
}

if (-not $Kill) {
  Write-Host "List only. Re-run with -Kill to stop extras (keeps at most one). Does not start the agent." -ForegroundColor Yellow
  return
}

$keep = $procs | Where-Object { $_.Path -and ($_.Path -ieq $prefer) } | Select-Object -First 1
if (-not $keep) { $keep = $procs | Select-Object -First 1 }

foreach ($p in $procs) {
  if ($p.Pid -eq $keep.Pid) { continue }
  if ($PSCmdlet.ShouldProcess("pid $($p.Pid) $($p.Path)", "Stop-Process")) {
    Stop-Process -Id $p.Pid -Force -ErrorAction Stop
    Write-Host ("Stopped pid {0} ({1})" -f $p.Pid, $p.Path) -ForegroundColor Yellow
  }
}
Write-Host ("Kept pid {0} ({1})" -f $keep.Pid, $keep.Path) -ForegroundColor Green
Write-Host "Did not start gbr-agent. Did not clear GBR_INJECT_HALT."
