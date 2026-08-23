# Plugin registry

This repo **is** the registry (not someone else’s). Drop-in configs and extensions so *users* attach Build Remote Agent. We do not commit vendor pages into other projects’ official examples.

Index: [registry.json](registry.json) · honesty: [docs/WHAT-THE-PHONE-SEES.md](../docs/WHAT-THE-PHONE-SEES.md) · install: [docs/PINNED-INSTALL.md](../docs/PINNED-INSTALL.md) · public: https://grokbuildremote.com/integrations.html

| Id | Drop in |
|----|---------|
| Grok Build | `.grok-plugin/` — `/plugin marketplace add LinespottingOrg/GrokBuildRemote-Agents` |
| Claude Code | `.claude-plugin/` — same marketplace add |
| Codex CLI / ChatGPT | `.codex-plugin/` — **scanner-ready catalog entry in this repo, not Codex core.** Do not PR `openai/codex`. How-to: [codex/README.md](codex/README.md) |
| OpenCode V2 | `plugins/opencode/mcp.servers.json` → user `mcp.servers` |
| AiderDesk | `plugins/aider-desk/` → `~/.aider-desk/extensions/gbr-pair` |
| Any MCP host | `mcp/gbr-mcp` stdio |

Phone roster = **terminal windows on the paired PC**. Headless servers need MCP on the *desktop* agent; they still do not show as phone sessions unless they run in a TTY.

## Codex — scanner-ready catalog, not Codex core

`.codex-plugin/plugin.json` is a **scanner-ready catalog entry** for Codex / ChatGPT plugin scanners (`plugin-scanner`, HOL Guard, `codex plugin marketplace add`).

It is **not** OpenAI Codex core. It is **not** in the universal OpenAI plugin directory. **Do not open a PR against `openai/codex`.** Users install from *this* registry.

| | |
|--|--|
| Manifest | [../.codex-plugin/plugin.json](../.codex-plugin/plugin.json) |
| Marketplace | [../.agents/plugins/marketplace.json](../.agents/plugins/marketplace.json) |
| How-to | [codex/README.md](codex/README.md) |
| Agent binary | pin GitHub Release **v0.6.0** + SHA-256 — [docs/PINNED-INSTALL.md](../docs/PINNED-INSTALL.md) |
| Integrations | https://grokbuildremote.com/integrations.html |

```bash
# Catalog (this repo). Pin --ref to a commit SHA when you freeze it.
codex plugin marketplace add LinespottingOrg/GrokBuildRemote-Agents --ref main
codex plugin install build-remote-agent
```

Then pin-install `gbr-agent` (checksum the installer, then the binary). Do not `curl | bash`.
