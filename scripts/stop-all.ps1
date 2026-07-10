param(
    [switch]$StopRedis
)

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$BackendDir = Join-Path $RepoRoot "backend"
$AdminWebDir = Join-Path $RepoRoot "admin-web"
$BackendPidFile = Join-Path $BackendDir ".server.pid"
$WorkerPidFile = Join-Path $BackendDir ".worker.pid"
$AdminPidFile = Join-Path $AdminWebDir ".dev.pid"

function Remove-FileIfExists([string]$Path) {
    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    }
}

function Stop-ProcessIfRunning([int]$ProcessIdToStop) {
    if ($ProcessIdToStop -le 0) {
        return
    }
    $process = Get-Process -Id $ProcessIdToStop -ErrorAction SilentlyContinue
    if ($process) {
        Stop-Process -Id $ProcessIdToStop -Force -ErrorAction SilentlyContinue
    }
}

function Stop-ProcessTreeIfRunning([int]$RootProcessId) {
    if ($RootProcessId -le 0) {
        return
    }
    $children = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object {
        $_.ParentProcessId -eq $RootProcessId
    }
    foreach ($child in $children) {
        Stop-ProcessTreeIfRunning ([int]$child.ProcessId)
    }
    Stop-ProcessIfRunning $RootProcessId
}

function Stop-FromPidFile([string]$PidFile) {
    if (-not (Test-Path -LiteralPath $PidFile)) {
        return
    }
    $raw = Get-Content -LiteralPath $PidFile -ErrorAction SilentlyContinue | Select-Object -First 1
    [int]$processId = 0
    if ([int]::TryParse(($raw | Out-String).Trim(), [ref]$processId)) {
        Stop-ProcessIfRunning $processId
    }
    Remove-FileIfExists $PidFile
}

function Stop-TreeFromPidFile([string]$PidFile) {
    if (-not (Test-Path -LiteralPath $PidFile)) {
        return
    }
    $raw = Get-Content -LiteralPath $PidFile -ErrorAction SilentlyContinue | Select-Object -First 1
    [int]$processId = 0
    if ([int]::TryParse(($raw | Out-String).Trim(), [ref]$processId)) {
        Stop-ProcessTreeIfRunning $processId
    }
    Remove-FileIfExists $PidFile
}

function Stop-Listener8080 {
    $listener = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($listener) {
        Stop-Process -Id $listener.OwningProcess -Force -ErrorAction SilentlyContinue
    }
}

function Stop-RecommenderByCommandLine {
    $processes = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object {
        $_.CommandLine -and $_.CommandLine -like "*recommender.worker*"
    }
    foreach ($process in $processes) {
        Stop-Process -Id $process.ProcessId -Force -ErrorAction SilentlyContinue
    }
}

Write-Host "Stopping admin-web, backend and recommender worker..."
Stop-TreeFromPidFile $AdminPidFile
Stop-FromPidFile $WorkerPidFile
Stop-RecommenderByCommandLine
Stop-FromPidFile $BackendPidFile
Stop-Listener8080

if ($StopRedis) {
    Push-Location $BackendDir
    try {
        docker compose stop redis | Out-Host
    } finally {
        Pop-Location
    }
}

Write-Host "Stopped."
