# Use case — Hermes / OpenClaw / NemoClaw + Grok Build CLI

Canonical page: https://grokbuildremote.com/use-cases/claw.html

These three are unaffiliated. One attach surface. Default coding agent is **Grok Build CLI** (`grok`).

```
Hermes or OpenClaw or NemoClaw (MCP client)
    │  stdio gbr-mcp   (not :8788 as MCP)
    ▼
gbr-agent run  →  gbr_open spawns `grok` in a TTY
    │
    ▼
Phone (optional) spectates that grok window after pair
```

| Claw | How |
|------|-----|
| Hermes | `hermes mcp add gbr -- stdio -- node …/gbr-mcp.js` |
| OpenClaw | Skill + same stdio MCP. ClawHub publish is human login. OpenClaw *phone* pair ≠ `gbr/1`. |
| NemoClaw | MCP / Bot API on the **host**. Not inside OpenShell. |

Setup: `scripts/setup-gbr-mcp.sh` (pins tag v0.6.0). Skills: `skills/hermes`, `skills/openclaw`, `skills/nemoclaw`.

`:8788` = Bot API REST. MCP = stdio `gbr-mcp`. Mixing them is why attach “doesn’t work”.
