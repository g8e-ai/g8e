#!/bin/bash
# Full Security Audit Script
# Runs all security scanners against the g8e stack
#
# Usage: ./run-full-audit.sh [target]
# Default target: https://nginx (internal docker network)

set -e

"$(dirname "$0")/install-scan-tools.sh" all

TARGET="${1:-https://nginx}"
REPORT_DIR="/reports/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$REPORT_DIR"

echo "========================================"
echo "g8e Security Audit Suite"
echo "Target: $TARGET"
echo "Reports: $REPORT_DIR"
echo "========================================"
echo ""

# Wait for target to be available
echo "[*] Checking target availability..."
timeout 30 bash -c "until curl -ks $TARGET > /dev/null 2>&1; do sleep 1; done" || {
    echo "[!] Target $TARGET not reachable"
    exit 1
}
echo "[+] Target is reachable"
echo ""

# TLS/SSL Audit
echo "========================================"
echo "[1/4] TLS/SSL Configuration (testssl.sh)"
echo "========================================"
HOST=$(echo "$TARGET" | sed -E 's|https?://||' | cut -d'/' -f1)
testssl.sh --quiet --color 0 --jsonfile "$REPORT_DIR/testssl.json" "$HOST" 2>/dev/null || true
echo "[+] TLS report: $REPORT_DIR/testssl.json"
echo ""

# Web Application Scan (Nuclei - fast templates only)
echo "========================================"
echo "[2/4] Vulnerability Scan (Nuclei)"
echo "========================================"
nuclei -u "$TARGET" \
    -t /root/nuclei-templates/http/misconfiguration/ \
    -t /root/nuclei-templates/http/exposures/ \
    -t /root/nuclei-templates/http/vulnerabilities/ \
    -severity low,medium,high,critical \
    -silent \
    -json-export "$REPORT_DIR/nuclei.json" \
    2>/dev/null || true
echo "[+] Nuclei report: $REPORT_DIR/nuclei.json"
echo ""

# Port Scan
echo "========================================"
echo "[3/4] Port Scan (nmap)"
echo "========================================"
nmap -sT -p 1-1000 --open -oN "$REPORT_DIR/nmap.txt" "$HOST" 2>/dev/null || true
echo "[+] Nmap report: $REPORT_DIR/nmap.txt"
echo ""

# HTTP Security Headers
echo "========================================"
echo "[4/4] HTTP Security Headers"
echo "========================================"
{
    echo "Security Headers Analysis for $TARGET"
    echo "======================================"
    echo ""
    curl -ks -I "$TARGET" | grep -iE "^(strict-transport|content-security|x-frame|x-content-type|x-xss|referrer-policy|permissions-policy|cache-control|pragma|set-cookie)" || echo "No security headers found"
    echo ""
    echo "Cookie Analysis:"
    curl -ks -I "$TARGET" | grep -i "set-cookie" | while read -r line; do
        echo "  $line"
        echo "$line" | grep -qi "httponly" && echo "    [+] HttpOnly: YES" || echo "    [-] HttpOnly: NO"
        echo "$line" | grep -qi "secure" && echo "    [+] Secure: YES" || echo "    [-] Secure: NO"
        echo "$line" | grep -qi "samesite" && echo "    [+] SameSite: YES" || echo "    [-] SameSite: NO"
    done
} > "$REPORT_DIR/headers.txt"
echo "[+] Headers report: $REPORT_DIR/headers.txt"
echo ""

# Public Security Grades (for external targets only)
if [[ "$TARGET" != *"nginx"* ]] && [[ "$TARGET" != *"localhost"* ]]; then
    echo "========================================"
    echo "[5/5] Public Security Grades"
    echo "========================================"
    DOMAIN=$(echo "$TARGET" | sed -E 's|https?://||' | cut -d'/' -f1 | cut -d':' -f1)
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    "$SCRIPT_DIR/fetch-public-grades.sh" "$DOMAIN" 2>/dev/null || true
    if [ -f "/reports/security-grades-latest.json" ]; then
        cp /reports/security-grades-latest.json "$REPORT_DIR/"
    fi
fi

# Summary
echo "========================================"
echo "Audit Complete"
echo "========================================"
echo "Reports saved to: $REPORT_DIR"
echo ""
ls -la "$REPORT_DIR"
echo ""

# Generate summary
echo "Quick Summary:" | tee "$REPORT_DIR/summary.txt"
echo "==============" | tee -a "$REPORT_DIR/summary.txt"

if [ -f "$REPORT_DIR/nuclei.json" ]; then
    VULN_COUNT=$(wc -l < "$REPORT_DIR/nuclei.json" 2>/dev/null || echo 0)
    echo "Nuclei findings: $VULN_COUNT" | tee -a "$REPORT_DIR/summary.txt"
fi

if [ -f "$REPORT_DIR/testssl.json" ]; then
    ISSUES=$(jq -r '.[] | select(.severity != "OK" and .severity != "INFO") | .id' "$REPORT_DIR/testssl.json" 2>/dev/null | wc -l || echo 0)
    echo "TLS issues: $ISSUES" | tee -a "$REPORT_DIR/summary.txt"
fi

if [ -f "$REPORT_DIR/nmap.txt" ]; then
    PORTS=$(grep -c "open" "$REPORT_DIR/nmap.txt" 2>/dev/null || echo 0)
    echo "Open ports: $PORTS" | tee -a "$REPORT_DIR/summary.txt"
fi

echo ""
echo "Done."
