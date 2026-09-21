#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot
. (Join-Path $PSScriptRoot "project-tool.ps1")
$golangciLint = Get-ProjectTool "golangci-lint"

& $golangciLint run
