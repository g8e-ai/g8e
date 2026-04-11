#!/bin/bash
# TLS/SSL Configuration Scanner
# Tests TLS configuration against best practices
#
# Usage: ./scan-tls.sh [host:port]
# Default: nginx:443

set -e

"$(dirname "$0")/install-scan-tools.sh" testssl

TARGET="${1:-nginx:443}"
REPORT_DIR="/reports"

echo "TLS/SSL Scan: $TARGET"
echo "========================"

testssl.sh \
    --color 0 \
    --quiet \
    --sneaky \
    --protocols \
    --server-defaults \
    --headers \
    --vulnerabilities \
    --cipher-per-proto \
    --jsonfile "$REPORT_DIR/tls-$(date +%Y%m%d-%H%M%S).json" \
    "$TARGET"

echo ""
echo "Scan complete. Report saved to $REPORT_DIR"
