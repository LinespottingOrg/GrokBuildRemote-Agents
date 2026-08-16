# Bot API — Grok bots and HTTP clients

**Agent / relay 0.5.3+.** One Grok bot instance drives **this PC locally** and **other Mac / Linux / Windows PCs** over the relay. The phone paired to the hub mailbox gets short status lines.

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
curl -sS http://127.0.0.1:8788/v1/sessions
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

`agent_online`, `last_seen`, `session_count`, `agent_version`, `sessions`.

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
# Pair each PC as usual (gbr-agent pair / run). Copy mailbox id+key from that PC
# (gbr-agent status, or that phone Settings → Bot API if it has its own pair).

# On the hub PC (the one the phone stays paired to):
gbr-agent fleet add -name studio-linux -mailbox gbr-XXXX -key KEY -os linux
gbr-agent fleet add -name mac-mini     -mailbox gbr-YYYY -key KEY -os darwin
gbr-agent fleet

# Grok bot — local OR remote, same JSON:
curl -sS http://127.0.0.1:8788/v1/devices
curl -sS -X POST http://127.0.0.1:8788/v1/inject \
  -d '{"device":"local","text":"fix tests","submit":true}'
curl -sS -X POST http://127.0.0.1:8788/v1/inject \
  -d '{"device":"studio-linux","text":"make test","submit":true}'
```

The phone on the **hub** mailbox shows short system lines:

`bot · studio-linux · inject queued · session grok-build-…`

Set `"notify_phone": false` to skip. The remote PC still runs the inject; only the chat peek is optional.

Fleet list is stored on the hub mailbox (relay) as well as `~/.gbr/fleet.json`, so a cloud Grok bot using the hub Bot URL sees the same devices.

## Grok bot recipe

1. Pair the **hub** phone + `gbr-agent pair` / `gbr-agent run`.
2. Pair every extra Mac/Linux PC. Add them with `gbr-agent fleet add`.
3. Point **one** Grok bot at `127.0.0.1:8788` or the hub relay Bot URL.
4. `GET /devices` → `POST /inject` with `device` → phone shows status.

Never put the mailbox key in a public issue, screenshot, or store listing.
