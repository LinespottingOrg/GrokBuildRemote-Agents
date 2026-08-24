---
name: gbr-nemoclaw
description: >
  Drive Grok Build CLI from NemoClaw. gbr-agent and gbr-mcp stay on the HOST.
  Do not copy them into OpenShell. Phone spectates the host grok TTY.
compatibility: Host tool. Sandbox must call host loopback / host stdio only.
metadata:
  version: "0.6.1"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/use-cases/claw.html"
---

# NemoClaw → Grok Build CLI (host)

NemoClaw sandboxes **its** agent. Build Remote Agent is the **host** tool.

## Host

```bash
gbr-agent pair && gbr-agent run     # keep on the Mac/PC, not in the sandbox
export GBR_BOT_REQUIRE_KEY=1
bash scripts/setup-gbr-mcp.sh       # pins gbr-mcp @ v0.6.2 under ~/.gbr/gbr-mcp-src
```

Point NemoClaw at **host** stdio `node ~/.gbr/gbr-mcp-src/mcp/gbr-mcp/bin/gbr-mcp.js`
(or Bot API `http://127.0.0.1:8788` with the key). Do not install `gbr-agent` inside
OpenShell. No fourth pair protocol.

## Loop

`gbr_diagnose` → `gbr_open` `command=grok` `holder=nemoclaw` → inject → `gbr_result`.

That `grok` process is a host TTY. The phone sees it after pair. The sandbox UI is not on the roster.

Recipe dest for NVIDIA: community `examples/recipes/community/gbr-pair/` — do not PR NemoClaw core.
