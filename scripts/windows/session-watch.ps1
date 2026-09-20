#Requires -Version 5.1
<#
.SYNOPSIS
  Append session_mismatch / session_ok JSON lines to gbr-agent daily jsonl.

.DESCRIPTION
  Every ~30s: read gbr-agent and grok SessionIds; compare to console session.
  On mismatch (agent session 0, or agent != console while grok exists elsewhere,
  or agent session 0 while :8788 is up): append one session_mismatch line.
  When healthy: append session_ok about once per 5 minutes.
  No admin required. Grep: session_mismatch, session_ok, agent.session_watch, SESSION-ISOLATION
#>
[CmdletBinding()]
param(
  [string]$LogDir = $(if ($env:GBR_LOG_DIR) { $env:GBR_LOG_DIR } else { 'C:\pc-build\gbr-agent-out' }),
  [int]$IntervalSec = 30,
  [int]$OkEverySec = 300
)

$ErrorActionPreference = 'SilentlyContinue'
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null

function Get-ConsoleSessionId {
  try {
    $q = (query user 2>$null) | Where-Object { $_ -match '>' }
    if ($q -match '\s+(\d+)\s+Active') { return [int]$Matches[1] }
  } catch {}
  $ex = Get-Process explorer -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($ex) { return [int]$ex.SessionId }
  return 1
}

function Write-AgentJsonl([hashtable]$Obj) {
  $day = (Get-Date).ToString('yyyy-MM-dd')
  $path = Join-Path $LogDir ("agent-{0}.jsonl" -f $day)
  if (-not $Obj.ContainsKey('ts')) {
    $Obj['ts'] = [DateTime]::UtcNow.ToString('o').Replace('+00:00', 'Z')
    if ($Obj['ts'] -notmatch 'Z$') {
      $Obj['ts'] = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ss.fffZ')
    }
  }
  ($Obj | ConvertTo-Json -Compress -Depth 6) | Add-Content -Path $path -Encoding UTF8
}

function Test-Port8788 {
  try {
    $c = Get-NetTCPConnection -LocalPort 8788 -State Listen -ErrorAction SilentlyContinue |
      Select-Object -First 1
    return [bool]$c
  } catch {
    try {
      $tcp = New-Object System.Net.Sockets.TcpClient
      $iar = $tcp.BeginConnect('127.0.0.1', 8788, $null, $null)
      $ok = $iar.AsyncWaitHandle.WaitOne(200) -and $tcp.Connected
      $tcp.Close()
      return $ok
    } catch { return $false }
  }
}

$lastOkUtc = [DateTime]::MinValue
Write-AgentJsonl @{
  ts     = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ss.fffZ')
  hop    = 'agent.session_watch'
  actor  = 'session-watch'
  type   = 'session_watch_start'
  ok     = $true
  detail = "interval=${IntervalSec}s logdir=$LogDir SESSION-ISOLATION"
}

while ($true) {
  try {
    $console = Get-ConsoleSessionId
    $agents = @(Get-Process gbr-agent -ErrorAction SilentlyContinue)
    $groks  = @(Get-Process grok -ErrorAction SilentlyContinue)
    $portUp = Test-Port8788

    $agentSession = if ($agents.Count -gt 0) { [int]$agents[0].SessionId } else { -1 }
    $grokSessions = @($groks | Select-Object -ExpandProperty SessionId -Unique)
    $grokInOther = $false
    if ($groks.Count -gt 0 -and $agentSession -ge 0) {
      foreach ($gs in $grokSessions) {
        if ([int]$gs -ne $agentSession) { $grokInOther = $true; break }
      }
    }

    $mismatch = $false
    $reasons = New-Object System.Collections.Generic.List[string]
    if ($agentSession -eq 0) { $mismatch = $true; [void]$reasons.Add('agent_session=0') }
    if ($agentSession -ge 0 -and $agentSession -ne $console -and $groks.Count -gt 0) {
      $mismatch = $true; [void]$reasons.Add('agent_ne_console_with_grok')
    }
    if ($grokInOther) { $mismatch = $true; [void]$reasons.Add('grok_in_other_session') }
    if ($agentSession -eq 0 -and $portUp) { $mismatch = $true; [void]$reasons.Add('session0_with_8788') }

    $base = "agent_session=$agentSession console_session=$console grok_in_other_session=$($grokInOther.ToString().ToLowerInvariant()) grok_count=$($groks.Count) port8788=$($portUp.ToString().ToLowerInvariant())"

    if ($mismatch) {
      Write-AgentJsonl @{
        ts     = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ss.fffZ')
        hop    = 'agent.session_watch'
        actor  = 'session-watch'
        type   = 'session_mismatch'
        ok     = $false
        detail = "$base fix=run_agent_interactive_disable_session0_task reasons=$($reasons -join ',') SESSION-ISOLATION"
      }
    } else {
      $now = [DateTime]::UtcNow
      if (($now - $lastOkUtc).TotalSeconds -ge $OkEverySec) {
        $lastOkUtc = $now
        Write-AgentJsonl @{
          ts     = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ss.fffZ')
          hop    = 'agent.session_watch'
          actor  = 'session-watch'
          type   = 'session_ok'
          ok     = $true
          detail = "$base SESSION-ISOLATION"
        }
      }
    }
  } catch {
    Write-AgentJsonl @{
      ts     = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ss.fffZ')
      hop    = 'agent.session_watch'
      actor  = 'session-watch'
      type   = 'session_watch_error'
      ok     = $false
      detail = ("error={0} SESSION-ISOLATION" -f $_.Exception.Message)
    }
  }
  Start-Sleep -Seconds $IntervalSec
}
