#Requires -Version 7.4
param(
    [switch]$Check
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot
$lockfile = Get-Item "web/package-lock.json"
$stampPath = Join-Path $repoRoot ".local/web-deps.stamp"
$viteCommand = if ($IsWindows) { "web/node_modules/.bin/vite.cmd" } else { "web/node_modules/.bin/vite" }
$fingerprint = (Get-FileHash -Algorithm SHA256 $lockfile).Hash

if ($Check) {
    if (-not (Test-Path $viteCommand -PathType Leaf) -or -not (Test-Path $stampPath -PathType Leaf)) {
        exit 1
    }
    if ((Get-Content $stampPath -Raw) -ne $fingerprint) {
        exit 1
    }
    exit 0
}

npm --prefix web ci
New-Item -ItemType Directory -Force -Path (Split-Path $stampPath) | Out-Null
Set-Content -LiteralPath $stampPath -Value $fingerprint -NoNewline
