---
name: gbr-password-managers
description: >
  Use 1Password CLI (op) or LastPass CLI (lpass) on a PC already paired with
  Build Remote Agent. Host-side only, like Hermes MCP. Not a pair protocol.
  Phone never sees vault items. No SDK and no secrets in this file.
compatibility: >
  Requires gbr-agent ≥ 0.6.0 already paired (QR + 8-char + run).
  Attach stays :8788 / gbr-mcp. Unaffiliated with 1Password and LastPass.
metadata:
  version: "0.6.0"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/integrations.html"
---

# 1Password / LastPass — host CLI (not a pair)

One adapter. Protocol `gbr/1`. No fourth pair protocol.

Unaffiliated. This skill lives **here**, not in 1Password or LastPass official trees.
Plan: [docs/PASSWORD-MANAGERS.md](../../docs/PASSWORD-MANAGERS.md)

| Surface | Role |
|---------|------|
| `gbr-agent pair` then `run` | The **only** pair |
| Bot API `:8788` / `gbr-mcp` | The **only** attach |
| `op` / `lpass` on this PC | Host CLI after the user is already signed in |
| Phone | Spectator. Never a vault. |

## Pair (unchanged)

Checksummed install: [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md). Then:

```bash
gbr-agent version    # v0.6.0+
gbr-agent pair && gbr-agent run
```

Phone: Build Remote Agent → scan QR **or** type the 8-char code. Unpair in Settings before a new mailbox.

## After pair: call the CLI on the host

Same pattern as Hermes attaching `gbr-mcp` on the **host** (not in a sandbox, not on the phone).

**1Password** (official CLI — user already unlocked the desktop app / `op signin`):

```bash
op account list
op user get --me
op run --env-file=./.env.tpl -- your-command
```

**LastPass** (official `lastpass-cli` — user already `lpass login` on this machine):

```bash
lpass status
```

Placeholders only. Never put tokens, master passwords, or item values in skills, git, issues, or chat.

## Never

- Invent a 1Password / LastPass pair QR or mailbox.
- `gbr_inject` / `POST /v1/inject` a password, TOTP, or `op read` output into a listed TTY (the phone would see it).
- Return vault fields from `gbr-mcp` or the Bot API / relay.
- Commit `OP_SERVICE_ACCOUNT_TOKEN` or LastPass credentials.
- Open PRs in 1Password or LastPass official repos.

`op item get`, `op read`, and `lpass show --password` print secrets. If you must use them, do it in a **non-listed** local shell, not a GBR session.

Loopback `:8788` is unauthenticated unless `export GBR_BOT_REQUIRE_KEY=1`.

## Attach (only these)

| How | Where |
|-----|--------|
| Bot API | `http://127.0.0.1:8788` after `gbr-agent run` |
| MCP | `gbr-mcp` stdio |

Phone is spectator + veto. Never commit `mailbox_key`.
