#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $false

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$cases = @(
    @{ Script = "setup"; Failure = "go"; Forbidden = "Installing web dependencies" },
    @{ Script = "setup"; Failure = "npm"; Forbidden = "Preparing golangci-lint" },
    @{ Script = "generate"; Failure = "go"; Forbidden = "openapi-typescript" }
)

foreach ($case in $cases) {
    $output = & pwsh -NoLogo -NoProfile -Command {
        param($Root, $Script, $Failure)
        $global:localDevTestFailure = $Failure
        function go {
            if ($global:localDevTestFailure -eq "go") { & pwsh -NoProfile -Command "exit 17" }
        }
        function npm {
            if ($global:localDevTestFailure -eq "npm") { & pwsh -NoProfile -Command "exit 17" }
        }
        function npx {}
        & (Join-Path $Root "scripts/$Script.ps1")
    } -args $repoRoot, $case.Script, $case.Failure 2>&1
    $code = $LASTEXITCODE
    $log = $output | Out-String
    if ($code -eq 0 -or $log.Contains($case.Forbidden) -or -not $log.Contains("17")) {
        throw "$($case.Script)/$($case.Failure) did not stop at the failed command (exit $code):`n$log"
    }
    Write-Host "PASS $($case.Script) stops after $($case.Failure) fails"
}

foreach ($scenario in @("healthy", "missing-git", "missing-task", "broken-jq", "empty-jq")) {
    $output = & pwsh -NoLogo -NoProfile -Command {
        param($Root, $Scenario)
        $global:doctorScenario = $Scenario
        function Get-Command {
            param($Name, $ErrorAction)
            if ($Name -eq "git" -and $global:doctorScenario -eq "missing-git") { return }
            if ($Name -eq "task" -and $global:doctorScenario -eq "missing-task") { return }
            [pscustomobject]@{ Source = "test-$Name" }
        }
        foreach ($name in @("git", "go", "node", "npm", "ffmpeg", "ffprobe", "bash", "mise", "task", "docker")) {
            Set-Item "function:$name" { "test-version" }
        }
        function jq {
            switch ($global:doctorScenario) {
                "broken-jq" { & pwsh -NoProfile -Command "exit 17" }
                "empty-jq" { return }
                default { "jq-test-version" }
            }
        }
        & (Join-Path $Root "scripts/doctor.ps1")
    } -args $repoRoot, $scenario *>&1
    $code = $LASTEXITCODE
    $log = $output | Out-String
    $expected = if ($scenario -in @("missing-git", "missing-task")) { 1 } else { 0 }
    $tool = switch ($scenario) {
        "missing-git" { "git" }
        "missing-task" { "task" }
        default { "jq" }
    }
    if ($code -ne $expected -or ($expected -eq 1 -and $log -notmatch "\[ERR\]\s+$tool\s")) {
        throw "doctor/$scenario failed (exit $code):`n$log"
    }
    Write-Host "PASS doctor/$scenario"
}

$webDepsRoot = Join-Path $repoRoot ".local/web-deps-test"
Remove-Item -Recurse -Force $webDepsRoot -ErrorAction SilentlyContinue
try {
    New-Item -ItemType Directory -Force -Path (Join-Path $webDepsRoot "scripts"), (Join-Path $webDepsRoot "web/node_modules"), (Join-Path $webDepsRoot ".local") | Out-Null
    Copy-Item (Join-Path $repoRoot "scripts/web-deps.ps1") (Join-Path $webDepsRoot "scripts/web-deps.ps1")
    New-Item -ItemType File -Path (Join-Path $webDepsRoot "web/package-lock.json"), (Join-Path $webDepsRoot ".local/web-deps.stamp") | Out-Null

    & (Join-Path $webDepsRoot "scripts/web-deps.ps1") -Check
    if ($LASTEXITCODE -ne 0) {
        throw "web-deps rejected a current stamp with node_modules present"
    }
    Write-Host "PASS web-deps accepts installed dependencies"

    Remove-Item -Recurse -Force (Join-Path $webDepsRoot "web/node_modules")
    & (Join-Path $webDepsRoot "scripts/web-deps.ps1") -Check
    if ($LASTEXITCODE -eq 0) {
        throw "web-deps accepted a stamp without node_modules"
    }
    Write-Host "PASS web-deps rejects missing node_modules"
}
finally {
    Remove-Item -Recurse -Force $webDepsRoot -ErrorAction SilentlyContinue
}

exit 0
