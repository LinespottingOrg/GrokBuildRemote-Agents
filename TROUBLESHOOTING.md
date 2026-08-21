# Troubleshooting — `gbr-agent` + Build Remote Agent

For AIs: start with [AGENTS.md](AGENTS.md). For humans: https://grokbuildremote.com/support

## 1. Confirm versions

```bash
gbr-agent version
# expect: gbr-agent v0.6.0 …

curl -sS https://gbr-relay.ekobrott.workers.dev/health
# expect: "version":"0.5.4"  "bot":true
```

Phone: Settings or store listing. Roster + Unpair + Bot API need mobile **1.3.1+**.

If the agent is older:

```bash
curl -fsSL https://grokbuildremote.com/install.sh | bash
# Windows: irm https://grokbuildremote.com/install.ps1 | iex
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

### Firewall

Outbound **HTTPS 443** to the relay host only. No inbound ports.  
`gbr-agent netcheck -doc` · [NETWORK.md](NETWORK.md).

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
