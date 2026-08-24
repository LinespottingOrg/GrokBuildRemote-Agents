# gbr-mcp — install

**Kind:** MCP server (stdio)
**Platforms:** macOS · Linux · Windows
**Requires:** Node ≥20 · `gbr-agent` ≥ 0.5.4 running
**Official:** yes — Linespotting AB · https://grokbuildremote.com/

## What it gives you

Thirteen tools that let any MCP client drive Grok Build / CLI sessions on this machine
and on every machine registered in the fleet:

`gbr_diagnose` · `gbr_status` · `gbr_sessions` · `gbr_devices` · `gbr_open` · `gbr_lock`
`gbr_result` · `gbr_tasks` · `gbr_inject` · `gbr_inject_and_wait` · `gbr_output`
`gbr_fleet_add` · `gbr_discovery`

## Prerequisite: the agent

The Bot API landed in **0.5.3**. Open / lock / idle-wait result need **0.5.4**.
Anything older than 0.5.3 has no HTTP surface at all and `gbr-mcp` cannot work.

Pin the agent installer (do not pipe the live website): [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md).

**bash (macOS / Linux):**
```
VER=v0.6.2
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
SHA=0a7963dc668750bfcb907bb72f6f6f8db30881b02636e417e08e102352309301
curl -fsSL -o /tmp/gbr-install.sh "$BASE/install.sh"
echo "$SHA  /tmp/gbr-install.sh" | shasum -a 256 -c -
bash /tmp/gbr-install.sh
```

**PowerShell (Windows):**
```
$sha = "b604a21b5dae5a874487a597778d15742b3c2afb2470c93a8e8ba0a76e486cdf"
$i = Join-Path $env:TEMP "gbr-install.ps1"
Invoke-WebRequest "https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/v0.6.2/install.ps1" -OutFile $i
if ((Get-FileHash $i -Algorithm SHA256).Hash.ToLowerInvariant() -ne $sha) { throw "checksum" }
& $i
```

The installer writes to `~/.local/bin` and appends that to `~/.zshrc` — but it does
**not** reload your current shell. This is the single most common "it installed but
`command not found`" report:

**bash (macOS):**
```
cd ~ && export PATH="$HOME/.local/bin:$PATH" && gbr-agent version
```

Must print `v0.5.4` or higher. Then pair and run:

**bash (macOS):**
```
cd ~ && gbr-agent pair && gbr-agent run
```

Keep `run` alive. To persist across logins:

**bash (macOS):**
```
cd ~ && gbr-agent service install
```

Confirm the Bot API answers:

**bash (macOS):**
```
cd ~ && curl -sS http://127.0.0.1:8788/v1/status
```

## Install gbr-mcp

Pin the repo tag (not default branch):

```
git clone --branch v0.6.2 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
bash GrokBuildRemote-Agents/scripts/setup-gbr-mcp.sh
```

Or from a clone:

```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && npm install && chmod +x bin/gbr-mcp.js
```

Hermes / OpenClaw / NemoClaw: stdio `node bin/gbr-mcp.js`. Do not register `http://127.0.0.1:8788` as an MCP server — that URL is Bot API REST. `gbr_open` spawns **Grok Build CLI** (`grok`). NemoClaw is a sandbox, not a fourth pair.

**bash (Linux):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && npm install && chmod +x bin/gbr-mcp.js
```

There is **no npm package**. Always run from this clone. Pin source tag **v0.6.2**.

## mcp add (Claude / Grok CLI / Cursor / Hermes / OpenClaw)

`ABS` = absolute path of `bin/gbr-mcp.js`. Same stdio server, 13 tools, same host as `gbr-agent run`. Pair is still QR or 8-char.

```bash
# Claude Code / Claude Cowork
claude mcp add gbr -- node ABS

# Grok CLI
grok mcp add gbr -- node ABS

# Cursor — ~/.cursor/mcp.json or project .cursor/mcp.json
# { "mcpServers": { "gbr": { "command": "node", "args": ["ABS"] } } }

# Hermes (stdio on this host — never point MCP at :8788)
hermes mcp add gbr -- stdio -- node ABS

# OpenClaw — copy ../../skills/openclaw/SKILL.md (ClawHub skill gbr).
# Not a core OpenClaw PR. Not a fourth pair.
```

## Config block

```json
{
  "gbr": {
    "command": "node",
    "args": ["/ABSOLUTE/PATH/GrokBuildRemote-Agents/mcp/gbr-mcp/bin/gbr-mcp.js"],
    "env": { "GBR_MCP_LOG_LEVEL": "info" }
  }
}
```

On this Mac the Dropbox work copy is `/Users/user/Dropbox/MCP/gbr-mcp/bin/gbr-mcp.js`.

Merge into `~/.claude.json` under `mcpServers`.

**Grok CLI reads `~/.claude.json` too — verified, with a caveat.** `grok mcp doctor`
lists it as a config source but *dedupes against `~/.grok/config.toml`*, so a server
declared in both shows as "0 servers" from claude.json. That is dedup, not failure.
To declare it natively in Grok instead:

**bash (macOS):**
```
cd ~ && grok mcp add gbr -- node /ABSOLUTE/PATH/GrokBuildRemote-Agents/mcp/gbr-mcp/bin/gbr-mcp.js
```

## Verify

**bash (macOS):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && node bin/gbr-mcp.js --diagnose
```

Expect `all N checks passed` and exit code 0 (N is 13 on a healthy macOS/Linux box; the version check is skipped when the binary is absent, and a duplicate-agent check appears only when more than one is running). Then the protocol test:

**bash (macOS):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && npm test
```

Expect `37 passed` (unit) then `30 passed` (smoke; 25 without a live agent). Then confirm your client sees it:

**bash (macOS):**
```
cd ~ && grok mcp doctor 2>&1 | grep -A4 "^  gbr "
```

Expect `✓ handshake OK` and `✓ 13 tools discovered`.

## Environment

| Variable | Default | Purpose |
|---|---|---|
| `GBR_BOT_URL` | `http://127.0.0.1:8788` | Local hub |
| `GBR_RELAY_URL` | — | Relay base for remote mode |
| `GBR_MAILBOX_ID` | — | `gbr-xxxx`, required for relay mode |
| `GBR_MAILBOX_KEY` | — | Mailbox key, required for relay mode |
| `GBR_MCP_TIMEOUT_MS` | `20000` | HTTP timeout |
| `GBR_MCP_LOG_LEVEL` | `info` | `trace\|debug\|info\|warn\|error\|silent` |
| `GBR_MCP_LOG_DIR` | `~/.gbr/logs` | JSONL destination |
| `GBR_MCP_LOG_STDERR` | `1` | Human lines on stderr |
| `GBR_MCP_LOG_BODIES` | `1` | Log full HTTP bodies |
| `GBR_MCP_LOG_MAX_BODY` | `4000` | Truncation length |

Setting `GBR_RELAY_URL` + `GBR_MAILBOX_ID` + `GBR_MAILBOX_KEY` switches the client
from local mode to relay mode automatically — same tools, remote target.

## Logs

Two streams, by design:

- **stderr** — human-readable, one line per event. Your MCP client usually captures
  this (Grok: `~/.grok/logs/mcp/gbr.stderr.log`).
- **JSONL** — `~/.gbr/logs/gbr-mcp-<date>.jsonl`, one object per line, machine-parseable.

Every tool call gets an 8-char correlation id (`cid`) stamped on every line it
produces, so one call can be traced MCP request → HTTP request → HTTP response →
MCP response. Failed tool calls return that same `correlation_id` in the payload.

Raise verbosity:

**bash (macOS):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && GBR_MCP_LOG_LEVEL=trace node bin/gbr-mcp.js --diagnose
```

Follow live:

**bash (macOS):**
```
cd ~ && tail -f ~/.gbr/logs/gbr-mcp-$(date +%F).jsonl
```

Mailbox keys, bearer tokens and any bare 48+ char hex string are redacted before
they reach either stream. This is enforced in `src/logger.js` and covered by a test.

## Security

- The local Bot API binds `127.0.0.1` only. No inbound firewall hole.
- `require_key=false` by default on loopback. Harden with `GBR_BOT_REQUIRE_KEY=1`.
- The mailbox key is a password — anyone holding it can type into your paired
  sessions. Never commit it, screenshot it, or paste it into an issue.
