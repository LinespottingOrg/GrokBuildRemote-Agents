---
name: gbr
description: >
  Drive Grok Build CLI (grok) through Build Remote Agent from OpenClaw / Hermes / NemoClaw.
  Attach via stdio gbr-mcp. Pair the phone to spectate that grok TTY.
compatibility: Requires gbr-agent run on the host. Loopback. No mailbox keys in this file.
metadata:
  version: "0.6.1"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/use-cases/claw.html"
---

# OpenClaw / claw → Grok Build CLI

One adapter. Not 50 one-off integrations. Default target is **Grok Build CLI** (`grok`).

Independent Linespotting AB. Not affiliated with OpenClaw, Hermes, NVIDIA, xAI, or SpaceX.

## Pair (phone spectator — optional, same mailbox)

1. Pin `gbr-agent` v0.6.0 — https://grokbuildremote.com/PINNED-INSTALL.md
2. PC: `gbr-agent pair` then `gbr-agent run` (keep running).
3. Phone: Build Remote Agent → scan QR.
4. OpenClaw **device-pair** (LAN/Bonjour) is a **different** protocol. Do not mix with `gbr/1`.

Unpair on the phone before a new mailbox.

## Attach (MCP client → grok)

| How | Where |
|-----|--------|
| MCP | stdio `gbr-mcp` (`node …/bin/gbr-mcp.js`) |
| Bot API | `http://127.0.0.1:8788` after `gbr-agent run` — **REST, not MCP** |

Do **not** `hermes mcp add` / OpenClaw MCP the `:8788` URL. That is JSON REST. MCP is stdio.

```bash
export GBR_BOT_REQUIRE_KEY=1
bash scripts/setup-gbr-mcp.sh
# then:
# hermes mcp add gbr -- stdio -- node $HOME/.gbr/gbr-mcp-src/mcp/gbr-mcp/bin/gbr-mcp.js
```

Pin: `git clone --branch v0.6.0 --depth 1` if you clone by hand. Never default branch + `npm install`.

## Loop — this is what makes grok work

1. `gbr_diagnose`
2. `gbr_open` — omit session_id; `command` defaults to **grok**; set `holder=openclaw` (or `hermes` / `nemoclaw`)
3. `gbr_inject` the coding task with `submit=true` (or `gbr_inject_and_wait`)
4. `gbr_result` `wait_ms=60000`
5. `gbr_lock` `action=release` when done

If `grok` is not on PATH, `gbr_open` opens a shell and tells you to install Grok Build.

Phone roster = that **grok TTY**, not the OpenClaw / Hermes / NemoClaw UI.

## NemoClaw

GBR stays on the **host**. Do not copy `gbr-agent` into OpenShell. See `skills/nemoclaw/SKILL.md`.

## Hermes

See `skills/hermes/SKILL.md`. Same loop.

## ClawHub

Publish this skill with human `clawhub login` — not a GitHub PR on `openclaw/openclaw`.
