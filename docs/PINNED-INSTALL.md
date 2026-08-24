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

Or skip the installer and fetch the binary yourself. **Pin the digest from
the table below** (do not treat a co-downloaded `SHA256SUMS` as the only
trust root — that file can drift if release assets are replaced).

```bash
VER=v0.6.0
ASSET=gbr-agent-darwin-arm64
SHA=7baa1a8e214cd71b60e3f2b5063713e00ff740939749c3cab3d702784a1432f8
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
curl -fsSL -o "$ASSET" "$BASE/$ASSET"
echo "$SHA  $ASSET" | shasum -a 256 -c -
mkdir -p "$HOME/.local/bin"
install -m 0755 "$ASSET" "$HOME/.local/bin/gbr-agent"
export PATH="$HOME/.local/bin:$PATH"
gbr-agent version   # must print v0.6.0
```

## Linux amd64 (binary only)

```bash
VER=v0.6.0
ASSET=gbr-agent-linux-amd64
SHA=fb54724367882497f2e8e05e40ecdeb4be29e008e6c865fc5c426cf464e6ad6e
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
curl -fsSL -o "$ASSET" "$BASE/$ASSET"
echo "$SHA  $ASSET" | sha256sum -c -
mkdir -p "$HOME/.local/bin"
install -m 0755 "$ASSET" "$HOME/.local/bin/gbr-agent"
export PATH="$HOME/.local/bin:$PATH"
gbr-agent version
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

Hashed from the GitHub Release assets on **2026-08-24**. `SHA256SUMS` on
that tag was regenerated the same day so it matches these files. If you
re-upload a binary, regenerate `SHA256SUMS` and every `*.sha256` in the
same turn.

```
62673a6856342a87d4a2a659bc1de92200aa19a5b60d88d252254940820f0b7f  gbr-agent-darwin-amd64
7baa1a8e214cd71b60e3f2b5063713e00ff740939749c3cab3d702784a1432f8  gbr-agent-darwin-arm64
fb54724367882497f2e8e05e40ecdeb4be29e008e6c865fc5c426cf464e6ad6e  gbr-agent-linux-amd64
9e9d7ca45bb0c4ded9d04226136013e9b64ae30f16bcf03069d35e9c38171cb9  gbr-agent-linux-arm64
40355b2be6cd68f3be68f2a06dfd30307ec1a60f16f87f1d6174012b35aa4a49  gbr-agent-windows-amd64.exe
8fb9efcbc7e2ac91c11964944bf0f45e31bb23f4356d9dcb4b305d7cb9b0fe8c  gbr-agent-windows-arm64.exe
```

## MCP (`gbr-mcp`) — pin the clone

Do not `git clone` default branch + `npm install` as a trust root.

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp
npm install --ignore-scripts
MCP_PATH="$(pwd)/bin/gbr-mcp.js"
node "$MCP_PATH" --diagnose
```

Put the **absolute** `$MCP_PATH` in host MCP config. Relative `gbr-mcp.js`
paths are CWD-dependent. Never put mailbox keys in MCP config.

## Convenience (mutable — our site only)

Website `https://grokbuildremote.com/install.sh` still works for humans. It **refuses**
`VERSION=latest`, downloads from the **tagged GitHub release**, and aborts on binary
SHA-256 mismatch. The script **itself** is still a live file. Pin with the recipes
above. Frozen filename aliases: `/install-v0.6.0.sh` · `/install-v0.6.0.ps1`.
