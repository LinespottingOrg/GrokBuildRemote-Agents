# Troubleshooting — `gbr-agent` + Build Remote Agent

For AIs: start with [AGENTS.md](AGENTS.md). For humans: https://grokbuildremote.com/support

## 1. Confirm versions

```bash
gbr-agent version
# expect: gbr-agent v0.6.2 …

curl -sS https://gbr-relay.ekobrott.workers.dev/health
# expect: "version":"0.5.4"  "bot":true
```

Phone: Settings or store listing. Roster + Unpair + Bot API need mobile **1.3.1+**.

If the agent is older:

```bash
# pin the installer — see docs/PINNED-INSTALL.md
VER=v0.6.2
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
SHA=f91ce49afbc21ac51ccf8b69b95ee407ff2d8a60926e2868bb192bb03eca796d
curl -fsSL -o /tmp/gbr-install.sh "$BASE/install.sh"
echo "$SHA  /tmp/gbr-install.sh" | shasum -a 256 -c -
bash /tmp/gbr-install.sh
```

## 2. Collect a support dump

```bash
gbr-agent status
gbr-agent sessions
gbr-agent bot
gbr-agent netcheck
gbr-agent support-log
./scripts/gbr-diag.sh
curl -sS http://127.0.0.1:8788/          # while run is up
curl -sS https://gbr-relay.ekobrott.workers.dev/v1/bot
```

Send the support-log to **info@linespotting.com**. Redact `mailbox_key`.

## 3. Symptom table

### Phone stuck on “Waiting for sessions”

1. On the PC, is `gbr-agent run` actually running? Keep the CLI window open too.
2. `gbr-agent sessions` — empty means the agent sees no terminals.
3. Unpair on the phone → `gbr-agent pair` → scan again (new mailbox).
4. `gbr-agent status` — mailbox id on phone and PC must match (`gbr-` + code).

### Session names frozen / only six / `conhost`

That is the **0.5.0 six-session cap**. Install **0.5.1**, Unpair, re-pair.  
Rename: `/rename My title` in Grok Build, or `gbr-agent rename -session ID -name "My title"`.  
See [SESSION-NAMES.md](SESSION-NAMES.md).

### 401 / `mailbox_key` missing

Pair again with 0.5.1+. Do not keep a 0.3.x agent against an enforcing relay.

```bash
# last resort (also forgets device name):
# move ~/.gbr aside, then pair again
```

On the phone use **Unpair**, not only force-close.

### Pair QR / 401 on scan

Phone and agent must use the **same relay URL**. Default production:

`https://gbr-relay.ekobrott.workers.dev`

Self-host: set `GBR_RELAY_URL` on the PC **and** Settings → Relay on the phone, then Unpair + pair.

### Inject does nothing

```bash
gbr-agent sessions          # pick the grok-build-… or the real terminal
gbr-agent netcheck
./scripts/gbr-diag.sh probe
```

On Windows, focus the target window. One agent per mailbox.

### Two agents / “already running”

```bash
gbr-agent service status
# stop extras; only one run per mailbox
```

Stale **0.5.0** on `%LOCALAPPDATA%\GrokBuildRemote` plus a newer `dist` binary both polling the same mailbox will double-inject. `gbr-agent version` on every PATH copy; keep one `run`.

### Grok / GBR approval cards looping on Windows

Same inject was being re-typed every poll (2–5s) when handle/capture failed — relay `PollOverlap` is 30s and used to skip ack on error. Agent now acks anyway and refuses a replay of the same `command_id`.

Kill-switch (one inject cannot open N cards):

```bash
# refuse every inject
set GBR_INJECT_HALT=1
# or: gbr-agent run -inject-halt

# cap injects per session / 2 minutes
set GBR_INJECT_MAX=1

# refuse agent-spawned grok consoles
set GBR_NO_AUTO_OPEN=1
```

`GET /v1/result` sets `retry: false` on timeout / splash / quiet-without-prompt. Do not re-open + re-inject that command.

### Mac Mini clone root / GBR logs (inbox #119)

Product builds: `/Users/user/Developer/<slug>` only. Not Dropbox. Not `~/pc-build`.

| | Path |
|--|------|
| LaunchAgent | `com.linespotting.grok-build-remote` |
| WorkingDirectory / `GBR_OPEN_CWD` | `~/Developer` |
| Logs `GBR_LOG_DIR` | `~/Developer/gbr-agent-out` |
| Inbox watch (NI) | `GBR_INBOX_WATCH=0` unless David enables it |

Installer: `scripts/darwin/install-service.sh`. Do not point logs at `~/pc-build`. PC1 logs stay `C:\pc-build\gbr-agent-out`.

### Firewall

Outbound **HTTPS 443** to the relay host only. No inbound ports.  
`gbr-agent netcheck -doc` · [NETWORK.md](NETWORK.md).


### Windows: discover `grok_build=0` / session isolation

**Symptom:** discover logs `windows=0 grok_build=0` while `grok.exe` windows are open.

**Cause:** `gbr-agent` is in Windows **session 0** (S4U / service-style task such as `\GrokBuildRemoteAgentService`) while Grok Build lives in the interactive **session 1**. Session 0 cannot `EnumWindows` the interactive desktop.

**Verify:**

```powershell
Get-Process gbr-agent,grok -ErrorAction SilentlyContinue |
  Format-Table Id,ProcessName,SessionId -AutoSize
curl http://127.0.0.1:8788/v1/status
Get-Content $env:GBR_LOG_DIR\agent-$(Get-Date -Format yyyy-MM-dd).jsonl -Tail 80 |
  Select-String 'discover|session_mismatch|session_ok|SESSION-ISOLATION'
```

On PC1, `GBR_LOG_DIR=C:\pc-build\gbr-agent-out`. Healthy: same `SessionId` for agent and grok (typically `1`), and discover `grok_build` > 0 when Grok windows exist.

**Fix:**

1. Disable the session-0 task (`\GrokBuildRemoteAgentService`). Do **not** re-enable S4U for window discovery.
2. Auto-start `gbr-agent` at logon **in the interactive session** (Startup shortcut or `schtasks` `/IT` / "Run only when user is logged on").
3. Args: `-log=info run`. Env: `GBR_INJECT_HALT=0` `GBR_INBOX_WATCH=1` `GBR_NO_AUTO_OPEN=0` `GBR_LOG_DIR=C:\pc-build\gbr-agent-out`.
4. Optional watcher: `scripts/windows/session-watch.ps1` (writes `session_mismatch` / `session_ok` into the daily jsonl). Apply helper: `scripts/windows/apply-session-isolation-pc1.ps1`.

Full write-up: [docs/SESSION-ISOLATION.md](docs/SESSION-ISOLATION.md). Homepage / support FAQ: [Why does discover show grok_build=0 / windows=0 on Windows even though Grok Build is open?](FAQ.md#why-does-discover-show-grok_build0-windows0-on-windows-even-though-grok-build-is-open). WinSW / NI service install that forces session 0 remains **on HOLD** for discover hosts — see `scripts/windows/README.md`.

**Grep keys:** `session_mismatch` · `agent.session` · `agent.session_watch` · `grok_build=0` · `SESSION-ISOLATION` · `session_ok`

## 4. Phone: Unpair vs Disconnect vs Clear data

| | Keeps pairing | Keeps Relay URL | Use when |
|--|---------------|-----------------|----------|
| Disconnect | yes | yes | Pause only |
| **Unpair / Forget this PC** | **no** | **yes** | New PC, new mailbox, after agent upgrade |
| Clear chat | yes | yes | Wipe transcript |
| System Clear data | no | **no** | Last resort on old app builds |

## 5. Relay health

```bash
curl -sS https://gbr-relay.ekobrott.workers.dev/health
```

`auth_mode: enforce` is expected. Clients must send `X-GBR-Key` from pair.

## 6. Still stuck

1. https://grokbuildremote.com/support  
2. Email **info@linespotting.com** with `gbr-agent version`, OS, `support-log`, and whether Unpair + re-pair was tried.  
3. Open a GitHub issue on this repo for **agent** bugs (not App Store / Play review).
