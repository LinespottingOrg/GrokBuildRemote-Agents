# Plugin registry

This repo **is** the registry (not someone else’s). Drop-in configs and extensions so *users* attach Build Remote Agent. We do not commit vendor pages into other projects’ official examples.

Index: [registry.json](registry.json) · honesty: [docs/WHAT-THE-PHONE-SEES.md](../docs/WHAT-THE-PHONE-SEES.md) · install: [docs/PINNED-INSTALL.md](../docs/PINNED-INSTALL.md)

| Id | Drop in |
|----|---------|
| Grok Build | `.grok-plugin/` — `/plugin marketplace add LinespottingOrg/GrokBuildRemote-Agents` |
| Claude Code | `.claude-plugin/` — same marketplace add |
| OpenCode V2 | `plugins/opencode/mcp.servers.json` → user `mcp.servers` |
| AiderDesk | `plugins/aider-desk/` → `~/.aider-desk/extensions/gbr-pair` |
| Goose | [skills/goose/SKILL.md](../skills/goose/SKILL.md) — TTY roster; else user MCP on desktop Goose |
| Cline | [skills/cline/SKILL.md](../skills/cline/SKILL.md) — TTY roster; else user MCP on desktop Cline |
| Any MCP host | `mcp/gbr-mcp` stdio |

Phone roster = **terminal windows on the paired PC**. Headless servers need MCP on the *desktop* agent; they still do not show as phone sessions unless they run in a TTY. Canonical index: https://grokbuildremote.com/integrations.html
