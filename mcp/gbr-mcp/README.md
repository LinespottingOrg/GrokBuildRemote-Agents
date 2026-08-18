# gbr-mcp

**MCP server for the Grok Build Remote Bot API.** Lets any MCP client — Claude Code,
Grok CLI, Cursor, or your own — list and drive Grok Build / CLI sessions on this
machine and across a registered fleet of Mac, Linux and Windows boxes.

Requires **gbr-agent ≥ 0.5.3** (the release that introduced the Bot API) running
locally, or a relay mailbox + key for remote mode.

```
MCP client ──stdio──► gbr-mcp ──HTTP──► gbr-agent ──► Grok Build / terminals
                                   │
                                   └──► relay ──► remote agents (fleet)
```

## Tools

| Tool | Purpose |
|---|---|
| `gbr_diagnose` | 12–14 point environment check (varies by platform) with a literal repair command per failure. **Call this first when anything breaks.** |
| `gbr_status` | agent_online, version, uptime, session roster |
| `gbr_sessions` | Live sessions with id, title, cwd, shell, os |
| `gbr_devices` | Machines this hub can dispatch to |
| `gbr_inject` | Type a prompt into a session; returns `command_id` |
| `gbr_inject_and_wait` | Inject, then poll output until EOF. The one you usually want |
| `gbr_output` | Read buffered output by `command_id` or `session_id` |
| `gbr_fleet_add` | Register a remote machine by name |
| `gbr_discovery` | Raw Bot API discovery doc |

## Quick start

**bash (macOS):**
```
cd ~ && curl -fsSL https://grokbuildremote.com/install.sh | bash
export PATH="$HOME/.local/bin:$PATH"
gbr-agent pair && gbr-agent run
```

**bash (macOS):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && npm install && node bin/gbr-mcp.js --diagnose
```

Full instructions: [INSTALL.md](INSTALL.md) · When it breaks: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## Why this exists

The Bot API is a clean REST surface, but three of its behaviours will silently
mislead an autonomous agent. This server normalises all three:

1. **Logical errors arrive as HTTP 200** with `{"ok":false,"error":"..."}`. Checking
   the HTTP status is not enough. gbr-mcp raises these as `GBR_AGENT_REFUSED`.
2. **`inject` blocks server-side** when no live window is attached to the session,
   so a naive client hangs forever. Every call carries an abort timeout, and the
   timeout message names the actual cause.
3. **Unknown device names silently fall back to `local`.** An agent can believe it
   dispatched work to a remote box that never received it. gbr-mcp compares the
   echoed device against the requested one and attaches a `_warning`.

## Logging

Built for unattended debugging. Every tool call gets a correlation id stamped on
every line it produces, across two streams:

- **stderr** — human-readable
- **JSONL** — `~/.gbr/logs/gbr-mcp-<date>.jsonl`, one object per line

Failed calls return that `correlation_id` plus the log path in the error payload, so
an agent can go read its own failure. Secrets — mailbox keys, bearer tokens, any bare
48+ char hex string — are redacted before they reach either stream, at every level
including `trace`.

Levels: `trace` `debug` `info` `warn` `error` `silent` via `GBR_MCP_LOG_LEVEL`.

## Tests

**bash (macOS):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && npm test
```

Spawns the real server and speaks JSON-RPC over stdio exactly as a client would:
handshake, tool discovery, schema validation, live agent calls, structured-error
shape, and a secret-leak assertion.

`npm test` runs both suites: **37 unit assertions** (offline, stub-server based)
and **30 smoke assertions** against a live agent (25 when no agent is running).

## Compatibility

| | |
|---|---|
| Node | ≥ 20 |
| gbr-agent | ≥ 0.5.3 |
| Protocol | `gbr/1` |
| MCP protocol | 2024-11-05 and 2025-11-25 both negotiated OK |
| OS | macOS · Linux · Windows |

Verified side by side on **macOS 26.5.2 / Node 26.7.0 / arm64** and
**Ubuntu 22.04.5 / Node 22.22.3 / aarch64** — 37/37 offline assertions on both,
plus a full fleet round trip (dispatch to a named remote, output returned,
both gotchas detected). See `test/fixtures/` to reproduce with no agent.

## Licence

MIT. See [LICENSE](LICENSE).

Part of [Build Remote Agent](https://grokbuildremote.com/) by Linespotting AB.
Not affiliated with xAI or SpaceX.
