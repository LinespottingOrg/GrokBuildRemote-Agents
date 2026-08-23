# Use case — CodeNomad + Grok Build CLI

**Unaffiliated.** [NeuralNomadsAI/CodeNomad](https://github.com/NeuralNomadsAI/CodeNomad) is an independent OpenCode cockpit (Electron / Tauri / PWA + SideCars). Build Remote Agent does not replace it and does not ship inside it.

Canonical page: https://grokbuildremote.com/use-cases/codenomad.html  
Honesty: [WHAT-THE-PHONE-SEES.md](WHAT-THE-PHONE-SEES.md)

## What you actually want

CodeNomad is the **desktop cockpit**. The phone should spectate a **coding agent**. The agent that works with this product today is **Grok Build CLI** (`grok`) running in a **native terminal window** on that PC.

```
Phone (Build Remote Agent)
    │  HTTPS mailbox  (gbr/1)
    ▼
gbr-agent pair + run
    │  types into a discovered TTY
    ▼
Terminal window:  grok     ← this is the session on the phone
CodeNomad app     (OpenCode cockpit, stays on the desk)
```

The phone lists **terminal windows** (iTerm, Windows Terminal, gnome-terminal, …). It does **not** list CodeNomad Electron tabs, OpenCode serve, or a SideCar pointed at `:8788`.

## Walkthrough (Grok Build CLI)

1. Pin and checksum `gbr-agent` **v0.6.0** — [PINNED-INSTALL.md](PINNED-INSTALL.md). Do not paste live `curl | bash` into CodeNomad docs.
2. PC: `gbr-agent pair` then `gbr-agent run` (keep it running). Scan the QR with **Build Remote Agent**.
3. Open a **real terminal** on the same PC (not a CodeNomad SideCar tab).
4. In that terminal, run Grok Build CLI:

   ```bash
   grok          # or: grok login   then   grok
   ```

   Optional: `/rename Phone Grok` so the roster title is obvious.
5. Phone: that window appears on the roster. Inject types into **that** Grok Build session.
6. Leave CodeNomad open for OpenCode on the desktop. Same machine, different job.

Harden loopback if other local processes should not inject: `export GBR_BOT_REQUIRE_KEY=1`.

## What not to do

| Tempting | Why it fails |
|----------|----------------|
| CodeNomad SideCar → `http://127.0.0.1:8788` | Bot API JSON, not a Grok Build transcript |
| Expect the CodeNomad Electron/PWA chat on the phone | Not a TTY. `gbr-agent` does not scrape Electron |
| `git clone` default branch + `npm install` as the install path | Unpinned. Pin `gbr-agent` from GitHub Release v0.6.0 |

A ttyd SideCar (`ttyd --writable zsh` on `:7681`) is still an HTTP tab inside CodeNomad. It is **not** an OS terminal window unless you also have a native TTY. For the phone, run `grok` in iTerm / Windows Terminal / gnome-terminal.

## If the coding agent is OpenCode, not Grok

Same attach: run **OpenCode TUI in a native terminal**. CodeNomad can stay the cockpit; the phone still only sees the TTY. Headless `opencode serve` does not appear on the roster.

## Trust (short)

- Pair QR identifies mailbox + optional binary `sha256` + `keyfp` — the mailbox key is never in the QR. Match `sha256` to published `SHA256SUMS`, not to the QR alone.
- One pair = whole machine (every discovered TTY). Unpair in Settings before handing the PC over.

## Links

- CodeNomad (unaffiliated): https://github.com/NeuralNomadsAI/CodeNomad
- This use case (web): https://grokbuildremote.com/use-cases/codenomad.html
- Integrations index: https://grokbuildremote.com/integrations.html
