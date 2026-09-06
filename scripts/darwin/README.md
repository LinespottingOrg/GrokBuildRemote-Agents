# macOS — non-interactive Grok Build Remote LaunchAgent

**Product:** Grok Build Remote  
**Inbox:** [grok-build-inbox#75](https://github.com/LinespottingOrg/grok-build-inbox/issues/75) · Windows twin [PR #54](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/pull/54)  
**Related:** Agents PR #40 (ack-on-fail / single `command_id` — do not regress)

Same contract as Windows NI: **no Interactive UI**, halt on, no auto-open, one process. CLI binary stays `gbr-agent`. Login Items show **Grok Build Remote**.

**Flag order:** `-inject-halt` is a **`run` subcommand** flag. Correct: `-log=info run -inject-halt`.

## Hard rules

| Rule | Detail |
| --- | --- |
| Binary | `$HOME/.local/bin/gbr-agent` with PR #40 (`-inject-halt` / `GBR_INJECT_HALT`). Never `.aiprojects/gbr/agents/dist`. Refuse `commit=6f451ac`. |
| ProcessType | **Background** (not Interactive) |
| Inject | Default `GBR_INJECT_HALT=1` + `-inject-halt`. David clears halt for live inject. |
| No auto-open | Default `GBR_NO_AUTO_OPEN=1` |
| Inbox | Default `GBR_INBOX_WATCH=0` |
| Logs | `$HOME/pc-build/gbr-agent-out` (`GBR_LOG_DIR`) |
| Legacy | `com.linespotting.gbr-agent` → **bootout**, do **not** delete plist without David yes |
| NI label | `com.linespotting.grok-build-remote` (matches CFBundleIdentifier) |
| Secrets | Never in commits, scripts, plists, or PR bodies |

`gbr-agent service install` (Go) still kickstarts the **legacy** label and does not set halt. This folder is the supported NI path on Mac Mini.

## Install (Mac Mini / local Mac)

```bash
cd <repo>/scripts/darwin
chmod +x install-service.sh uninstall-service.sh kill-duplicates.sh
# Registers plist; does NOT start unless --start
./install-service.sh

# When David wants it up:
./install-service.sh --start
```

| Flag | Meaning |
| --- | --- |
| `--start` | `launchctl bootstrap` + `kickstart` (default **off**) |
| `--allow-inject` | **David live trial only** — skip halt |
| `--skip-disable-legacy` | Leave `com.linespotting.gbr-agent` loaded |
| `--binary PATH` | Override (still refuse dist / 6f451ac / stub) |
| `--log-dir PATH` | Default `$HOME/pc-build/gbr-agent-out` |

## Test checklist (Mac Mini)

- [ ] `./install-service.sh` **without** `--start`
- [ ] Plist at `~/Library/LaunchAgents/com.linespotting.grok-build-remote.plist`
- [ ] `ProcessType` = Background; args `-log=info run -inject-halt`
- [ ] Env halt + no-auto-open + inbox off; logs under `~/pc-build/gbr-agent-out`
- [ ] Legacy `com.linespotting.gbr-agent` bootout, plist **not** deleted
- [ ] Then `--start`: `curl -sS http://127.0.0.1:8788/health` 200; inject halted; no new Terminal/Grok windows
- [ ] Login Items show **Grok Build Remote** (`~/Applications/Grok Build Remote.app`)

Do not run this installer from PC1. Do not clear halt without David.
