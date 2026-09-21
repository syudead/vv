#Requires -Version 7.4
param(
    [string]$DataDir = $env:DEV_DATA_DIR
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

function Assert-PortAvailable {
    param(
        [Parameter(Mandatory = $true)][int]$Port,
        [Parameter(Mandatory = $true)][System.Net.IPAddress]$Address
    )

    $listener = [System.Net.Sockets.TcpListener]::new($Address, $Port)
    try {
        $listener.Start()
    } catch {
        throw "Port $Port is already in use. Stop the existing development process and retry."
    } finally {
        $listener.Stop()
    }
}

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

Require-Command "go"
Require-Command "npm"
Require-Command "ffmpeg"
Require-Command "ffprobe"
. (Join-Path $PSScriptRoot "project-tool.ps1")
. (Join-Path $PSScriptRoot "process-tree.ps1")
$airPath = Get-ProjectTool "air"
$npmPath = if ($IsWindows) {
    (Get-Command "npm.cmd" -ErrorAction Stop).Source
} else {
    (Get-Command "npm" -ErrorAction Stop).Source
}

Assert-PortAvailable -Port 5173 -Address ([System.Net.IPAddress]::Loopback)

if ([string]::IsNullOrWhiteSpace($DataDir)) {
    $DataDir = Join-Path $repoRoot ".local/data"
}

New-Item -ItemType Directory -Force -Path $DataDir | Out-Null

$backendAddress = if ([string]::IsNullOrWhiteSpace($env:MDM_ADDR)) { ":8080" } else { $env:MDM_ADDR }
$apiTarget = if ([string]::IsNullOrWhiteSpace($env:MDM_API_TARGET)) { "http://localhost:8080" } else { $env:MDM_API_TARGET }
Write-Host "Go:   $backendAddress"
Write-Host "Vite: http://localhost:5173 (/api proxies to $apiTarget)"
Write-Host "Data:  $DataDir"
Write-Host ""

$goProcess = $null
$webProcess = $null

try {
    $goProcess = Start-Process -FilePath $airPath -ArgumentList @("-c", ".air.toml") `
        -WorkingDirectory $repoRoot -Environment @{ MDM_DATA_DIR = $DataDir } -NoNewWindow -PassThru
    $webProcess = Start-Process -FilePath $npmPath `
        -ArgumentList @("--prefix", "web", "run", "dev", "--", "--host", "127.0.0.1", "--strictPort") `
        -WorkingDirectory $repoRoot -NoNewWindow -PassThru

    while ($true) {
        $finished = @(@($goProcess, $webProcess) | Where-Object { $_.HasExited })
        if ($finished.Count -gt 0) {
            $details = $finished | ForEach-Object { "PID $($_.Id) exited with code $($_.ExitCode)" }
            throw "A development server stopped: $($details -join '; ')."
        }

        Start-Sleep -Milliseconds 500
    }
} finally {
    try {
        Stop-ProcessTree $webProcess
    } finally {
        Stop-ProcessTree $goProcess
    }
}
