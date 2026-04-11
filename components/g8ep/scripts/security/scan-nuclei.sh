#!/bin/bash
# Nuclei Vulnerability Scanner
# Template-based scanning for known vulnerabilities
#
# Usage: ./scan-nuclei.sh [target_url] [severity]
# Default: https://nginx, all severities

set -e

"$(dirname "$0")/install-scan-tools.sh" nuclei

TARGET="${1:-https://nginx}"
SEVERITY="${2:-low,medium,high,critical}"
REPORT_DIR="/reports"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

echo "Nuclei Scan: $TARGET"
echo "Severity: $SEVERITY"
echo "========================"

nuclei \
    -u "$TARGET" \
    -severity "$SEVERITY" \
    -json-export "$REPORT_DIR/nuclei-$TIMESTAMP.json" \
    -markdown-export "$REPORT_DIR/nuclei-$TIMESTAMP.md" \
    -stats

echo ""
echo "Scan complete."
echo "JSON Report: $REPORT_DIR/nuclei-$TIMESTAMP.json"
echo "Markdown Report: $REPORT_DIR/nuclei-$TIMESTAMP.md"
