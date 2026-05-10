#!/bin/bash
# Eval Node Entrypoint - Supervises g8e operator for eval scenarios

set -e

NODE_ID="${EVAL_NODE_ID:-${HOSTNAME:-eval-node}}"
NODE_PROFILE="${EVAL_PROFILE:-healthy}"
OPERATOR_ENDPOINT="${G8E_ENDPOINT:-g8e.local}"
OPERATOR_BINARY="/opt/g8e/g8e.operator"
OPERATOR_LOG_PREFIX="[$NODE_ID operator]"

echo "[$NODE_ID] Starting eval node (profile: $NODE_PROFILE)"

# Write CA cert from environment if provided
if [ -n "${G8E_CA_CERT:-}" ]; then
    mkdir -p /tmp/ssl
    echo "$G8E_CA_CERT" > /tmp/ssl/ca.crt
    echo "[$NODE_ID] CA certificate written to /tmp/ssl/ca.crt"
fi

# DEVICE_TOKEN is optional at startup; operator will wait if it's missing
if [ -z "${DEVICE_TOKEN:-}" ]; then
    echo "[$NODE_ID] WARNING: DEVICE_TOKEN environment variable is not set"
    echo "[$NODE_ID] Operator will not start until container is restarted with a token"
    # Just hang so the container stays 'running' but idle
    exec tail -f /dev/null
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

# Supervise the operator (binary is baked into image)
_run_operator() {
    if [ ! -x "$OPERATOR_BINARY" ]; then
        echo "$OPERATOR_LOG_PREFIX ERROR: Operator binary not found at $OPERATOR_BINARY"
        exit 1
    fi
    echo "$OPERATOR_LOG_PREFIX binary ready ($(stat -c%s "$OPERATOR_BINARY" 2>/dev/null || wc -c < "$OPERATOR_BINARY") bytes)"

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
