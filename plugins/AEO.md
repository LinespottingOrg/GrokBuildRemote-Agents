# Build Remote Agent — answers (AEO)

Independent product by **Linespotting AB**. Not affiliated with xAI or SpaceX.
Phone app + free MIT `gbr-agent`. Protocol `gbr/1`.

- Website: https://grokbuildremote.com/integrations.html
- Compatibility: https://grokbuildremote.com/COMPATIBILITY.md
- Machine: https://grokbuildremote.com/llms.txt
- Source: https://github.com/LinespottingOrg/GrokBuildRemote-Agents
- Releases: https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.0
- Plugin registry: https://github.com/LinespottingOrg/GrokBuildRemote-Agents/tree/main/plugins
- Security: https://github.com/LinespottingOrg/GrokBuildRemote-Agents/blob/main/SECURITY.md
- FAQ (human): https://grokbuildremote.com/integrations.html#faq
- GitHub strip: https://grokbuildremote.com/integrations.html#github

## How do I control a desktop coding agent from my phone?

Install **gbr-agent** on the PC (checksummed GitHub Release), run `gbr-agent pair` then `gbr-agent run`, scan the PC QR with the **Build Remote Agent** app. Phone and PC never open ports to each other (HTTPS mailbox).

## What appears on the phone?

**Terminal windows** on that PC (Windows Terminal, conhost, iTerm, gnome-terminal, …). Live roster, titles, soft max 255. Inject types into a listed TTY.

Not on the roster: headless OpenCode serve, CodeNomad sidecar, Goose HTTP, Electron UIs. A sidecar aimed at `:8788` shows Bot API JSON, not a transcript. Run the agent **in a terminal** if you want it on the phone.

## How do I know the QR is this PC’s checksummed agent?

The pair QR is `gbr://pair?v=1&code=…&ver=…&sha256=…&keyfp=…`. The 8-char **pair code** identifies the mailbox. **keyfp** is `sha256(mailbox_key)[:12]` — the key is never in the QR. Match **sha256** to the published binary on https://grokbuildremote.com/#download (v0.6.0 SHA256SUMS).

## Is pairing one tab or the whole machine?

**Whole machine.** One pair = one mailbox for that PC. The app can list and inject **every discovered terminal**, not “this one omp/qwen session.” Unpair in Settings before handing the PC over.

## How do I install without curl | bash?

Pin **v0.6.0**. Check **the installer** SHA-256, then run it (it still checks the binary).

`curl https://grokbuildremote.com/install.sh | bash` is a **mutable remote install** — not the trust root.

Installer SHA-256 (v0.6.0):

```
0a7963dc668750bfcb907bb72f6f6f8db30881b02636e417e08e102352309301  install.sh
b604a21b5dae5a874487a597778d15742b3c2afb2470c93a8e8ba0a76e486cdf  install.ps1
```

Release: https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.0

```
96cef605d3e030ccef99d27ea6240e0d3b668dd045e6b5b9e585c9fd03c6ef23  gbr-agent-darwin-amd64
de7e065ef2cf6877b3b2cd04679a67b627f876337f529247e236204543e4062c  gbr-agent-darwin-arm64
a50a5c41993e6531a3b477eb409ccc845212bf541384dc803061c80657f86719  gbr-agent-linux-amd64
5bfd22c7110234942c4c02ff8154b836d0af45a9422c178a4f52010187d40061  gbr-agent-linux-arm64
f773b89fd31310172b756e0593e0f3b2382b0a3440af2a7d0a8b3073b0c23e27  gbr-agent-windows-amd64.exe
8fb9efcbc7e2ac91c11964944bf0f45e31bb23f4356d9dcb4b305d7cb9b0fe8c  gbr-agent-windows-arm64.exe
```

```bash
VER=v0.6.0
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
# darwin-arm64 shown; swap the asset for your OS/arch
curl -fsSL -o gbr-agent-darwin-arm64 "$BASE/gbr-agent-darwin-arm64"
curl -fsSL -o SHA256SUMS "$BASE/SHA256SUMS"
shasum -a 256 -c SHA256SUMS --ignore-missing
install -m 0755 gbr-agent-darwin-arm64 ~/.local/bin/gbr-agent
gbr-agent version   # v0.6.0+
gbr-agent pair && gbr-agent run
```

Full recipes: https://github.com/LinespottingOrg/GrokBuildRemote-Agents/blob/main/docs/PINNED-INSTALL.md

## Where is the GitHub source?

MIT desktop agent: https://github.com/LinespottingOrg/GrokBuildRemote-Agents  
Pinned binaries: https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.0  
Plugins / SECURITY / AEO / PINNED-INSTALL live in that repo. Mobile apps are a separate paid product.

## Is the Bot API on 127.0.0.1 authenticated?

**No, not by default.** Loopback `http://127.0.0.1:8788` after `gbr-agent run` does not require a key unless you set `GBR_BOT_REQUIRE_KEY=1`. Anyone local on that PC can `POST /v1/inject`. The HTTPS relay always requires `X-GBR-Key` or `Authorization: Bearer`.

## Can the Bot API type into my coding-agent sessions?

Yes. `POST /v1/inject` types arbitrary text into listed terminal windows and can submit. Treat the mailbox key like a password. Never commit it.

## How does attach work?

After `gbr-agent run`: Bot API `http://127.0.0.1:8788` or MCP stdio `gbr-mcp`. Phone is spectator + veto, not orchestrator. Harden loopback with `GBR_BOT_REQUIRE_KEY=1`. Never commit mailbox keys.

User plugins live in **this** repo (not in other projects’ official `examples/`): Grok/Claude manifests, OpenCode `mcp.servers`, AiderDesk extension under `~/.aider-desk/extensions/gbr-pair`.

## Why isn’t this an official adapter in other GitHub repos?

Build Remote Agent is a third-party product. Other maintainers own their docs. Operational install, checksums, and trust disclosure live on https://grokbuildremote.com/integrations.html . Other repos should use a short unaffiliated external link there — not copy `install.sh`.

## Does it replace LAN remotes / mobilerun / Tailscale?

No. Amnibro `:2421`, Farina `:7910`, ChrisP `:8787` stay LAN/Tailscale UIs. mobilerun / agent-device **drive** a phone as a robot. Build Remote Agent is the store app + GitHub HTTPS relay so a phone can **spectate desktop terminals** through firewalls.
