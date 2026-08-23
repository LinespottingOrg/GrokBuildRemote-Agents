# Go-to solution — Grok multi-agent QA

Canonical page: https://grokbuildremote.com/use-cases/qa.html

Build Remote Agent is the **control layer** for a Grok Build multi-agent QA loop that
runs until the task is `done`. The **desktop agent and HTTPS relay are free**. The
**mobile app is the premium spectator** ($13 one-time).

| Layer | Model | What you get |
|-------|--------|----------------|
| `gbr-agent` | **Freemium / MIT free** | Pair, run, Bot API, `gbr_open` → `grok`, lock, result, tasks, fleet |
| Hosted relay | **Freemium** | `gbr-relay.ekobrott.workers.dev` — HTTPS mailbox, no inbound ports. Self-host is MIT. |
| Mobile app | **Premium** | App Store / Play, $13 one-time. Spectator + veto + Bot API key copy. Not required for the QA loop. |

Independent Linespotting AB. Not affiliated with xAI.

## Why this is the QA setup

Grok Build already plans/builds/tests in a TTY. What you need for **multi-agent QA** is:

1. Several `grok` windows (writer, tester, optional fleet box)
2. **One lease per window** so two bots do not type into the same TTY
3. Inject → wait idle → harvest excerpt → judge → iterate until `gbr_tasks` is `done`
4. Optional phone watching the roster (premium)

That contract is already in agent/relay **0.5.4+** / **0.6.0** (`gbr_open`, `gbr_lock`, `gbr_inject`, `gbr_result`, `gbr_tasks`).

```
Grok Bot or gbr-mcp client (orchestrator)     FREE
        │  127.0.0.1:8788  or  stdio gbr-mcp
        ▼
gbr-agent run  +  free HTTPS relay            FREE
        │  gbr_open × N  (each spawns grok)
        ▼
TTY A: grok  (holder=writer)
TTY B: grok  (holder=tester)
        │
        ▼
Phone Build Remote Agent                      PREMIUM spectator
```

## Loop until solution

Skill: [skills/qa/SKILL.md](../skills/qa/SKILL.md)

1. Pin install `gbr-agent` v0.6.0 — [PINNED-INSTALL.md](PINNED-INSTALL.md)
2. `gbr-agent pair && gbr-agent run` (pair only if you want the phone)
3. `export GBR_BOT_REQUIRE_KEY=1`
4. Orchestrator: `gbr_diagnose`
5. `gbr_open` `command=grok` `holder=writer` `goal="fix the failing tests"`
6. `gbr_open` `command=grok` `holder=tester` `goal="run tests and report"`
7. Inject the fix into **writer**; `gbr_result`; judge excerpt
8. Inject the test command into **tester**; `gbr_result`
9. `gbr_tasks` upsert `status=running|done|failed` each iteration
10. Repeat until `status=done`. Release locks.

Phone never orchestrates (`cannot_inject_phone`). It sees both TTYs if paired.

## Fleet QA (optional)

Hub mailbox + `gbr-agent fleet add` for a second machine. Inject with `"device":"studio-linux"`. Same free agent + free relay.

## What you do not need to pay for

- Running the loop on the PC
- Hosted relay envelopes
- Self-hosted relay
- MCP / Grok Bot / Hermes / OpenClaw / NemoClaw driving `grok`

You pay **only** if you want the store app on a phone/tablet as spectator + veto.
