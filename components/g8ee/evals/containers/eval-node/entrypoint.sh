#!/bin/bash
# Eval Node Entrypoint - Supervises g8e operator for eval scenarios

set -e

NODE_ID="${EVAL_NODE_ID:-eval-node-01}"
NODE_PROFILE="${EVAL_PROFILE:-healthy}"
OPERATOR_ENDPOINT="${G8E_ENDPOINT:-g8e.local}"
OPERATOR_BINARY="/opt/g8e.operator"
OPERATOR_LOG_PREFIX="[$NODE_ID operator]"

echo "[$NODE_ID] Starting eval node (profile: $NODE_PROFILE)"

# DEVICE_TOKEN is required for evals
if [ -z "${DEVICE_TOKEN:-}" ]; then
    echo "[$NODE_ID] ERROR: DEVICE_TOKEN environment variable is required"
    exit 1
fi

# Write realistic filesystem fixtures for scenarios
mkdir -p /var/log/app /etc/app

# Create a realistic log file with some entries
cat > /var/log/app/app.log <<EOF
$(date -u '+%Y-%m-%dT%H:%M:%SZ') [$NODE_ID] Application starting
$(date -u '+%Y-%m-%dT%H:%M:%SZ') [$NODE_ID] Loading configuration from /etc/app/config.json
$(date -u '+%Y-%m-%dT%H:%M:%SZ') [$NODE_ID] Connected to database
$(date -u '+%Y-%m-%dT%H:%M:%SZ') [$NODE_ID] Ready to accept connections
EOF

# Create a config file
cat > /etc/app/config.json <<EOF
{
  "node_id": "$NODE_ID",
  "version": "1.0.0",
  "environment": "production",
  "profile": "$NODE_PROFILE"
}
EOF

# Download and supervise the operator
_run_operator() {
    local attempt=0
    while [ ! -x "$OPERATOR_BINARY" ]; do
        attempt=$((attempt + 1))
        echo "$OPERATOR_LOG_PREFIX downloading binary from $OPERATOR_ENDPOINT (attempt $attempt)..."

        if curl -fsSL --cacert /g8es/ca.crt \
                -H "Authorization: Bearer $DEVICE_TOKEN" \
                -o "$OPERATOR_BINARY" \
                "https://$OPERATOR_ENDPOINT/operator/download/linux/amd64" 2>/dev/null; then
            chmod +x "$OPERATOR_BINARY"
            echo "$OPERATOR_LOG_PREFIX binary ready ($(stat -c%s "$OPERATOR_BINARY" 2>/dev/null || wc -c < "$OPERATOR_BINARY") bytes)"
        else
            echo "$OPERATOR_LOG_PREFIX download failed; retrying in 5s"
            rm -f "$OPERATOR_BINARY"
            sleep 5
        fi

        if [ $attempt -ge 10 ]; then
            echo "$OPERATOR_LOG_PREFIX ERROR: Failed to download operator after $attempt attempts"
            exit 1
        fi
    done

    # Supervised restart loop
    while true; do
        echo "$OPERATOR_LOG_PREFIX starting: $OPERATOR_BINARY -e $OPERATOR_ENDPOINT -D *** --no-git"
        "$OPERATOR_BINARY" \
            -e "$OPERATOR_ENDPOINT" \
            -D "$DEVICE_TOKEN" \
            --no-git 2>&1 \
            | sed -u "s/^/$OPERATOR_LOG_PREFIX /"
        rc=${PIPESTATUS[0]}
        echo "$OPERATOR_LOG_PREFIX exited rc=$rc; restarting in 5s"
        sleep 5
    done
}

_run_operator
