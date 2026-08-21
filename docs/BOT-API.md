# Bot API — Grok bots and HTTP clients

**Agent / relay 0.5.4+.** One Grok Bot instance (public beta **2026-08-11**) *or* Claude Cowork (via `gbr-mcp`) drives **this PC locally** and **other Mac / Linux / Windows PCs** over the GitHub HTTPS relay. Same JSON. The phone paired to the hub mailbox is spectator + veto (status lines), not the orchestrator.

**Agent 0.6.0+ device classes.** `GET /v1/devices` includes `class`: `phone` | `linux` | `pc` | `laptop` | `mac_mini`. `POST /v1/inject` accepts an id, a name, or a **unique** class (`"device":"mac-mini"`). Unknown names return **404 `unknown_device`** (0.5.x used to fall back to local). Two hits for one class → **409 `ambiguous_device`**. `"device":"phone"` → **400 `cannot_inject_phone`**.

`GET /health` (and `GET /v1/health`) is the watchdog: roster quality `ok/stale/zombie`, plus loopback probes for companion remotes (Amnibro `:2421`, Farina `:7910`, ChrisP hub `:8787`). GBR Bot API stays on **`:8788`**.

| Surface | Who | Bind | Auth |
|---------|-----|------|------|
| **Local agent** | Grok Build / a coding agent on the **same PC** | `127.0.0.1:8788` only | Loopback. Optional `X-GBR-Key` (`GBR_BOT_REQUIRE_KEY=1` to require it). |
| **Relay** | A bot anywhere on the internet | `https://gbr-relay.ekobrott.workers.dev/v1/mb/{mailbox_id}/bot` | **Required** `X-GBR-Key` or `Authorization: Bearer <key>` |

Phone **Settings → Bot API** copies the relay URL, mailbox id, and mailbox key after pairing.

Treat the mailbox key like a password. Anyone who has it can type into the paired PC sessions.

## Local (same PC)

`gbr-agent run` listens on `http://127.0.0.1:8788` (override `-bot-port` / `GBR_BOT_PORT`, `0` = off).

```bash
gbr-agent bot                  # print curl examples
curl -sS http://127.0.0.1:8788/
curl -sS http://127.0.0.1:8788/v1/devices
curl -sS http://127.0.0.1:8788/v1/sessions
curl -sS "http://127.0.0.1:8788/v1/status?device=local"
curl -sS -X POST http://127.0.0.1:8788/v1/inject \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"SESSION","text":"hello from bot","submit":true}'
curl -sS 'http://127.0.0.1:8788/v1/output?session_id=SESSION&live=1'
curl -sS http://127.0.0.1:8788/v1/status
```

This is the path a Grok bot on the desktop should use. No inbound firewall hole. Not reachable from the phone or the internet.

## Relay (remote bot)

```bash
RELAY=https://gbr-relay.ekobrott.workers.dev
MB=gbr-yourmailbox
KEY=the-mailbox-key-from-phone-settings

curl -sS "$RELAY/v1/bot"                       # discovery, no key
curl -sS -H "X-GBR-Key: $KEY" "$RELAY/v1/mb/$MB/bot"
curl -sS -H "X-GBR-Key: $KEY" "$RELAY/v1/mb/$MB/bot/sessions"
curl -sS -H "X-GBR-Key: $KEY" -H 'Content-Type: application/json' \
  -X POST "$RELAY/v1/mb/$MB/bot/inject" \
  -d '{"session_id":"SESSION","text":"fix the tests","submit":true}'
# { "ok": true, "command_id": "…", "queued": true }

curl -sS -H "X-GBR-Key: $KEY" \
  "$RELAY/v1/mb/$MB/bot/output?command_id=THAT_ID"
curl -sS -H "X-GBR-Key: $KEY" "$RELAY/v1/mb/$MB/bot/status"
```

Bearer also works: `Authorization: Bearer $KEY`.

Relay inject is rate-limited (60/min per mailbox). The agent must be running (`gbr-agent run`) to consume the queued inject.

## JSON

### `POST …/inject`

```json
{
  "device": "local",
  "session_id": "grok-build-40a22",
  "text": "the prompt to type",
  "submit": true,
  "mode": "text",
  "command_id": "optional-uuid"
}
```

Aliases: `prompt` / `nl_prompt` for `text`, `session` for `session_id`.

### `GET …/sessions`

```json
{ "ok": true, "sessions": [{ "session_id": "…", "title": "Phone Grok" }], "replace": true }
```

### `GET …/output`

`items[]`: `{ ts, session_id, command_id, stream, chunk, eof, reason, method }`

### `GET …/status`

Hub fields: `ok`, `mailbox_id`, `agent_online`, `last_seen` (relay), `session_count`, `agent_version`, `os`, `sessions`, `bot`, `relay_bot`, `uptime_s`, `now`.

`devices[]`: `{ id, name, kind, mailbox_id, os, has_key, online? }`. Local is always `id=local`, `kind=local`, `online=true`. Remotes come from the hub fleet. `mailbox_key` is never included (`has_key` only). `online` is omitted or `false` when unknown — this process does not invent remote liveness.

`GET /v1/status?device=NAME` (or `GET /v1/devices/:id/status`) still returns the full `devices[]` plus `"device": { … }` for the selected machine. Unknown name → `404 {"error":"unknown_device"}`.

## One Grok bot · many PCs (0.5.3+)

The customer keeps **one** Grok bot. That bot talks to **one** API:

| Where the bot runs | URL |
|--------------------|-----|
| On a hub PC | `http://127.0.0.1:8788` |
| Anywhere else | hub mailbox `…/v1/mb/{hub}/bot` + hub `X-GBR-Key` |

`device` picks the machine:

- `local` / omitted — the hub PC (this agent / this mailbox)
- `studio-linux`, `mac-mini`, … — remotes you registered

```bash
# On each remote (no phone): gbr-agent pair-as-mailbox -name mac && gbr-agent service install
# On the hub PC (the one the phone stays paired to). Prefer the offer one-liner —
# it does not need the key on the command line:
gbr-agent fleet add -name mac -os darwin
gbr-agent fleet add -name studio-linux -mailbox gbr-XXXX -key KEY -os linux
gbr-agent fleet
# GET /v1/status?device=mac → 404 until fleet add. That is correct.

# List → pick → inject → status (same JSON local or remote):
curl -sS http://127.0.0.1:8788/v1/devices
curl -sS "http://127.0.0.1:8788/v1/status?device=local"
curl -sS -X POST http://127.0.0.1:8788/v1/inject \
  -d '{"device":"local","text":"fix tests","submit":true}'
curl -sS -X POST http://127.0.0.1:8788/v1/inject \
  -d '{"device":"studio-linux","text":"make test","submit":true}'
curl -sS http://127.0.0.1:8788/v1/status
```

The phone on the **hub** mailbox shows short system lines:

`bot · studio-linux · inject queued · session grok-build-…`

Set `"notify_phone": false` to skip. The remote PC still runs the inject; only the chat peek is optional.

Fleet list is stored on the hub mailbox (relay) as well as `~/.gbr/fleet.json`, so a cloud Grok bot using the hub Bot URL sees the same devices.

## Feedback loop (0.5.4+) — Grok bot **and** Claude Cowork

Same contract on `http://127.0.0.1:8788` and `…/v1/mb/{hub}/bot`. MCP tools map 1:1 (`gbr_open`, `gbr_lock`, `gbr_inject`, `gbr_result`, `gbr_tasks`).

```
diagnose → open/attach → lock → inject → wait idle → harvest excerpt → judge → iterate or close
```

| Step | HTTP | MCP |
|------|------|-----|
| See what's there | `GET /` + `GET /v1/sessions` | `gbr_diagnose` then `gbr_sessions` |
| Start or attach | `POST /v1/sessions/open` | `gbr_open` |
| Exclusive window | `POST /v1/lock` | `gbr_lock` `action=acquire` |
| Type the task | `POST /v1/inject` | `gbr_inject` / `gbr_inject_and_wait` |
| Wait + excerpt | `GET /v1/result?session_id=&wait_ms=60000` | `gbr_result` |
| Persist the loop | `GET\|POST /v1/tasks` | `gbr_tasks` |
| Let go | `DELETE /v1/lock` | `gbr_lock` `action=release` |

Rules that keep the two ecosystems from fighting:

- **Do not hide `unknown-*`.** Those windows are injectable. Only the literal id `session` is the agent pseudo-session (inject hangs).
- **One lease per window.** Grok bot and Claude Cowork must not share a `session_id` without `steal=true`. Holders: `grok-bot`, `claude-coworker`, `phone`.
- **Do not wait for full Grok TUI scrollback.** `/result` stops on a prompt, ~2.5s of quiet output, or `wait_ms`. Empty excerpt on a Grok UI window is honest — judge what you have.
- **Phone is spectator.** Short `bot · local · open|lock|inject` lines. It does not orchestrate.
- **Relay `/result` is a snapshot harvest.** Local `127.0.0.1:8788/v1/result?wait_ms=` is the one that actually waits for idle.

```bash
# Open (or attach) and take a lease
curl -sS -X POST http://127.0.0.1:8788/v1/sessions/open \
  -H 'Content-Type: application/json' \
  -d '{"cwd":"'"$PWD"'","holder":"grok-bot","goal":"fix the tests"}'
# { session_id, opened|attached, lock: {holder, token, expires}, task? }

# Work + wait until idle, then read the tail
curl -sS -X POST http://127.0.0.1:8788/v1/inject \
  -d '{"session_id":"SESSION","text":"fix the failing tests","submit":true,"wait_idle":true,"wait_ms":60000}'

curl -sS 'http://127.0.0.1:8788/v1/result?session_id=SESSION&wait_ms=60000'
# { state: idle|busy|timeout, excerpt, prompt, lock }

# Other client is blocked until you release
curl -sS -X DELETE 'http://127.0.0.1:8788/v1/lock?session_id=SESSION&holder=grok-bot'
```

`POST /v1/sessions/open` with `"resume":"<uuid>"` runs `grok --resume`. `"attach":true` only leases an existing id. `"command":"shell"` opens a managed shell if the grok CLI is missing.

## Grok bot recipe

1. Pair the **hub** phone + `gbr-agent pair` / `gbr-agent run`.
2. Pair every extra Mac/Linux PC. Add them with `gbr-agent fleet add`.
3. Point **one** Grok bot at `127.0.0.1:8788` or the hub relay Bot URL.
4. Run the loop above (`open` → `lock` → `inject` → `result`). Phone shows status.

Never put the mailbox key in a public issue, screenshot, or store listing.
