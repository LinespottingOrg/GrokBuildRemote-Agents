# Go-to solution — Grok multi-agent QA

Canonical page: https://grokbuildremote.com/use-cases/qa.html

Build Remote Agent is the **control layer** for a Grok Build multi-agent QA loop until `gbr_tasks` is `done`. Desktop agent + hosted relay are **free**. The mobile app is a **premium remote-control client** ($13 one-time) and is **not** required for the loop.

Independent Linespotting AB. Not affiliated with xAI.

```
Grok Bot or gbr-mcp (orchestrator)     FREE
        │  127.0.0.1:8788  or  stdio gbr-mcp
        ▼
gbr-agent run  +  free HTTPS relay     FREE
        │  gbr_open × N  (each a grok TTY; spawn cap 3/10min)
        ▼
TTY A: grok  (holder=writer)
TTY B: grok  (holder=tester)
```

Skill: [skills/qa/SKILL.md](../skills/qa/SKILL.md)

Pin **v0.6.2** — [PINNED-INSTALL.md](PINNED-INSTALL.md). Split `pair` and `run`. Set `GBR_BOT_REQUIRE_KEY=1`. Relay pair is keyless/throttled; push/poll/ack need `X-GBR-Key`. No fourth pair protocol. No secrets in git.
