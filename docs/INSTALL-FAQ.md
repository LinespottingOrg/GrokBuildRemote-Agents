# Install FAQ — agent · relay · MCP

Webpage-ready. Written to drop into `grokbuildremote.com/support.html` and to be
mirrored in the repo as `docs/INSTALL-FAQ.md`. Every answer verified against agent
**v0.5.3** on macOS 26.5, 2026-08-17.

---

## Part 1 — Desktop agent

### The installer said "installed" but my shell says `command not found: gbr-agent`

The installer writes to `~/.local/bin` and appends that directory to `~/.zshrc` —
but it cannot reload the shell you are already sitting in. Open a new terminal, or
export the path for the current one:

**bash (macOS / Linux):**
```
cd ~ && export PATH="$HOME/.local/bin:$PATH" && gbr-agent version
```

This is the single most common install report. Nothing is broken.

### Which version do I need?

**0.5.3 or newer** for the Bot API and live session names. Anything on the `0.4.x`
line has no HTTP Bot API at all — `curl http://127.0.0.1:8788/` will refuse the
connection no matter what else you do.

### `curl: (7) Failed to connect to 127.0.0.1 port 8788`

The Bot API only exists while the agent is running.

**bash (macOS / Linux):**
```
cd ~ && gbr-agent run
```

Keep that window open, or install it as a login service:

**bash (macOS / Linux):**
```
cd ~ && gbr-agent service install
```

Then confirm:

**bash (macOS / Linux):**
```
cd ~ && curl -sS http://127.0.0.1:8788/v1/status
```

### Do I have to re-pair after upgrading?

Yes, if you were below 0.5.x. Under the enforce relay both client and agent must
hold a mailbox key. Use **Settings → Unpair / Forget this PC** — not Disconnect,
which only pauses live updates — then `gbr-agent pair` again.

### `gbr-agent sessions` only lists my shell, not Grok Build

Start Grok Build **first**, with a window actually open, then run the agent. If the
roster still looks wrong you are likely below 0.5.3 — names froze after six windows
in older builds. Upgrade, unpair, pair again.

### Two agents are running and the port keeps flapping

An upgrade left the old binary running. There must be exactly one:

**bash (macOS / Linux):**
```
cd ~ && pkill -f gbr-agent ; sleep 1 ; gbr-agent run
```

### Where are the logs?

`~/.gbr/logs/agent-<date>.jsonl`. For a shareable redacted bundle:

**bash (macOS / Linux):**
```
cd ~ && gbr-agent support-log
```

It lands in `~/Downloads`. On the phone: Settings → **Copy support log**.

---

## Part 2 — Relay

### Do I need to open any ports?

No. Phone and PC never connect to each other. Both make **outbound HTTPS 443** calls
to a durable mailbox — which is why this works on corporate and locked-down
networks. Test a restrictive network with:

**bash (macOS / Linux):**
```
cd ~ && gbr-agent netcheck
```

### `401 unauthorized` / `mailbox_key missing`

The agent paired but never stored the key — a real bug in `0.3.x`, fixed in 0.4.1.
On a modern build this means the pairing is stale. Install 0.5.3+, unpair, pair
again, then confirm:

**bash (macOS / Linux):**
```
cd ~ && gbr-agent status
```

Look for `mailbox_key: set (64 chars) — requests authenticated`.

### Can I self-host the relay?

Yes, since v0.5. Set `GBR_RELAY_URL` on the agent and the matching field in phone
Settings. The protocol stays `gbr/1`.

### What are the rate limits?

Relay inject is capped at **60 per minute per mailbox**. Batch prompts rather than
firing many small injects. A `429` surfaces as `GBR_RATE_LIMITED` in gbr-mcp.

### Is the mailbox key sensitive?

Treat it as a password — anyone holding it can type into your paired sessions. Never
put it in a public issue, a screenshot, or a store listing. Phone Settings →
**Bot API** copies it when you legitimately need it.

---

## Part 3 — Bot API and the MCP server

### Can a bot or CLI drive Grok Build without the phone?

Yes, from 0.5.3. The agent exposes a REST Bot API on `127.0.0.1:8788` (loopback
only), and the same surface is reachable through the relay for a bot running
anywhere. Print worked examples:

**bash (macOS / Linux):**
```
cd ~ && gbr-agent bot
```

### What is `gbr-mcp`?

An MCP server wrapping the Bot API, so Claude Code, Grok CLI, Cursor and others can
drive sessions as first-class tools instead of shelling out to `curl`. Nine tools,
structured errors, and a `gbr_diagnose` tool that returns a literal repair command
for every failed check.

**bash (macOS / Linux):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && npm install && node bin/gbr-mcp.js --diagnose
```

### One config for both Claude and Grok?

Yes. Grok CLI reads `~/.claude.json` in addition to `~/.grok/config.toml`.

**Caveat worth knowing:** `grok mcp doctor` **dedupes**. A server declared in both
files is reported as "0 servers" from `~/.claude.json` — that is deduplication, not
a parse failure. Verified by probe on 2026-08-17. What matters is that the server
shows `✓ handshake OK` further down the doctor output.

### My MCP server fails to start the very first time, then works

`npx` downloads the package on first launch, which can exceed Grok's default
30-second `startup_timeout_sec`. Raise it in `~/.grok/config.toml`:

```toml
[mcp_servers.example]
command = "npx"
args = ["-y", "some-mcp@latest"]
startup_timeout_sec = 120
```

Or pre-warm the package once with `npx -y some-mcp@latest --help`.

### `inject` just hangs

The agent holds the request open when **no live Grok Build window is attached** to
that `session_id`. Two rules:

1. Always call `sessions` first and use a real id.
2. A session literally named `session` is the agent's own pseudo-session, not a
   window. Injecting into it will always hang.

`gbr-mcp` bounds every call with a timeout and names this exact cause in the error.

### `inject` returned HTTP 200 but nothing happened

**The Bot API returns HTTP 200 on logical errors.** An empty `session_id` responds
`200` with `{"ok":false,"error":"inject: empty session_id refused"}`. Clients must
check the `ok` field in the body, not the HTTP status. `gbr-mcp` converts these into
a `GBR_AGENT_REFUSED` error with a hint.

### I targeted a remote machine but it ran locally

**Unknown device names silently fall back to `local`.** Posting `{"device":"nope"}`
returns `device.id === "local"` with no error. Register remotes first:

**bash (macOS / Linux):**
```
cd ~ && gbr-agent fleet add -name studio-linux -mailbox gbr-XXXX -key KEY -os linux
gbr-agent fleet
```

`gbr-mcp` compares the echoed device against the requested one and attaches a
`_warning` when they differ.

### How do I debug an MCP failure with no human watching?

Every failed tool call returns `code`, `hint`, `correlation_id` and `log_file`. Grep
the JSONL by that correlation id to see the exact request and response:

**bash (macOS / Linux):**
```
cd ~ && grep '"cid":"YOURCID"' ~/.gbr/logs/gbr-mcp-$(date +%F).jsonl | python3 -m json.tool
```

Full trace for one run:

**bash (macOS / Linux):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && GBR_MCP_LOG_LEVEL=trace npm test
```

### Are secrets ever written to the logs?

No. Mailbox keys, bearer tokens, and any bare 48+ character hex string are redacted
before reaching stderr or the JSONL file, at every level including `trace`. A test
asserts that a 64-char key never appears in a response.

### Does gbr-mcp run on Linux and Windows?

Yes — Node ≥20, no native modules, no platform-specific code paths. The only
platform-specific check is the `pgrep` liveness probe, skipped on Windows.

---

## Part 4 — Errors actually hit during a real install

Every entry below was reproduced on macOS 26.5.2 / Homebrew 6.0.13 on 2026-08-17
while installing this stack from scratch. Verbatim error text, verified fix.

### `Error: Refusing to load formula ... from untrusted tap`

```
Error: Refusing to load formula getsentry/xcodebuildmcp/xcodebuildmcp from
untrusted tap getsentry/xcodebuildmcp.
Run `brew trust --formula getsentry/xcodebuildmcp/xcodebuildmcp` or
`brew trust getsentry/xcodebuildmcp` to trust it.
```

**Homebrew 6 added tap trust.** Every third-party tap must be explicitly trusted
before its formulae will load, and no install guide written before Homebrew 6
mentions this. It bites `xcodebuildmcp` (getsentry tap) and `idb-companion`
(facebook tap) — the two Homebrew-installed pieces of the iOS QA stack.

**bash (macOS):**
```
cd ~ && brew trust getsentry/xcodebuildmcp && brew trust facebook/fb && brew trust grafana/grafana
```

Then re-run the install. Trust is per-tap and persists.

### `zsh: command not found: gbr-agent` immediately after a successful install

Covered in Part 1, repeated here because it is the highest-frequency report of
all. The installer appends `~/.local/bin` to `~/.zshrc` but cannot reload the
shell you are already in.

**bash (macOS / Linux):**
```
cd ~ && export PATH="$HOME/.local/bin:$PATH" && gbr-agent version
```

### Pre-warming `@postman/postman-mcp-server` exits non-zero

```
warming @postman/postman-mcp-server            rc=1
```

That package has no `--help` flag, so the usual `npx -y <pkg> --help` pre-warm
trick reports failure even though the download succeeded. The download is what
matters; the exit code is noise. Pre-warm it without asserting on the code:

**bash (macOS / Linux):**
```
cd ~ && npx -y @postman/postman-mcp-server --help >/dev/null 2>&1 || true
```

### `fb-idb` installs but its scripts are not on PATH

```
WARNING: The script idb is installed in '~/Library/Python/3.14/bin' which is
not on PATH.
```

`ios-simulator-mcp` shells out to `idb`, so this silently breaks it later.

**bash (macOS):**
```
cd ~ && echo 'export PATH="$HOME/Library/Python/3.14/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc && idb --version
```

Note the Python version is in the path — check yours with `python3 -V`.

### `idb-companion` and `ios-simulator-mcp` need TWO installs, not one

A frequent misread: `brew install idb-companion` alone is not enough. The MCP
server needs both the companion binary and the Python client.

**bash (macOS):**
```
cd ~ && brew trust facebook/fb && brew install idb-companion && python3 -m pip install --user --break-system-packages fb-idb
```

### Verifying the whole stack in one command

`grok mcp doctor` starts every configured server, completes a handshake and
counts tools. It is the fastest way to prove an install.

**bash (macOS / Linux):**
```
cd ~ && grok mcp doctor
```

A healthy stack ends with a line like `Found 13 healthy, 0 failing.` Any server
that fails prints the reason inline. Note it reports each config source
separately and **dedupes** — a server present in both `~/.grok/config.toml` and
`~/.claude.json` is only counted once.

### Versions this stack was verified against

```
macOS 26.5.2 · Homebrew 6.0.13 · Node 26.7.0 · npm 12.0.2 · pnpm 11.22.0
gbr-agent v0.5.3 · grok 1.0.4 · maestro 0.17.3 · xcodebuildmcp 2.7.0
mcp-k6 0.5.1 · eas-cli 22.0.0 · firebase-tools 15.27.0 · Java 21.0.11
```
