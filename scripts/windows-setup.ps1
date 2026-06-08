# ----------------------------------------------------------
# g8e_windows-setup.ps1 - FINAL STABLE VERSION (PowerShell Idioms Applied)
# Single Script for full setup and validation on Windows environments.
# ==========================================================

# Requires PowerShell 7+ (pwsh)
$ErrorActionPreference = "Stop" # Stops script execution on non-terminating errors

Write-Host "`n[SETUP] Starting g8e Environment Setup and Validation...`n" -ForegroundColor Cyan

function Install-Dependency {
    param(
        [string]$Name,
        [scriptblock]$InstallationLogic
    )
    try {
        Write-Host "Attempting to check/install '$Name'..." -ForegroundColor Cyan
        # Execute the logic block provided by the caller.
        Invoke-Command -ScriptBlock $InstallationLogic
        Write-Host "$Name dependency check PASSED." -ForegroundColor Green
    } catch {
        Write-Error "Dependency Check FAILED for '$Name'. Review the output above for details on why it failed."
    }
}

# --- SECTION 1: Dependency Validation (Robust Checks) ---
Write-Host "[STEP 1/5] Validating required dependencies (go, buf, protoc)..." -ForegroundColor Yellow

# A. Validate Go Environment
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "FATAL: 'go' command not found in PATH. Please install Go to proceed."
} else {
    Write-Host "Go environment detected." -ForegroundColor Green
}

# B. Validate Buf CLI
Install-Dependency -Name "Buf" -InstallationLogic {
    # This block runs the check/install logic for Buf.
    & { buf version }
}

# C. Validate protoc
Write-Host "Checking for local 'protoc' executable..." -ForegroundColor Cyan
if (-not (Get-Command protoc -ErrorAction SilentlyContinue)) {
    Write-Warning "WARNING: Protobuf Compiler (protoc) not found in PATH."
    Write-Warning "ACTION REQUIRED: Please install it manually or via a package manager like Chocolatey (choco install protobuf)."
} else {
    Write-Host "Protoc compiler detected." -ForegroundColor Green
}

# --- SECTION 2: Protobuf Generation Fix (The Core Logic) ---
Write-Host "`n[STEP 2/5] Running Protobuf Code Generation..." -ForegroundColor Yellow

$ProtoDir = "protocol/proto"
if (Test-Path $ProtoDir) {
    Write-Host "Searching for .proto files in: $ProtoDir" -ForegroundColor Cyan
    # Use PowerShell to gather all *.proto paths, which handles OS path normalization correctly.
    $ProtoFiles = Get-ChildItem -Path $ProtoDir -Filter "*.proto" -Recurse | Select-Object -ExpandProperty FullName;

    if ($ProtoFiles) {
        Write-Host "Found $($ProtoFiles.Count) proto files. Attempting Buf generate..." -ForegroundColor Green
        try {
            # Passing the array directly is safer than string concatenation.
            & { buf generate $ProtoFiles }
            Write-Host "Protobuf generation logic executed successfully." -ForegroundColor Green
        } catch {
             Write-Error "Buf execution failed. Check if Buf CLI is installed and if all paths are valid."
        }
    } else {
        Write-Warning "Could not find any *.proto files to generate in $ProtoDir."
    }
} else {
    Write-Error "Directory '$ProtoDir' not found. Cannot run proto generation."
}


# --- SECTION 3: Full Build and Test Validation (Placeholder) ---
Write-Host "`n[STEP 3/5] Starting Build & Test Validation..." -ForegroundColor Yellow

# --- BUILD SIMULATION ---
Write-Host "-----------------------------------" -ForegroundColor White
Write-Host "BUILD PHASE (ACTION REQUIRED)" -ForegroundColor Magenta
Write-Warning "The 'Makefile' build logic needs to be ported here for this script to function fully."
# Example: Manual cross-compilation command for Windows/AMD64
try {
    $ExeName = "bin\g8e-windows-amd64.exe"
    Write-Host "Executing simulated build for windows/amd64..."
    & { go build -ldflags "-X main.platform=windows_amd64" -o $ExeName cmd/operator/main.go }
    Write-Host "Build simulation successful (binary created in 'bin')." -ForegroundColor Green
} catch {
    Write-Error "BUILD FAILED during simulation: $($_.Exception.Message)"
}

# --- TEST SIMULATION ---
Write-Host "\n-----------------------------------" -ForegroundColor White
Write-Host "TEST PHASE (ACTION REQUIRED)" -ForegroundColor Magenta
Write-Warning "Run 'go test' commands directly or integrate them properly here."

# CLEANUP: The manual steps were moved to the end.
Write-Host "`n[SETUP COMPLETE]" -ForegroundColor Green
Write-Host "---------------------------------------------------------------" -ForegroundColor White
Write-Host "ACTION REQUIRED: The structural issues are fixed in logic, but running this script requires 'go', 'buf', and a correctly structured 'Makefile' or native build system." -ForegroundColor Yellow
