# Build Remote Agent — compatibility

Product: **Build Remote Agent** (store apps) + free MIT `gbr-agent`.
Website: https://grokbuildremote.com/integrations.html
Plugin registry: [plugins/README.md](plugins/README.md)
Protocol: `gbr/1` · agent **v0.6.0+**
Independent Linespotting AB product. Not affiliated with xAI or SpaceX.

## What the phone sees

Read [docs/WHAT-THE-PHONE-SEES.md](docs/WHAT-THE-PHONE-SEES.md).

`gbr-agent` discovers **terminal windows** on the paired PC. One pair = one **mailbox for the whole machine**. The app can list and inject **every discovered TTY** (soft max 255), not “this one omp/qwen tab.”

Headless servers (OpenCode serve, CodeNomad sidecar, Electron UIs) are **not** in that roster. A sidecar pointed at `:8788` shows Bot API JSON, not a transcript.

## Pair (one protocol)

```
gbr-agent pair    # QR in browser AND printed 8-char code
gbr-agent run     # keep running
```

Unpair in Settings before a new mailbox. Force-close is not enough.

## Attach (only these)

| How | Where |
|-----|--------|
| Bot API | `http://127.0.0.1:8788` after `gbr-agent run` |
| MCP | `gbr-mcp` stdio (`mcp/gbr-mcp`) — desktop agent talks to terminals via Bot API |

Phone is spectator + veto. Never put `mailbox_key` in git.

## Install (pinned)

Canonical: [docs/PINNED-INSTALL.md](docs/PINNED-INSTALL.md) — GitHub Release **v0.6.0** + `SHA256SUMS`. Do not paste `curl | bash` into other projects’ official docs.

## Plugins (this repo is the registry)

| Host | User drop-in |
|------|----------------|
| Grok Build | `.grok-plugin/` |
| Claude Code | `.claude-plugin/` |
| OpenCode V2 | `plugins/opencode/mcp.servers.json` |
| AiderDesk | `plugins/aider-desk/` → `~/.aider-desk/extensions/gbr-pair` |
| Any MCP | `mcp/gbr-mcp` |

## Compatible hosts

| Class | How |
|-------|-----|
| Agent **in a terminal** (Grok Build TUI, Aider CLI, OpenCode TUI, …) | Phone sees that window |
| Grok remotes Amnibro/Farina/ChrisP | Companions on loopback; not a substitute for pairing |
| Headless OpenCode / CodeNomad | MCP on the desktop agent; phone still needs a TTY |
| Mobile GUI (mobilerun, agent-device) | Different job — they drive a phone as a robot |
