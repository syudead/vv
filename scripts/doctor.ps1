#Requires -Version 7.4
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Find-Command {
    param([Parameter(Mandatory = $true)][string]$Name)
    Get-Command $Name -ErrorAction SilentlyContinue | Select-Object -First 1
}

function First-Line {
    param([Parameter(Mandatory = $true)][scriptblock]$Command)
    try {
        $output = & $Command 2>&1
        if ($LASTEXITCODE -ne 0 -and $null -ne $LASTEXITCODE) {
            return $null
        }
        return ($output | Select-Object -First 1)
    } catch {
        return $null
    }
}

function Check-Tool {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][scriptblock]$VersionCommand,
        [Parameter(Mandatory = $true)][string]$InstallHint,
        [bool]$Required = $true
    )

    $cmd = Find-Command $Name
    if ($null -eq $cmd) {
        $mise = Find-Command "mise"
        if ($null -ne $mise) {
            $misePath = First-Line { mise which $Name }
            if (-not [string]::IsNullOrWhiteSpace($misePath)) {
                return [pscustomobject]@{
                    Name = $Name
                    Required = $Required
                    Ok = $true
                    Detail = "available via mise: $misePath"
                    Hint = ""
                }
            }
        }

        return [pscustomobject]@{
            Name = $Name
            Required = $Required
            Ok = $false
            Detail = "not found"
            Hint = $InstallHint
        }
    }

    $version = First-Line $VersionCommand
    if ([string]::IsNullOrWhiteSpace($version)) {
        $version = "found at $($cmd.Source)"
    }

    return [pscustomobject]@{
        Name = $Name
        Required = $Required
        Ok = $true
        Detail = $version
        Hint = ""
    }
}

$checks = @(
    (Check-Tool "jq" { jq --version } "Install with mise: mise install"),
    (Check-Tool "pwsh" { pwsh --version } "Install PowerShell 7.4 or later."),
    (Check-Tool "go" { go version } "Install with mise: mise install"),
    (Check-Tool "node" { node --version } "Install with mise: mise install"),
    (Check-Tool "npm" { npm --version } "Install with mise: mise install"),
    (Check-Tool "ffmpeg" { ffmpeg -version } "Install ffmpeg and ensure ffmpeg is on PATH."),
    (Check-Tool "ffprobe" { ffprobe -version } "Install ffmpeg and ensure ffprobe is on PATH."),
    (Check-Tool "bash" { bash --version } "Install Git for Windows or another bash provider."),
    (Check-Tool "mise" { mise --version } "Install mise to use the pinned tool versions in mise.toml." $false),
    (Check-Tool "task" { task --version } "Install with mise: mise install" $false),
    (Check-Tool "make" { make --version } "Install GNU make, or use scripts/check.ps1 and Taskfile.yml." $false),
    (Check-Tool "docker" { docker --version } "Install Docker Desktop for make up / make down." $false)
)

$docker = $checks | Where-Object { $_.Name -eq "docker" }
if ($docker.Ok) {
    $dockerInfo = First-Line { docker info --format "{{.ServerVersion}}" }
    if ([string]::IsNullOrWhiteSpace($dockerInfo)) {
        $docker.Detail = "$($docker.Detail); daemon not reachable"
    } else {
        $docker.Detail = "$($docker.Detail); daemon $dockerInfo"
    }
}

Write-Host "Local development environment"
Write-Host ""

foreach ($check in $checks) {
    $marker = if ($check.Ok) { "OK " } elseif ($check.Required) { "ERR" } else { "WARN" }
    $kind = if ($check.Required) { "required" } else { "optional" }
    Write-Host ("[{0}] {1,-8} ({2}) {3}" -f $marker, $check.Name, $kind, $check.Detail)
    if (-not $check.Ok) {
        Write-Host ("      {0}" -f $check.Hint)
    }
}

$missingRequired = @($checks | Where-Object { $_.Required -and -not $_.Ok })
if ($missingRequired.Count -gt 0) {
    Write-Host ""
    Write-Host "Missing required tools for local dev: $($missingRequired.Name -join ', ')"
    exit 1
}

Write-Host ""
Write-Host "All required local-dev tools are available."
