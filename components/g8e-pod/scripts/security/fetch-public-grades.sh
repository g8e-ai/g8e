#!/bin/bash
# Fetch Public Security Grades
# Queries industry-standard security grading services and generates a report
#
# Usage: ./fetch-public-grades.sh <domain>
# Example: ./fetch-public-grades.sh g8e.ai
#
# Outputs JSON report suitable for embedding on security page

set -e

DOMAIN="${1:-}"
REPORT_DIR="/reports"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
OUTPUT_FILE="${REPORT_DIR}/public-grades-${TIMESTAMP}.json"

if [ -z "$DOMAIN" ]; then
    echo "Usage: $0 <domain>"
    echo "Example: $0 g8e.ai"
    exit 1
fi

echo "Fetching public security grades for: $DOMAIN"
echo "=============================================="
echo ""

# Initialize JSON structure
cat > "$OUTPUT_FILE" << EOF
{
  "domain": "$DOMAIN",
  "scan_timestamp": "$(date -Iseconds)",
  "grades": {},
  "details": {}
}
EOF

# Function to update JSON
update_json() {
    local key="$1"
    local value="$2"
    local tmp=$(mktemp)
    jq --arg k "$key" --argjson v "$value" '.grades[$k] = $v' "$OUTPUT_FILE" > "$tmp" && mv "$tmp" "$OUTPUT_FILE"
}

update_details() {
    local key="$1"
    local value="$2"
    local tmp=$(mktemp)
    jq --arg k "$key" --argjson v "$value" '.details[$k] = $v' "$OUTPUT_FILE" > "$tmp" && mv "$tmp" "$OUTPUT_FILE"
}

# 1. Qualys SSL Labs
echo "[1/3] Qualys SSL Labs (TLS configuration)..."
echo "      This may take 2-3 minutes for a full scan..."

SSL_LABS_RESULT=$(curl -s "https://api.ssllabs.com/api/v3/analyze?host=${DOMAIN}&fromCache=on&maxAge=24" 2>/dev/null || echo '{"status":"ERROR"}')
SSL_STATUS=$(echo "$SSL_LABS_RESULT" | jq -r '.status // "ERROR"')

if [ "$SSL_STATUS" = "READY" ]; then
    SSL_GRADE=$(echo "$SSL_LABS_RESULT" | jq -r '.endpoints[0].grade // "Unknown"')
    echo "      Grade: $SSL_GRADE"
    update_json "ssl_labs" "{\"grade\": \"$SSL_GRADE\", \"status\": \"ready\", \"source\": \"Qualys SSL Labs\"}"
    update_details "ssl_labs" "$SSL_LABS_RESULT"
elif [ "$SSL_STATUS" = "IN_PROGRESS" ] || [ "$SSL_STATUS" = "DNS" ]; then
    echo "      Scan in progress, triggering new scan..."
    # Trigger a new scan and wait
    curl -s "https://api.ssllabs.com/api/v3/analyze?host=${DOMAIN}&startNew=on" > /dev/null 2>&1
    
    # Poll for results (max 3 minutes)
    for i in {1..18}; do
        sleep 10
        SSL_LABS_RESULT=$(curl -s "https://api.ssllabs.com/api/v3/analyze?host=${DOMAIN}" 2>/dev/null)
        SSL_STATUS=$(echo "$SSL_LABS_RESULT" | jq -r '.status // "ERROR"')
        if [ "$SSL_STATUS" = "READY" ]; then
            SSL_GRADE=$(echo "$SSL_LABS_RESULT" | jq -r '.endpoints[0].grade // "Unknown"')
            echo "      Grade: $SSL_GRADE"
            update_json "ssl_labs" "{\"grade\": \"$SSL_GRADE\", \"status\": \"ready\", \"source\": \"Qualys SSL Labs\"}"
            update_details "ssl_labs" "$SSL_LABS_RESULT"
            break
        fi
        echo "      Still scanning... ($i/18)"
    done
    
    if [ "$SSL_STATUS" != "READY" ]; then
        echo "      Scan timeout - check https://www.ssllabs.com/ssltest/analyze.html?d=$DOMAIN"
        update_json "ssl_labs" "{\"grade\": \"Pending\", \"status\": \"timeout\", \"source\": \"Qualys SSL Labs\"}"
    fi
else
    echo "      Error or not available"
    update_json "ssl_labs" "{\"grade\": \"N/A\", \"status\": \"error\", \"source\": \"Qualys SSL Labs\"}"
fi

# 2. Mozilla Observatory
echo ""
echo "[2/3] Mozilla Observatory (security headers & best practices)..."

# Trigger scan - validate JSON response
OBSERVATORY_SCAN=$(curl -s -X POST "https://http-observatory.security.mozilla.org/api/v1/analyze?host=${DOMAIN}&hidden=true" 2>/dev/null)
if ! echo "$OBSERVATORY_SCAN" | jq -e . >/dev/null 2>&1; then
    echo "      API returned invalid response, skipping"
    update_json "mozilla_observatory" "{\"grade\": \"N/A\", \"status\": \"api_error\", \"source\": \"Mozilla Observatory\"}"
else
    OBSERVATORY_STATE=$(echo "$OBSERVATORY_SCAN" | jq -r '.state // "FAILED"')
    
    if [ "$OBSERVATORY_STATE" = "FINISHED" ]; then
        OBSERVATORY_GRADE=$(echo "$OBSERVATORY_SCAN" | jq -r '.grade // "Unknown"')
        OBSERVATORY_SCORE=$(echo "$OBSERVATORY_SCAN" | jq -r '.score // 0')
        echo "      Grade: $OBSERVATORY_GRADE (Score: $OBSERVATORY_SCORE/100)"
        update_json "mozilla_observatory" "{\"grade\": \"$OBSERVATORY_GRADE\", \"score\": $OBSERVATORY_SCORE, \"status\": \"ready\", \"source\": \"Mozilla Observatory\"}"
    elif [ "$OBSERVATORY_STATE" = "PENDING" ] || [ "$OBSERVATORY_STATE" = "RUNNING" ]; then
        for i in {1..12}; do
            sleep 5
            OBSERVATORY_SCAN=$(curl -s "https://http-observatory.security.mozilla.org/api/v1/analyze?host=${DOMAIN}" 2>/dev/null)
            if echo "$OBSERVATORY_SCAN" | jq -e . >/dev/null 2>&1; then
                OBSERVATORY_STATE=$(echo "$OBSERVATORY_SCAN" | jq -r '.state // "FAILED"')
                if [ "$OBSERVATORY_STATE" = "FINISHED" ]; then
                    OBSERVATORY_GRADE=$(echo "$OBSERVATORY_SCAN" | jq -r '.grade // "Unknown"')
                    OBSERVATORY_SCORE=$(echo "$OBSERVATORY_SCAN" | jq -r '.score // 0')
                    echo "      Grade: $OBSERVATORY_GRADE (Score: $OBSERVATORY_SCORE/100)"
                    update_json "mozilla_observatory" "{\"grade\": \"$OBSERVATORY_GRADE\", \"score\": $OBSERVATORY_SCORE, \"status\": \"ready\", \"source\": \"Mozilla Observatory\"}"
                    break
                fi
            fi
        done
        
        if [ "$OBSERVATORY_STATE" != "FINISHED" ]; then
            echo "      Scan timeout"
            update_json "mozilla_observatory" "{\"grade\": \"Pending\", \"status\": \"timeout\", \"source\": \"Mozilla Observatory\"}"
        fi
    else
        echo "      Error: $OBSERVATORY_STATE"
        update_json "mozilla_observatory" "{\"grade\": \"N/A\", \"status\": \"error\", \"source\": \"Mozilla Observatory\"}"
    fi
fi

# 3. SecurityHeaders.com (no official API, scrape grade from response headers)
echo ""
echo "[3/3] SecurityHeaders.com (HTTP security headers)..."

# SecurityHeaders returns grade in X-Grade header when you request their scan page
SECHEADERS_RESPONSE=$(curl -sI "https://securityheaders.com/?q=https://${DOMAIN}&followRedirects=on" 2>/dev/null)
SECHEADERS_GRADE=$(echo "$SECHEADERS_RESPONSE" | grep -i "^x-grade:" | awk '{print $2}' | tr -d '\r\n')

if [ -n "$SECHEADERS_GRADE" ]; then
    echo "      Grade: $SECHEADERS_GRADE"
    update_json "security_headers" "{\"grade\": \"$SECHEADERS_GRADE\", \"status\": \"ready\", \"source\": \"SecurityHeaders.com\"}"
else
    echo "      Could not fetch grade (check https://securityheaders.com/?q=https://$DOMAIN)"
    update_json "security_headers" "{\"grade\": \"N/A\", \"status\": \"error\", \"source\": \"SecurityHeaders.com\"}"
fi

# Generate summary
echo ""
echo "=============================================="
echo "Summary"
echo "=============================================="
jq -r '.grades | to_entries[] | "  \(.key): \(.value.grade) (\(.value.source))"' "$OUTPUT_FILE"
echo ""
echo "Full report: $OUTPUT_FILE"

# Create a simplified version for embedding
EMBED_FILE="${REPORT_DIR}/security-grades-latest.json"
jq '{
  domain: .domain,
  scan_date: .scan_timestamp,
  ssl_labs: .grades.ssl_labs.grade,
  mozilla_observatory: .grades.mozilla_observatory.grade,
  mozilla_score: .grades.mozilla_observatory.score,
  security_headers: .grades.security_headers.grade
}' "$OUTPUT_FILE" > "$EMBED_FILE"

echo "Embed-ready report: $EMBED_FILE"
cat "$EMBED_FILE"
