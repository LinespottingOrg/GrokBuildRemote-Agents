# Grok Build Remote — agent build (Windows PowerShell)
# Usage:
#   .\scripts\build.ps1
#   .\scripts\build.ps1 -Cross
#   .\scripts\build.ps1 -Version 0.1.0

param(
    [switch]$Cross,
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "Go toolchain not found on PATH. Install Go 1.22+ from https://go.dev/dl/"
    exit 1
}

if (-not $Version) {
    try {
        $Version = (git describe --tags --always --dirty 2>$null)
    } catch { }
    if (-not $Version) { $Version = "0.1.0-dev" }
}

$Dist = Join-Path $Root "dist"
New-Item -ItemType Directory -Force -Path $Dist | Out-Null
$Ldflags = "-s -w -X main.Version=$Version"
$Cmd = "./cmd/gbr-agent"

function Build-One {
    param($Goos, $Goarch, $Out)
    $env:GOOS = $Goos
    $env:GOARCH = $Goarch
    $env:CGO_ENABLED = "0"
    Write-Host "Building $Out (GOOS=$Goos GOARCH=$Goarch)..."
    go build -ldflags $Ldflags -o (Join-Path $Dist $Out) $Cmd
    if ($LASTEXITCODE -ne 0) { throw "build failed: $Out" }
}

Write-Host "go mod tidy..."
go mod tidy
if ($LASTEXITCODE -ne 0) { throw "go mod tidy failed" }

if ($Cross) {
    Build-One windows amd64 "gbr-agent-windows-amd64.exe"
    Build-One windows arm64 "gbr-agent-windows-arm64.exe"
    Build-One darwin  amd64 "gbr-agent-darwin-amd64"
    Build-One darwin  arm64 "gbr-agent-darwin-arm64"
    Build-One linux   amd64 "gbr-agent-linux-amd64"
    Build-One linux   arm64 "gbr-agent-linux-arm64"
} else {
    # Host native
    $out = "gbr-agent.exe"
    if ($IsLinux -or $IsMacOS -or $env:OS -notmatch "Windows") {
        $out = "gbr-agent"
    }
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    $env:CGO_ENABLED = "0"
    Write-Host "Building host native $out..."
    go build -ldflags $Ldflags -o (Join-Path $Dist $out) $Cmd
    if ($LASTEXITCODE -ne 0) { throw "build failed" }
}

Write-Host "Done. Artifacts in $Dist"
Get-ChildItem $Dist | Format-Table Name, Length, LastWriteTime
