Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

Write-Host "Downloading Go modules..."
go mod download

Write-Host "Installing web dependencies..."
npm --prefix web ci

Write-Host "Preparing golangci-lint..."
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 --version

Write-Host "Warming Go build cache..."
go build ./...

Write-Host "Setup complete."

