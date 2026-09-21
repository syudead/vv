#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot
$toolVersions = Get-Content (Join-Path $PSScriptRoot "tool-versions.json") -Raw | ConvertFrom-Json

go run "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$($toolVersions.oapiCodegen)" `
    -config api/oapi-codegen.yaml api/openapi.yaml
npx --yes "openapi-typescript@$($toolVersions.openapiTypescript)" `
    api/openapi.yaml -o web/src/api/gen/openapi.ts
