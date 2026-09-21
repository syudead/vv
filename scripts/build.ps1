#Requires -Version 7.4
param(
    [string]$Version = $env:VERSION
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = "dev"
}

$distPath = Join-Path $repoRoot "web/dist"
$backupRoot = Join-Path $repoRoot ".local/build-dist-backup-$([guid]::NewGuid().ToString('N'))"
$backupDist = Join-Path $backupRoot "dist"
$hadDist = Test-Path $distPath

New-Item -ItemType Directory -Force -Path $backupRoot | Out-Null
if ($hadDist) {
    Move-Item -LiteralPath $distPath -Destination $backupDist
}

$previousCgo = $env:CGO_ENABLED
try {
    npm --prefix web run build
    New-Item -ItemType Directory -Force -Path bin | Out-Null
    $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o bin/mdm ./cmd/mdm
} finally {
    $env:CGO_ENABLED = $previousCgo
    if (Test-Path $distPath) {
        Remove-Item -LiteralPath $distPath -Recurse -Force
    }
    if ($hadDist) {
        Move-Item -LiteralPath $backupDist -Destination $distPath
    }
    if (Test-Path $backupRoot) {
        Remove-Item -LiteralPath $backupRoot -Recurse -Force
    }
}

Write-Host "Built bin/mdm (version=$Version)"
