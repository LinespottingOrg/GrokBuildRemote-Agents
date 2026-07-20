<#
  Grok Build Remote — relay regression + trace test
  Proves the v0.4.0 trace additions did not alter push/poll/pair/ack behaviour.
  Usage:  cd C:\Users\User\.aiprojects\gbr\qa ; .\run-relay-regression.ps1
#>
param(
  [string]$Relay = "https://gbr-relay.ekobrott.workers.dev",
  [string]$Mailbox = "gbr-regress01"
)

$ErrorActionPreference = "Stop"
$script:pass = 0
$script:fail = 0

function Check($name, $cond, $detail = "") {
  if ($cond) {
    Write-Host ("  PASS  {0}  {1}" -f $name, $detail) -ForegroundColor Green
    $script:pass++
  } else {
    Write-Host ("  FAIL  {0}  {1}" -f $name, $detail) -ForegroundColor Red
    $script:fail++
  }
}

# Mailbox secret, captured from /pair. The relay enforces X-GBR-Key on
# push/poll/ack, so every call after pairing must carry it.
$script:Key = ""

function AuthHeaders() {
  if ($script:Key) { return @{ "X-GBR-Key" = $script:Key } }
  return @{}
}

function PostJson($url, $obj) {
  return Invoke-RestMethod -Uri $url -Method Post -ContentType "application/json" `
    -Headers (AuthHeaders) -Body ($obj | ConvertTo-Json -Depth 10 -Compress)
}

function GetJson($url) {
  return Invoke-RestMethod -Uri $url -Headers (AuthHeaders)
}

function DeleteJson($url) {
  return Invoke-RestMethod -Uri $url -Method Delete -Headers (AuthHeaders)
}

Write-Host "=== GBR relay regression ===" -ForegroundColor Cyan
Write-Host "relay=$Relay mailbox=$Mailbox"

# clean slate
try { DeleteJson "$Relay/v1/mb/$Mailbox/trace" | Out-Null } catch {}

# 1 health
$h = Invoke-RestMethod -Uri "$Relay/health"
Check "health.ok"      ($h.ok -eq $true)          "service=$($h.service)"
Check "health.proto"   ($h.proto -eq "gbr/1")     "proto=$($h.proto)"
# Assert shape, not a pinned number — capability flags below are the real check.
Check "health.version" ($h.version -match '^\d+\.\d+\.\d+$') "version=$($h.version)"
Check "health.trace"   ($h.trace -eq $true)       "trace=$($h.trace)"

# 2 pair (original behaviour)
$p = PostJson "$Relay/v1/mb/$Mailbox/pair" @{ pairing_code="REGRESS01"; device_id="dev-pc"; device_name="RegressPC" }
Check "pair.ok"        ($p.ok -eq $true)                 "mailbox=$($p.mailbox_id)"
Check "pair.mailbox"   ($p.mailbox_id -eq $Mailbox)      ""
Check "pair.device"    ($p.device_id -eq "dev-pc")       ""

# Capture the mailbox secret — everything after this point is authenticated.
$script:Key = $p.mailbox_key
Check "pair.key_issued" ([bool]$script:Key) "key len=$($script:Key.Length)"

# 2b pair rejects short code
$badCode = $false
try { PostJson "$Relay/v1/mb/$Mailbox/pair" @{ pairing_code="AB"; device_id="x" } | Out-Null }
catch { $badCode = $true }
Check "pair.short_code_400" $badCode "short code rejected"

# 3 push inject
$cmd = [guid]::NewGuid().ToString()
$inject = @{
  proto="gbr/1"; type="inject"; device_id="dev-phone"; session_id="regress-session";
  command_id=$cmd; ts=(Get-Date).ToUniversalTime().ToString("o")
  payload=@{ mode="text"; text="echo regression"; submit=$true }
}
$r = PostJson "$Relay/v1/mb/$Mailbox/push" $inject
Check "push.inject.ok"  ($r.ok -eq $true) "size=$($r.size)"

# 3b invalid envelope rejected
$badEnv = $false
try { PostJson "$Relay/v1/mb/$Mailbox/push" @{ nope=$true } | Out-Null } catch { $badEnv = $true }
Check "push.invalid_400" $badEnv "invalid envelope rejected"

# 3c CLOCK SKEW: an envelope whose client ts is older than the poll cursor must
# still be delivered. Regression for a silently-dropped inject observed live.
$skewCmd = [guid]::NewGuid().ToString()
$cursorNow = (Get-Date).ToUniversalTime().ToString("o")
Start-Sleep -Milliseconds 500
$staleEnv = @{
  proto="gbr/1"; type="inject"; device_id="dev-phone"; session_id="regress-session";
  command_id=$skewCmd
  ts=(Get-Date).ToUniversalTime().AddMinutes(-5).ToString("o")   # phone clock behind
  payload=@{ mode="text"; text="echo skew"; submit=$true }
}
PostJson "$Relay/v1/mb/$Mailbox/push" $staleEnv | Out-Null
$skewPoll = GetJson "$Relay/v1/mb/$Mailbox/poll?role=agent&for=dev-pc&after=$([uri]::EscapeDataString($cursorNow))"
$skewSeen = @($skewPoll.envelopes | Where-Object { $_.command_id -eq $skewCmd }).Count
Check "push.stale_client_ts_delivered" ($skewSeen -eq 1) "stale-ts inject delivered=$skewSeen"
PostJson "$Relay/v1/mb/$Mailbox/ack" @{ command_ids=@($skewCmd) } | Out-Null

# 4 role routing: agent sees inject, mobile does not
$agentPoll  = GetJson "$Relay/v1/mb/$Mailbox/poll?role=agent&for=dev-pc"
$mobilePoll = GetJson "$Relay/v1/mb/$Mailbox/poll?role=mobile&for=dev-phone"
$agentSees  = @($agentPoll.envelopes  | Where-Object { $_.command_id -eq $cmd }).Count
$mobileSees = @($mobilePoll.envelopes | Where-Object { $_.command_id -eq $cmd }).Count
Check "poll.agent_gets_inject"      ($agentSees  -eq 1) "count=$agentSees"
Check "poll.mobile_skips_inject"    ($mobileSees -eq 0) "count=$mobileSees"

# 5 push output (agent -> mobile)
$out = @{
  proto="gbr/1"; type="output"; device_id="dev-pc"; session_id="regress-session";
  command_id=$cmd; ts=(Get-Date).ToUniversalTime().ToString("o")
  payload=@{ stream="stdout"; chunk="regression ok"; eof=$true }
}
PostJson "$Relay/v1/mb/$Mailbox/push" $out | Out-Null
$mobilePoll2 = GetJson "$Relay/v1/mb/$Mailbox/poll?role=mobile&for=dev-phone"
$gotOut = @($mobilePoll2.envelopes | Where-Object { $_.type -eq "output" -and $_.command_id -eq $cmd }).Count
Check "poll.mobile_gets_output" ($gotOut -eq 1) "count=$gotOut"

# 5b agent must NOT receive output/heartbeat/register
$agentPoll2 = GetJson "$Relay/v1/mb/$Mailbox/poll?role=agent&for=dev-pc"
$agentOut = @($agentPoll2.envelopes | Where-Object { $_.type -eq "output" }).Count
Check "poll.agent_skips_output" ($agentOut -eq 0) "count=$agentOut"

# 6 ack removes inject
$a = PostJson "$Relay/v1/mb/$Mailbox/ack" @{ command_ids=@($cmd) }
Check "ack.ok" ($a.ok -eq $true) "removed=$($a.removed)"
$agentPoll3 = GetJson "$Relay/v1/mb/$Mailbox/poll?role=agent&for=dev-pc"
$stillThere = @($agentPoll3.envelopes | Where-Object { $_.command_id -eq $cmd -and $_.type -eq "inject" }).Count
Check "ack.inject_dropped" ($stillThere -eq 0) "remaining=$stillThere"

# 7 unknown route still 404
$got404 = $false
try { GetJson "$Relay/v1/mb/$Mailbox/bogus" | Out-Null } catch { $got404 = $true }
Check "route.unknown_404" $got404 "bogus action rejected"

# 7b PUSH/ACK RACE: acking one command must never erase a concurrently pushed one.
# Regression for the blocker found on 2026-07-19 — injects were pushed, traced,
# then vanished from the queue with no agent hop and no error. Root cause: push
# and ack were both KV read-modify-write on the same key, so an ack writing back
# a pre-push snapshot silently dropped the new envelope.
$raceIds = 1..6 | ForEach-Object { [guid]::NewGuid().ToString() }
# Seed 6 injects, then concurrently ack the first 3 while pushing 3 more.
foreach ($id in $raceIds) {
  PostJson "$Relay/v1/mb/$Mailbox/push" @{
    proto="gbr/1"; type="inject"; device_id="dev-phone"; session_id="race";
    command_id=$id; payload=@{ mode="text"; text="seed"; submit=$true }
  } | Out-Null
}
$newIds = 1..6 | ForEach-Object { [guid]::NewGuid().ToString() }
$raceJobs = @()
$raceJobs += 0..2 | ForEach-Object {
  Start-Job -ScriptBlock {
    param($relay,$mb,$id,$key)
    Invoke-RestMethod -Uri "$relay/v1/mb/$mb/ack" -Method Post -ContentType "application/json" `
      -Headers @{ "X-GBR-Key" = $key } `
      -Body (@{ command_ids=@($id) } | ConvertTo-Json -Compress) | Out-Null
  } -ArgumentList $Relay, $Mailbox, $raceIds[$_], $script:Key
}
$raceJobs += $newIds | ForEach-Object {
  Start-Job -ScriptBlock {
    param($relay,$mb,$id,$key)
    Invoke-RestMethod -Uri "$relay/v1/mb/$mb/push" -Method Post -ContentType "application/json" `
      -Headers @{ "X-GBR-Key" = $key } `
      -Body (@{ proto="gbr/1"; type="inject"; device_id="dev-phone"; session_id="race";
                command_id=$id; payload=@{ mode="text"; text="concurrent"; submit=$true }
              } | ConvertTo-Json -Depth 10 -Compress) | Out-Null
  } -ArgumentList $Relay, $Mailbox, $_, $script:Key
}
$raceJobs | Wait-Job -Timeout 90 | Out-Null
$raceJobs | Remove-Job -Force
Start-Sleep -Seconds 2
$qAfter = GetJson "$Relay/v1/mb/$Mailbox/poll?role=agent"
$survived = @($newIds | Where-Object { $id = $_; @($qAfter.envelopes | Where-Object { $_.command_id -eq $id }).Count -gt 0 }).Count
Check "queue.push_ack_no_loss" ($survived -eq 6) "$survived/6 concurrently-pushed injects survived 3 parallel acks"
PostJson "$Relay/v1/mb/$Mailbox/ack" @{ command_ids=($raceIds + $newIds) } | Out-Null

# 7c SELF-ACK: an ack must not delete the acker's OWN reply.
# The agent answers a `list` reusing the request's command_id, then acks it.
# A blind ack-by-command_id deleted that reply — invisible while the KV ack
# raced, deterministic once the DO queue made acks reliable.
$listCmd = [guid]::NewGuid().ToString()
$agentDev = "dev-agent-selfack"
PostJson "$Relay/v1/mb/$Mailbox/push" @{
  proto="gbr/1"; type="list"; device_id="dev-phone"; command_id=$listCmd; payload=@{}
} | Out-Null
PostJson "$Relay/v1/mb/$Mailbox/push" @{
  proto="gbr/1"; type="list"; device_id=$agentDev; command_id=$listCmd
  payload=@{ sessions=@(@{ session_id="qa-selfack" }) }
} | Out-Null
PostJson "$Relay/v1/mb/$Mailbox/ack" @{ command_ids=@($listCmd); from_device=$agentDev } | Out-Null
$sa = GetJson "$Relay/v1/mb/$Mailbox/poll?role=mobile"
$replyLeft   = @($sa.envelopes | Where-Object { $_.command_id -eq $listCmd -and $_.device_id -eq $agentDev }).Count
$agentView   = GetJson "$Relay/v1/mb/$Mailbox/poll?role=agent"
$requestLeft = @($agentView.envelopes | Where-Object { $_.command_id -eq $listCmd -and $_.device_id -eq "dev-phone" }).Count
Check "ack.keeps_own_reply"     ($replyLeft -eq 1)   "agent list reply survived=$replyLeft"
Check "ack.drops_peer_request"  ($requestLeft -eq 0) "phone list request remaining=$requestLeft"
PostJson "$Relay/v1/mb/$Mailbox/ack" @{ command_ids=@($listCmd) } | Out-Null

# 8 NEW: trace write + read
$tid = [guid]::NewGuid().ToString()
$t = PostJson "$Relay/v1/mb/$Mailbox/trace" @{
  events = @(
    @{ trace_id=$tid; hop="phone.send";  actor="android"; type="inject"; command_id=$tid; detail="unit test" },
    @{ trace_id=$tid; hop="agent.recv";  actor="agent";   type="inject"; command_id=$tid; ms=42 }
  )
}
Check "trace.push.ok"    ($t.ok -eq $true)   "added=$($t.added) size=$($t.size)"
$tr = GetJson "$Relay/v1/mb/$Mailbox/trace?limit=200"
$mine = @($tr.events | Where-Object { $_.trace_id -eq $tid })
Check "trace.read.count" ($mine.Count -ge 2) "found=$($mine.Count)"
Check "trace.normalized" ($null -ne ($mine | Where-Object { $_.hop -eq "agent.recv" -and $_.ms -eq 42 })) "ms preserved"

# 8b relay self-trace recorded the push/pair hops
$relayHops = @($tr.events | Where-Object { $_.actor -eq "relay" })
Check "trace.relay_selftrace" ($relayHops.Count -ge 2) "relay hops=$($relayHops.Count)"
$hasPush = @($relayHops | Where-Object { $_.hop -eq "relay.push" }).Count
Check "trace.relay_push_hop" ($hasPush -ge 1) "relay.push=$hasPush"

# 8c filter by command_id
$byCmd = GetJson "$Relay/v1/mb/$Mailbox/trace?command_id=$tid"
Check "trace.filter_command_id" (@($byCmd.events).Count -ge 2) "count=$(@($byCmd.events).Count)"

# 8d CONCURRENCY: parallel writers must not lose events (append-only keys).
# Regression for the read-modify-write race found during the first E2E run,
# where 2 of 4 agent hops vanished from the relay while the local JSONL had all 4.
$batchTag = [guid]::NewGuid().ToString()
$writers = 1..8
$jobs = $writers | ForEach-Object {
  $i = $_
  Start-Job -ScriptBlock {
    param($relay, $mb, $tag, $idx)
    try {
      $body = @{ events = @(
        @{ trace_id=$tag; hop="race.w$idx.a"; actor="agent"; command_id=$tag; detail="writer $idx event a" },
        @{ trace_id=$tag; hop="race.w$idx.b"; actor="agent"; command_id=$tag; detail="writer $idx event b" }
      )} | ConvertTo-Json -Depth 10 -Compress
      $r = Invoke-RestMethod -Uri "$relay/v1/mb/$mb/trace" -Method Post -ContentType "application/json" -Body $body
      "OK idx=$idx added=$($r.added)"
    } catch {
      "ERR idx=$idx : $($_.Exception.Message)"
    }
  } -ArgumentList $Relay, $Mailbox, $batchTag, $i
}
$jobs | Wait-Job -Timeout 60 | Out-Null
$jobOut = $jobs | Receive-Job
$jobs | Remove-Job -Force
$jobErrs = @($jobOut | Where-Object { $_ -like "ERR*" })
Check "trace.concurrent_writers_ok" ($jobErrs.Count -eq 0) "$($jobOut.Count) writers reported, $($jobErrs.Count) errors"
Start-Sleep -Seconds 2
$raceRead = GetJson "$Relay/v1/mb/$Mailbox/trace?command_id=$batchTag&limit=400"
$raceCount = @($raceRead.events).Count
Check "trace.concurrent_no_loss" ($raceCount -eq 16) "expected 16 got $raceCount from 8 parallel writers"

# 8e AUTH: mailbox key issuance + verification (warn mode during rollout).
# The mailbox id alone used to authorise pushing an inject — i.e. keystrokes into
# the user's terminal — and it is derived from the on-screen pairing code by a
# formula published in the open-source agent. /pair now issues a real secret.
$authMb = "gbr-authtest01"
$h = Invoke-RestMethod -Uri "$Relay/health"
Check "auth.health_advertises" ($h.auth_header -eq "X-GBR-Key") "mode=$($h.auth_mode) header=$($h.auth_header)"
Check "auth.enforcing"         ($h.auth_mode -eq "enforce")      "auth_mode=$($h.auth_mode)"

$p1 = PostJson "$Relay/v1/mb/$authMb/pair" @{ pairing_code="AUTHTEST01"; device_id="dev-pc"; device_name="PC" }
Check "auth.key_issued" ($p1.mailbox_key -and $p1.mailbox_key.Length -ge 32) "len=$($p1.mailbox_key.Length)"

# Second party (phone) pairing with the same code must receive the SAME key.
$p2 = PostJson "$Relay/v1/mb/$authMb/pair" @{ pairing_code="AUTHTEST01"; device_id="dev-phone"; device_name="Phone" }
Check "auth.key_stable_across_pairers" ($p2.mailbox_key -eq $p1.mailbox_key) "phone key matches agent key"

# Correct key accepted.
$goodEnv = @{ proto="gbr/1"; type="inject"; device_id="dev-phone"; session_id="auth"
              command_id=[guid]::NewGuid().ToString(); payload=@{ mode="text"; text="echo auth"; submit=$true } }
$okPush = Invoke-RestMethod -Uri "$Relay/v1/mb/$authMb/push" -Method Post `
  -Headers @{ "X-GBR-Key" = $p1.mailbox_key } -ContentType "application/json" `
  -Body ($goodEnv | ConvertTo-Json -Depth 10 -Compress)
Check "auth.valid_key_accepted" ($okPush.ok -eq $true) "size=$($okPush.size)"

# Wrong key: traced as a rejection. In warn mode it is still ACCEPTED on purpose —
# iOS is already released and Play is in review; enforcing now would brick them.
$badRejected = $false
try {
  Invoke-RestMethod -Uri "$Relay/v1/mb/$authMb/push" -Method Post `
    -Headers @{ "X-GBR-Key" = "definitely-not-the-key" } -ContentType "application/json" `
    -Body ($goodEnv | ConvertTo-Json -Depth 10 -Compress) | Out-Null
} catch { if ($_.Exception.Response.StatusCode.value__ -eq 401) { $badRejected = $true } }
Check "auth.bad_key_rejected" $badRejected "401 on wrong key"

# No key at all — this is the original vulnerability, and it must now fail.
$noKeyRejected = $false
try {
  Invoke-RestMethod -Uri "$Relay/v1/mb/$authMb/push" -Method Post `
    -ContentType "application/json" -Body ($goodEnv | ConvertTo-Json -Depth 10 -Compress) | Out-Null
} catch { if ($_.Exception.Response.StatusCode.value__ -eq 401) { $noKeyRejected = $true } }
Check "auth.unauthenticated_push_rejected" $noKeyRejected "401 with no X-GBR-Key"

# Reading terminal output without the key must also be refused.
$noKeyPoll = $false
try { Invoke-RestMethod -Uri "$Relay/v1/mb/$authMb/poll?role=mobile" | Out-Null }
catch { if ($_.Exception.Response.StatusCode.value__ -eq 401) { $noKeyPoll = $true } }
Check "auth.unauthenticated_poll_rejected" $noKeyPoll "401 polling without key"

Start-Sleep -Seconds 2
$authTrace = GetJson "$Relay/v1/mb/$authMb/trace?limit=200"
$rejects = @($authTrace.events | Where-Object { $_.hop -eq "relay.auth_reject" }).Count
Check "auth.rejections_traced" ($rejects -ge 2) "relay.auth_reject events=$rejects"

# Pair throttling — the brute-force surface on an 8-char code.
$throttled = $false
for ($i = 0; $i -lt 16; $i++) {
  try { PostJson "$Relay/v1/mb/gbr-throttle01/pair" @{ pairing_code="THROTTLE01"; device_id="d$i" } | Out-Null }
  catch { if ($_.Exception.Response.StatusCode.value__ -eq 429) { $throttled = $true; break } }
}
Check "auth.pair_rate_limited" $throttled "429 after repeated pair attempts"

# A mailbox that has never been paired must refuse traffic entirely, or an
# attacker who guesses a code can pre-load an inject that the agent executes on
# its first poll after pairing.
$prePairBlocked = $false
try {
  Invoke-RestMethod -Uri "$Relay/v1/mb/gbr-neverpaired-$([guid]::NewGuid().ToString('N').Substring(0,6))/push" `
    -Method Post -ContentType "application/json" `
    -Body '{"proto":"gbr/1","type":"inject","payload":{"text":"pwn","submit":true}}' | Out-Null
} catch { if ($_.Exception.Response.StatusCode.value__ -eq 401) { $prePairBlocked = $true } }
Check "auth.unpaired_mailbox_refused" $prePairBlocked "401 pushing to a never-paired mailbox"

DeleteJson "$Relay/v1/mb/$authMb/trace" | Out-Null

# 9 cleanup
DeleteJson "$Relay/v1/mb/$Mailbox/trace" | Out-Null
$cleared = GetJson "$Relay/v1/mb/$Mailbox/trace"
Check "trace.clear" (@($cleared.events).Count -eq 0) "after clear=$(@($cleared.events).Count)"

Write-Host ""
$resultColor = "Green"; if ($script:fail -gt 0) { $resultColor = "Red" }
Write-Host ("RESULT: {0} passed, {1} failed" -f $script:pass, $script:fail) -ForegroundColor $resultColor
if ($script:fail -gt 0) { exit 1 }
exit 0
