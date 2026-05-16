#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Install the g8e platform CA certificate into the Windows trusted root store.

.DESCRIPTION
    Fetches the CA cert from the platform, removes any previously installed
    g8e CA cert, and installs the new one into LocalMachine\Root.

    Run this whenever SSL certificates are rotated (e.g. after platform rebuild).

.PARAMETER Url
    Platform URL to fetch the root CA from. Example: http://localhost

.EXAMPLE
    .\trust-ca.ps1 -Url http://localhost
#>

param(
    [Parameter(Mandatory = $true)]
    [string]$Url
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
Write-Host "[2/3] Fetching CA cert from ${Url}..."
$certUrl = "${Url}/.well-known/g8e/pki/root.pem"
$certPem = curl -s $certUrl --insecure

if (-not $certPem) {
    Write-Error "No certificate data received from ${certUrl}. Is the platform running?"
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
