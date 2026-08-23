# Pinned install (checksum)

Canonical install is a **GitHub Release asset + SHA-256**, not an unverified pipe-to-shell.

Website one-liners (`install.sh` / `install.ps1`) **download SHA256SUMS and refuse to install on mismatch**. Release **v0.6.0**:

https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.0

Checksums: `SHA256SUMS` on that release (same bytes as https://grokbuildremote.com/downloads/latest/SHA256SUMS).

```
96cef605d3e030ccef99d27ea6240e0d3b668dd045e6b5b9e585c9fd03c6ef23  gbr-agent-darwin-amd64
de7e065ef2cf6877b3b2cd04679a67b627f876337f529247e236204543e4062c  gbr-agent-darwin-arm64
a50a5c41993e6531a3b477eb409ccc845212bf541384dc803061c80657f86719  gbr-agent-linux-amd64
5bfd22c7110234942c4c02ff8154b836d0af45a9422c178a4f52010187d40061  gbr-agent-linux-arm64
f773b89fd31310172b756e0593e0f3b2382b0a3440af2a7d0a8b3073b0c23e27  gbr-agent-windows-amd64.exe
8fb9efcbc7e2ac91c11964944bf0f45e31bb23f4356d9dcb4b305d7cb9b0fe8c  gbr-agent-windows-arm64.exe
```

## macOS Apple Silicon

```bash
VER=v0.6.0
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
curl -fsSL -o gbr-agent-darwin-arm64 "$BASE/gbr-agent-darwin-arm64"
curl -fsSL -o SHA256SUMS "$BASE/SHA256SUMS"
shasum -a 256 -c SHA256SUMS --ignore-missing
mkdir -p ~/.local/bin
install -m 0755 gbr-agent-darwin-arm64 ~/.local/bin/gbr-agent
gbr-agent version    # must print v0.6.0+
```

## Linux amd64

```bash
VER=v0.6.0
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
curl -fsSL -o gbr-agent-linux-amd64 "$BASE/gbr-agent-linux-amd64"
curl -fsSL -o SHA256SUMS "$BASE/SHA256SUMS"
sha256sum -c SHA256SUMS --ignore-missing
install -m 0755 gbr-agent-linux-amd64 ~/.local/bin/gbr-agent
```

## Windows (PowerShell)

```powershell
$ver = "v0.6.0"
$base = "https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$ver"
Invoke-WebRequest "$base/gbr-agent-windows-amd64.exe" -OutFile gbr-agent-windows-amd64.exe
Invoke-WebRequest "$base/gbr-agent-windows-amd64.exe.sha256" -OutFile gbr-agent-windows-amd64.exe.sha256
Get-FileHash .\gbr-agent-windows-amd64.exe -Algorithm SHA256
Get-Content .\gbr-agent-windows-amd64.exe.sha256
# hashes must match; then copy to %LOCALAPPDATA%\GrokBuildRemote\
```

Website one-liners still exist; they now **verify SHA-256 before install**. They are still not what to paste into someone else’s official docs — link this file or the Release page instead.
