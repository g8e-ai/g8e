# g8e Auto-Detect Deploy Script for Windows
# Detects architecture automatically and deploys the appropriate g8e binary.
# This script is embedded in the g8e gateway binary and served at:
#   http://<gateway-ip>:8080/g8e-operator.ps1
# Run on remote hosts to download and deploy the g8e binary.

$ErrorActionPreference = "Stop"

$GatewayHost = if ($env:GATEWAY_HOST) { $env:GATEWAY_HOST } else { "{{.GatewayHost}}" }
$GatewayPort = if ($env:GATEWAY_PORT) { $env:GATEWAY_PORT } else { "{{.GatewayPort}}" }

Write-Host "Deploying g8e..." -ForegroundColor Green

# Clean up existing certificates
Write-Host "Cleaning up existing certificates..." -ForegroundColor Yellow
$PkiDir = Join-Path $env:USERPROFILE ".g8e\pki"
if (Test-Path $PkiDir) {
    Remove-Item -Recurse -Force $PkiDir -ErrorAction SilentlyContinue
}

# Detect architecture
$arch = $env:PROCESSOR_ARCHITECTURE
switch ($arch) {
    "AMD64" { $arch = "amd64" }
    "ARM64" { $arch = "arm64" }
    "x86"   { $arch = "386" }
    default {
        Write-Host "Unsupported architecture: $arch" -ForegroundColor Red
        Write-Host "Supported architectures: amd64, arm64, 386"
        exit 1
    }
}

$BinaryName = "g8e-windows-$arch.exe"
$BinaryUrl = "http://${GatewayHost}:${GatewayPort}/.well-known/g8e/bin/${BinaryName}"

Write-Host "Detected: Windows $arch" -ForegroundColor Yellow
Write-Host "Downloading from: $BinaryUrl" -ForegroundColor Yellow

# Remove existing binary to ensure overwrite
Write-Host "Removing existing g8e.exe binary..." -ForegroundColor Yellow
Remove-Item -Force "g8e.exe" -ErrorAction SilentlyContinue

# Download binary
try {
    Invoke-RestMethod -Uri $BinaryUrl -OutFile "g8e.exe"
} catch {
    Write-Host "Failed to download g8e: $_" -ForegroundColor Red
    exit 1
}

Write-Host "g8e deployed successfully!" -ForegroundColor Green

Write-Host "Starting g8e Operator to connect to Gateway at ${GatewayHost}..." -ForegroundColor Yellow
Write-Host "DEBUG: GatewayHost = ${GatewayHost}" -ForegroundColor Yellow
& .\g8e.exe -e $GatewayHost
