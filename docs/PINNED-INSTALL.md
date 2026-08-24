# Pinned install (checksum the installer, then the binary)

`curl https://grokbuildremote.com/install.sh | bash` is a **mutable remote install**.
The website file can change. Do not paste that into other projects’ official docs.

**Pin is v0.6.2.** Do not rewrite GitHub Release **v0.6.0** or **v0.6.1** assets.
v0.6.0 sums on GitHub were regenerated after publish; v0.6.1 is a docs train with
incomplete assets. v0.6.2 is the immutable binary pin.

Canonical path (preferred — **installer SHA-256 is the trust anchor**):

1. Download `install.sh` / `install.ps1` from GitHub Release **v0.6.2**.
2. Check **this script’s** SHA-256 (table below).
3. Run it. It then downloads the matching `gbr-agent` asset and checks `SHA256SUMS`.

A co-downloaded `SHA256SUMS` from the **same** GitHub release is **not** a trust
anchor — binary and sums can be replaced together. Skip `curl SHA256SUMS && shasum
-c`. For binary-only installs, compare against a **hard-coded digest** from this
file (table below).

Release: https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.2

## Installer SHA-256 (v0.6.2)

```
f91ce49afbc21ac51ccf8b69b95ee407ff2d8a60926e2868bb192bb03eca796d  install.sh
572e7008400d16e7b666ba42b6fcd81ef03c0ab45190a1151f09f6d97716b5e1  install.ps1
```

These hashes are also on the release (`install.sh.sha256`, `SHA256SUMS`) and at
https://grokbuildremote.com/integrations.html#trust

## macOS / Linux — verify the installer, then run it

```bash
VER=v0.6.2
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
SHA=f91ce49afbc21ac51ccf8b69b95ee407ff2d8a60926e2868bb192bb03eca796d
curl -fsSL -o /tmp/gbr-install.sh "$BASE/install.sh"
echo "$SHA  /tmp/gbr-install.sh" | shasum -a 256 -c -
bash /tmp/gbr-install.sh
export PATH="$HOME/.local/bin:$PATH"
gbr-agent version   # must print v0.6.2
```

## Binary only — hard-coded digest (darwin-arm64)

Do **not** `curl SHA256SUMS && shasum -c`. Compare against the digest below.
A fresh Mac does not have `~/.local/bin` on `PATH`.

```bash
VER=v0.6.2
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
SHA=3202b775dec80600005dd1df78c717e1909320b958563f08cd96d4db7a819c01
curl -fsSL -o gbr-agent-darwin-arm64 "$BASE/gbr-agent-darwin-arm64"
echo "$SHA  gbr-agent-darwin-arm64" | shasum -a 256 -c -
mkdir -p ~/.local/bin
install -m 0755 gbr-agent-darwin-arm64 ~/.local/bin/gbr-agent
export PATH="$HOME/.local/bin:$PATH"
gbr-agent version
```

Swap the asset name and digest for other OS/arch (table below).

## Linux amd64 (binary only)

```bash
VER=v0.6.2
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
SHA=2590be27b2461deb1e9859109347eb9bf2531c67811f68949eea74af5134a9dd
curl -fsSL -o gbr-agent-linux-amd64 "$BASE/gbr-agent-linux-amd64"
echo "$SHA  gbr-agent-linux-amd64" | sha256sum -c -
mkdir -p ~/.local/bin
install -m 0755 gbr-agent-linux-amd64 ~/.local/bin/gbr-agent
export PATH="$HOME/.local/bin:$PATH"
gbr-agent version
```

## Windows PowerShell — verify the installer, then run it

```powershell
$ver = "v0.6.2"
$sha = "572e7008400d16e7b666ba42b6fcd81ef03c0ab45190a1151f09f6d97716b5e1"
$base = "https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$ver"
$i = Join-Path $env:TEMP "gbr-install.ps1"
Invoke-WebRequest "$base/install.ps1" -OutFile $i -UseBasicParsing
if ((Get-FileHash $i -Algorithm SHA256).Hash.ToLowerInvariant() -ne $sha) { throw "installer SHA-256 mismatch" }
& $i
gbr-agent version
```

## Binary SHA-256 (v0.6.2)

Built at commit `24ff503`. Do not replace these assets on the v0.6.2 tag.

```
594568b27d4eb69fa230800017db6ea54ae06c7f1548d5a99a19080c23ca240d  gbr-agent-darwin-amd64
3202b775dec80600005dd1df78c717e1909320b958563f08cd96d4db7a819c01  gbr-agent-darwin-arm64
2590be27b2461deb1e9859109347eb9bf2531c67811f68949eea74af5134a9dd  gbr-agent-linux-amd64
a7b2f5750b3bb8f97fa94fcafa027e5480342a43f0d1b7d934a977289000dfbf  gbr-agent-linux-arm64
6dfbf4a706a71482355aad469549088af103938f3d1abe0c896a1b34c657bb67  gbr-agent-windows-amd64.exe
f5d45d8cc7f784288597632e828506e68c3c1e8b98fabd424213eff912311c7c  gbr-agent-windows-arm64.exe
```

## Trust

- Loopback Bot API `http://127.0.0.1:8788` is **unauthenticated** unless
  `GBR_BOT_REQUIRE_KEY=1`.
- `POST /v1/inject` types into listed TTYs and can submit.
- Relay `https://gbr-relay.ekobrott.workers.dev` is outbound HTTPS. It sees
  session titles and output for the mailbox.
- macOS inject needs **Accessibility** permission for the agent.
- Do not paste `curl | bash` website installer into other projects’ official docs.

More: [SECURITY.md](../SECURITY.md) · [BOT-API.md](BOT-API.md)

## MCP (`gbr-mcp`) — pin the clone

Do not `git clone` default branch + `npm install` as a trust root.

```bash
git clone --branch v0.6.2 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp
npm install --ignore-scripts
node bin/gbr-mcp.js --diagnose
```

## Convenience (mutable — our site only)

Website `https://grokbuildremote.com/install.sh` still works for humans. It **refuses**
`VERSION=latest`, downloads from the **tagged GitHub release**, and aborts on binary
SHA-256 mismatch. The script **itself** is still a live file. Pin with the recipes
above. Frozen aliases: `/install-v0.6.2.sh` · `/install-v0.6.2.ps1`
(historical `/install-v0.6.0.sh` left in place; do not refresh that file).
