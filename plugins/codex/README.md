# Codex — scanner-ready catalog (not Codex core)

The Codex / ChatGPT plugin manifest lives at [`.codex-plugin/plugin.json`](../../.codex-plugin/plugin.json).

This is a **catalog entry in this registry**. It is **not** OpenAI Codex core. **Do not PR `openai/codex`.**

Index: https://grokbuildremote.com/integrations.html

## Install (pinned v0.6.2)

```bash
codex plugin marketplace add LinespottingOrg/GrokBuildRemote-Agents --ref main
codex plugin install build-remote-agent
```

Then pin-install the **agent binary** from GitHub Release **v0.6.2** + SHA-256 — [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md). Do not `curl | bash`.

```bash
gbr-agent version    # v0.6.2
gbr-agent pair
gbr-agent run
```

Attach: Bot API `http://127.0.0.1:8788` or `gbr-mcp` stdio (`mcp/gbr-mcp`, clone `--branch v0.6.2`). The phone lists **terminal windows** on this PC. A Codex GUI is not a TTY unless it runs in one.

No fourth pair protocol. No mailbox keys in this file.
