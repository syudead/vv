#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $false

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$cases = @(
    @{ Script = "check"; Failure = "jq"; Forbidden = "==> fmt-check-go" },
    @{ Script = "setup"; Failure = "go"; Forbidden = "Installing web dependencies" },
    @{ Script = "setup"; Failure = "npm"; Forbidden = "Preparing golangci-lint" },
    @{ Script = "check"; Failure = "npm"; Forbidden = "==> lint-go" },
    @{ Script = "check"; Failure = "make"; Forbidden = "==> fmt-check-go" }
)

foreach ($case in $cases) {
    $output = & pwsh -NoLogo -NoProfile -Command {
        param($Root, $Script, $Failure)
        $global:localDevTestFailure = $Failure
        function Get-Command {
            param($Name, $ErrorAction)
            if ($Name -eq "make" -and $global:localDevTestFailure -ne "make") { return }
            Microsoft.PowerShell.Core\Get-Command $Name -ErrorAction $ErrorAction
        }
        function go {
            if ($global:localDevTestFailure -eq "go") { & pwsh -NoProfile -Command "exit 17" }
        }
        function npm {
            if ($global:localDevTestFailure -eq "npm") { & pwsh -NoProfile -Command "exit 17" }
        }
        function gofmt {}
        function jq {
            if ($global:localDevTestFailure -eq "jq") { & pwsh -NoProfile -Command "exit 17" }
        }
        function make { & pwsh -NoProfile -Command "exit 17" }
        & (Join-Path $Root "scripts/$Script.ps1")
    } -args $repoRoot, $case.Script, $case.Failure 2>&1
    $code = $LASTEXITCODE
    $log = $output | Out-String
    if ($code -eq 0 -or $log.Contains($case.Forbidden) -or -not $log.Contains("17")) {
        throw "$($case.Script)/$($case.Failure) did not stop at the failed command (exit $code):`n$log"
    }
    Write-Host "PASS $($case.Script) stops after $($case.Failure) fails"
}

foreach ($scenario in @("healthy", "missing-git", "broken-jq", "empty-jq")) {
    $output = & pwsh -NoLogo -NoProfile -Command {
        param($Root, $Scenario)
        $global:doctorScenario = $Scenario
        function Get-Command {
            param($Name, $ErrorAction)
            if ($Name -eq "git" -and $global:doctorScenario -eq "missing-git") { return }
            [pscustomobject]@{ Source = "test-$Name" }
        }
        foreach ($name in @("git", "go", "node", "npm", "ffmpeg", "ffprobe", "bash", "mise", "task", "make", "docker")) {
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
    $expected = if ($scenario -eq "healthy") { 0 } else { 1 }
    $tool = if ($scenario -eq "missing-git") { "git" } else { "jq" }
    if ($code -ne $expected -or ($expected -eq 1 -and $log -notmatch "\[ERR\]\s+$tool\s")) {
        throw "doctor/$scenario failed (exit $code):`n$log"
    }
    Write-Host "PASS doctor/$scenario"
}
