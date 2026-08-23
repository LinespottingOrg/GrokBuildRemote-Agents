# Security

Desktop agent: MIT. Mobile apps: separate paid product.

## Threat model (pairing)

- `gbr-agent pair` binds a **mailbox for this PC**, not a single CLI tab.
- After `gbr-agent run`, the phone can **list and inject every discovered terminal window** on that machine (roster, soft max 255).
- Headless HTTP agents are **not** in that roster. Pointing a sidecar at `127.0.0.1:8788` shows Bot API JSON, not a transcript.
- Anyone with the mailbox key (phone **Settings → Bot API**) can inject. Treat it like a password. Never commit it.

## Install

Prefer [docs/PINNED-INSTALL.md](docs/PINNED-INSTALL.md) (GitHub Release + SHA-256). Do not publish `curl | bash` in other projects’ official trees.

## Report

info@linespotting.com — no mailbox keys or `device.json` in the mail.
