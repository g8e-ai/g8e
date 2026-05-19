// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package listen

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
)

var hostSanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9\.-]`)

func sanitizeHost(host string) string {
	// Only allow alphanumeric, dots, and dashes. Reject everything else.
	return hostSanitizeRe.ReplaceAllString(host, "")
}

func executeTemplate(name, tmpl string, data interface{}) string {
	t := template.Must(template.New(name).Parse(tmpl))
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		// Should never happen with fixed templates
		panic(fmt.Sprintf("template execution failed: %v", err))
	}
	return buf.String()
}

// WindowsTrustScriptBat returns a Windows batch script that trusts the platform CA.
func WindowsTrustScriptBat(host string, port int) string {
	host = sanitizeHost(host)
	portSuffix := ""
	if port != 80 {
		portSuffix = fmt.Sprintf(":%d", port)
	}
	url := fmt.Sprintf("http://%s%s/ca.crt", host, portSuffix)

	const tmpl = `@echo off
:: Escalate to Administrator if not already elevated
>nul 2>&1 "%SYSTEMROOT%\system32\cacls.exe" "%SYSTEMROOT%\system32\config\system"
if '%errorlevel%' NEQ '0' (
    echo Requesting administrative privileges...
    echo Set UAC = CreateObject^("Shell.Application"^) > "%temp%\getadmin.vbs"
    echo UAC.ShellExecute "%~s0", "", "", "runas", 1 >> "%temp%\getadmin.vbs"
    "%temp%\getadmin.vbs"
    exit /B
)
if exist "%temp%\getadmin.vbs" ( del "%temp%\getadmin.vbs" )

set HOST={{.Host}}

echo.
echo ----------------------------------------------------
echo   g8e CA Trust Refresh
echo ----------------------------------------------------
echo.

echo [1/3] Removing any existing g8e certificates...
:: Try to delete from both Root and My stores
certutil -delstore Root g8e >nul 2>&1
certutil -delstore Root "g8e Root CA" >nul 2>&1
certutil -delstore My g8e >nul 2>&1
echo      Done.

echo [2/3] Downloading g8e CA Certificate...
curl -sSf -L -o "%temp%\g8e-ca.crt" {{.URL}}
if %errorlevel% NEQ 0 (
    echo.
    echo ERROR: Failed to download CA certificate from {{.URL}}
    echo Make sure the g8e platform is running and reachable on port {{.Port}}.
    pause
    exit /B 1
)

:: Validate certificate before installing
certutil -dump "%temp%\g8e-ca.crt" >nul 2>&1
if %errorlevel% NEQ 0 (
    del /f /q "%temp%\g8e-ca.crt" >nul 2>&1
    echo.
    echo ERROR: Downloaded file is not a valid certificate.
    pause
    exit /B 1
)

echo [3/3] Trusting g8e CA Certificate...
certutil -addstore -f "Root" "%temp%\g8e-ca.crt"
if %errorlevel% NEQ 0 (
    del /f /q "%temp%\g8e-ca.crt" >nul 2>&1
    echo.
    echo ERROR: Failed to trust the certificate. Run this script as Administrator.
    pause
    exit /B 1
)
del /f /q "%temp%\g8e-ca.crt" >nul 2>&1

echo.
echo ----------------------------------------------------
echo   Success!
echo ----------------------------------------------------
echo.

echo %HOST% | findstr /R "^[0-9][0-9]*\\.[0-9][0-9]*\\.[0-9][0-9]*\\.[0-9][0-9]*$" >nul
if %errorlevel% EQU 0 (
    for /f "tokens=2 delims= " %%A in ('nslookup %HOST% 2^>nul ^| findstr /i "Name:"') do (
        if not "%%A"=="" (
            echo Resolved %HOST% to %%A
            set HOST=%%A
        ) else (
            echo Could not resolve hostname for %HOST%, using IP address
        )
    )
)

echo Restart your browser and navigate to https://%HOST%/
echo.
pause
`
	return executeTemplate(string(constants.Status.Platform.Windows), tmpl, map[string]interface{}{
		"Host": host,
		"URL":  url,
		"Port": port,
	})
}

// UniversalTrustScript returns a POSIX shell script for macOS and Linux.
func UniversalTrustScript(host string, port int) string {
	host = sanitizeHost(host)
	portSuffix := ""
	if port != 80 {
		portSuffix = fmt.Sprintf(":%d", port)
	}
	urlTmpl := fmt.Sprintf("http://%%s%s/ca.crt", portSuffix)

	const tmpl = `#!/bin/sh
set -eu

HOST="{{.Host}}"
URL="{{.URL}}"
CERT_FILE="/tmp/g8e-ca.crt"

_log() { printf "[g8e] %s\n" "$*"; }

if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: This script must run as root (sudo)."
    echo "Usage: curl -fsSL http://${HOST}{{.PortSuffix}}/trust | sudo sh"
    exit 1
fi

_log "----------------------------------------------------"
_log "  g8e CA Trust Refresh"
_log "----------------------------------------------------"
_log ""

_log "[1/3] Removing any existing g8e certificates..."
_OS="$(uname -s)"
case "$_OS" in
  Darwin)
    KEYCHAIN="/Library/Keychains/System.keychain"
    security find-certificate -a -Z "$KEYCHAIN" 2>/dev/null \
        | awk '/^SHA-1/{sha=$NF} /"alis"|<blob>="g8e/ { 
            if ($0 ~ /"alis"/) {
                gsub(/.*"alis"<blob>="|"$/, "");
                print $0
            } else if ($0 ~ /"cn  "/) {
                gsub(/.*"cn  "<blob>="|"$/, "");
                print $0
            }
          }' \
        | sort -u \
        | while IFS= read -r name; do
            [ -z "$name" ] && continue
            _log "      Removing: $name"
            security delete-certificate -c "$name" "$KEYCHAIN" 2>/dev/null || true
          done
    ;;

  Linux)
    # Remove from common locations
    find /usr/local/share/ca-certificates/ -iname '*g8e*' -exec echo "      Removing: {}" \; -delete 2>/dev/null || true
    find /etc/pki/ca-trust/source/anchors/ -iname '*g8e*' -exec echo "      Removing: {}" \; -delete 2>/dev/null || true
    ;;
esac
_log "      Done."

_log ""
_log "[2/3] Fetching CA cert from $URL..."
if ! curl -fsSL -o "$CERT_FILE" "$URL"; then
    echo "ERROR: Failed to download CA certificate from $URL"
    exit 1
fi
_log "      Done."

_log ""
_log "[3/3] Installing CA certificate..."
case "$_OS" in
  Darwin)
    security add-trusted-cert -d -r trustRoot -k "$KEYCHAIN" "$CERT_FILE"
    ;;

  Linux)
    if [ -d /usr/local/share/ca-certificates/ ]; then
        cp "$CERT_FILE" /usr/local/share/ca-certificates/g8e-ca.crt
        update-ca-certificates
    elif [ -d /etc/pki/ca-trust/source/anchors/ ]; then
        cp "$CERT_FILE" /etc/pki/ca-trust/source/anchors/g8e-ca.crt
        update-ca-trust extract
    fi
    ;;
esac
_log "      Done."

rm -f "$CERT_FILE"
if echo "$HOST" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'; then
    RESOLVED_HOST=$(host "$HOST" 2>/dev/null | awk '/pointer/ {print $NF}' | sed 's/\.$//' || echo "")
    [ -n "$RESOLVED_HOST" ] && HOST="$RESOLVED_HOST"
fi

_log ""
_log "----------------------------------------------------"
_log "  Success!"
_log "----------------------------------------------------"
_log "Restart your browser and navigate to https://$HOST/"
_log ""
`
	return executeTemplate(string(constants.ToolScopeUniversal), tmpl, map[string]interface{}{
		"Host":       host,
		"URL":        fmt.Sprintf(urlTmpl, host),
		"PortSuffix": portSuffix,
		"Port":       port,
	})
}

// G8eDeployScript returns a POSIX shell script that deploys the operator binary on any Linux system.
func G8eDeployScript(host string, httpsPort int, httpPort int) string {
	host = sanitizeHost(host)
	httpPortSuffix := ""
	if httpPort != 80 {
		httpPortSuffix = fmt.Sprintf(":%d", httpPort)
	}
	httpsPortSuffix := ""
	if httpsPort != 443 {
		httpsPortSuffix = fmt.Sprintf(":%d", httpsPort)
	}
	httpUrl := fmt.Sprintf("http://%s%s", host, httpPortSuffix)
	httpsHost := fmt.Sprintf("%s%s", host, httpsPortSuffix)

	portFlags := ""
	if httpsPort != 443 {
		portFlags = fmt.Sprintf(" --wss-port %d --http-port %d", httpsPort, httpsPort)
	}

	const tmpl = `#!/bin/sh
# g8e-deploy - deploy the g8e Operator on any Linux system
# Generated by the g8e platform at {{.Timestamp}}
#
# Usage:
#   curl -fsSL {{.HttpUrl}}/g8e | sh -s -- <device-link-token>
#   G8E_TOKEN=<token> curl -fsSL {{.HttpUrl}}/g8e | sh
#
# Requirements: curl or wget, Linux (x64, arm64, or x86)
set -u

G8E_HOST="{{.Host}}"
G8E_HTTPS_HOST="{{.HttpsHost}}"
G8E_HTTP_URL="{{.HttpUrl}}"

# Reverse DNS lookup if host is an IP address
if echo "$G8E_HOST" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' > /dev/null 2>&1; then
    if command -v host >/dev/null 2>&1; then
        RESOLVED_HOST=$(host "$G8E_HOST" 2>/dev/null | awk '/pointer/ {print $NF}' | sed 's/\.$//')
        if [ -n "$RESOLVED_HOST" ]; then
            _log "Resolved $G8E_HOST to $RESOLVED_HOST"
            G8E_HOST="$RESOLVED_HOST"
            G8E_HTTPS_HOST="${RESOLVED_HOST}{{.HttpsPortSuffix}}"
            G8E_HTTP_URL="http://${RESOLVED_HOST}{{.HttpPortSuffix}}"
        fi
    fi
fi

# --- Output helpers (color only when attached to a terminal) ---
_C=0; [ -t 1 ] 2>/dev/null && _C=1
_log() { if [ $_C -eq 1 ]; then printf '\033[36m[g8e]\033[0m %s\n' "$*"; else printf '[g8e] %s\n' "$*"; fi; }
_ok()  { if [ $_C -eq 1 ]; then printf '\033[32m[g8e]\033[0m %s\n' "$*"; else printf '[g8e] %s\n' "$*"; fi; }
_err() { if [ $_C -eq 1 ]; then printf '\033[31m[g8e] %s\033[0m\n' "$*" >&2; else printf '[g8e] %s\n' "$*" >&2; fi; }
_die() { _err "$@"; exit 1; }

# --- Linux check ---
case "$(uname -s)" in
  Linux) ;;
  *) _die "This script is for Linux. Detected: $(uname -s)" ;;
esac

# --- Architecture detection ---
_arch=""
case "$(uname -m)" in
  x86_64|amd64)        _arch=amd64 ;;
  aarch64|arm64)       _arch=arm64 ;;
  i386|i486|i586|i686) _arch=386   ;;
  *) _die "Unsupported architecture: $(uname -m)" ;;
esac

# --- HTTP client detection (curl preferred, wget fallback) ---
_hc=0 _hw=0
command -v curl >/dev/null 2>&1 && _hc=1
command -v wget >/dev/null 2>&1 && _hw=1
[ $_hc -eq 1 ] || [ $_hw -eq 1 ] || _die "curl or wget is required"

_fetch() {
  if [ $_hc -eq 1 ]; then curl -fsSL -o "$2" "$1"
  else wget -q -O "$2" "$1"; fi
}

_fetch_tls() {
  if [ $_hc -eq 1 ]; then curl -fsSL --cacert "$3" -H "Authorization: Bearer $4" -o "$2" "$1"
  else wget -q --ca-certificate="$3" --header="Authorization: Bearer $4" -O "$2" "$1"; fi
}

# --- Token (positional arg or G8E_TOKEN env var) ---
_token="${1:-${G8E_TOKEN:-}}"
if [ -z "$_token" ]; then
  if [ -t 0 ] && [ -t 1 ]; then
    printf '[g8e] Device link token: '
    read _token
  else
    _die "Token required. Usage: curl -fsSL $G8E_HTTP_URL/g8e | sh -s -- <token>"
  fi
fi
[ -n "$_token" ] || _die "Token cannot be empty"

# --- Temp CA cert with cleanup ---
_ca="/tmp/.g8e-ca-$$.pem"
_cleanup() { rm -f "$_ca"; }
trap _cleanup EXIT INT TERM

_log "g8e Operator Deploy"
_log "  Host: $G8E_HOST"
_log "  Arch: linux/$_arch"
_log ""

# Step 1: Fetch platform CA certificate (plain HTTP)
_log "Fetching platform CA certificate..."
if ! _fetch "$G8E_HTTP_URL/ca.crt" "$_ca"; then
  _die "Failed to fetch CA certificate. Ensure the platform is running and port {{.HttpPort}} is reachable."
fi

# Validate certificate before use
if command -v openssl >/dev/null 2>&1; then
  if ! openssl x509 -in "$_ca" -noout >/dev/null 2>&1; then
    _die "Downloaded file is not a valid X.509 certificate."
  fi
elif ! grep -q "BEGIN CERTIFICATE" "$_ca"; then
  _die "Downloaded file does not appear to be a valid PEM certificate."
fi

_ok "CA certificate ready"

# Step 2: Download operator binary (HTTPS with platform CA + bearer auth)
_log "Downloading operator binary (linux/$_arch)..."
if ! _fetch_tls "https://$G8E_HTTPS_HOST/blob/operator-binary/linux-$_arch" \
     "./g8e.operator" "$_ca" "$_token"; then
  _die "Download failed. Check that the token is valid and the platform is accessible on port {{.HttpsPort}}."
fi
_ok "Operator binary ready ($(wc -c < ./g8e.operator | tr -d ' ') bytes)"

# Cleanup CA cert before exec replaces this process
rm -f "$_ca"
trap - EXIT INT TERM

# Step 3: Launch
_log "Starting operator..."
exec ./g8e.operator --device-token "$_token" --endpoint "$G8E_HOST"{{.PortFlags}}
`
	return executeTemplate("deploy", tmpl, map[string]interface{}{
		"Timestamp":       time.Now().Format(time.RFC3339),
		"Host":            host,
		"HttpsHost":       httpsHost,
		"HttpUrl":         httpUrl,
		"HttpsPortSuffix": httpsPortSuffix,
		"HttpPortSuffix":  httpPortSuffix,
		"HttpPort":        httpPort,
		"HttpsPort":       httpsPort,
		"PortFlags":       portFlags,
	})
}

// WindowsPowerShellTrustScript returns a PowerShell script for Windows.
func WindowsPowerShellTrustScript(host string, port int) string {
	host = sanitizeHost(host)
	portSuffix := ""
	if port != 80 {
		portSuffix = fmt.Sprintf(":%d", port)
	}
	url := fmt.Sprintf("http://%s%s/ca.crt", host, portSuffix)

	const tmpl = `#Requires -RunAsAdministrator
$ErrorActionPreference = "Stop"

$url = "{{.URL}}"
$certFile = Join-Path $env:TEMP "g8e-ca.crt"
$g8eHost = "{{.Host}}"

Write-Host ""
Write-Host "----------------------------------------------------"
Write-Host "  g8e CA Trust Refresh"
Write-Host "----------------------------------------------------"
Write-Host ""

Write-Host "[1/3] Removing any existing g8e certificates..."
$stores = @("Cert:\LocalMachine\Root", "Cert:\CurrentUser\Root")
$found = 0
foreach ($storePath in $stores) {
    if (Test-Path $storePath) {
        $certs = Get-ChildItem $storePath | Where-Object { 
            $_.Subject -like "*g8e*" -or 
            $_.FriendlyName -like "*g8e*" -or
            $_.Issuer -like "*g8e*"
        }
        foreach ($cert in $certs) {
            Write-Host "      Removing from $($storePath.Split(':')[-1]): $($cert.Subject)"
            $cert | Remove-Item -Force -ErrorAction SilentlyContinue
            $found++
        }
    }
}
if ($found -eq 0) {
    Write-Host "      None found."
} else {
    Write-Host "      Removed $found existing certificate(s)."
}

Write-Host ""
Write-Host "[2/3] Fetching CA cert from $url..."
try {
    Invoke-WebRequest -Uri $url -OutFile $certFile -UseBasicParsing
    Write-Host "      Done."
} catch {
    Write-Error "Failed to download CA certificate from $url. Is the platform running?"
    exit 1
}

Write-Host ""
Write-Host "[3/3] Installing CA certificate into LocalMachine\Root..."
try {
    certutil -addstore -f "Root" $certFile | Out-Null
    Remove-Item $certFile -Force -ErrorAction SilentlyContinue
    Write-Host "      Done."
} catch {
    Write-Error "Failed to trust g8e CA certificate: $_"
    exit 1
}

$ipRegex = "^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$"
if ($g8eHost -match $ipRegex) {
    try {
        $g8eHost = [System.Net.Dns]::GetHostEntry($g8eHost).HostName
    } catch {}
}

Write-Host ""
Write-Host "----------------------------------------------------"
Write-Host "  Success!"
Write-Host "----------------------------------------------------"
Write-Host "Restart your browser and navigate to https://$g8eHost/"
Write-Host ""
`
	return executeTemplate("powershell", tmpl, map[string]interface{}{
		"URL":  url,
		"Host": host,
		"Port": port,
	})
}
