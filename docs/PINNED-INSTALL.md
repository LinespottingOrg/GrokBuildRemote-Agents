# Pinned install (checksum the installer, then the binary)

`curl https://grokbuildremote.com/install.sh | bash` is a **mutable remote install**.
The website file can change. Do not paste that into other projects’ official docs.

Canonical path:

1. Download `install.sh` / `install.ps1` from a **GitHub Release tag** (v0.6.0).
2. Check **this script’s** SHA-256 (table below).
3. Run it. It then downloads the matching `gbr-agent` asset and checks `SHA256SUMS`.

Release: https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.0

## Installer SHA-256 (v0.6.0)

```
0a7963dc668750bfcb907bb72f6f6f8db30881b02636e417e08e102352309301  install.sh
b604a21b5dae5a874487a597778d15742b3c2afb2470c93a8e8ba0a76e486cdf  install.ps1
```

These hashes are also on the release (`install.sh.sha256`, `SHA256SUMS`) and at
https://grokbuildremote.com/integrations.html#trust

## macOS / Linux — verify the installer, then run it

```bash
VER=v0.6.0
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
SHA=0a7963dc668750bfcb907bb72f6f6f8db30881b02636e417e08e102352309301
curl -fsSL -o /tmp/gbr-install.sh "$BASE/install.sh"
echo "$SHA  /tmp/gbr-install.sh" | shasum -a 256 -c -
bash /tmp/gbr-install.sh
gbr-agent version   # must print v0.6.0+
```

Or skip the installer and fetch the binary yourself:

```bash
VER=v0.6.0
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
# darwin-arm64 shown; swap the asset for your OS/arch
curl -fsSL -o gbr-agent-darwin-arm64 "$BASE/gbr-agent-darwin-arm64"
curl -fsSL -o SHA256SUMS "$BASE/SHA256SUMS"
shasum -a 256 -c SHA256SUMS --ignore-missing
mkdir -p ~/.local/bin
install -m 0755 gbr-agent-darwin-arm64 ~/.local/bin/gbr-agent
gbr-agent version
```

## Linux amd64 (binary only)

```bash
VER=v0.6.0
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
curl -fsSL -o gbr-agent-linux-amd64 "$BASE/gbr-agent-linux-amd64"
curl -fsSL -o SHA256SUMS "$BASE/SHA256SUMS"
sha256sum -c SHA256SUMS --ignore-missing
install -m 0755 gbr-agent-linux-amd64 ~/.local/bin/gbr-agent
```

## Windows PowerShell — verify the installer, then run it

```powershell
$ver = "v0.6.0"
$sha = "b604a21b5dae5a874487a597778d15742b3c2afb2470c93a8e8ba0a76e486cdf"
$base = "https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$ver"
$i = Join-Path $env:TEMP "gbr-install.ps1"
Invoke-WebRequest "$base/install.ps1" -OutFile $i -UseBasicParsing
if ((Get-FileHash $i -Algorithm SHA256).Hash.ToLowerInvariant() -ne $sha) { throw "installer SHA-256 mismatch" }
& $i
gbr-agent version
```

## Binary SHA-256 (v0.6.0)

```
96cef605d3e030ccef99d27ea6240e0d3b668dd045e6b5b9e585c9fd03c6ef23  gbr-agent-darwin-amd64
de7e065ef2cf6877b3b2cd04679a67b627f876337f529247e236204543e4062c  gbr-agent-darwin-arm64
a50a5c41993e6531a3b477eb409ccc845212bf541384dc803061c80657f86719  gbr-agent-linux-amd64
5bfd22c7110234942c4c02ff8154b836d0af45a9422c178a4f52010187d40061  gbr-agent-linux-arm64
f773b89fd31310172b756e0593e0f3b2382b0a3440af2a7d0a8b3073b0c23e27  gbr-agent-windows-amd64.exe
8fb9efcbc7e2ac91c11964944bf0f45e31bb23f4356d9dcb4b305d7cb9b0fe8c  gbr-agent-windows-arm64.exe
```

## MCP (`gbr-mcp`) — pin the clone

Do not `git clone` default branch + `npm install` as a trust root.

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp
npm install
node bin/gbr-mcp.js --diagnose
```

## Convenience (mutable — our site only)

Website `https://grokbuildremote.com/install.sh` still works for humans. It **refuses**
`VERSION=latest`, downloads from the **tagged GitHub release**, and aborts on binary
SHA-256 mismatch. The script **itself** is still a live file. Pin with the recipes
above. Frozen filename aliases: `/install-v0.6.0.sh` · `/install-v0.6.0.ps1`.
