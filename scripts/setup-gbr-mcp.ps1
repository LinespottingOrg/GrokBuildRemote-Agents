# Pin gbr-mcp and print Hermes / OpenClaw attach commands.
# Same protocol as scripts/setup-gbr-mcp.sh: stdio gbr-mcp, proto gbr/1.
# Do not register http://127.0.0.1:8788 as MCP. That URL is Bot API REST.
$ErrorActionPreference = 'Stop'

$Ver = if ($env:GBR_VERSION) { $env:GBR_VERSION } else { 'v0.6.2' }
$Dest = if ($env:GBR_MCP_SRC) { $env:GBR_MCP_SRC } else { Join-Path $env:USERPROFILE '.gbr\gbr-mcp-src' }
$Repo = 'https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git'

function Die([string]$Msg) { Write-Error $Msg; exit 1 }

if (-not (Get-Command node -ErrorAction SilentlyContinue)) { Die 'need Node >= 20' }
if (-not (Get-Command git -ErrorAction SilentlyContinue)) { Die 'need git' }
if (-not (Get-Command gbr-agent -ErrorAction SilentlyContinue)) {
  Die 'gbr-agent not on PATH — pin install: https://grokbuildremote.com/PINNED-INSTALL.md'
}
if (-not (Get-Command grok -ErrorAction SilentlyContinue)) {
  Write-Warning 'grok CLI not on PATH — gbr_open will fail until Grok Build is installed'
}

$gv = & gbr-agent version 2>$null
Write-Host "    gbr-agent: $(if ($gv) { $gv } else { 'unknown' })"

Write-Host "==> pin $Repo @ $Ver -> $Dest"
New-Item -ItemType Directory -Force -Path (Split-Path $Dest) | Out-Null
if (Test-Path (Join-Path $Dest '.git')) {
  git -C $Dest fetch --tags --depth 1 origin $Ver
  if ($LASTEXITCODE -ne 0) { Die 'git fetch failed' }
  git -C $Dest checkout -q FETCH_HEAD
  if ($LASTEXITCODE -ne 0) { Die 'git checkout failed' }
} else {
  if (Test-Path $Dest) { Remove-Item -Recurse -Force $Dest }
  git clone --branch $Ver --depth 1 $Repo $Dest
  if ($LASTEXITCODE -ne 0) { Die 'git clone failed' }
}

Set-Location (Join-Path $Dest 'mcp\gbr-mcp')
if (Test-Path 'package-lock.json') {
  npm ci --ignore-scripts --omit=dev
} else {
  Write-Warning 'no package-lock.json in this pin — falling back to npm install --ignore-scripts --omit=dev'
  npm install --ignore-scripts --omit=dev
}
if ($LASTEXITCODE -ne 0) { Die 'npm failed' }

$Mcp = Join-Path $Dest 'mcp\gbr-mcp\bin\gbr-mcp.js'
if (-not (Test-Path $Mcp)) { Die "missing $Mcp" }
node $Mcp --diagnose
if (-not $env:GBR_BOT_REQUIRE_KEY) { $env:GBR_BOT_REQUIRE_KEY = '1' }

Write-Host '==> Hermes (stdio MCP — do NOT point Hermes at :8788; that is Bot API REST, not MCP)'
Write-Host "  hermes mcp add gbr -- stdio -- node $Mcp"

Write-Host '==> OpenClaw'
Write-Host "  copy $Dest\skills\openclaw\SKILL.md into the OpenClaw skills dir"
Write-Host '  or: clawhub skill publish (human login) — slug gbr'
Write-Host "  MCP stdio: node $Mcp"

Write-Host '==> NemoClaw (host tool, not inside OpenShell)'
Write-Host '  Keep gbr-agent + this node process on the HOST.'
Write-Host '  Point the sandboxed agent at host stdio MCP, or HTTP Bot API'
Write-Host '  http://127.0.0.1:8788 only after GBR_BOT_REQUIRE_KEY=1'
Write-Host '  Do not copy gbr-agent into the sandbox.'

Write-Host '==> Then on this host'
Write-Host '  gbr-agent pair ; gbr-agent run'
Write-Host '  # MCP client: gbr_diagnose -> gbr_open (holder=hermes|openclaw|nemoclaw) -> gbr_inject -> gbr_result'
Write-Host 'Docs: https://grokbuildremote.com/use-cases/claw.html'
Write-Host "MCP:  $Mcp"
