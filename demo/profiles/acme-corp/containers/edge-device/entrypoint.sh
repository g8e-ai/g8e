#!/bin/bash
# ACME edge-device entrypoint.
#
# Responsibilities:
#   1. One-time: initialise device config + failure artifacts based on DEVICE_PROFILE.
#   2. Start the supervised device-service (the simulated edge application).
#   3. If DEVICE_TOKEN is set, supervise the g8e operator alongside it.
#
# Every step logs through hostname-prefixed streams so `docker logs` tells a
# coherent story across 1000 devices.

set -u

_host="$(hostname)"
_fn="${DEVICE_FUNCTION:-unknown}"
_site="${DEVICE_SITE:-unknown}"
_loc="${DEVICE_LOCATION:-unknown}"
_profile="${DEVICE_PROFILE:-healthy}"

echo "[$_host boot] device=$_fn site=$_site location=$_loc profile=$_profile"

# ---------------------------------------------------------------------------
# 1. Apply initial profile state (writes /etc/device/config.yaml + artifacts)
# ---------------------------------------------------------------------------
/opt/device/init-profile.sh || {
    echo "[$_host boot] init-profile failed"
    exit 1
}

# ---------------------------------------------------------------------------
# 2. Start supervised device-service in background
# ---------------------------------------------------------------------------
/opt/device/supervisor.sh 2>&1 | sed -u "s/^/[$_host service] /" &
_service_pid=$!

# ---------------------------------------------------------------------------
# 3. Operator (optional). Reuses the large-fleet pattern.
# ---------------------------------------------------------------------------
_operator_endpoint="${G8E_ENDPOINT:-localhost}"
_operator_binary="/home/appuser/g8e.operator"
_operator_prefix="[$_host operator]"

if [[ -z "${DEVICE_TOKEN:-}" ]]; then
    echo "$_operator_prefix DEVICE_TOKEN not set; running device-only"
    wait "$_service_pid"
    exit $?
fi

CURL_OPTS="-fsSL"
if [[ -f /operator/ca.crt ]]; then
    CURL_OPTS="$CURL_OPTS --cacert /operator/ca.crt"
else
    CURL_OPTS="$CURL_OPTS -k"
fi

attempt=0
while [[ ! -x "$_operator_binary" ]]; do
    attempt=$((attempt + 1))
    echo "$_operator_prefix downloading operator from $_operator_endpoint (attempt $attempt)..."
    if curl $CURL_OPTS \
            -H "Authorization: Bearer $DEVICE_TOKEN" \
            -o "$_operator_binary" \
            "https://$_operator_endpoint/operator/download/linux/amd64" 2>/dev/null; then
        chmod +x "$_operator_binary"
        echo "$_operator_prefix binary ready"
    else
        echo "$_operator_prefix download failed; retrying in 5s"
        rm -f "$_operator_binary"
        sleep 5
    fi
done

while true; do
    echo "$_operator_prefix starting..."
    sudo "$_operator_binary" \
        --endpoint "$_operator_endpoint" \
        --working-dir /home/appuser \
        --log info \
        --cloud=false \
        -D "$DEVICE_TOKEN" 2>&1 \
        | sed -u "s/^/$_operator_prefix /"

    echo "$_operator_prefix exited; restarting in 5s"
    sleep 5
done
