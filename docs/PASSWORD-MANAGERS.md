# 1Password / LastPass on a GBR host

**Status:** adapter plan + skill stub. No SDK. No fourth pair protocol.

Unaffiliated. This file lives **here**. We do **not** PR into 1Password or LastPass official repos.

Skill: [skills/password-managers/SKILL.md](../skills/password-managers/SKILL.md)  
Pair + attach: [COMPATIBILITY.md](../COMPATIBILITY.md) · [SECURITY.md](../SECURITY.md)

## Fit (one adapter)

Password managers are **host CLIs**, the same class as `gh` and `git`. They are **not** pairing peers and **not** a second attach surface.

```
Phone  --gbr/1 QR+8-char-->  gbr-agent run
                                 │
                    attach only: :8788  /  gbr-mcp
                                 │
                    already signed-in on this PC:
                      op  (1Password CLI)
                      lpass (LastPass CLI)
```

| Layer | Stays |
|-------|--------|
| Pair | `gbr-agent pair` (QR **and** 8-char) then `gbr-agent run`. No fourth protocol. |
| Attach | Bot API `http://127.0.0.1:8788` or `gbr-mcp` stdio. Nothing else. |
| Phone | Spectator + veto. Never a vault UI. Never vault items. |
| Secrets | Stay on the host. Never git, issues, chat, MCP tool results, relay, or TTY inject. |

Hermes / OpenClaw / NemoClaw already attach **host-side** (`gbr-mcp` stdio or loopback `:8788`). The password CLI is the same pattern: the **desktop** agent calls `op` / `lpass` on the machine that is already paired. The phone does not participate.

## Do not

- Pair 1Password or LastPass as a GBR session / mailbox / device class.
- Add `gbr_secret_get` (or any tool that **returns** a password over MCP or Bot API).
- `POST /v1/inject` a vault item into a listed TTY — the phone spectator would see it.
- Put `OP_SERVICE_ACCOUNT_TOKEN`, LastPass master passwords, or mailbox keys in this repo.
- Open PRs in `1Password/*` or `lastpass/lastpass-cli`.

## 1Password (preferred host path)

Official CLI: https://developer.1password.com/docs/cli/reference  
Desktop app integration (biometric unlock) is the usual sign-in. Service accounts (`OP_SERVICE_ACCOUNT_TOKEN`) are optional and **host-local only**.

Placeholders only:

```bash
# Already signed in on this PC (desktop app + CLI, or prior `op signin`).
op account list
op user get --me

# Secrets as env for a child process — do not print them.
op run --env-file=./.env.tpl -- your-build-command

# Template inject to a file the phone cannot see.
op inject --in-file=./app.env.tpl --out-file=./app.env
```

`op item get` / `op read op://Vault/Item/field` print plaintext. Do **not** run those in a terminal that GBR lists, and do not paste the output into `gbr_inject`.

## LastPass (host CLI)

Official open-source CLI: https://github.com/lastpass/lastpass-cli (`lpass`).  
Runs on GNU/Linux, macOS, Cygwin. Not a native Windows binary.

```bash
lpass status
lpass ls "Group/"          # titles / ids — still avoid in a spectated TTY if names are sensitive
```

`lpass show --password` prints plaintext. Same rule as `op read`: never inject, never MCP-return, never log.

LastPass also has unofficial APIs. **Do not** wrap those in GBR. Stick to the official CLI the user already logged into on the host.

## Later (not this pass)

If a thin MCP wrapper is ever useful, it stays **host-side** and **non-secret**:

| Tool (hypothetical) | Returns |
|---------------------|---------|
| `gbr_secret_status` | `{ok, cli: "op"|"lpass", signed_in: bool}` — no item bodies |
| `gbr_secret_run` | spawn `op run -- <argv>` and return **exit code + non-secret stdout** |

Hard no: a tool that returns field values. Hard no: sending vault bytes through the HTTPS relay. Harden loopback with `GBR_BOT_REQUIRE_KEY=1` before any wrapper.

## Outreach lesson

PRs that drop GBR install pages into other projects’ official trees get rejected (promotion + mutable `curl \| bash`). Password-manager support follows the same rule: **this repo + grokbuildremote.com/integrations.html**. Short unaffiliated links only. No spam into 1Password / LastPass GitHub.
