# Plugin registry

This repo **is** the registry (not someone else’s). Drop-in configs and extensions so *users* attach Build Remote Agent. We do not commit vendor pages into other projects’ official examples.

Index: [registry.json](registry.json) · honesty: [docs/WHAT-THE-PHONE-SEES.md](../docs/WHAT-THE-PHONE-SEES.md) · install: [docs/PINNED-INSTALL.md](../docs/PINNED-INSTALL.md)

| Id | Drop in |
|----|---------|
| Grok Build | `.grok-plugin/` — `/plugin marketplace add LinespottingOrg/GrokBuildRemote-Agents` |
| Claude Code | `.claude-plugin/` — same marketplace add |
| Codex CLI / ChatGPT | `.codex-plugin/` — **catalog in this repo, not Codex core.** Do not PR `openai/codex`. [codex/README.md](codex/README.md) |
| OpenCode V2 | `plugins/opencode/mcp.servers.json` → user `mcp.servers` |
| AiderDesk | `plugins/aider-desk/` → `~/.aider-desk/extensions/gbr-pair` |
| Goose | [skills/goose/SKILL.md](../skills/goose/SKILL.md) — TTY roster; else user MCP on desktop Goose |
| Cline | [skills/cline/SKILL.md](../skills/cline/SKILL.md) — TTY roster; else user MCP on desktop Cline |
| Any MCP host | `mcp/gbr-mcp` stdio — [skills/gbr-mcp/SKILL.md](../skills/gbr-mcp/SKILL.md), pin `--branch v0.6.2` |
| Hermes | `plugins/hermes/mcp.json` — stdio only; `:8788` is not MCP |
| OpenClaw | `plugins/openclaw/mcp.json` + `skills/openclaw/SKILL.md` |
| NemoClaw | `plugins/nemoclaw/mcp.json` on the **host** |

Claw family default target is **Grok Build CLI** (`gbr_open` → `grok`). Setup: `scripts/setup-gbr-mcp.sh`. Use case: https://grokbuildremote.com/use-cases/claw.html

Phone roster = **terminal windows on the paired PC**. Headless servers need MCP on the *desktop* agent; they still do not show as phone sessions unless they run in a TTY.

**Go-to Grok multi-agent QA** (agent + relay **free**; mobile app optional): [skills/qa/SKILL.md](../skills/qa/SKILL.md) · https://grokbuildremote.com/use-cases/qa.html

Other GitHub projects get a **short unaffiliated link** to https://grokbuildremote.com/integrations.html — not a vendor install PR.
