#!/bin/bash
set -e

# Start the edge device microservice in background
_microservice_log_prefix="[$(hostname) microservice]"
echo "$_microservice_log_prefix starting edge device simulator..."
/opt/microservice.sh 2>&1 | sed -u "s/^/$_microservice_log_prefix /" &
_microservice_pid=$!

# Supervise the g8e operator in-container
_operator_endpoint="${G8E_ENDPOINT:-g8e.local}"
_operator_binary="/home/appuser/g8e.operator"
_operator_log_prefix="[$(hostname) operator]"

if [ -z "${DEVICE_TOKEN:-}" ]; then
    echo "$_operator_log_prefix DEVICE_TOKEN not set; skipping operator"
    # Keep container alive with microservice only
    wait $_microservice_pid
fi

# In production k3s/docker setups, the ca.crt might be mounted. If not, use standard CAs.
# We'll allow insecure curl if needed for local dev, but try with system CA first.
CURL_OPTS="-fsSL"
if [ -f /g8es/ca.crt ]; then
    CURL_OPTS="$CURL_OPTS --cacert /g8es/ca.crt"
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
