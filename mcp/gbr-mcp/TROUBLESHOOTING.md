# gbr-mcp — troubleshooting

**Start here, always:**

**bash (macOS):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && node bin/gbr-mcp.js --diagnose
```

12–14 checks (platform-dependent), each with `{ok, detail, fix}`. The `fix` field is a literal command.
Exit 0 = healthy, exit 1 = at least one check failed. An AI agent should call the
`gbr_diagnose` tool and act on `nextActions[]` before doing anything else.

## Error codes

Every failure returns a JSON payload with `code`, `hint`, `correlation_id` and
`log_file`. Grep the JSONL by correlation id to see the full request/response pair.

| Code | Meaning | Fix |
|---|---|---|
| `GBR_UNREACHABLE` | Nothing listening on the Bot API | `gbr-agent run`. Verify: `curl -sS http://127.0.0.1:8788/` |
| `GBR_TIMEOUT` | Request exceeded `GBR_MCP_TIMEOUT_MS` | For `inject`: no live window on that `session_id`. See "inject hangs" below. |
| `GBR_NOT_FOUND` | Endpoint 404 | Agent older than 0.5.3. `gbr-agent version` |
| `GBR_UNAUTHORIZED` | 401/403 from relay | Stale mailbox key. Re-pair, copy key from phone Settings → Bot API |
| `GBR_RATE_LIMITED` | 429, relay cap is 60 injects/min/mailbox | Back off; batch prompts instead of many small injects |
| `GBR_AGENT_REFUSED` | HTTP 200 but `ok:false` | Read `body.error` — usually an empty or unknown `session_id` |
| `GBR_BAD_ARGS` | Rejected before any HTTP call | Missing `session_id` or `text` |
| `GBR_BAD_RESPONSE` | Non-JSON reply | Something other than gbr-agent is bound to 8788 |
| `GBR_HTTP_ERROR` | Other non-2xx | See `body` |
| `GBR_UNKNOWN_TOOL` | Bad tool name | `hint` lists valid names |

## Symptom → cause → fix

| Symptom | Cause | Fix |
|---|---|---|
| `zsh: command not found: gbr-agent` right after a successful install | Installer appended PATH to `~/.zshrc` but did not reload the current shell | `export PATH="$HOME/.local/bin:$PATH"` — or open a new terminal |
| `curl: (7) Failed to connect to 127.0.0.1 port 8788` | `gbr-agent run` is not alive. The Bot API only exists while `run` is in the foreground | `gbr-agent run`, or `gbr-agent service install` to persist |
| `--diagnose` says `gbr_agent_running` FAIL but `bot_api_reachable` PASS | You are pattern-matching `"gbr-agent run"` but the process is `gbr-agent -log=info run` — flags sit between binary and subcommand | Fixed in 0.1.0. If you see it, upgrade gbr-mcp |
| Two agents in `pgrep`, port fights | An old build left running after upgrade | `pkill -f gbr-agent` then start exactly one |
| `inject` hangs then `GBR_TIMEOUT` | No real Grok Build window attached to that `session_id`. The agent holds the request open | Run `gbr_sessions`. A session literally named `session` is the agent's own pseudo-session — injecting into it always hangs. Open a real window |
| `inject` returns 200 but nothing happens | Logical failure inside a 200. The agent returns `ok:false` with HTTP 200 | gbr-mcp already converts this to `GBR_AGENT_REFUSED`. Read `body.error` |
| Dispatched to a remote but it ran locally | Agent **0.5.x** unknown names silently fell back to `local`. **0.6.0+** returns `404 unknown_device` | Run `gbr_devices`. Route by id, name, or unique class (`linux` \| `pc` \| `laptop` \| `mac_mini`). Register remotes with `gbr-agent fleet add -name … -class …` |
| `GBR_UNAUTHORIZED` after upgrading the agent | The enforce relay requires both client and agent to hold a mailbox key; an upgrade invalidates the old pairing | Unpair in the phone app (Settings → **Unpair / Forget this PC**, not Disconnect), then `gbr-agent pair` again |
| Grok says `~/.claude.json → 0 servers` | Dedup, not failure — the server is also in `~/.grok/config.toml` | Harmless. Confirm with `grok mcp doctor` that the server shows `handshake OK` |
| Grok: server fails to start on first launch only | `npx` cold-download exceeds the default 30s `startup_timeout_sec` | Set `startup_timeout_sec = 120` in `~/.grok/config.toml`, or pre-warm once |
| MCP client shows garbled protocol errors | Something wrote to **stdout**. stdout is the transport | gbr-mcp only ever writes MCP frames to stdout; logs go to stderr. If you fork this, keep that invariant |
| No JSONL file appears | Log dir not writable | `--diagnose` reports `logging FAIL` with the reason. Set `GBR_MCP_LOG_DIR` |
| Mailbox key visible somewhere | Should be impossible | Redaction lives in `src/logger.js` and is covered by the test `mailbox key never echoed in response`. Report it as a bug |

## Reading the logs

**bash (macOS):**
```
cd ~ && tail -f ~/.gbr/logs/gbr-mcp-$(date +%F).jsonl
```

Trace one tool call end to end by its correlation id:

**bash (macOS):**
```
cd ~ && grep '"cid":"b893e0cb"' ~/.gbr/logs/gbr-mcp-$(date +%F).jsonl | python3 -m json.tool
```

Only failures:

**bash (macOS):**
```
cd ~ && grep '"level":"error"' ~/.gbr/logs/gbr-mcp-$(date +%F).jsonl
```

Maximum detail for one run:

**bash (macOS):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp && GBR_MCP_LOG_LEVEL=trace GBR_MCP_LOG_MAX_BODY=20000 npm test
```

The agent keeps its own trace log alongside: `~/.gbr/logs/agent-<date>.jsonl`.
When something is wrong end-to-end, read both — gbr-mcp shows what was asked, the
agent log shows what it did with it.

## Escalation

**bash (macOS):**
```
cd ~ && gbr-agent support-log
```

Writes a redacted bundle to `~/Downloads`. Attach that plus the relevant
`gbr-mcp-<date>.jsonl` lines. Never attach a raw mailbox key.
