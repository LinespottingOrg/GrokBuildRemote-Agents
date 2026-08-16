# Bot API — Grok bots and HTTP clients

**Agent / relay 0.5.2+.** Two HTTP surfaces, same JSON shape.

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

## Grok bot recipe

1. Pair phone + `gbr-agent pair` / `gbr-agent run` as usual.
2. **On the PC:** call `http://127.0.0.1:8788`.
3. **From the cloud:** copy Bot URL + key from phone Settings → Bot API, then call the relay.
4. List sessions → inject → poll output with `command_id`.

Never put the mailbox key in a public issue, screenshot, or store listing.
