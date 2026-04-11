#!/bin/sh
# g8ee Entrypoint script - waits for VSODB then starts the application

set -e

SSL_DIR="/vsodb"

# Wait for VSODB to be ready
echo "[G8EE-ENTRYPOINT] Waiting for VSODB health check..."
MAX_RETRIES=30
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    # Check if VSODB is responding on the health endpoint
    # The health endpoint is now open without a token.
    if curl -s --cacert "${SSL_DIR}/ca.crt" https://vsodb:9000/health > /dev/null; then
        echo "[G8EE-ENTRYPOINT] VSODB is ready"
        break
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "[G8EE-ENTRYPOINT] VSODB not ready yet (attempt $RETRY_COUNT/$MAX_RETRIES), waiting 2s..."
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "[G8EE-ENTRYPOINT] ERROR: VSODB health check failed after $MAX_RETRIES attempts"
    exit 1
fi

# Execute the main application - bootstrap service handles secret loading
exec uvicorn app.main:app --host 0.0.0.0 --port 443 \
    --ssl-keyfile "${SSL_DIR}/server.key" \
    --ssl-certfile "${SSL_DIR}/server.crt"
