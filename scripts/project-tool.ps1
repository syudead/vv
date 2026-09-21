function Get-ProjectTool {
    param([Parameter(Mandatory = $true)][string]$Name)

    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
    $fileName = if ($IsWindows) { "$Name.exe" } else { $Name }
    $toolPath = Join-Path $repoRoot ".local/bin/$fileName"
    if (-not (Test-Path -LiteralPath $toolPath -PathType Leaf)) {
        throw "$Name is not installed. Run task setup."
    }
    return $toolPath
}
