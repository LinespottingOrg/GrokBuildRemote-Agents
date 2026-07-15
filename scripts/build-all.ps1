# Cross-compile gbr-agent for all supported platforms (Windows PowerShell).
# Run from repo root:
#   pwsh -File .\scripts\build-all.ps1
#   .\scripts\build-all.ps1
[CmdletBinding()]
param(
    [string]$Binary = "gbr-agent",
    [string]$Pkg = "./cmd/gbr-agent",
    [string]$OutDir = "",
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $Root

if (-not $OutDir) {
    $OutDir = Join-Path $Root "dist"
}
if (-not $Version) {
    try {
        $Version = (git describe --tags --always --dirty 2>$null)
        if (-not $Version) { $Version = "dev" }
    } catch {
        $Version = "dev"
    }
}

try {
    $Commit = (git rev-parse --short HEAD 2>$null)
    if (-not $Commit) { $Commit = "none" }
} catch {
    $Commit = "none"
}

$Date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$Ldflags = "-s -w -X main.version=$Version -X main.commit=$Commit -X main.date=$Date"

$Targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64" },
    @{ GOOS = "darwin";  GOARCH = "amd64" },
    @{ GOOS = "darwin";  GOARCH = "arm64" },
    @{ GOOS = "linux";   GOARCH = "amd64" },
    @{ GOOS = "linux";   GOARCH = "arm64" }
)

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "go not found on PATH"
}

if (-not (Test-Path (Join-Path $Root "cmd\gbr-agent"))) {
    Write-Warning "cmd/gbr-agent not found yet — ensure agent core package exists before release builds"
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
Write-Host "==> building $Binary version=$Version commit=$Commit"
Write-Host "    out=$OutDir"

foreach ($t in $Targets) {
    $os = $t.GOOS
    $arch = $t.GOARCH
    $ext = ""
    if ($os -eq "windows") { $ext = ".exe" }
    $name = "$Binary-$os-$arch$ext"
    $dest = Join-Path $OutDir $name

    Write-Host "--> GOOS=$os GOARCH=$arch -> $name"
    $env:CGO_ENABLED = "0"
    $env:GOOS = $os
    $env:GOARCH = $arch
    & go build -trimpath -ldflags $Ldflags -o $dest $Pkg
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed for $os/$arch"
    }
}

# Clear cross-compile env so the shell is not left dirty
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
$env:CGO_ENABLED = "0"

Write-Host "==> checksums"
$sumsPath = Join-Path $OutDir "SHA256SUMS"
$files = Get-ChildItem -Path $OutDir -File | Where-Object { $_.Name -like "$Binary-*" }
$lines = foreach ($f in $files) {
    $hash = (Get-FileHash -Algorithm SHA256 -Path $f.FullName).Hash.ToLowerInvariant()
    "{0}  {1}" -f $hash, $f.Name
}
$lines | Set-Content -Path $sumsPath -Encoding utf8
Get-ChildItem $OutDir | Format-Table Name, Length -AutoSize
Write-Host "==> done -> $OutDir"
