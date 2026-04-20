#!/bin/sh
# g8ed Entrypoint script - waits for g8es then starts the application

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

# Wait for g8es to be ready and platform_settings to be initialized
echo "[G8ED-ENTRYPOINT] Waiting for g8es health check and platform_settings..."
MAX_RETRIES=30
RETRY_COUNT=0
SSL_DIR="${G8E_SSL_DIR:-/g8es}"

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

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if [ -f "${SSL_DIR}/internal_auth_token" ] && [ -f "${SSL_DIR}/ca.crt" ]; then
        INTERNAL_TOKEN="$G8E_INTERNAL_AUTH_TOKEN"
        # Check if g8es is responding on the health endpoint
        # AND check if platform_settings is initialized (returns 200 instead of 401)
        if curl -s -f --cacert "${SSL_DIR}/ca.crt" -H "X-Internal-Auth: ${INTERNAL_TOKEN}" https://g8es:9001/db/settings/platform_settings > /dev/null; then
            echo "[G8ED-ENTRYPOINT] g8es is ready and platform_settings are initialized"
            break
        fi
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "[G8ED-ENTRYPOINT] g8es not fully ready yet (attempt $RETRY_COUNT/$MAX_RETRIES), waiting 2s..."
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "[G8ED-ENTRYPOINT] ERROR: g8es health check failed or platform_settings missing after $MAX_RETRIES attempts"
    exit 1
fi

# Trust the g8es CA certificate for Node.js built-in fetch (undici)
export NODE_EXTRA_CA_CERTS="${SSL_DIR}/ca.crt"

exec node server.js
