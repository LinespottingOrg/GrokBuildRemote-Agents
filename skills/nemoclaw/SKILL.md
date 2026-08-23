---
name: gbr-nemoclaw
description: >
  Drive Grok Build CLI from NemoClaw on the HOST only.
  Do not copy gbr-agent into OpenShell. Phone spectates the host grok TTY,
  not the NemoClaw or OpenShell session.
compatibility: Host tool. Sandbox must call host loopback / host stdio only.
metadata:
  version: "0.6.1"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/use-cases/claw.html"
---

# NemoClaw → Grok Build CLI (host)

NemoClaw sandboxes **its** agent. Build Remote Agent is the **host** tool.

This is **not** a NemoClaw/OpenShell adapter. There is no in-sandbox plugin.
The phone lists **host native TTYs** after `gbr-agent pair`. It does **not**
spectate OpenShell or the NemoClaw UI.

## Before you install (disclosure)

Read this **before** `pair` / `run`:

- **Outbound relay:** after pair, the PC talks outbound HTTPS 443 to
  `https://gbr-relay.ekobrott.workers.dev` (`gbr/1`). No inbound ports.
- **What crosses it:** roster titles, harvested TTY output, and inject text
  from the phone or Bot API. Treat the mailbox key as a password.
- **Remote input:** `POST /v1/inject` types into listed host TTYs and can submit.
  Loopback `:8788` is unauthenticated unless `GBR_BOT_REQUIRE_KEY=1`. Relay always
  wants `X-GBR-Key`.
- **Host permissions:** macOS Accessibility (type into terminals). Screen Recording
  may be requested for capture.
- **Mobile app:** paid closed-source spectator (about $13 one-time). Desktop agent
  is MIT. Bot API / gbr-mcp on the host does not require the app.
- **Pin:** GitHub Release **v0.6.0** — checksum the installer, then the binary.
  Not live `curl | bash`, not “v0.6.0 or newer”.
  [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md).

## Host

```bash
gbr-agent version                # must print v0.6.0
gbr-agent pair                   # one terminal — keep this window
# other terminal:
gbr-agent run                    # keep on the Mac/PC, not in the sandbox
export GBR_BOT_REQUIRE_KEY=1
bash scripts/setup-gbr-mcp.sh    # pins gbr-mcp @ v0.6.0 under ~/.gbr/gbr-mcp-src
```

Point NemoClaw at **host** stdio `node ~/.gbr/gbr-mcp-src/mcp/gbr-mcp/bin/gbr-mcp.js`
(or Bot API `http://127.0.0.1:8788` with the key). Do not install `gbr-agent` inside
OpenShell. No fourth pair protocol.

## Loop

`gbr_diagnose` → `gbr_open` `command=grok` `holder=nemoclaw` → inject → `gbr_result`.

That `grok` process is a **host TTY**. The phone sees it after pair. The sandbox UI
and OpenShell are **not** on the roster.

## NVIDIA recipe dest (do not PR NemoClaw core)

Partner provenance is `examples/recipes/partners/linespotting/` — **not**
`examples/recipes/community/gbr-pair/`. Do not comment on foreign PRs.
