<#
.SYNOPSIS
  Build, sign and (optionally) WACK-validate the Grok Build Remote MSIX.

.DESCRIPTION
  Stages gbr-tray.exe (GUI entry) + gbr-agent.exe + Assets + AppxManifest.xml,
  packs with makeappx, signs with the CN=Linespotting code-signing cert, and
  runs the Windows App Certification Kit. The MSIX auto-starts the tray at login
  via the desktop StartupTask in the manifest, so no HKCU Run entry is needed in
  the packaged install.

.NOTES
  Builds stay off Dropbox (output under C:\pc-build). Never Stop-Process -Force
  unrelated processes.
#>
[CmdletBinding()]
param(
  [string]$TrayExe   = "C:\Users\User\AppData\Local\GrokBuildRemote\gbr-tray.exe",
  [string]$AgentExe  = "C:\Users\User\AppData\Local\GrokBuildRemote\gbr-agent.exe",
  [string]$Version   = "0.6.5.0",
  [string]$OutDir    = "C:\pc-build\APPAR\GrokBuildRemote\msix",
  [switch]$RunWack
)
$ErrorActionPreference = "Stop"
$here = Split-Path $MyInvocation.MyCommand.Path -Parent

function Find-SdkTool([string]$name) {
  Get-ChildItem "C:\Program Files (x86)\Windows Kits\10\bin" -Recurse -Filter $name -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -like '*\x64\*' } | Sort-Object FullName -Descending | Select-Object -First 1 -ExpandProperty FullName
}
$makeappx = Find-SdkTool "makeappx.exe"
$signtool = Find-SdkTool "signtool.exe"
if (-not $makeappx) { throw "makeappx.exe not found (Windows SDK)" }
if (-not $signtool) { throw "signtool.exe not found (Windows SDK)" }

$stage = Join-Path $OutDir "stage"
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Force -Path $stage, (Join-Path $stage "Assets"), $OutDir | Out-Null

Copy-Item $TrayExe  (Join-Path $stage "gbr-tray.exe")  -Force
Copy-Item $AgentExe (Join-Path $stage "gbr-agent.exe") -Force
Copy-Item (Join-Path $here "Assets\*") (Join-Path $stage "Assets") -Force

# Stamp version into the manifest copy.
$manifest = Get-Content (Join-Path $here "AppxManifest.xml") -Raw
$manifest = $manifest -replace 'Version="[0-9.]+"\s+ProcessorArchitecture', ("Version=""{0}"" ProcessorArchitecture" -f $Version)
Set-Content (Join-Path $stage "AppxManifest.xml") $manifest -Encoding UTF8

$msix = Join-Path $OutDir ("GrokBuildRemote-{0}-x64.msix" -f $Version)
Write-Host "1 pack -> $msix" -ForegroundColor Cyan
& $makeappx pack /o /d $stage /p $msix
if ($LASTEXITCODE -ne 0) { throw "makeappx failed" }

$cert = Get-ChildItem Cert:\CurrentUser\My -CodeSigningCert |
  Where-Object { $_.Subject -eq "CN=Linespotting" } | Select-Object -First 1
if ($null -eq $cert) { throw "CN=Linespotting code-signing cert not found" }
Write-Host "2 sign (thumb $($cert.Thumbprint))" -ForegroundColor Cyan
& $signtool sign /fd SHA256 /a /sha1 $cert.Thumbprint /tr "http://timestamp.digicert.com" /td SHA256 $msix
if ($LASTEXITCODE -ne 0) { throw "signtool failed" }

Write-Host "MSIX: $msix" -ForegroundColor Green
if ($RunWack) {
  $appcert = "C:\Program Files (x86)\Windows Kits\10\App Certification Kit\appcert.exe"
  $report = Join-Path $OutDir "wack-report.xml"
  Write-Host "3 WACK -> $report" -ForegroundColor Cyan
  & $appcert reset
  & $appcert test -appxpackagepath $msix -reportoutputpath $report
  Write-Host "WACK report: $report"
}
