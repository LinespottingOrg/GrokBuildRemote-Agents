# Codex — scanner-ready catalog (not Codex core)

The Codex / ChatGPT plugin manifest lives at repo root [`.codex-plugin/plugin.json`](../../.codex-plugin/plugin.json).

This is a **scanner-ready catalog entry** in *this* registry (`plugins/registry.json`). It is **not** OpenAI Codex core and is **not** submitted to [`openai/codex`](https://github.com/openai/codex). Do not open a PR there.

Public index: https://grokbuildremote.com/integrations.html

## Install (pinned)

1. Add **this** repo as a Codex marketplace (catalog). Pin `--ref` to a commit SHA when you freeze it:

```bash
codex plugin marketplace add LinespottingOrg/GrokBuildRemote-Agents --ref main
codex plugin install build-remote-agent
```

2. Pin-install the **agent binary** from GitHub Release **v0.6.0** + SHA-256. Do not `curl | bash`.

Recipes: [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md)

```bash
gbr-agent version    # v0.6.0+
gbr-agent pair && gbr-agent run
```

3. Codex talks to terminals via Bot API `http://127.0.0.1:8788` or `gbr-mcp` stdio (`mcp/gbr-mcp`, pin clone `--branch v0.6.0`).

The phone still only sees **terminal windows** on this PC. A Codex GUI / headless job is not a TTY unless it runs in one.
