---
name: gbr-qa
description: >
  Grok multi-agent QA loop on this PC via Build Remote Agent.
  Spawn grok windows, lock each, inject, wait idle, judge excerpt, iterate until gbr_tasks status=done.
  Desktop agent and relay are free. Mobile app is optional premium spectator.
compatibility: gbr-agent ≥ 0.6.0. Loopback Bot API or stdio gbr-mcp. No mailbox keys in this file.
metadata:
  version: "0.6.1"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/use-cases/qa.html"
---

# Multi-agent QA → solution

You are the orchestrator. `gbr-agent` is free MIT. Hosted relay is free. The phone app is premium and **must not** drive injects.

## Setup (free)

```bash
gbr-agent version    # v0.6.0+
gbr-agent run        # pair first only if a phone should watch
export GBR_BOT_REQUIRE_KEY=1
```

MCP: `bash scripts/setup-gbr-mcp.sh` then stdio `gbr-mcp`. Do not treat `:8788` as MCP.

## Loop

1. `gbr_diagnose`
2. `gbr_open` `command=grok` `holder=writer` `goal=<fix>`
3. `gbr_open` `command=grok` `holder=tester` `goal=<verify>` — **different session_id**
4. `gbr_lock` acquire on each (do not share a window)
5. Writer: `gbr_inject` the fix (`submit=true`) → `gbr_result` `wait_ms=60000`
6. Tester: `gbr_inject` the test command → `gbr_result`
7. `gbr_tasks` upsert `iteration+=1` `last_excerpt` `last_judge` `status=running|done|failed`
8. If tests fail, go to 5. If pass, `status=done`, `gbr_lock` release both.

Stop on `failed` after a sensible iteration cap (e.g. 8). Empty excerpt is honest — judge what you have.

Phone (if paired) is spectator only.
