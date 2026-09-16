#Requires -Version 7.4
param(
    [string]$MediaDir = "",
    [string]$DataDir = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new()

function Require-Command {
    param([Parameter(Mandatory = $true)][string]$Name)
    if ($null -eq (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name is required. Run scripts/doctor.ps1 for setup hints."
    }
}

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

Require-Command "go"
Require-Command "npm"
Require-Command "ffmpeg"
Require-Command "ffprobe"

if ([string]::IsNullOrWhiteSpace($MediaDir)) {
    $MediaDir = Join-Path $repoRoot "media"
}
if ([string]::IsNullOrWhiteSpace($DataDir)) {
    $DataDir = Join-Path $repoRoot ".local/data"
}

New-Item -ItemType Directory -Force -Path $MediaDir, $DataDir | Out-Null

Write-Host "Go:   http://localhost:8080"
Write-Host "Vite: http://localhost:5173 (/api proxies to :8080)"
Write-Host "Media: $MediaDir"
Write-Host "Data:  $DataDir"
Write-Host ""

$goJob = Start-Job -Name "vv-go" -ScriptBlock {
    param($Root, $Media, $Data)
    $ErrorActionPreference = "Stop"
    $PSNativeCommandUseErrorActionPreference = $true
    [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new()
    Set-Location $Root
    $env:MDM_MEDIA_DIR = $Media
    $env:MDM_DATA_DIR = $Data
    go run ./cmd/mdm
} -ArgumentList $repoRoot, $MediaDir, $DataDir

$webJob = Start-Job -Name "vv-web" -ScriptBlock {
    param($Root)
    $ErrorActionPreference = "Stop"
    $PSNativeCommandUseErrorActionPreference = $true
    [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new()
    Set-Location $Root
    npm --prefix web run dev -- --host 127.0.0.1 --strictPort
} -ArgumentList $repoRoot

$jobs = @($goJob, $webJob)

try {
    while ($true) {
        foreach ($job in $jobs) {
            Receive-Job $job
        }

        $finished = @($jobs | Where-Object { $_.State -ne "Running" })
        if ($finished.Count -gt 0) {
            foreach ($job in $finished) {
                Receive-Job $job
                Write-Host "$($job.Name) exited with state $($job.State)."
            }
            throw "A development server stopped. See the output above."
        }

        Start-Sleep -Seconds 1
    }
} finally {
    foreach ($job in $jobs) {
        if ($job.State -eq "Running") {
            Stop-Job $job
        }
        Receive-Job $job -ErrorAction SilentlyContinue
        Remove-Job $job -Force
    }
}
