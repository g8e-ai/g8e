#!/bin/sh
set -e

# Note: g8e.operator --listen reads tokens from --ssl-dir directly; no env exports needed.

# ---------------------------------------------------------------------------
# Publish baked operator binaries to the blob store.
#
# The g8es image ships pre-compiled, UPX-compressed operator binaries for
# linux/amd64, linux/arm64, and linux/386 at /opt/operator-binaries/.
# On every container start we upload them into the blob store so they are
# available for remote operator deployment via g8ed.
#
# The upload runs as a background job after the listen server is healthy.
# This keeps startup fast — the health check passes before the uploads
# finish, and other services can begin connecting immediately.
# ---------------------------------------------------------------------------
_upload_operator_binaries() {
    AUTH_TOKEN=$(cat /ssl/internal_auth_token 2>/dev/null | tr -d ' \n\r')
    if [ -z "$AUTH_TOKEN" ]; then
        echo "[ENTRYPOINT] WARNING: No internal auth token — skipping operator binary upload"
        return
    fi

    BLOB_URL="https://localhost:9000/blob/operator-binary"

    for arch in amd64 arm64 386; do
        BIN="/opt/operator-binaries/linux-${arch}/g8e.operator"
        if [ ! -f "$BIN" ]; then
            echo "[ENTRYPOINT] WARNING: Binary not found: $BIN"
            continue
        fi
        SIZE=$(ls -lh "$BIN" | awk '{print $5}')
        HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' \
            -X PUT \
            --cacert /ssl/ca.crt \
            -H 'Content-Type: application/octet-stream' \
            -H "X-Internal-Auth: ${AUTH_TOKEN}" \
            --data-binary "@${BIN}" \
            "${BLOB_URL}/linux-${arch}")
        if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
            echo "[ENTRYPOINT] Uploaded linux/${arch} Operator binary to blob store (${SIZE})"
        else
            echo "[ENTRYPOINT] ERROR: Failed to upload linux/${arch} (HTTP ${HTTP_CODE})" >&2
        fi
    done
}

_wait_and_upload() {
    until curl -sfk https://localhost:9000/health >/dev/null 2>&1; do
        sleep 0.5
    done
    _upload_operator_binaries
}

_wait_and_upload &

exec g8e.operator --listen \
    --data-dir /data \
    --ssl-dir /ssl \
    --http-listen-port 9000 \
    --wss-listen-port 9001
