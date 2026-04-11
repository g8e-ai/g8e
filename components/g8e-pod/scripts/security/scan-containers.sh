#!/bin/bash
# Container and Dependency Vulnerability Scanner
# Scans Docker images and filesystems for CVEs
#
# Usage: ./scan-containers.sh [image_name]
# If no image specified, scans all vso-* images

set -e

"$(dirname "$0")/install-scan-tools.sh" trivy

REPORT_DIR="/reports"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

if [ -n "$1" ]; then
    IMAGES=("$1")
else
    echo "Discovering VSO images..."
    IMAGES=(vsod vse vso-nginx)
fi

echo "Container Vulnerability Scan"
echo "============================"
echo ""

for IMAGE in "${IMAGES[@]}"; do
    echo "Scanning: $IMAGE"
    echo "----------------------------"
    
    # Trivy scan
    trivy image \
        --severity HIGH,CRITICAL \
        --format json \
        --output "$REPORT_DIR/trivy-$IMAGE-$TIMESTAMP.json" \
        "$IMAGE" 2>/dev/null || {
            echo "  [!] Could not scan $IMAGE (image may not exist locally)"
            continue
        }
    
    # Count vulnerabilities
    HIGH=$(jq '[.Results[]?.Vulnerabilities[]? | select(.Severity == "HIGH")] | length' "$REPORT_DIR/trivy-$IMAGE-$TIMESTAMP.json" 2>/dev/null || echo 0)
    CRIT=$(jq '[.Results[]?.Vulnerabilities[]? | select(.Severity == "CRITICAL")] | length' "$REPORT_DIR/trivy-$IMAGE-$TIMESTAMP.json" 2>/dev/null || echo 0)
    
    echo "  HIGH: $HIGH, CRITICAL: $CRIT"
    echo "  Report: $REPORT_DIR/trivy-$IMAGE-$TIMESTAMP.json"
    echo ""
done

echo "Container scan complete."
