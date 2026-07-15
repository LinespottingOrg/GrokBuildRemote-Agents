# Grok Build Remote — Windows agent install helper
# Installs gbr-agent.exe to %LOCALAPPDATA%\gbr\ and optionally starts it.
# Requires: PowerShell 5.1+

$ErrorActionPreference = "Stop"
$Product = "Grok Build Remote"
$InstallDir = Join-Path $env:LOCALAPPDATA "gbr"
$BinName = "gbr-agent.exe"

Write-Host "$Product — Windows install" -ForegroundColor Green

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

# Prefer repo-local dist if present (dev machine)
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoDist = Join-Path $here "..\..\dist\gbr-agent.exe"
$repoDist = [IO.Path]::GetFullPath($repoDist)

$src = $null
if (Test-Path $repoDist) {
  $src = $repoDist
  Write-Host "Using local build: $src"
} elseif ($env:GBR_AGENT_PATH -and (Test-Path $env:GBR_AGENT_PATH)) {
  $src = $env:GBR_AGENT_PATH
  Write-Host "Using GBR_AGENT_PATH: $src"
} else {
  # Download from GitHub release (private repo needs GH_TOKEN)
  $repo = if ($env:GBR_REPO) { $env:GBR_REPO } else { "LinespottingOrg/GrokBuildRemote-Agents" }
  $tag = if ($env:GBR_VERSION) { $env:GBR_VERSION } else { "latest" }
  $asset = "gbr-agent-windows-amd64.exe"
  $headers = @{ "User-Agent" = "gbr-install" }
  if ($env:GH_TOKEN) { $headers["Authorization"] = "Bearer $($env:GH_TOKEN)" }
  elseif ($env:GITHUB_TOKEN) { $headers["Authorization"] = "Bearer $($env:GITHUB_TOKEN)" }

  if ($tag -eq "latest") {
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers $headers
    $tag = $rel.tag_name
    $url = ($rel.assets | Where-Object { $_.name -eq $asset } | Select-Object -First 1).browser_download_url
  } else {
    $url = "https://github.com/$repo/releases/download/$tag/$asset"
  }
  if (-not $url) { throw "Release asset $asset not found for $tag (set GH_TOKEN for private repos)" }
  $tmp = Join-Path $env:TEMP $asset
  Write-Host "Downloading $url"
  Invoke-WebRequest -Uri $url -Headers $headers -OutFile $tmp
  $src = $tmp
}

$dest = Join-Path $InstallDir $BinName
Copy-Item -Force $src $dest
Write-Host "Installed: $dest"

# PATH hint
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
  Write-Host "Added $InstallDir to User PATH (open a new terminal)"
}

Write-Host ""
Write-Host "Next:" -ForegroundColor Cyan
Write-Host "  gbr-agent pair -code YOURCODE"
Write-Host "  gbr-agent -log=info run"
Write-Host "  gbr-agent status"
