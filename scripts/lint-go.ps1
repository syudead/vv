#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot
$toolVersions = Get-Content (Join-Path $PSScriptRoot "tool-versions.json") -Raw | ConvertFrom-Json

go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$($toolVersions.golangciLint)" run
