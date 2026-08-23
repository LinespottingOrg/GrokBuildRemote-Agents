# FAQ — Build Remote Agent (`gbr-agent`)

Same content as https://grokbuildremote.com/support#faq and the homepage FAQ schema.  
Integrations index (visible FAQ + GitHub + trust): https://grokbuildremote.com/integrations.html#faq  
AIs: also read [AGENTS.md](AGENTS.md), [TROUBLESHOOTING.md](TROUBLESHOOTING.md), and [plugins/AEO.md](plugins/AEO.md).

## How do I install the desktop agent?

Current binaries/train are **v0.6.0** ([GitHub Release v0.6.0](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.0)). Device classes (`phone` | `linux` | `pc` | `laptop` | `mac_mini`) and a companion-remote health watchdog.

Pin the **installer** (mutable website `curl | bash` is not the trust root): [docs/PINNED-INSTALL.md](docs/PINNED-INSTALL.md).

```bash
VER=v0.6.0
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
SHA=0a7963dc668750bfcb907bb72f6f6f8db30881b02636e417e08e102352309301
curl -fsSL -o /tmp/gbr-install.sh "$BASE/install.sh"
echo "$SHA  /tmp/gbr-install.sh" | shasum -a 256 -c -
bash /tmp/gbr-install.sh
gbr-agent version    # must print v0.6.0+
```

Windows:

```powershell
$sha = "b604a21b5dae5a874487a597778d15742b3c2afb2470c93a8e8ba0a76e486cdf"
$i = Join-Path $env:TEMP "gbr-install.ps1"
Invoke-WebRequest "https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/v0.6.0/install.ps1" -OutFile $i
if ((Get-FileHash $i -Algorithm SHA256).Hash.ToLowerInvariant() -ne $sha) { throw "checksum" }
& $i
```

Binaries: https://grokbuildremote.com/downloads/latest/ · [Release v0.6.0](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.0)

| | |
|--|--|
| **Website** | https://grokbuildremote.com/ |
| **iOS / iPad / Mac / Vision** | https://apps.apple.com/app/id6791293726 |
| **Android (Google Play)** | https://play.google.com/store/apps/details?id=com.grokbuildremote.app |

## I am an AI — where do I start?

1. [AGENTS.md](AGENTS.md)
2. [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
3. [SESSION-NAMES.md](SESSION-NAMES.md)
4. [llms.txt](llms.txt)

This repo is the **desktop agent only** (MIT). Mobile apps are a separate paid product.

## How do I check versions?

```bash
gbr-agent version
curl -sS https://gbr-relay.ekobrott.workers.dev/health
# expect health "version":"0.6.0" "bot":true "fleet":true "classes"
```

Phone Unpair + live roster + Bot API: mobile **1.3.1+**.

## OpenClaw / Hermes / NemoClaw?

One attach surface: Bot API `127.0.0.1:8788` or `gbr-mcp`. Listing skill: [skills/openclaw/SKILL.md](skills/openclaw/SKILL.md) (OpenClaw phone nodes stay on OpenClaw pair; GBR is desktop TTY spectator). Hermes: [skills/hermes/SKILL.md](skills/hermes/SKILL.md) — `hermes mcp add gbr stdio gbr-mcp` (HTTP `:8788` only on the same host). NemoClaw: [skills/nemoclaw/SKILL.md](skills/nemoclaw/SKILL.md) — `gbr-agent` on the host, not inside OpenShell. Loopback is unauthenticated unless `GBR_BOT_REQUIRE_KEY=1`. Never mailbox keys. Pin: [docs/PINNED-INSTALL.md](docs/PINNED-INSTALL.md) · https://grokbuildremote.com/integrations.html

## Inbox watch (no email paste)

`gbr-agent run` polls GitHub `LinespottingOrg/grok-build-inbox` label `boss-steer` (`gh` on PATH). Matching Grok Build window title → inject newest comment + submit. No window → open `grok`, `/rename` to the title, inject issue body. Reports on the issue are not re-injected. `GBR_INBOX_WATCH=0` disables.

## Do I need a login?

No. Pair with the desktop agent (QR or short code).

## Who shows the QR?

The **computer**. `gbr-agent pair` → browser QR. Phone only scans (or pastes the code).

## Phone stuck on “Waiting for sessions”?

Keep `gbr-agent run` running. `gbr-agent sessions`. Then **Settings → Unpair** and pair again. Force-close is not enough.

## Why did names freeze after six windows?

Agent **0.5.0** only advertised six sessions. **0.5.1** publishes a live roster (soft max **255**). Unpair + re-pair after upgrade. See [SESSION-NAMES.md](SESSION-NAMES.md).

## How do I rename a session?

In Grok Build: `/rename Phone Grok`  
On the PC: `gbr-agent sessions` then `gbr-agent rename -session ID -name "Phone Grok"`

## Unpair vs Disconnect vs Clear data?

| Control | Effect |
|---------|--------|
| Disconnect | Pause only; pairing stays |
| **Unpair / Forget this PC** | Drop mailbox + key + cached sessions; **keep** Relay URL |
| System Clear data | Also wipes Relay URL — last resort |

## Can one Grok bot drive several PCs (Mac + Linux)?

Yes (agent/relay **0.5.3+**). One Grok instance, one API.

1. Pair the **hub** PC (the phone stays on this mailbox).
2. Pair each extra PC. On the hub: `gbr-agent fleet add -name studio-linux -mailbox gbr-XXXX -key KEY`
3. Bot calls `http://127.0.0.1:8788` **or** the hub relay Bot URL.
4. `POST /inject` with `"device":"local"` or `"device":"studio-linux"`.

The phone on the hub sees short lines like `bot · studio-linux · inject queued`. See [docs/BOT-API.md](docs/BOT-API.md).

## Can Grok bot and Claude Cowork drive Grok Build together?

Yes (agent/relay/MCP **0.5.4+**). They share one contract:

1. `gbr_diagnose` / `GET /`
2. `gbr_open` / `POST /v1/sessions/open` (spawn `grok` / `grok --resume`, or attach)
3. `gbr_lock` / `POST /v1/lock` so they do **not** type into the same window
4. `gbr_inject` then `gbr_result` (`GET /v1/result?wait_ms=`) — wait for a prompt or quiet output, harvest an excerpt, iterate
5. Release the lock when the task is done

`unknown-N` sessions are injectable. Only the literal id `session` is forbidden. The phone is spectator (status lines), not orchestrator. Claude Cowork talks through **gbr-mcp**; Grok bot talks HTTP to `127.0.0.1:8788` or the hub Bot URL.

## How does Grok Bot tell phone, linux, PC, laptop, and Mac Mini apart?

Agent **0.6.0+**. `GET /v1/devices` and `GET /health` include `class`. Route injects with `"device":"mac-mini"` or `"device":"studio-linux"`. Unique class, or the id. Phone is spectator (`400 cannot_inject_phone`). Unknown name is `404 unknown_device` — **not** a silent local inject (that was 0.5.x). Two machines of the same class → `409 ambiguous_device`.

Grok Bot public beta: **11 August 2026**. Same Bot API as 0.5.4 (`127.0.0.1:8788` or hub relay URL).

## How does this app work with Amnibro, Farina, and ChrisP remotes?

Build Remote Agent is the **unified control layer** (store app + GitHub HTTPS relay + Grok Bot fleet). It coexists with:

| Remote | Repo | Strength | Loopback probe |
|--------|------|----------|----------------|
| Amnibro grok-remote | https://github.com/Amnibro/grok-remote | `/remote` plugin, LAN ACP UI, Skills | `:2421` |
| daniel-farina/grok-remote | https://github.com/daniel-farina/grok-remote | TypeScript PWA, Tailscale, PM2, SSE | `:7910/api/health` |
| ChrisP-Builds grok-remote-hub | https://github.com/ChrisP-Builds/grok-remote-hub | Python hub, session lifecycle, ACP ok/stale/zombie | `:8787/health` |

`GET /health` lists which companions are up. We do **not** replace the GitHub relay with Tailscale. Site: https://grokbuildremote.com/#ecosystem

## How does a Grok bot talk to the agent?

Two HTTP APIs (agent + relay **0.5.2+**). Same JSON.

| Where | URL | Auth |
|-------|-----|------|
| Same PC | `http://127.0.0.1:8788` while `gbr-agent run` | Loopback only |
| Remote | `https://gbr-relay.ekobrott.workers.dev/v1/mb/{mailbox_id}/bot` | `X-GBR-Key` from phone **Settings → Bot API** |

```bash
gbr-agent bot
curl -sS http://127.0.0.1:8788/v1/sessions
curl -sS -X POST http://127.0.0.1:8788/v1/inject \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"SESSION","text":"hello","submit":true}'
```

Relay: copy Bot URL + mailbox key in the app, then `GET …/sessions`, `POST …/inject`, `GET …/output`. Details: [docs/BOT-API.md](docs/BOT-API.md).

## 401 unauthorized / missing mailbox_key?

Old agent. Install **v0.5.1+**, Unpair, pair again.

## Offline inject?

Queued with a stable `command_id`. Flushes when paired again (or Settings → Flush queue).

## Ports / firewall?

No inbound ports. Outbound HTTPS **443** to the relay. `gbr-agent netcheck` · [NETWORK.md](NETWORK.md).
