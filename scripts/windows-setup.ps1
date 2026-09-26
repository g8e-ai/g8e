# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8e Windows dev setup: validate toolchain, build evaluation-explorer, make build,
# and add the repository root to PATH.
# Requires PowerShell 7+ (pwsh). See docs/architecture/scripts.md.

#Requires -Version 7.0

$ErrorActionPreference = "Stop"

$Script:RepoRoot = (Resolve-Path "$PSScriptRoot\..").Path
$Script:ExplorerDir = Join-Path $Script:RepoRoot "dashboard\g8e-adapter\evaluation-explorer"
$Script:ExplorerDist = Join-Path $Script:ExplorerDir "dist\index.html"
$Script:NodeMinMajor = 22
$Script:AutoYes = $false

foreach ($arg in $args) {
    if ($arg -in @("-y", "--yes")) {
        $Script:AutoYes = $true
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
        Write-Host "  node: missing (need >= $NeedMajor for evaluation-explorer build)" -ForegroundColor Yellow
        return $false
    }
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
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
        Write-Host "  npm: detected ($(npm --version))" -ForegroundColor Green
        return $true
    }
    Write-Host "  node: detected (v$($have.ToString())) but need >= $NeedMajor.0" -ForegroundColor Yellow
    return $false
}

function Refresh-SessionPath {
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
        [System.Environment]::GetEnvironmentVariable("Path", "User")
}

function Install-WindowsPackages {
    param([string[]]$Missing)

    $winget = Get-Command winget -ErrorAction SilentlyContinue
    $choco = Get-Command choco -ErrorAction SilentlyContinue
    if (-not $winget -and -not $choco) {
        Write-Host "FATAL: install winget or Chocolatey, then rerun this script." -ForegroundColor Red
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
    } elseif ($choco) {
        $packages = @()
        if ($Missing -contains "git") { $packages += "git" }
        if ($Missing -contains "go") { $packages += "golang" }
        if ($Missing -contains "node") { $packages += "nodejs-lts" }
        if ($Missing -contains "make") { $packages += "make" }
        if ($packages.Count -gt 0) {
            $commands += "choco install -y $($packages -join ' ')"
        }
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
    }
}

function Build-EvaluationExplorer {
    if (Test-Path $Script:ExplorerDist) {
        Write-Host "  evaluation-explorer dist already present — skipping frontend build" -ForegroundColor Green
        return
    }

    Write-Host "  building evaluation-explorer assets (required by make build)..." -ForegroundColor Cyan
    Push-Location $Script:ExplorerDir
    if (Test-Path "package-lock.json") {
        npm ci
    } else {
        npm install
    }
    npm run build
    Pop-Location
}

function Configure-Path {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $segments = @()
    if ($userPath) {
        $segments = $userPath.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
    }
    if ($segments -contains $Script:RepoRoot) {
        Write-Host "  repository root already present in user PATH — skipping." -ForegroundColor Green
    } else {
        $newPath = if ($userPath) { "$userPath;$($Script:RepoRoot)" } else { $Script:RepoRoot }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Host "  added $($Script:RepoRoot) to user PATH." -ForegroundColor Green
    }
    $env:Path = "$env:Path;$($Script:RepoRoot)"
}

function Show-NextSteps {
    $binary = if (Test-Path (Join-Path $Script:RepoRoot "g8e.exe")) { ".\g8e.exe" } else { ".\g8e" }
    Write-Host ""
    Write-Host "[SETUP COMPLETE]" -ForegroundColor Green
    Write-Host "---------------------------------------------------------------"
    Write-Host "Binary: $binary (repository root added to PATH for this session)"
    Write-Host ""
    Write-Host "Verify:"
    Write-Host "  $binary --version"
    Write-Host ""
    Write-Host "Recommended next steps:"
    Write-Host "  Docker stack:"
    Write-Host "    Copy-Item .env.example .env"
    Write-Host "    $binary docker start --full"
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

Write-Host "`n[SETUP] g8e Windows dev environment setup`n" -ForegroundColor Cyan
Write-Host "[STEP 1/4] Checking prerequisites (git, make, go >= $($goMin.ToString()), node >= $Script:NodeMinMajor)..." -ForegroundColor Yellow

$missing = @()
if (-not (Test-Git)) { $missing += "git" }
if (-not (Test-Make)) { $missing += "make" }
if (-not (Test-Go -Need $goMin)) { $missing += "go" }
if (-not (Test-Node -NeedMajor $Script:NodeMinMajor)) { $missing += "node" }

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
    if (-not (Test-Go -Need $goMin)) { $missing += "go" }
    if (-not (Test-Node -NeedMajor $Script:NodeMinMajor)) { $missing += "node" }
    if ($missing.Count -gt 0) {
        Write-Host "FATAL: still missing: $($missing -join ', ')" -ForegroundColor Red
        Write-Host "Install the remaining tools, then rerun: pwsh scripts/windows-setup.ps1"
        exit 1
    }
}

Write-Host "`n[STEP 2/4] Building evaluation-explorer assets..." -ForegroundColor Yellow
Build-EvaluationExplorer

Write-Host "`n[STEP 3/4] Building g8e..." -ForegroundColor Yellow
Set-Location $Script:RepoRoot
make build
Write-Host "Build successful." -ForegroundColor Green

Write-Host "`n[STEP 4/4] Adding repository root to PATH..." -ForegroundColor Yellow
Configure-Path
Show-NextSteps
