#!/bin/bash
set -e

# Set hostname based on container name if not already set properly
if [ -S /var/run/docker.sock ]; then
    # Get container ID from cgroup
    CONTAINER_ID=$(cat /proc/self/cgroup | head -n 1 | cut -d'/' -f3)
    if [ -n "$CONTAINER_ID" ]; then
        # Query Docker API for container name
        CONTAINER_NAME=$(curl -s --unix-socket /var/run/docker.sock "http://localhost/containers/${CONTAINER_ID}/json" | python3 -c "import sys, json; print(json.load(sys.stdin)['Name'][1:])" 2>/dev/null)
        if [ -n "$CONTAINER_NAME" ]; then
            # Set the system hostname to match
            hostname "$CONTAINER_NAME"
        fi
    fi
fi

# Supervise the g8e operator in-container
_operator_endpoint="${G8E_ENDPOINT:-localhost}"
_operator_binary="/home/appuser/g8e.operator"
_operator_log_prefix="[$(hostname) operator]"

if [ -z "${DEVICE_TOKEN:-}" ]; then
    echo "$_operator_log_prefix DEVICE_TOKEN not set; skipping operator"
    # Keep container alive but idle
    exec tail -f /dev/null
fi

# In production k3s/docker setups, the ca.crt might be mounted. If not, use standard CAs.
# We'll allow insecure curl if needed for local dev, but try with system CA first.
CURL_OPTS="-fsSL"
if [ -f /operator/ca.crt ]; then
    CURL_OPTS="$CURL_OPTS --cacert /operator/ca.crt"
else
    # Fallback to insecure if hitting a .local endpoint without certs mounted
    CURL_OPTS="$CURL_OPTS -k"
fi

# Download binary
attempt=0
while [ ! -x "$_operator_binary" ]; do
    attempt=$((attempt + 1))
    echo "$_operator_log_prefix downloading binary from $_operator_endpoint (attempt $attempt)..."
    if curl $CURL_OPTS \
            -H "Authorization: Bearer $DEVICE_TOKEN" \
            -o "$_operator_binary" \
            "https://$_operator_endpoint/operator/download/linux/amd64" 2>/dev/null; then
        chmod +x "$_operator_binary"
        echo "$_operator_log_prefix binary ready"
    else
        echo "$_operator_log_prefix download failed; retrying in 5s"
        rm -f "$_operator_binary"
        sleep 5
    fi
done

# Start a background task to generate fake metrics for the dashboard
(
    mkdir -p /var/log/edge-service
    while true; do
        cat <<EOF > /var/log/edge-service/metrics.json
{
    "cpu_usage": $((RANDOM % 20 + 5)),
    "mem_usage": $((RANDOM % 30 + 10)),
    "disk_usage": $((RANDOM % 10 + 2)),
    "uptime_seconds": $SECONDS
}
EOF
        sleep 5
    done
) &

# Supervised restart loop
while true; do
    echo "$_operator_log_prefix starting..."
    sudo "$_operator_binary" \
        --endpoint "$_operator_endpoint" \
        --working-dir /home/appuser \
        --log info \
        --cloud false \
        -D "$DEVICE_TOKEN" 2>&1 \
        | sed -u "s/^/$_operator_log_prefix /"
    
    echo "$_operator_log_prefix exited; restarting in 5s"
    sleep 5
done
