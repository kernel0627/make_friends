$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$BackendDir = Join-Path $RepoRoot "backend"
$AdminWebDir = Join-Path $RepoRoot "admin-web"
$BackendPidFile = Join-Path $BackendDir ".server.pid"
$WorkerPidFile = Join-Path $BackendDir ".worker.pid"
$AdminPidFile = Join-Path $AdminWebDir ".dev.pid"

function Read-Pid([string]$PidFile) {
    if (-not (Test-Path -LiteralPath $PidFile)) {
        return ""
    }
    return ((Get-Content -LiteralPath $PidFile | Select-Object -First 1) | Out-String).Trim()
}

function Test-PidRunning([string]$PidText) {
    [int]$processId = 0
    if (-not [int]::TryParse($PidText, [ref]$processId)) {
        return $false
    }
    return [bool](Get-Process -Id $processId -ErrorAction SilentlyContinue)
}

function Test-WebReady([string]$Url) {
    try {
        $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2
        return ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500)
    } catch {
        return $false
    }
}

$backendPid = Read-Pid $BackendPidFile
$workerPid = Read-Pid $WorkerPidFile
$adminPid = Read-Pid $AdminPidFile
$listener8080 = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
$listener5173 = Get-NetTCPConnection -LocalPort 5173 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
$redisStatus = "unknown"

Push-Location $BackendDir
try {
    $raw = cmd /c "docker compose ps redis --format json 2>nul"
    if ($LASTEXITCODE -eq 0 -and $raw) {
        $redisStatus = (($raw | Select-Object -Last 1) | Out-String).Trim()
    }
} finally {
    Pop-Location
}

Write-Host "Backend PID file : $backendPid"
Write-Host "Backend running  : $(Test-PidRunning $backendPid)"
Write-Host "8080 listening   : $([bool]$listener8080)"
Write-Host "Worker PID file  : $workerPid"
Write-Host "Worker running   : $(Test-PidRunning $workerPid)"
Write-Host "Admin PID file   : $adminPid"
Write-Host "Admin running    : $(Test-PidRunning $adminPid)"
Write-Host "5173 listening   : $([bool]$listener5173)"
Write-Host "Admin available  : $(Test-WebReady 'http://127.0.0.1:5173/')"
Write-Host "Redis status     : $redisStatus"

try {
    $health = Invoke-RestMethod -Uri "http://127.0.0.1:8080/healthz" -Method Get -TimeoutSec 2
    Write-Host "Health payload   : $(($health | ConvertTo-Json -Compress))"
} catch {
    Write-Host "Health payload   : unavailable"
}
