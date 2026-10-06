# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8e Windows dev setup: install the toolchain, build evaluation-explorer, make
# build, install the contributor toolchain that `make ci` needs (Go dev tools,
# Python venv, Node deps), and add the repository root to PATH.
# Pass --build-only to stop after `make build`.
# Requires PowerShell 7+ (pwsh). See docs/architecture/scripts.md.

#Requires -Version 7.0

$ErrorActionPreference = "Stop"

$Script:RepoRoot = (Resolve-Path "$PSScriptRoot\..").Path
$Script:ExplorerDir = Join-Path $Script:RepoRoot "evaluation-explorer"
$Script:ExplorerDist = Join-Path $Script:ExplorerDir "dist\runtime.json"
$Script:NodeMinMajor = 22
$Script:AutoYes = $false
$Script:BuildOnly = $false

function Show-Usage {
    @"
Usage: pwsh scripts/windows-setup.ps1 [options]

Installs the toolchain needed to build g8e and, by default, to run 'make ci'
(Go dev tools, a Python venv, and Node dependencies).

Options:
  -y, --yes       Install missing tooling without prompting (or set G8E_SETUP_YES=1)
  --build-only    Only set up what 'make build' needs; skip the contributor toolchain
  -h, --help      Show this help
"@
}

foreach ($arg in $args) {
    if ($arg -in @("-y", "--yes")) {
        $Script:AutoYes = $true
    } elseif ($arg -eq "--build-only") {
        $Script:BuildOnly = $true
    } elseif ($arg -in @("-h", "--help")) {
        Show-Usage
        exit 0
    } else {
        Write-Host "Unknown option: $arg"
        Show-Usage
        exit 2
    }
}
if ($env:G8E_SETUP_YES -eq "1") {
    $Script:AutoYes = $true
}

function Confirm-Setup {
    param([string]$Prompt)
    if ($Script:AutoYes) { return $true }
    $response = Read-Host $Prompt
    return $response -match '^[Yy]$'
}

function Get-MakeVar {
    param([string]$Name)
    $pattern = "^$Name\s*:=\s*(.*)$"
    $line = Select-String -Path (Join-Path $Script:RepoRoot "Makefile") -Pattern $pattern | Select-Object -First 1
    if ($line -and $line.Matches[0].Groups[1].Value) {
        return $line.Matches[0].Groups[1].Value.Trim()
    }
    return $null
}

function Get-GoMinVersion {
    $line = Select-String -Path (Join-Path $Script:RepoRoot "go.mod") -Pattern '^go ' | Select-Object -First 1
    if (-not $line) {
        throw "could not read the required Go version from go.mod"
    }
    return $line.Line.Substring(3).Trim()
}

function Parse-MajorMinor {
    param([string]$Version)
    if ($Version -match '(\d+)\.(\d+)') {
        return [Version]"$($Matches[1]).$($Matches[2])"
    }
    return $null
}

function Test-VersionAtLeast {
    param(
        [Version]$Have,
        [Version]$Need
    )
    return $Have -ge $Need
}

function Test-Git {
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $version = (git --version) -replace '^git version ', ''
        Write-Host "  git: detected ($version)" -ForegroundColor Green
        return $true
    }
    Write-Host "  git: missing" -ForegroundColor Yellow
    return $false
}

function Test-Make {
    if (Get-Command make -ErrorAction SilentlyContinue) {
        Write-Host "  make: detected" -ForegroundColor Green
        return $true
    }
    Write-Host "  make: missing" -ForegroundColor Yellow
    return $false
}

function Test-Curl {
    if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
        Write-Host "  curl: detected" -ForegroundColor Green
        return $true
    }
    Write-Host "  curl: missing (used to download Go, Node.js, and uv)" -ForegroundColor Yellow
    return $false
}

function Test-Go {
    param([Version]$Need)
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Host "  go: missing (need >= $($Need.ToString()) from go.mod)" -ForegroundColor Yellow
        return $false
    }
    $goOutput = go version 2>$null
    $have = $null
    if ($goOutput -match 'go(\d+\.\d+)') {
        $have = [Version]$Matches[1]
    }
    if (-not $have) {
        Write-Host "  go: detected but version unknown" -ForegroundColor Yellow
        return $false
    }
    if (Test-VersionAtLeast -Have $have -Need $Need) {
        Write-Host "  go: detected (v$($have.ToString()), need >= $($Need.ToString()))" -ForegroundColor Green
        return $true
    }
    Write-Host "  go: detected (v$($have.ToString())) but go.mod requires >= $($Need.ToString())" -ForegroundColor Yellow
    return $false
}

function Test-Node {
    param([int]$NeedMajor)
    if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
        Write-Host "  node: missing (need >= $NeedMajor for evaluation-explorer and console builds)" -ForegroundColor Yellow
        return $false
    }
    if (-not (Get-Command npm, npm.cmd -ErrorAction SilentlyContinue)) {
        Write-Host "  npm: missing" -ForegroundColor Yellow
        return $false
    }
    $have = Parse-MajorMinor (node --version)
    $need = [Version]"$NeedMajor.0"
    if (-not $have) {
        Write-Host "  node: detected but version unknown" -ForegroundColor Yellow
        return $false
    }
    if (Test-VersionAtLeast -Have $have -Need $need) {
        Write-Host "  node: detected (v$($have.ToString()))" -ForegroundColor Green
        $npmVer = (npm.cmd --version 2>$null)
        Write-Host "  npm: detected ($npmVer)" -ForegroundColor Green
        return $true
    }
    Write-Host "  node: detected (v$($have.ToString())) but need >= $NeedMajor.0" -ForegroundColor Yellow
    return $false
}

function Test-Python3 {
    $py = Get-Command python.exe, python3.exe -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $py) {
        Write-Host "  python3: missing (used by Makefile doctrines-validate and bsl-headers-check)" -ForegroundColor Yellow
        return $false
    }
    $verOut = & $py.Source --version 2>&1
    Write-Host "  python3: detected ($verOut)" -ForegroundColor Green
    return $true
}

function Test-Uv {
    param([string]$NeedVersion)
    $uv = Get-Command uv.exe, uv -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $uv) {
        $localUv = Join-Path $env:USERPROFILE ".local\bin\uv.exe"
        if (Test-Path $localUv) {
            $uv = Get-Item $localUv
        }
    }
    if (-not $uv) {
        Write-Host "  uv: missing (need $NeedVersion; creates the Python venv and locks ensemble deps)" -ForegroundColor Yellow
        return $false
    }
    $uvPath = if ($uv -is [System.Management.Automation.CommandInfo]) { $uv.Source } else { $uv.FullName }
    $ver = (& $uvPath --version 2>&1)
    Write-Host "  uv: detected ($ver)" -ForegroundColor Green
    return $true
}

function Test-Ripgrep {
    if (Get-Command rg.exe, rg -ErrorAction SilentlyContinue) {
        Write-Host "  rg: detected" -ForegroundColor Green
        return $true
    }
    Write-Host "  rg: missing (ripgrep)" -ForegroundColor Yellow
    return $false
}

function Refresh-SessionPath {
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
        [System.Environment]::GetEnvironmentVariable("Path", "User")
}

function Ensure-NpmBashShims {
    $localBin = Join-Path $env:USERPROFILE ".local\bin"
    if (-not (Test-Path $localBin)) {
        New-Item -ItemType Directory -Path $localBin -Force | Out-Null
    }
    $npmShim = Join-Path $localBin "npm"
    if (-not (Test-Path $npmShim)) {
        $content = "#!/bin/sh`nexec npm.cmd `"`$@`"`n"
        [System.IO.File]::WriteAllText($npmShim, $content, [System.Text.Encoding]::ASCII)
    }
    $npxShim = Join-Path $localBin "npx"
    if (-not (Test-Path $npxShim)) {
        $content = "#!/bin/sh`nexec npx.cmd `"`$@`"`n"
        [System.IO.File]::WriteAllText($npxShim, $content, [System.Text.Encoding]::ASCII)
    }
}

function Install-WindowsPackages {
    param([string[]]$Missing)

    $winget = Get-Command winget -ErrorAction SilentlyContinue
    $choco = Get-Command choco -ErrorAction SilentlyContinue
    $needsPackageManager = @($Missing | Where-Object { $_ -ne "uv" }).Count -gt 0
    if ($needsPackageManager -and -not $winget -and -not $choco) {
        Write-Host "FATAL: install winget (App Installer from the Microsoft Store) or Chocolatey, then rerun this script." -ForegroundColor Red
        exit 1
    }

    $commands = @()
    if ($winget) {
        if ($Missing -contains "git") {
            $commands += "winget install --id Git.Git -e --accept-package-agreements --accept-source-agreements"
        }
        if ($Missing -contains "go") {
            $commands += "winget install --id Golang.Go -e --accept-package-agreements --accept-source-agreements"
        }
        if ($Missing -contains "node") {
            $commands += "winget install --id OpenJS.NodeJS.LTS -e --accept-package-agreements --accept-source-agreements"
        }
        if ($Missing -contains "make") {
            $commands += "winget install --id ezwinports.make -e --accept-package-agreements --accept-source-agreements"
        }
        if ($Missing -contains "python3") {
            $commands += "winget install --id Python.Python.3.12 -e --accept-package-agreements --accept-source-agreements"
        }
        if ($Missing -contains "rg") {
            $commands += "winget install --id BurntSushi.ripgrep.MSVC -e --accept-package-agreements --accept-source-agreements"
        }
    } elseif ($choco) {
        $packages = @()
        if ($Missing -contains "git") { $packages += "git" }
        if ($Missing -contains "go") { $packages += "golang" }
        if ($Missing -contains "node") { $packages += "nodejs-lts" }
        if ($Missing -contains "make") { $packages += "make" }
        if ($Missing -contains "python3") { $packages += "python312" }
        if ($Missing -contains "rg") { $packages += "ripgrep" }
        if ($packages.Count -gt 0) {
            $commands += "choco install -y $($packages -join ' ')"
        }
    }

    if ($Missing -contains "uv") {
        $commands += 'powershell -ExecutionPolicy Bypass -Command "irm https://astral.sh/uv/install.ps1 | iex"'
    }

    if ($commands.Count -eq 0) {
        return
    }

    Write-Host "Suggested installs:" -ForegroundColor Yellow
    $commands | ForEach-Object { Write-Host "  $_" }
    if (Confirm-Setup "Install missing tooling now? [y/N] ") {
        foreach ($cmd in $commands) {
            Invoke-Expression $cmd
        }
        Refresh-SessionPath
        Ensure-NpmBashShims
    }
}

function Ensure-WritableGoPath {
    # A GOPATH pointing at a read-only location (e.g. C:\Program Files\Go\bin) makes
    # `go install` and the module checksum DB fail with "Access is denied".
    $gopath = (go env GOPATH 2>$null)
    $writable = $false
    if ($gopath) {
        try {
            New-Item -ItemType Directory -Path $gopath -Force -ErrorAction Stop | Out-Null
            $probe = Join-Path $gopath ".g8e-write-test"
            [System.IO.File]::WriteAllText($probe, "x")
            Remove-Item $probe -Force -ErrorAction SilentlyContinue
            $writable = $true
        } catch {
            $writable = $false
        }
    }
    $goRoot = (go env GOROOT 2>$null)
    $insideGoRoot = $goRoot -and $gopath -and $gopath.StartsWith($goRoot, [System.StringComparison]::OrdinalIgnoreCase)
    $parent = if ($gopath) { Split-Path $gopath -Leaf } else { "" }
    if ($writable -and -not $insideGoRoot -and $parent -ne "bin") {
        return
    }
    $newGoPath = Join-Path $env:USERPROFILE "go"
    Write-Host "  GOPATH '$gopath' is not usable (read-only or inside the Go install); using $newGoPath." -ForegroundColor Yellow
    $env:GOPATH = $newGoPath
    Remove-Item Env:\GOBIN -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Path $newGoPath -Force | Out-Null
    $userGoPath = [Environment]::GetEnvironmentVariable("GOPATH", "User")
    if ($userGoPath -and $userGoPath -ne $newGoPath) {
        [Environment]::SetEnvironmentVariable("GOPATH", $newGoPath, "User")
        Write-Host "  updated user GOPATH (was '$userGoPath') to $newGoPath." -ForegroundColor Yellow
    }
}

function Build-EvaluationExplorer {
    if (Test-Path $Script:ExplorerDist) {
        Write-Host "  evaluation-explorer dist already present — skipping frontend build" -ForegroundColor Green
        return
    }

    Write-Host "  building evaluation-explorer assets (required by make build)..." -ForegroundColor Cyan
    Push-Location $Script:ExplorerDir
    try {
        $installed = $false
        if (Test-Path "package-lock.json") {
            npm.cmd ci
            $installed = ($LASTEXITCODE -eq 0)
            if (-not $installed) {
                Write-Host "  npm ci failed (lockfile out of sync, e.g. missing Windows optional deps); falling back to npm install without touching package-lock.json..." -ForegroundColor Yellow
            }
        }
        if (-not $installed) {
            npm.cmd install --no-package-lock
            if ($LASTEXITCODE -ne 0) {
                Write-Host "FATAL: npm install failed in evaluation-explorer." -ForegroundColor Red
                exit 1
            }
        }
        npm.cmd run build
        if ($LASTEXITCODE -ne 0) {
            Write-Host "FATAL: evaluation-explorer build failed." -ForegroundColor Red
            exit 1
        }
    } finally {
        Pop-Location
    }
}

function Configure-Path {
    param([bool]$IncludeDevTools = $false)
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $segments = @()
    if ($userPath) {
        $segments = $userPath.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
    }

    $toAdd = @($Script:RepoRoot)
    if ($IncludeDevTools) {
        $goBin = if (Get-Command go -ErrorAction SilentlyContinue) {
            $bin = (go env GOBIN)
            if (-not $bin) { $bin = Join-Path (go env GOPATH) "bin" }
            $bin
        } else {
            Join-Path $env:USERPROFILE "go\bin"
        }
        if ($goBin -and (Test-Path $goBin)) {
            $toAdd += $goBin
        }
        $localBin = Join-Path $env:USERPROFILE ".local\bin"
        if (Test-Path $localBin) {
            $toAdd += $localBin
        }
    }

    $updated = $false
    foreach ($dir in $toAdd) {
        if ($segments -contains $dir) {
            Write-Host "  $dir already present in user PATH — skipping." -ForegroundColor Green
        } else {
            $segments += $dir
            $updated = $true
            Write-Host "  added $dir to user PATH." -ForegroundColor Green
        }
        if (-not ($env:Path.Split(';') -contains $dir)) {
            $env:Path = "$env:Path;$dir"
        }
    }

    if ($updated) {
        $newPath = $segments -join ';'
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    }
}

function Show-NextSteps {
    param([bool]$BuildOnly = $false)
    $binary = if (Test-Path (Join-Path $Script:RepoRoot "g8e.exe")) { ".\g8e.exe" } else { ".\g8e" }
    Write-Host ""
    Write-Host "[SETUP COMPLETE]" -ForegroundColor Green
    Write-Host "---------------------------------------------------------------"
    Write-Host "Binary: $binary (repository root added to PATH for this session)"
    Write-Host ""
    Write-Host "Verify:"
    Write-Host "  $binary --version"
    Write-Host ""
    if (-not $BuildOnly) {
        Write-Host "  make dev-check        # confirms every tool 'make ci' needs is installed"
        Write-Host ""
        Write-Host "Contributor workflow:"
        Write-Host "  make ci               # full local CI (platform, protocol, ensemble, console, website, scripts)"
        Write-Host "  make help             # all targets"
        Write-Host ""
    }
    Write-Host "Recommended next steps:"
    Write-Host "  Docker stack:"
    Write-Host "    Copy-Item .env.example .env"
    Write-Host "    $binary docker start"
    Write-Host ""
    Write-Host "  Native gateway:"
    Write-Host "    $binary gw start"
    Write-Host ""
    Write-Host "  Owner enrollment against a running gateway:"
    Write-Host "    $binary auth enroll user -e localhost"
    Write-Host ""
    Write-Host "Docs: docs/guides/getting_started.md"
    Write-Host "Note: open a new terminal for PATH changes to take effect." -ForegroundColor Yellow
}

if (-not (Test-Path (Join-Path $Script:RepoRoot "go.mod"))) {
    Write-Host "FATAL: run this script from a g8e repository clone (go.mod not found)." -ForegroundColor Red
    exit 1
}

$goMin = [Version](Parse-MajorMinor (Get-GoMinVersion))
$uvVersion = Get-MakeVar "UV_VERSION"
$pythonVersion = Get-MakeVar "PYTHON_VERSION"

if ($Script:BuildOnly) {
    $totalSteps = 4
    $prereqsText = "git, make, curl, go >= $($goMin.ToString()), node >= $Script:NodeMinMajor"
} else {
    $totalSteps = 6
    $prereqsText = "git, make, curl, go >= $($goMin.ToString()), node >= $Script:NodeMinMajor, python3, uv, rg"
}

Write-Host "`n[SETUP] g8e Windows dev environment setup`n" -ForegroundColor Cyan
Write-Host "[STEP 1/$totalSteps] Checking prerequisites ($prereqsText)..." -ForegroundColor Yellow

$missing = @()
if (-not (Test-Git)) { $missing += "git" }
if (-not (Test-Make)) { $missing += "make" }
if (-not (Test-Curl)) { $missing += "curl" }
if (-not (Test-Go -Need $goMin)) { $missing += "go" }
if (-not (Test-Node -NeedMajor $Script:NodeMinMajor)) { $missing += "node" }

if (-not $Script:BuildOnly) {
    if (-not (Test-Python3)) { $missing += "python3" }
    if (-not (Test-Uv -NeedVersion $uvVersion)) { $missing += "uv" }
    if (-not (Test-Ripgrep)) { $missing += "rg" }
}

if ($missing.Count -gt 0) {
    Write-Host ""
    Write-Host "Missing or outdated tooling: $($missing -join ', ')" -ForegroundColor Yellow
    Install-WindowsPackages -Missing $missing
    Refresh-SessionPath
    Write-Host ""
    Write-Host "Re-checking prerequisites..." -ForegroundColor Yellow
    $missing = @()
    if (-not (Test-Git)) { $missing += "git" }
    if (-not (Test-Make)) { $missing += "make" }
    if (-not (Test-Curl)) { $missing += "curl" }
    if (-not (Test-Go -Need $goMin)) { $missing += "go" }
    if (-not (Test-Node -NeedMajor $Script:NodeMinMajor)) { $missing += "node" }
    if (-not $Script:BuildOnly) {
        if (-not (Test-Python3)) { $missing += "python3" }
        if (-not (Test-Uv -NeedVersion $uvVersion)) { $missing += "uv" }
        if (-not (Test-Ripgrep)) { $missing += "rg" }
    }
    if ($missing.Count -gt 0) {
        Write-Host "FATAL: still missing: $($missing -join ', ')" -ForegroundColor Red
        Write-Host "Install the remaining tools, then rerun: pwsh scripts/windows-setup.ps1"
        exit 1
    }
}

Ensure-NpmBashShims
Ensure-WritableGoPath

function Invoke-Step {
    param([string]$Label, [scriptblock]$Command)
    & $Command
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FATAL: $Label failed (exit code $LASTEXITCODE). Fix the error above and rerun: pwsh scripts/windows-setup.ps1" -ForegroundColor Red
        exit 1
    }
}

Write-Host "`n[STEP 2/$totalSteps] Building evaluation-explorer assets..." -ForegroundColor Yellow
Build-EvaluationExplorer

Write-Host "`n[STEP 3/$totalSteps] Building g8e..." -ForegroundColor Yellow
Set-Location $Script:RepoRoot
Invoke-Step "make build" { make build }
Write-Host "Build successful." -ForegroundColor Green

if ($Script:BuildOnly) {
    Write-Host "`n[STEP 4/$totalSteps] Adding repository root to PATH..." -ForegroundColor Yellow
    Configure-Path -IncludeDevTools $false
    Show-NextSteps -BuildOnly $true
    exit 0
}

Write-Host "`n[STEP 4/$totalSteps] Installing the contributor toolchain (Go dev tools, Python venv, Node deps)..." -ForegroundColor Yellow
Invoke-Step "make dev-setup" { make dev-setup }

Write-Host "`n[STEP 5/$totalSteps] Adding repository root and dev tool directories to PATH..." -ForegroundColor Yellow
Configure-Path -IncludeDevTools $true

Write-Host "`n[STEP 6/$totalSteps] Verifying the toolchain (make dev-check)..." -ForegroundColor Yellow
Invoke-Step "make dev-check" { make dev-check }
Show-NextSteps -BuildOnly $false

