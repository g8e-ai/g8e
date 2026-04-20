#!/bin/sh
set -e

# Encryption/decryption helper functions
_decrypt_secret() {
    local encrypted="$1"
    local key="$2"
    if [ -z "$key" ]; then
        echo "$encrypted"
        return
    fi
    python3 /usr/local/bin/encrypt_secret.py decrypt "$encrypted" "$key"
}

# Load security tokens into environment if files exist
# Secrets are encrypted in volume, decrypt them
if [ -f /ssl/internal_auth_token ]; then
    encrypted_token=$(cat /ssl/internal_auth_token | tr -d ' \n\r')
    export G8E_INTERNAL_AUTH_TOKEN=$(_decrypt_secret "$encrypted_token" "$G8E_SECRETS_KEY")
fi

if [ -f /ssl/session_encryption_key ]; then
    encrypted_key=$(cat /ssl/session_encryption_key | tr -d ' \n\r')
    export G8E_SESSION_ENCRYPTION_KEY=$(_decrypt_secret "$encrypted_key" "$G8E_SECRETS_KEY")
fi

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
    # Decrypt the token if it's encrypted
    AUTH_TOKEN=$(_decrypt_secret "$AUTH_TOKEN" "$G8E_SECRETS_KEY")

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
            --cacert /ssl/ca/ca.crt \
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
