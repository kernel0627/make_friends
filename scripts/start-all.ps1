param(
    [switch]$Reseed,
    [switch]$RebuildRecommendations,
    [switch]$SkipBuild,
    [switch]$SkipAdminInstall
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$BackendDir = Join-Path $RepoRoot "backend"
$BackendBinDir = Join-Path $BackendDir "bin"
$BackendExe = Join-Path $BackendBinDir "backend-server.exe"
$LogsDir = Join-Path $BackendDir "logs"
$BackendPidFile = Join-Path $BackendDir ".server.pid"
$WorkerPidFile = Join-Path $BackendDir ".worker.pid"
$BackendStdout = Join-Path $LogsDir "backend.out.log"
$BackendStderr = Join-Path $LogsDir "backend.err.log"
$WorkerStdout = Join-Path $LogsDir "recommender.out.log"
$WorkerStderr = Join-Path $LogsDir "recommender.err.log"
$AdminWebDir = Join-Path $RepoRoot "admin-web"
$AdminPidFile = Join-Path $AdminWebDir ".dev.pid"
$AdminStdout = Join-Path $AdminWebDir "dev.out.log"
$AdminStderr = Join-Path $AdminWebDir "dev.err.log"
$AdminUrl = "http://127.0.0.1:5173/"
$WorkerEnvName = "make_friends_env"

function Repair-ProcessPathEnvironment {
    $vars = [Environment]::GetEnvironmentVariables()
    $pathNames = @($vars.Keys | Where-Object { $_ -ieq "Path" })
    if ($pathNames.Count -le 1) {
        return
    }

    $seen = @{}
    $merged = New-Object System.Collections.Generic.List[string]
    foreach ($name in @("Path", "PATH")) {
        if (-not $vars.Contains($name) -or -not $vars[$name]) {
            continue
        }
        foreach ($entry in ([string]$vars[$name]).Split(';')) {
            $trimmed = $entry.Trim()
            if (-not $trimmed) {
                continue
            }
            $key = $trimmed.ToLowerInvariant()
            if (-not $seen.ContainsKey($key)) {
                $seen[$key] = $true
                $merged.Add($trimmed) | Out-Null
            }
        }
    }

    [Environment]::SetEnvironmentVariable("Path", $null, "Process")
    [Environment]::SetEnvironmentVariable("PATH", $null, "Process")
    [Environment]::SetEnvironmentVariable("Path", ($merged -join ";"), "Process")
}

function Ensure-Directory([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType Directory -Path $Path -Force | Out-Null
    }
}

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

function Resolve-CondaExe {
    $cmd = Get-Command conda -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Source) {
        return $cmd.Source
    }
    return $null
}

function Resolve-WorkerPython {
    $repoVenvPython = Join-Path $RepoRoot ".venv\Scripts\python.exe"
    if (Test-Path -LiteralPath $repoVenvPython) {
        return $repoVenvPython
    }

    $condaExe = Resolve-CondaExe
    if ($condaExe) {
        $condaInfo = & $condaExe env list --json | ConvertFrom-Json
        $resolved = $condaInfo.envs | Where-Object {
            (Split-Path -Leaf $_) -eq $WorkerEnvName
        } | Select-Object -First 1
        if ($resolved) {
            $condaPython = Join-Path $resolved "python.exe"
            if (Test-Path -LiteralPath $condaPython) {
                return $condaPython
            }
        }
    }

    throw "recommender Python was not found. Create .venv in the repo root or create the $WorkerEnvName Conda environment."
}

function Resolve-NpmCmd {
    $cmd = Get-Command npm.cmd -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Source) {
        return $cmd.Source
    }
    $candidates = @(
        "$env:ProgramFiles\nodejs\npm.cmd",
        "${env:ProgramFiles(x86)}\nodejs\npm.cmd",
        "$env:APPDATA\npm\npm.cmd"
    )
    foreach ($candidate in $candidates) {
        if ($candidate -and (Test-Path -LiteralPath $candidate)) {
            return $candidate
        }
    }
    throw "npm.cmd was not found. Please install Node.js and npm first."
}

function Start-LoggedProcess([string]$FilePath, [string[]]$ArgumentList, [string]$WorkingDirectory, [string]$StdoutPath, [string]$StderrPath) {
    Remove-FileIfExists $StdoutPath
    Remove-FileIfExists $StderrPath
    if ($ArgumentList -and $ArgumentList.Count -gt 0) {
        return Start-Process -FilePath $FilePath `
            -ArgumentList $ArgumentList `
            -WorkingDirectory $WorkingDirectory `
            -RedirectStandardOutput $StdoutPath `
            -RedirectStandardError $StderrPath `
            -WindowStyle Hidden `
            -PassThru
    }
    return Start-Process -FilePath $FilePath `
        -WorkingDirectory $WorkingDirectory `
        -RedirectStandardOutput $StdoutPath `
        -RedirectStandardError $StderrPath `
        -WindowStyle Hidden `
        -PassThru
}

function Wait-BackendHealthy {
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Seconds 1
        try {
            $response = Invoke-RestMethod -Uri "http://127.0.0.1:8080/healthz" -Method Get -TimeoutSec 2
            if ($response.ok -eq $true) {
                return
            }
        } catch {
        }
    }
    throw "backend did not become healthy on :8080 within 30 seconds"
}

function Test-AdminWebReady {
    try {
        $response = Invoke-WebRequest -Uri $AdminUrl -UseBasicParsing -TimeoutSec 2
        return ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500)
    } catch {
        return $false
    }
}

function Wait-AdminWebReady {
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Seconds 1
        if (Test-AdminWebReady) {
            return $true
        }
    }
    return $false
}

function Ensure-AdminDependencies([string]$NpmCmd) {
    $nodeModules = Join-Path $AdminWebDir "node_modules"
    if ((Test-Path -LiteralPath $nodeModules) -or $SkipAdminInstall) {
        return
    }

    Write-Host "      admin-web/node_modules not found; running npm.cmd install..."
    Push-Location $AdminWebDir
    try {
        & $NpmCmd install
        if ($LASTEXITCODE -ne 0) {
            throw "npm.cmd install failed in admin-web"
        }
    } finally {
        Pop-Location
    }
}

function Start-AdminWeb {
    if (-not (Test-Path -LiteralPath (Join-Path $AdminWebDir "package.json"))) {
        throw "admin-web/package.json was not found."
    }

    Stop-TreeFromPidFile $AdminPidFile

    if (Test-AdminWebReady) {
        Write-Host "      admin-web is already available at $AdminUrl"
        return $null
    }

    $listener = Get-NetTCPConnection -LocalPort 5173 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($listener) {
        throw "port 5173 is already in use by PID $($listener.OwningProcess). Stop it first, then rerun start-all.bat."
    }

    $npmCmd = Resolve-NpmCmd
    Ensure-AdminDependencies $npmCmd

    $adminProc = Start-LoggedProcess `
        -FilePath $npmCmd `
        -ArgumentList @("run", "dev", "--", "--host", "127.0.0.1", "--port", "5173", "--strictPort") `
        -WorkingDirectory $AdminWebDir `
        -StdoutPath $AdminStdout `
        -StderrPath $AdminStderr
    Set-Content -LiteralPath $AdminPidFile -Value $adminProc.Id

    if (-not (Wait-AdminWebReady)) {
        throw "admin-web did not become ready at $AdminUrl within 30 seconds. Check $AdminStderr"
    }

    return $adminProc
}

Repair-ProcessPathEnvironment
Ensure-Directory $LogsDir
Ensure-Directory (Join-Path $RepoRoot ".gocache")
Ensure-Directory (Join-Path $RepoRoot ".gocache\tmp")
Ensure-Directory (Join-Path $RepoRoot ".gomodcache")
Ensure-Directory (Join-Path $RepoRoot ".hf")
Ensure-Directory (Join-Path $RepoRoot ".hf\transformers")
Ensure-Directory $BackendBinDir

if (-not $env:GOPROXY) { $env:GOPROXY = "https://goproxy.cn,direct" }
if (-not $env:GOSUMDB) { $env:GOSUMDB = "sum.golang.google.cn" }
if (-not $env:HF_ENDPOINT) { $env:HF_ENDPOINT = "https://hf-mirror.com" }

$env:GOCACHE = Join-Path $RepoRoot ".gocache"
$env:GOTMPDIR = Join-Path $RepoRoot ".gocache\tmp"
$env:GOMODCACHE = Join-Path $RepoRoot ".gomodcache"
$env:BACKEND_ADDR = ":8080"
$env:USE_REDIS = "true"
$env:REDIS_ADDR = "127.0.0.1:6379"
$env:HW_BACKEND_DIR = $BackendDir
$env:PYTHONPATH = $BackendDir
$env:HF_HOME = Join-Path $RepoRoot ".hf"
$env:TRANSFORMERS_CACHE = Join-Path $RepoRoot ".hf\transformers"
$env:REC_DEVICE = "cuda"

Push-Location $BackendDir
try {
    Write-Host "[1/7] starting redis..."
    docker compose up -d redis | Out-Host

    Write-Host "[2/7] resolving recommender Python..."
    $workerPython = Resolve-WorkerPython

    if (-not $SkipBuild) {
        Write-Host "[3/7] building backend..."
        & go build -o $BackendExe ./cmd/server
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed"
        }
    } else {
        Write-Host "[3/7] skipping backend build..."
    }

    if ($Reseed) {
        Write-Host "[4/7] reseeding database..."
        & go run ./cmd/seed-full -reset=true
        if ($LASTEXITCODE -ne 0) {
            throw "seed-full failed"
        }
    } else {
        Write-Host "[4/7] skipping reseed..."
    }

    if ($Reseed -or $RebuildRecommendations) {
        Write-Host "[5/7] rebuilding recommendation artifacts..."
        & $workerPython -m recommender.rebuild_all --device $env:REC_DEVICE
        if ($LASTEXITCODE -ne 0) {
            throw "recommendation rebuild failed"
        }
    } else {
        Write-Host "[5/7] skipping offline recommendation rebuild..."
    }

    Write-Host "[6/7] starting backend and recommender worker..."
    Stop-FromPidFile $WorkerPidFile
    Stop-FromPidFile $BackendPidFile
    Stop-Listener8080

    $backendProc = Start-LoggedProcess `
        -FilePath $BackendExe `
        -ArgumentList @() `
        -WorkingDirectory $BackendDir `
        -StdoutPath $BackendStdout `
        -StderrPath $BackendStderr
    Set-Content -LiteralPath $BackendPidFile -Value $backendProc.Id

    $workerProc = Start-LoggedProcess `
        -FilePath $workerPython `
        -ArgumentList @("-m", "recommender.worker") `
        -WorkingDirectory $BackendDir `
        -StdoutPath $WorkerStdout `
        -StderrPath $WorkerStderr
    Set-Content -LiteralPath $WorkerPidFile -Value $workerProc.Id

    Wait-BackendHealthy

    Write-Host "[7/7] starting admin-web..."
    $adminProc = Start-AdminWeb

    Write-Host ""
    Write-Host "All services are up."
    Write-Host "Backend PID : $($backendProc.Id)"
    Write-Host "Worker PID  : $($workerProc.Id)"
    if ($adminProc) {
        Write-Host "Admin PID   : $($adminProc.Id)"
    } else {
        Write-Host "Admin PID   : already running"
    }
    Write-Host "Health URL  : http://127.0.0.1:8080/healthz"
    Write-Host "Admin URL   : $AdminUrl"
    Write-Host "Logs        : $LogsDir"
    Write-Host "Admin logs  : $AdminStdout / $AdminStderr"
} finally {
    Pop-Location
}
