# 1Password / LastPass on a GBR host

**Not a pair.** Unaffiliated. Lives **here**. Do not PR into 1Password or LastPass official repos.

Skill: [skills/password-managers/SKILL.md](../skills/password-managers/SKILL.md)

```
Phone  --gbr/1 QR+8-char-->  gbr-agent run
                                 │
                    attach only: :8788  /  gbr-mcp
                                 │
                    already signed-in on this PC:
                      op     (1Password CLI)
                      lpass  (LastPass CLI)
```

| Layer | Stays |
|-------|--------|
| Pair | `gbr-agent pair` then `run`. No fourth protocol. |
| Attach | Bot API `:8788` or `gbr-mcp` stdio. |
| Phone | Remote-control client of **TTYs**. Never a vault. |
| Secrets | Host only. Never git, inject, MCP, or relay. |

## Do not

- Pair 1Password/LastPass as a mailbox or device class
- Add `gbr_secret_get`
- `POST /v1/inject` a vault item (the phone would see it)
- Commit `OP_SERVICE_ACCOUNT_TOKEN` or LastPass credentials

## Host CLIs (user already signed in)

```bash
op account list
op run --env-file=./.env.tpl -- your-build-command
lpass status
```

`op read` / `lpass show --password` print plaintext — not in a listed TTY.

Pin **v0.6.2**. Loopback `:8788` unauthenticated unless `GBR_BOT_REQUIRE_KEY=1`. Relay pair is keyless/throttled; push/poll/ack need `X-GBR-Key`.
