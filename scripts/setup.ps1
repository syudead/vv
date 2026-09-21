#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot
$toolVersions = Get-Content (Join-Path $PSScriptRoot "tool-versions.json") -Raw | ConvertFrom-Json

Write-Host "Downloading Go modules..."
go mod download

Write-Host "Installing web dependencies..."
& (Join-Path $PSScriptRoot "web-deps.ps1")

Write-Host "Preparing golangci-lint..."
go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$($toolVersions.golangciLint)" --version

Write-Host "Warming Go build cache..."
go build ./...

Write-Host "Setup complete."
