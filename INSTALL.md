# Install — Build Remote Agent (`gbr-agent`)

AIs: read this, then [AGENTS.md](AGENTS.md) and [llms.txt](llms.txt).

| | |
|--|--|
| **Product** | Build Remote Agent (Linespotting AB). Independent. Not xAI. |
| **Pin binaries** | GitHub Release **[v0.6.2](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.2)** (`v0.6.2`). Do not mutate v0.6.0 / v0.6.1. |
| **Source / docs** | Same tag **v0.6.2** |
| **Pair** | Unchanged: QR **or** printed 8-char, then `gbr-agent run` |
| **Attach** | One surface: stdio `gbr-mcp` (13 tools) **or** Bot API `http://127.0.0.1:8788` (REST, not MCP) |
| **Relay** | `https://gbr-relay.ekobrott.workers.dev` · proto `gbr/1` · `/health` `0.6.0` |

Live website `curl | bash` is **mutable**. Pin the GitHub **installer** SHA-256 (trust anchor): [docs/PINNED-INSTALL.md](docs/PINNED-INSTALL.md). A co-downloaded `SHA256SUMS` from the same release is **not** a trust anchor.

---

## 1. Install the desktop agent

**macOS / Linux** (checksum the installer, then run it):

```bash
VER=v0.6.2
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
SHA=f91ce49afbc21ac51ccf8b69b95ee407ff2d8a60926e2868bb192bb03eca796d
curl -fsSL -o /tmp/gbr-install.sh "$BASE/install.sh"
echo "$SHA  /tmp/gbr-install.sh" | shasum -a 256 -c -
bash /tmp/gbr-install.sh
export PATH="$HOME/.local/bin:$PATH"
gbr-agent version    # must print v0.6.2
```

**Windows:**

```powershell
$ver = "v0.6.2"
$sha = "572e7008400d16e7b666ba42b6fcd81ef03c0ab45190a1151f09f6d97716b5e1"
$base = "https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$ver"
$i = Join-Path $env:TEMP "gbr-install.ps1"
Invoke-WebRequest "$base/install.ps1" -OutFile $i -UseBasicParsing
if ((Get-FileHash $i -Algorithm SHA256).Hash.ToLowerInvariant() -ne $sha) { throw "installer SHA-256 mismatch" }
& $i
gbr-agent version
```

**Windows non-interactive service / AtLogon (no Interactive-only):** after the binary is at `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe`, use [`scripts/windows/`](scripts/windows/README.md) (`install-service.ps1`). Defaults: `GBR_INJECT_HALT=1`, logs `C:\pc-build\gbr-agent-out\`. Do not delete legacy task `\GrokBuildRemoteAgent` without David yes. See also [install/windows/service.md](install/windows/service.md).

**Binary only** (skip the installer): verify a **hard-coded** digest from [docs/PINNED-INSTALL.md](docs/PINNED-INSTALL.md), then `mkdir -p ~/.local/bin`, `install` onto that path, and `export PATH="$HOME/.local/bin:$PATH"`. Do not `curl SHA256SUMS && shasum -c`.

From source (inbox watch + claw skill live on **v0.6.1**):

```bash
git clone --branch v0.6.1 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents
make build          # → dist/gbr-agent
```

---

## 2. Pair (QR or 8-char — no fourth protocol)

```text
1. Phone: Build Remote Agent → Connect
2. PC:    gbr-agent pair          # browser QR AND printed 8-char code
3. Phone: scan the QR, or type the 8-char code
4. PC:    gbr-agent run           # keep this running (LaunchAgent/service optional)
```

Unpair on the phone before a new mailbox. Force-close is not enough.

NemoClaw is a **sandbox**, not a fourth pair. GBR stays the **host** tool (`gbr-agent` on the Mac/PC). Do not copy the agent into the sandbox.

---

## 3. MCP add (`gbr-mcp` · 13 tools)

Clone this repo (pin **v0.6.2**):

```bash
git clone --branch v0.6.1 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
bash GrokBuildRemote-Agents/scripts/setup-gbr-mcp.sh
# or: cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install && chmod +x bin/gbr-mcp.js
node GrokBuildRemote-Agents/mcp/gbr-mcp/bin/gbr-mcp.js --diagnose     # expect 13 tools
```

There is **no npm package**. MCP is **stdio** on the **same host** as `gbr-agent run`. Do **not** `mcp add` `http://127.0.0.1:8788` — that URL is Bot API REST. `gbr_open` spawns **Grok Build CLI** (`grok`). Full client recipes: [mcp/gbr-mcp/INSTALL.md](mcp/gbr-mcp/INSTALL.md).

Replace `ABS` with the absolute path of `bin/gbr-mcp.js`:

```bash
# Claude Code / Claude Cowork
claude mcp add gbr -- node ABS

# Grok CLI
grok mcp add gbr -- node ABS

# Cursor — merge into ~/.cursor/mcp.json (or project .cursor/mcp.json)
# { "mcpServers": { "gbr": { "command": "node", "args": ["ABS"] } } }

# Hermes (stdio on the GBR host — never point MCP at :8788)
hermes mcp add gbr -- stdio -- node ABS

# OpenClaw — copy skills/openclaw/SKILL.md (ClawHub skill gbr).
# Not a core OpenClaw PR. Not a fourth pair.
```

Tools: `gbr_diagnose` · `gbr_status` · `gbr_sessions` · `gbr_devices` · `gbr_open` · `gbr_lock` · `gbr_result` · `gbr_tasks` · `gbr_inject` · `gbr_inject_and_wait` · `gbr_output` · `gbr_fleet_add` · `gbr_discovery`.

---

## 4. Bot API (`127.0.0.1:8788`)

While `gbr-agent run`:

```bash
curl -sS http://127.0.0.1:8788/health
curl -sS http://127.0.0.1:8788/v1/status
gbr-agent bot                      # prints curl examples
```

Loopback only. REST, not MCP. Remote: relay `/v1/mb/{id}/bot` + `X-GBR-Key` from phone **Settings → Bot API**. Never commit that key. Spec: [docs/BOT-API.md](docs/BOT-API.md).

---

## 5. `/rename` and inbox (no paste)

`/rename TITLE` must be its **own submitted TUI line**. Do not bury it inside a pasted prompt. Alias: `/title`. Fallback: `gbr-agent sessions` then `gbr-agent rename -session ID -name "TITLE"`.

After the inbox watcher is running (`gbr-agent run` + `gh` on PATH), **do not paste** boss-steer comments. The watcher injects GitHub `LinespottingOrg/grok-build-inbox` label `boss-steer` into a matching Grok Build title (two submits: `/rename TITLE`, then the body). Disable: `GBR_INBOX_WATCH=0`.

---

## Do not

- Commit `~/.gbr`, mailbox keys, or `wrangler.toml` secrets
- Invent a second pair protocol
- Put `gbr-agent` inside a NemoClaw sandbox
- Register `http://127.0.0.1:8788` as an MCP server
- Resubmit iOS / reopen Play 1.3.2 from this repo
