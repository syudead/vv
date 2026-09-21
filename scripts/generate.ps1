#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot
. (Join-Path $PSScriptRoot "project-tool.ps1")
$oapiCodegen = Get-ProjectTool "oapi-codegen"

& $oapiCodegen `
    -config api/oapi-codegen.yaml api/openapi.yaml
npm exec --yes --package="openapi-typescript@7.13.0" -- openapi-typescript `
    api/openapi.yaml -o web/src/api/gen/openapi.ts
