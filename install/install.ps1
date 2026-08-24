# Build Remote Agent — gbr-agent installer (Windows PowerShell)
#
# This file is a GitHub Release asset. The website copy at
# https://grokbuildremote.com/install.ps1 is a convenience mirror and CAN CHANGE.
# Do not pipe a live URL into iex as a trust root.
#
# Canonical (tag v0.6.2):
#   https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/v0.6.2/install.ps1
# Verify THIS script against the SHA-256 in docs/PINNED-INSTALL.md, then run it.
#
# Pinned: docs/PINNED-INSTALL.md
# Convenience (mutable): irm https://grokbuildremote.com/install.ps1 | iex

param(
  [string]$Version = $(if ($env:GBR_VERSION) { $env:GBR_VERSION } else { "v0.6.2" }),
  [string]$InstallDir = $(if ($env:GBR_INSTALL_DIR) { $env:GBR_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "GrokBuildRemote" }),
  [string]$Site = $(if ($env:GBR_SITE) { $env:GBR_SITE } else { "https://grokbuildremote.com" })
)

$ErrorActionPreference = "Stop"
$Product = "Build Remote Agent"
$Repo = if ($env:GBR_REPO) { $env:GBR_REPO } else { "LinespottingOrg/GrokBuildRemote-Agents" }
$Asset = "gbr-agent-windows-amd64.exe"
$BinName = "gbr-agent.exe"

if ($Version -match '^(latest|LATEST|Latest|main|master|HEAD)$') {
  throw "refusing mutable version '$Version'. Pin -Version v0.6.2 (docs/PINNED-INSTALL.md)"
}
if ($Version -notmatch '^v[0-9]+\.[0-9]+') {
  throw "version must look like v0.6.0 (got '$Version')"
}

$Base = if ($env:GBR_DOWNLOAD_BASE) { $env:GBR_DOWNLOAD_BASE } else { "https://github.com/$Repo/releases/download/$Version" }
$Url = "$Base/$Asset"

if ($MyInvocation.InvocationName -eq '.' -or $MyInvocation.Line -match 'iex|Invoke-Expression') {
  if ($env:GBR_I_TRUST_MUTABLE -ne '1') {
    Write-Host "==> piped installer (mutable URL). Binary SHA-256 is still verified." -ForegroundColor Yellow
    Write-Host "    pin this script: https://github.com/$Repo/blob/$Version/docs/PINNED-INSTALL.md"
  }
}

Write-Host "==> $Product agent installer (Windows)" -ForegroundColor Green
Write-Host "    version=$Version arch=amd64"
Write-Host "    download $Url"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$tmp = Join-Path $env:TEMP $Asset
$dest = Join-Path $InstallDir $BinName

try {
  Invoke-WebRequest -Uri $Url -OutFile $tmp -UseBasicParsing
} catch {
  throw "Download failed: $Url — open $Site/#download or GitHub Releases $Version"
}

function Get-ExpectedSha256([string]$BaseUrl, [string]$AssetName) {
  $sumsUrl = "$BaseUrl/SHA256SUMS"
  $sideUrl = "$BaseUrl/$AssetName.sha256"
  try {
    $text = (Invoke-WebRequest -Uri $sumsUrl -UseBasicParsing).Content
    foreach ($line in ($text -split "`r?`n")) {
      $parts = $line.Trim() -split "\s+", 2
      if ($parts.Count -ge 2) {
        $name = $parts[1].TrimStart("*")
        if ($name -eq $AssetName) { return $parts[0].ToLowerInvariant() }
      }
    }
  } catch { }
  try {
    $side = ((Invoke-WebRequest -Uri $sideUrl -UseBasicParsing).Content).Trim()
    $h = ($side -split "\s+")[0].ToLowerInvariant()
    if ($h -match '^[0-9a-f]{64}$') { return $h }
  } catch { }
  throw "No SHA-256 for $AssetName at $BaseUrl (need SHA256SUMS or $AssetName.sha256)"
}

$expected = Get-ExpectedSha256 $Base $Asset
$actual = (Get-FileHash -Algorithm SHA256 -Path $tmp).Hash.ToLowerInvariant()
if ($actual -ne $expected) {
  throw "SHA-256 mismatch for $Asset`n    expected $expected`n    got      $actual`n    url      $Url"
}
Write-Host "    checksum ok $actual" -ForegroundColor Green

Copy-Item -Force $tmp $dest
Write-Host "    installed: $dest" -ForegroundColor Green

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
  $env:Path = "$env:Path;$InstallDir"
  Write-Host "    added $InstallDir to User PATH (open a new terminal for other shells)"
}

Write-Host ""
Write-Host "==> Next commands" -ForegroundColor Cyan
Write-Host @"
  # 1) PC generates QR — phone camera scans it
  gbr-agent pair
  # browser opens QR; mobile app → Scan QR from computer

  # 2) Run the agent (keep this running)
  gbr-agent run

  # Useful
  gbr-agent sessions
  gbr-agent status
  gbr-agent version   # expect $Version

Pinned install: https://github.com/$Repo/blob/$Version/docs/PINNED-INSTALL.md
Docs: $Site/integrations.html#trust
Support: $Site/support.html
"@
