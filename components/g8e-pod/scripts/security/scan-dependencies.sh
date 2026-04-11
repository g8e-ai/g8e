#!/bin/bash
# Dependency Vulnerability Scanner
# Scans project dependencies for known CVEs using Grype
#
# Usage: ./scan-dependencies.sh [directory]
# Default: Scans mounted /app directory

set -e

"$(dirname "$0")/install-scan-tools.sh" grype

TARGET_DIR="${1:-/app}"
REPORT_DIR="/reports"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

echo "Dependency Vulnerability Scan"
echo "=============================="
echo "Target: $TARGET_DIR"
echo ""

if [ ! -d "$TARGET_DIR" ]; then
    echo "[!] Directory $TARGET_DIR not found"
    echo "    Mount your project directory to /app or specify a path"
    exit 1
fi

# Scan with Grype
grype dir:"$TARGET_DIR" \
    --output json \
    --file "$REPORT_DIR/grype-deps-$TIMESTAMP.json" \
    --fail-on critical

# Summary
echo ""
echo "Scan complete."
echo "Report: $REPORT_DIR/grype-deps-$TIMESTAMP.json"

# Quick count
if [ -f "$REPORT_DIR/grype-deps-$TIMESTAMP.json" ]; then
    echo ""
    echo "Summary:"
    jq -r '.matches | group_by(.vulnerability.severity) | .[] | "\(.[0].vulnerability.severity): \(length)"' \
        "$REPORT_DIR/grype-deps-$TIMESTAMP.json" 2>/dev/null || true
fi
