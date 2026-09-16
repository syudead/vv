#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $false

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$cases = @(
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
