#!/bin/sh
# g8ee Entrypoint script - waits for g8es then starts the application

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

SSL_DIR="/g8es"

# Load security tokens into environment if files exist
# Secrets are encrypted in volume, decrypt them
if [ -f "${SSL_DIR}/internal_auth_token" ]; then
    encrypted_token=$(cat "${SSL_DIR}/internal_auth_token" | tr -d ' \n\r')
    export G8E_INTERNAL_AUTH_TOKEN=$(_decrypt_secret "$encrypted_token" "$G8E_SECRETS_KEY")
fi

if [ -f "${SSL_DIR}/session_encryption_key" ]; then
    encrypted_key=$(cat "${SSL_DIR}/session_encryption_key" | tr -d ' \n\r')
    export G8E_SESSION_ENCRYPTION_KEY=$(_decrypt_secret "$encrypted_key" "$G8E_SECRETS_KEY")
fi

# Wait for g8es to be ready
echo "[G8EE-ENTRYPOINT] Waiting for g8es health check..."
MAX_RETRIES=30
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    # Check if g8es is responding on the health endpoint
    # The health endpoint is now open without a token.
    if curl -s --cacert "${SSL_DIR}/ca.crt" https://g8es:9000/health > /dev/null; then
        echo "[G8EE-ENTRYPOINT] g8es is ready"
        break
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "[G8EE-ENTRYPOINT] g8es not ready yet (attempt $RETRY_COUNT/$MAX_RETRIES), waiting 2s..."
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "[G8EE-ENTRYPOINT] ERROR: g8es health check failed after $MAX_RETRIES attempts"
    exit 1
fi

# Execute the main application - bootstrap service handles secret loading
exec uvicorn app.main:app --host 0.0.0.0 --port 443 \
    --ssl-keyfile "${SSL_DIR}/server.key" \
    --ssl-certfile "${SSL_DIR}/server.crt"
