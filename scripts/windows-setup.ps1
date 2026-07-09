# ----------------------------------------------------------
# g8e_windows-setup.ps1 - Setup and validation for Windows environments
# Single Script for full setup and validation on Windows.
# ==========================================================

# Requires PowerShell 7+ (pwsh)
$ErrorActionPreference = "Stop" # Stops script execution on non-terminating errors

Write-Host "`n[SETUP] Starting g8e Environment Setup and Validation...`n" -ForegroundColor Cyan

# --- SECTION 1: Dependency Validation & Installation ---
Write-Host "[STEP 1/4] Validating required dependencies (make, go)..." -ForegroundColor Yellow

$Missing = @()

if (-not (Get-Command make -ErrorAction SilentlyContinue)) {
    $Missing += "make"
} else {
    Write-Host "  make: detected" -ForegroundColor Green
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    $Missing += "go"
} else {
    Write-Host "  go: detected" -ForegroundColor Green
}

if ($Missing.Count -gt 0) {
    Write-Host "`nThe following dependencies are missing: $($Missing -join ', ')" -ForegroundColor Yellow

    # Determine available package manager
    $PackageManager = $null
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        $PackageManager = "winget"
    } elseif (Get-Command choco -ErrorAction SilentlyContinue) {
        $PackageManager = "choco"
    } else {
        Write-Host "FATAL: No supported package manager found (winget or choco)." -ForegroundColor Red
        Write-Host "Install winget (App Installer from Microsoft Store) or Chocolatey (https://chocolatey.org)." -ForegroundColor Yellow
        Write-Host "Then install: $($Missing -join ', ')" -ForegroundColor Yellow
        exit 1
    }

    $InstallCmd = switch ($PackageManager) {
        "winget" { "winget install --id Golang.Go --accept-package-agreements --accept-source-agreements; if ($Missing -contains 'make') { winget install --id GnuWin32.Make --accept-package-agreements --accept-source-agreements }" }
        "choco"  { "choco install -y golang make" }
    }

    Write-Host "They can be installed via: $PackageManager" -ForegroundColor Yellow
    $response = Read-Host "Would you like to install them now? [y/N]"
    if ($response -match '^[Yy]$') {
        Write-Host "Installing: $($Missing -join ', ')..." -ForegroundColor Cyan
        Invoke-Expression $InstallCmd
        # Refresh PATH for the current session
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
        Write-Host "Installation complete." -ForegroundColor Green
    } else {
        Write-Host "FATAL: Required dependencies not installed. Please install them manually." -ForegroundColor Red
        exit 1
    }
}

# --- SECTION 2: Build ---
Write-Host "`n[STEP 2/4] Building g8e..." -ForegroundColor Yellow
make build
Write-Host "Build successful." -ForegroundColor Green

# --- SECTION 3: Add g8e to PATH ---
Write-Host "`n[STEP 3/4] Adding g8e to PATH..." -ForegroundColor Yellow

$G8E_DIR = (Resolve-Path "$PSScriptRoot\..").Path
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")

if ($UserPath -split ';' -contains $G8E_DIR) {
    Write-Host "  g8e directory already in user PATH — skipping." -ForegroundColor Green
} else {
    $NewPath = if ($UserPath) { "$UserPath;$G8E_DIR" } else { $G8E_DIR }
    [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
    Write-Host "  Added $G8E_DIR to user PATH." -ForegroundColor Green
}

# Also update PATH for the current session
$env:Path = "$env:Path;$G8E_DIR"

# --- SECTION 4: Complete ---
Write-Host "`n[SETUP COMPLETE]" -ForegroundColor Green
Write-Host "---------------------------------------------------------------" -ForegroundColor White
Write-Host "Binary available at: g8e.exe (in PATH)"
Write-Host "Start the gateway with: g8e.exe gw start"
Write-Host "Note: Open a new terminal for PATH changes to take effect." -ForegroundColor Yellow
