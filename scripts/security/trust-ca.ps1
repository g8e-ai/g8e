#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Install the g8e platform CA certificate into the Windows trusted root store.

.DESCRIPTION
    Fetches the CA cert from the platform via SSH, removes any previously installed
    g8e CA cert, and installs the new one into LocalMachine\Root.

    Run this whenever SSL certificates are rotated (e.g. after platform rebuild).

.PARAMETER Server
    SSH target in the format <user>@<server>. Example: admin@10.0.0.2 or admin@g8e.local

.EXAMPLE
    .\trust-ca.ps1 -Server admin@10.0.0.2

.EXAMPLE
    .\trust-ca.ps1 -Server admin@g8e.local
#>

param(
    [Parameter(Mandatory = $true)]
    [string]$Server
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$CertSubjectPattern = "*g8e*"
$TempCert = Join-Path $env:TEMP "g8e-ca.crt"

Write-Host ""
Write-Host "----------------------------------------------------"
Write-Host "  g8e CA trust refresh"
Write-Host "----------------------------------------------------"
Write-Host ""

Write-Host "[1/3] Removing any existing g8e CA certificates..."
$existing = @(Get-ChildItem Cert:\LocalMachine\Root |
    Where-Object { $_.Subject -like $CertSubjectPattern })

if ($existing.Count -gt 0) {
    $existing | Remove-Item
    Write-Host "      Removed $($existing.Count) existing certificate(s)."
} else {
    Write-Host "      None found."
}

Write-Host ""
Write-Host "[2/3] Fetching CA cert from ${Server}..."
$certPem = ssh $Server "docker exec g8ep cat /g8es/ssl/ca.crt"

if (-not $certPem) {
    Write-Error "No certificate data received from ${Server}. Is the platform running?"
    exit 1
}

$certPem | Out-File -Encoding ascii $TempCert

Write-Host ""
Write-Host "[3/3] Installing CA certificate into LocalMachine\Root..."
$result = Import-Certificate -FilePath $TempCert -CertStoreLocation Cert:\LocalMachine\Root

Write-Host ""
Write-Host "  Thumbprint : $($result.Thumbprint)"
Write-Host "  Subject    : $($result.Subject)"
Write-Host ""

Remove-Item $TempCert -ErrorAction SilentlyContinue

Write-Host "----------------------------------------------------"
Write-Host "  Done. Close all browser windows and reopen."
Write-Host "----------------------------------------------------"
Write-Host ""
