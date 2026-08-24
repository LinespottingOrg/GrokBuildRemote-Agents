# AGENTS.md — Build Remote Agent (desktop `gbr-agent`)

This file is for **coding agents and support AIs**. Read it before installing, pairing, or debugging.

| | |
|--|--|
| **Product** | Build Remote Agent |
| **This repo** | Desktop agent only · MIT · https://github.com/LinespottingOrg/GrokBuildRemote-Agents |
| **Website** | https://grokbuildremote.com/ |
| **Integrations** | https://grokbuildremote.com/integrations.html · [COMPATIBILITY.md](COMPATIBILITY.md) · [plugins/README.md](plugins/README.md) |
| **Phone sees** | **Terminal windows** on the paired PC (machine mailbox). Not headless OpenCode/CodeNomad. [docs/WHAT-THE-PHONE-SEES.md](docs/WHAT-THE-PHONE-SEES.md) |
| **Install** | [INSTALL.md](INSTALL.md) · pin GitHub Release + SHA-256 — [docs/PINNED-INSTALL.md](docs/PINNED-INSTALL.md) |
| **Support** | https://grokbuildremote.com/support |
| **Relay (production)** | `https://gbr-relay.ekobrott.workers.dev` · proto `gbr/1` · expect `/health` `version: 0.6.0` · Bot API `/v1/mb/:id/bot` |
| **Current agent** | **v0.6.2** pin (device classes · Bot API `127.0.0.1:8788` + `gbr-mcp` · inbox watch `boss-steer`). Leave v0.6.0 / v0.6.1 frozen. |
| **Mobile apps** | Separate private product. iOS/Android store name **Build Remote Agent**. Not in this repo. |

Do **not** invent a second code root. Official agent source is this GitHub repo (or the Dropbox workfolder `APPAR/Grok Build Remote/agents/` for LineSpotting machines).

**Attach (OpenClaw / Hermes / NemoClaw):** one surface — Bot API `:8788` (REST) **or** stdio `gbr-mcp` (13 tools). Do not register `:8788` as MCP. Skill: [skills/openclaw/SKILL.md](skills/openclaw/SKILL.md). Pair stays `gbr-agent pair` (QR **or** 8-char) then `run`. NemoClaw is a **sandbox**, not a fourth pair. Inbox comments (`gh`, label `boss-steer`) inject when a Grok Build title matches — **do not paste** after the watcher is running (`GBR_INBOX_WATCH=0` to disable). `/rename TITLE` must be its **own submitted TUI line**.

---

## Install (do this first)

Prefer a **pinned** GitHub Release — checksum the installer, then the binary.
Live `curl | bash` of grokbuildremote.com is mutable. Recipe: [docs/PINNED-INSTALL.md](docs/PINNED-INSTALL.md).

```bash
# macOS / Linux
VER=v0.6.2
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
SHA=f91ce49afbc21ac51ccf8b69b95ee407ff2d8a60926e2868bb192bb03eca796d
curl -fsSL -o /tmp/gbr-install.sh "$BASE/install.sh"
echo "$SHA  /tmp/gbr-install.sh" | shasum -a 256 -c -
bash /tmp/gbr-install.sh
gbr-agent version    # must print v0.6.2
```

From this repo (pin the tag):

```bash
git clone --branch v0.6.2 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents
make build          # → dist/gbr-agent
# or
VERSION=v0.6.2 ./scripts/build-all.sh
```

GitHub Releases: tagged `v*`. Pin **v0.6.2** (immutable). Do not mutate v0.6.0 or v0.6.1. Short path for AIs: [INSTALL.md](INSTALL.md) (install, pair, `mcp add`, Bot API `:8788`).

---

## Pair + run (happy path)

```text
1. Phone: open Build Remote Agent → Connect
2. PC:    gbr-agent pair          # browser QR
3. Phone: Scan QR (or type/paste the pair code)
4. PC:    gbr-agent run           # keep this + the CLI window open
5. Phone: sessions appear. Unpair in Settings if you change PC / mailbox.
```

Legacy: `gbr-agent pair -code CODE`.

**Fleet remote (no phone on this machine):**

```text
# On the Mac / Linux box that the hub will drive:
gbr-agent pair-as-mailbox -name mac
gbr-agent service install

# On the hub (Windows), after the remote is paired:
gbr-agent fleet add -name mac -os darwin
# GET /v1/status?device=mac is 404 until this add — that is correct.
```

`pair-as-mailbox` does not print pairing codes or keys. The hub command reads a local offer file written by the remote (Dropbox `_ops/fleet-offers/` on LineSpotting machines, or `~/.gbr/offers/`). Never paste the key into chat, commits, or README.

Relay is **outbound HTTPS 443 only**. No inbound ports. No VPN required.

```bash
gbr-agent netcheck
gbr-agent netcheck -doc
```

See [NETWORK.md](NETWORK.md). Self-host: [SELF-HOSTED-RELAY.md](SELF-HOSTED-RELAY.md).

---

## Debug commands (copy in order)

```bash
gbr-agent version
gbr-agent status
gbr-agent sessions
gbr-agent bot                  # localhost + relay Bot API curl examples
gbr-agent netcheck
gbr-agent support-log          # zip-friendly dump for info@linespotting.com
./scripts/gbr-diag.sh          # status + hops
./scripts/gbr-diag.sh faults
curl -sS https://gbr-relay.ekobrott.workers.dev/health
curl -sS https://gbr-relay.ekobrott.workers.dev/v1/bot
curl -sS http://127.0.0.1:8788/   # while gbr-agent run
```

Logs: `~/.gbr/logs/agent-YYYY-MM-DD.jsonl`  
Pair state: `~/.gbr/device.json` (`mailbox_conversation_id`, `mailbox_key`)  
Session labels: `~/.gbr/sessions.json`

**Never paste `mailbox_key` or `device.json` into a public issue.** Redact keys. `gbr-agent support-log` already summarizes key length.

---

## MCP add (`gbr-mcp` · 13 tools)

Same host as `gbr-agent run`. No npm package. `:8788` is Bot API REST — do not `mcp add` that URL. Recipes: [mcp/gbr-mcp/INSTALL.md](mcp/gbr-mcp/INSTALL.md). `gbr_open` spawns **Grok Build CLI** (`grok`).

```bash
git clone --branch v0.6.1 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
bash GrokBuildRemote-Agents/scripts/setup-gbr-mcp.sh
# ABS = absolute path of bin/gbr-mcp.js
claude mcp add gbr -- node ABS          # Claude Code / Cowork
grok mcp add gbr -- node ABS            # Grok CLI
hermes mcp add gbr -- stdio -- node ABS # Hermes
# Cursor: ~/.cursor/mcp.json → { "mcpServers": { "gbr": { "command": "node", "args": ["ABS"] } } }
# OpenClaw: skills/openclaw/SKILL.md — not a fourth pair; not a core OpenClaw PR
node ABS --diagnose                     # 13 tools
```

---

## Bot API (Grok bots / HTTP)

0.5.2+. Full spec: [docs/BOT-API.md](docs/BOT-API.md) · [FAQ](FAQ.md#how-does-a-grok-bot-talk-to-the-agent).

- **Same PC:** `http://127.0.0.1:8788` while `gbr-agent run` (loopback only).
- **Remote:** `https://gbr-relay.ekobrott.workers.dev/v1/mb/{mailbox_id}/bot` with `X-GBR-Key` from phone **Settings → Bot API**.
- **One bot · many PCs (0.5.3+):** `gbr-agent fleet add -name linux -mailbox ID -key KEY` then `POST /inject` with `"device":"linux"`. Phone on the hub mailbox gets short `bot · linux · inject queued` lines. `gbr-agent status` lists fleet; `gbr-agent fleet add` registers remotes.
- **Chain (0.5.4+):** same JSON for Grok bot and Claude Cowork (`gbr-mcp`). `POST /v1/sessions/open` → `POST /v1/lock` → `POST /v1/inject` → `GET /v1/result?wait_ms=` → `DELETE /v1/lock`. Do not hide `unknown-*`. Do not share a window without a lease.

```
GET  /v1/devices
GET  /v1/sessions
POST /v1/sessions/open   { cwd, resume, holder, goal }
POST /v1/lock            { session_id, holder, ttl_s }
POST /v1/inject          { session_id, text, submit, wait_idle? }
GET  /v1/result          ?session_id=&wait_ms=
GET|POST /v1/tasks
GET  /v1/output?command_id=
GET  /v1/status          # includes devices[]
```

---

## Session names (why 6 windows broke)

Agent **0.5.0** only advertised the first **six** sessions. Extra terminals stole those slots; titles stayed `conhost` / `c: program`.

**0.5.1** publishes a live roster:

- `list` payload `{ sessions: [{session_id, title}], replace: true, reason: "snapshot" }`
- `heartbeat.payload.sessions[]` same shape
- `register` updates title for an existing id
- Soft max **255** (log + drop extras; never crash)

Rename — `/rename` must be its **own submitted TUI line** (not inside a paste). Alias `/title`. Inbox watcher does two submits: (1) `/rename TITLE` (2) the body.

```text
# In Grok Build TUI (not an agent flag) — submit this line alone:
/rename Phone Grok

# On the PC:
gbr-agent sessions
gbr-agent rename -session grok-build-40a22 -name "Phone Grok"
```

Full story: [SESSION-NAMES.md](SESSION-NAMES.md).

After upgrade: **Settings → Unpair** on the phone, then scan again. Force-close is **not** enough.

---

## Phone buttons (tell the user this)

| Control | What it does |
|---------|----------------|
| **Disconnect** | Pause live updates. Pairing stays. |
| **Unpair / Forget this PC** | Drop mailbox + key + cached sessions. **Keep** Relay URL. Back to Scan QR. |
| **Clear chat** | History only. Pairing stays. |
| System **Clear data** | Nuclear — also wipes Relay URL. Not required when Unpair exists. |

Shipped in mobile **1.3.0** (Play vc22 live; iOS 1.3.0 b20 in App Review). Older store builds need system Clear data.

---

## Common failures

| Symptom | Cause | Fix |
|---------|--------|-----|
| `gbr-agent version` is 0.5.0 or older | Stale binary | Re-run website install.sh / install.ps1 |
| Phone shows 6 identical / stale names | Old agent or app not applying `list.replace` | Agent 0.5.1+ and app 1.3.0+; Unpair + re-pair |
| Waiting for sessions forever | Wrong mailbox / no `run` / 401 key | `gbr-agent run` in a window; Unpair + `pair` again |
| 401 `mailbox_key` / `unauthorized` | Agent paired without storing key | Upgrade to 0.4.1+ (0.5.1); wipe `~/.gbr` only if Unpair+re-pair fails |
| Empty sessions on Windows | Many terminals collapsed; old agent | 0.5.1+; `gbr-agent sessions`; close extras |
| Netcheck fail | Firewall / TLS intercept | Outbound 443 to `gbr-relay.ekobrott.workers.dev` |
| Two agents fighting | Two processes same mailbox | One `gbr-agent run` per mailbox; `service status` |

More: [TROUBLESHOOTING.md](TROUBLESHOOTING.md) · https://grokbuildremote.com/support

---

## Repo map (for code changes)

| Path | Role |
|------|------|
| `cmd/gbr-agent/` | CLI: pair, run, sessions, rename, support-log |
| `internal/session/` | Discover, titles, roster, 255 cap |
| `internal/inject/` | Type into terminals (Win / macOS / Linux) |
| `internal/relay/` | HTTPS client (`X-GBR-Key`) |
| `relay/src/index.js` | Cloudflare Worker (production is deployed separately) |
| `protocol/v1.md` | Wire spec `gbr/1` |
| `scripts/gbr-diag.sh` | Support dump |

Do not commit `relay/wrangler.toml`, `~/.gbr/`, pairing codes, or keys.

---

## Docs index

| File | Use |
|------|-----|
| [INSTALL.md](INSTALL.md) | AIs first: install, pair, `mcp add`, Bot API `:8788` |
| [README.md](README.md) | Humans: what it is, install |
| **This file (`AGENTS.md`)** | AIs: install + debug first |
| [mcp/gbr-mcp/INSTALL.md](mcp/gbr-mcp/INSTALL.md) | Claude / Grok CLI / Cursor / Hermes / OpenClaw |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | Symptom → command → fix |
| [SESSION-NAMES.md](SESSION-NAMES.md) | Titles, `/rename`, 6-cap, 255 |
| [NETWORK.md](NETWORK.md) | Ports, netcheck, no VPN |
| [SELF-HOSTED-RELAY.md](SELF-HOSTED-RELAY.md) | Own Worker |
| [protocol/v1.md](protocol/v1.md) | Envelopes |
| [APP-TODO.md](APP-TODO.md) | Mobile follow-ups |
| [FAQ.md](FAQ.md) | Same FAQ as grokbuildremote.com |
| [llms.txt](llms.txt) | Short machine summary |
