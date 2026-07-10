param(
    [switch]$SkipBuild,
    [switch]$SkipAdminInstall
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$BackendDir = Join-Path $RepoRoot "backend"
$AdminWebDir = Join-Path $RepoRoot "admin-web"
$CacheDir = Join-Path $RepoRoot ".gocache"
$TmpDir = Join-Path $CacheDir "tmp"
$ModCacheDir = Join-Path $RepoRoot ".gomodcache"
$BackendBinDir = Join-Path $BackendDir "bin"
$BackendExe = Join-Path $BackendBinDir "backend-server.exe"
$LogsDir = Join-Path $BackendDir "logs"
$BackendPidFile = Join-Path $BackendDir ".server.pid"
$BackendStdout = Join-Path $LogsDir "backend.out.log"
$BackendStderr = Join-Path $LogsDir "backend.err.log"
$AdminPidFile = Join-Path $AdminWebDir ".dev.pid"
$AdminStdout = Join-Path $AdminWebDir "dev.out.log"
$AdminStderr = Join-Path $AdminWebDir "dev.err.log"
$AdminUrl = "http://127.0.0.1:5173/"

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

function Test-BackendHealthy {
    try {
        $response = Invoke-RestMethod -Uri "http://127.0.0.1:8080/healthz" -Method Get -TimeoutSec 2
        return ($response.ok -eq $true)
    } catch {
        return $false
    }
}

function Wait-BackendHealthy {
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Seconds 1
        if (Test-BackendHealthy) {
            return $true
        }
    }
    return $false
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

function Start-BackendForAdmin {
    if (Test-BackendHealthy) {
        Write-Host "      backend is already healthy at http://127.0.0.1:8080/healthz"
        return $null
    }

    $listener = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($listener) {
        throw "port 8080 is in use by PID $($listener.OwningProcess), but /healthz is unavailable. Stop that process first."
    }

    if (-not $SkipBuild) {
        Write-Host "[1/3] building backend..."
        Push-Location $BackendDir
        try {
            & go build -o $BackendExe ./cmd/server
            if ($LASTEXITCODE -ne 0) {
                throw "go build failed"
            }
        } finally {
            Pop-Location
        }
    } elseif (-not (Test-Path -LiteralPath $BackendExe)) {
        throw "backend binary not found. Run without -SkipBuild first."
    } else {
        Write-Host "[1/3] skipping backend build..."
    }

    Stop-FromPidFile $BackendPidFile

    $backendProc = Start-LoggedProcess `
        -FilePath $BackendExe `
        -ArgumentList @() `
        -WorkingDirectory $BackendDir `
        -StdoutPath $BackendStdout `
        -StderrPath $BackendStderr
    Set-Content -LiteralPath $BackendPidFile -Value $backendProc.Id

    if (-not (Wait-BackendHealthy)) {
        throw "backend did not become healthy on :8080 within 30 seconds. Check $BackendStderr"
    }

    return $backendProc
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
        throw "port 5173 is already in use by PID $($listener.OwningProcess). Stop it first, then rerun start-admin-system.bat."
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
Ensure-Directory $CacheDir
Ensure-Directory $TmpDir
Ensure-Directory $ModCacheDir
Ensure-Directory $BackendBinDir
Ensure-Directory $LogsDir

if (-not $env:GOPROXY) { $env:GOPROXY = "https://goproxy.cn,direct" }
if (-not $env:GOSUMDB) { $env:GOSUMDB = "sum.golang.google.cn" }

$env:GOCACHE = $CacheDir
$env:GOTMPDIR = $TmpDir
$env:GOMODCACHE = $ModCacheDir
$env:BACKEND_ADDR = ":8080"
$env:USE_REDIS = "false"
$env:HW_BACKEND_DIR = $BackendDir

Write-Host "Starting admin system..."
$backendProc = Start-BackendForAdmin

Write-Host "[2/3] starting admin-web..."
$adminProc = Start-AdminWeb

Write-Host "[3/3] checking admin system..."
Write-Host ""
Write-Host "Admin system is up."
if ($backendProc) {
    Write-Host "Backend PID    : $($backendProc.Id)"
} else {
    Write-Host "Backend PID    : already running"
}
if ($adminProc) {
    Write-Host "Admin-web PID  : $($adminProc.Id)"
} else {
    Write-Host "Admin-web PID  : already running"
}
Write-Host "Backend health : http://127.0.0.1:8080/healthz"
Write-Host "Admin URL      : $AdminUrl"
Write-Host "Backend logs   : $BackendStdout / $BackendStderr"
Write-Host "Admin logs     : $AdminStdout / $AdminStderr"
