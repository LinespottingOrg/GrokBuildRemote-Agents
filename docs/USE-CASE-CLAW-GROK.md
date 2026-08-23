# Use case — Hermes / OpenClaw / NemoClaw + Grok Build CLI

Canonical page: https://grokbuildremote.com/use-cases/claw.html

These three are unaffiliated. One attach surface. Default coding agent is **Grok Build CLI** (`grok`).

The phone spectates the **host `grok` TTY**, not an OpenClaw / Hermes / NemoClaw / OpenShell session.

## Disclosure (read before pair)

- Outbound HTTPS relay: `https://gbr-relay.ekobrott.workers.dev` (`gbr/1`). Roster titles, harvested TTY output, and inject text cross it. Mailbox key = password.
- `POST /v1/inject` types into listed host TTYs. Loopback `:8788` unauthenticated unless `GBR_BOT_REQUIRE_KEY=1`.
- macOS Accessibility required to type into terminals.
- Mobile app is paid closed-source (~$13). Desktop agent MIT.
- Pin GitHub Release **v0.6.0** + installer SHA-256 — not live `curl | bash`, not “or newer”. [PINNED-INSTALL.md](PINNED-INSTALL.md).

NVIDIA community recipes (if any): partner path `examples/recipes/partners/linespotting/`, not `examples/recipes/community/gbr-pair/`.

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
