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

if ($Check) {
    if (-not (Test-Path $stampPath)) {
        exit 1
    }
    $stamp = Get-Item $stampPath
    if ($stamp.LastWriteTimeUtc -lt $lockfile.LastWriteTimeUtc) {
        exit 1
    }
    exit 0
}

npm --prefix web ci
New-Item -ItemType Directory -Force -Path (Split-Path $stampPath) | Out-Null
New-Item -ItemType File -Force -Path $stampPath | Out-Null
