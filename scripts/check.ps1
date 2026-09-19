#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot
$toolVersions = Get-Content (Join-Path $PSScriptRoot "tool-versions.json") -Raw | ConvertFrom-Json

jq --version

if ($null -ne (Get-Command make -ErrorAction SilentlyContinue)) {
    make check
    exit $LASTEXITCODE
}

if ($env:VV_SKIP_LOCAL_DEV_TESTS -ne "1") {
    pwsh -NoLogo -NoProfile -File scripts/local-dev.tests.ps1
}

function Run-Step {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][scriptblock]$Command
    )

    Write-Host ""
    Write-Host "==> $Name"
    & $Command
}

Run-Step "fmt-check-go" {
    $packages = go list -f "{{.Dir}}" ./...
    $unformatted = gofmt -l $packages
    if ($unformatted) {
        Write-Host "gofmt differences found. Run gofmt or make fmt:"
        $unformatted | ForEach-Object { Write-Host $_ }
        exit 1
    }
}

Run-Step "fmt-check-web" {
    npm --prefix web run format:check
}

Run-Step "lint-go" {
    go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$($toolVersions.golangciLint)" run
}

Run-Step "lint-web" {
    npm --prefix web run lint
}

Run-Step "test-go" {
    go test ./...
}

Run-Step "test-web" {
    npm --prefix web run test
}

Run-Step "test-agent-workflows" {
    bash -lc ".specify/tests/workflows/run.sh"
}

Run-Step "generate-check" {
    go run "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$($toolVersions.oapiCodegen)" `
        -config api/oapi-codegen.yaml api/openapi.yaml
    npx --yes "openapi-typescript@$($toolVersions.openapiTypescript)" `
        api/openapi.yaml -o web/src/api/gen/openapi.ts

    $generatedStatus = git status --porcelain -- internal/httpapi/gen/api.gen.go web/src/api/gen/openapi.ts
    if ($generatedStatus) {
        Write-Host "Generated files are not up to date. Commit the result of generation:"
        git --no-pager diff --stat HEAD -- internal/httpapi/gen/api.gen.go web/src/api/gen/openapi.ts
        exit 1
    }
}

Write-Host ""
Write-Host "check: all steps passed"
