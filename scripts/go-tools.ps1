#Requires -Version 7.4
param(
    [switch]$Check
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$toolDir = Join-Path $repoRoot "tools"
$binDir = Join-Path $repoRoot ".local/bin"
$stampPath = Join-Path $repoRoot ".local/go-tools.stamp"
$suffix = if ($IsWindows) { ".exe" } else { "" }
$toolNames = @("air", "golangci-lint", "oapi-codegen")

$fingerprintParts = @(
    (Get-FileHash -Algorithm SHA256 (Join-Path $toolDir "go.mod")).Hash,
    (Get-FileHash -Algorithm SHA256 (Join-Path $toolDir "go.sum")).Hash,
    (go version)
)
$fingerprint = $fingerprintParts -join "`n"
$installed = -not ($toolNames | Where-Object {
    -not (Test-Path -LiteralPath (Join-Path $binDir "$($_)$suffix") -PathType Leaf)
})
$current = (Test-Path -LiteralPath $stampPath -PathType Leaf) -and
    ((Get-Content -LiteralPath $stampPath -Raw) -eq $fingerprint)

if ($Check) {
    if ($installed -and $current) { exit 0 }
    exit 1
}

New-Item -ItemType Directory -Force -Path $binDir | Out-Null
$previousGobin = $env:GOBIN
try {
    $env:GOBIN = $binDir
    go -C tools install tool
} finally {
    if ($null -eq $previousGobin) {
        Remove-Item Env:GOBIN -ErrorAction SilentlyContinue
    } else {
        $env:GOBIN = $previousGobin
    }
}

New-Item -ItemType Directory -Force -Path (Split-Path $stampPath) | Out-Null
Set-Content -LiteralPath $stampPath -Value $fingerprint -NoNewline
