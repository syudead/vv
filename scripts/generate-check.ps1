#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

$generated = @("internal/httpapi/gen/api.gen.go", "web/src/api/gen/openapi.ts")
$before = @{}
foreach ($path in $generated) {
    $before[$path] = if (Test-Path $path) { (Get-FileHash -Algorithm SHA256 $path).Hash } else { $null }
}

& (Join-Path $PSScriptRoot "generate.ps1")

$changed = @($generated | Where-Object {
    $after = if (Test-Path $_) { (Get-FileHash -Algorithm SHA256 $_).Hash } else { $null }
    $before[$_] -ne $after
})
if ($changed.Count -gt 0) {
    Write-Host "Generated files were stale. Run task generate and keep the result:"
    $changed | ForEach-Object { Write-Host $_ }
    exit 1
}
