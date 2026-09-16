Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

if ($null -ne (Get-Command make -ErrorAction SilentlyContinue)) {
    make check
    exit $LASTEXITCODE
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
    go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run
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

Run-Step "test-sdd" {
    bash .claude/skills/sdd-next/tests/run.sh
}

Run-Step "generate-check" {
    go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 `
        -config api/oapi-codegen.yaml api/openapi.yaml
    npx --yes openapi-typescript@7.13.0 `
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

