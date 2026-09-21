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

exit 0
