# Build Remote Agent — compatibility

Product: **Build Remote Agent** (store apps) + free MIT `gbr-agent`.
Website: https://grokbuildremote.com/integrations.html
Agent: https://github.com/LinespottingOrg/GrokBuildRemote-Agents
Protocol: `gbr/1` · agent **v0.6.0+**
Independent Linespotting AB product. Not affiliated with xAI or SpaceX.

## Pair (one protocol)

```
gbr-agent pair    # QR in browser AND printed 8-char code
gbr-agent run     # keep running
```

Phone: Build Remote Agent → Scan QR (or type the code).
Unpair in Settings before a new mailbox. Force-close is not enough.
No fourth pair protocol.

## Attach (only these)

| How | Where |
|-----|--------|
| Bot API | `http://127.0.0.1:8788` after `gbr-agent run` |
| MCP | `gbr-mcp` stdio (`mcp/gbr-mcp`) |

Phone is spectator + veto. Never put `mailbox_key` in git or issues.

## Compatible hosts

| Class | Examples | How to attach |
|-------|----------|----------------|
| Grok Build | Grok Build TUI, Grok Bot 2026-08-11 | inject into the live window; Bot API device classes |
| Grok remotes (companions) | Amnibro `:2421`, Farina `:7910`, ChrisP `:8787`, Kojo | GBR probes loopback; LAN/Tailscale UI stays |
| Claude Code / Cowork | `gbr-mcp`, `skills/openclaw/SKILL.md` | MCP stdio |
| Git-native CLIs | Aider and forks | MCP snippet + docs |
| Codex ecosystem | community plugins / MCP servers (not closed Codex core) | plugin or docs |
| Terminal TUIs | OpenCode, Crush, Gemini CLI, Qwen Code | MCP config |
| IDE agents | Cline, Goose, Zed extensions | MCP / docs |
| Autonomous SWE | OpenHands, SWE-agent | docs / skill |
| Mobile GUI agents | mobilerun, agent-device | **different job** (they drive a phone). GBR pairs the *desktop* coding agent |

## Install

```
curl -fsSL https://grokbuildremote.com/install.sh | bash
gbr-agent version    # need v0.6.0+
```

Windows: `irm https://grokbuildremote.com/install.ps1 | iex`

MCP:

```
git clone https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install
node bin/gbr-mcp.js --diagnose
```

## See also

- https://grokbuildremote.com/integrations.html
- https://grokbuildremote.com/llms.txt
- AGENTS.md in this repo (AIs)
- docs/BOT-API.md
