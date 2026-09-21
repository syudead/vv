#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

Write-Host "Downloading Go modules..."
go mod download

Write-Host "Installing web dependencies..."
& (Join-Path $PSScriptRoot "web-deps.ps1")

Write-Host "Preparing Go development tools..."
& (Join-Path $PSScriptRoot "go-tools.ps1")

Write-Host "Warming Go build cache..."
go build ./...

Write-Host "Setup complete."
