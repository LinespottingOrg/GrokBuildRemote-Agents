# Security

Desktop agent: MIT. Mobile apps: separate paid product.

Canonical index (FAQ + GitHub + trust): https://grokbuildremote.com/integrations.html

## Threat model (pairing)

- `gbr-agent pair` binds a **mailbox for this PC**, not a single CLI tab.
- After `gbr-agent run`, the phone can **list and inject every discovered terminal window** on that machine (roster, soft max 255).
- Headless HTTP agents are **not** in that roster. Pointing a sidecar at `127.0.0.1:8788` shows Bot API JSON, not a transcript.
- Anyone with the mailbox key (phone **Settings → Bot API**) can inject. Treat it like a password. Never commit it.
- `POST /v1/inject` types **arbitrary text** into listed TTYs and can submit.

## Trust boundary (loopback vs relay)

| Surface | Bind | Auth |
|---------|------|------|
| Bot API on this PC | `http://127.0.0.1:8788` after `gbr-agent run` | **Unauthenticated by default.** Set `GBR_BOT_REQUIRE_KEY=1` to require the mailbox key even on loopback. |
| HTTPS relay | `https://gbr-relay.ekobrott.workers.dev/v1/mb/{id}/bot` | **Always** `X-GBR-Key` or `Authorization: Bearer` |
| MCP stdio | `gbr-mcp` on the same machine | Local process; still talks to loopback `:8788` |

“Attach only loopback / stdio” is not the whole story. Anyone local on that PC can inject unless `GBR_BOT_REQUIRE_KEY=1`. Details: [docs/BOT-API.md](docs/BOT-API.md).

## Install

Prefer [docs/PINNED-INSTALL.md](docs/PINNED-INSTALL.md): checksum **the installer** (GitHub Release tag `v0.6.0`), then run it; it checksums the binary. Website `curl | bash` is a mutable convenience URL for humans on grokbuildremote.com only — refuse `VERSION=latest`. Do not publish pipe-to-shell in other projects’ official trees.

## Password managers (host CLI)

1Password (`op`) and LastPass (`lpass`) are **not** a pair protocol and **not** an attach surface. After the PC is already paired, the desktop agent may call those CLIs **on this host** (same pattern as Hermes using `gbr-mcp` locally). Plan: [docs/PASSWORD-MANAGERS.md](docs/PASSWORD-MANAGERS.md).

- Never inject vault fields into a listed TTY (`POST /v1/inject`) — the phone spectator would see them.
- Never return passwords from `gbr-mcp` or the Bot API / relay.
- Never commit `OP_SERVICE_ACCOUNT_TOKEN`, LastPass credentials, mailbox keys, or `device.json`.
- Do not PR into 1Password or LastPass official repos.

## Report

info@linespotting.com — no mailbox keys or `device.json` in the mail.
