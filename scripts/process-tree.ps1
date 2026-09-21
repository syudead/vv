#Requires -Version 7.4
Set-StrictMode -Version Latest

function Get-UnixProcessTreeIds {
    param([Parameter(Mandatory = $true)][int]$RootProcessId)

    $childrenByParent = @{}
    $rows = & ps -e -o pid= -o ppid=
    if ($LASTEXITCODE -ne 0) {
        throw "Could not inspect the process tree with ps."
    }

    foreach ($row in $rows) {
        if ($row -notmatch '^\s*(\d+)\s+(\d+)\s*$') { continue }
        $processId = [int]$Matches[1]
        $parentId = [int]$Matches[2]
        if (-not $childrenByParent.ContainsKey($parentId)) {
            $childrenByParent[$parentId] = [System.Collections.Generic.List[int]]::new()
        }
        $childrenByParent[$parentId].Add($processId)
    }

    $ids = [System.Collections.Generic.List[int]]::new()
    $pending = [System.Collections.Generic.Stack[int]]::new()
    $pending.Push($RootProcessId)
    while ($pending.Count -gt 0) {
        $processId = $pending.Pop()
        $ids.Add($processId)
        if ($childrenByParent.ContainsKey($processId)) {
            foreach ($childId in $childrenByParent[$processId]) {
                $pending.Push($childId)
            }
        }
    }
    return $ids.ToArray()
}

function Stop-ProcessTree {
    param([System.Diagnostics.Process]$Process)

    if ($null -eq $Process -or $Process.HasExited) { return }
    $PSNativeCommandUseErrorActionPreference = $false
    if ($IsWindows) {
        taskkill /PID $Process.Id /T /F 2>$null | Out-Null
        $Process.WaitForExit(5000) | Out-Null
        return
    }

    $processIds = @(Get-UnixProcessTreeIds -RootProcessId $Process.Id)
    $killPath = (Get-Command "kill" -CommandType Application -ErrorAction Stop).Source
    & $killPath -TERM @processIds 2>$null

    $deadline = [DateTime]::UtcNow.AddSeconds(5)
    do {
        $remaining = @($processIds | Where-Object {
            $null -ne (Get-Process -Id $_ -ErrorAction SilentlyContinue)
        })
        if ($remaining.Count -eq 0) { break }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)

    if ($remaining.Count -gt 0) {
        & $killPath -KILL @remaining 2>$null
    }
    $Process.WaitForExit(5000) | Out-Null
}
