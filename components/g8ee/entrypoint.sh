#!/bin/sh
# g8ee Entrypoint script - waits for g8es then starts the application

set -e

SSL_DIR="/g8es"

# Load security tokens into environment if files exist
if [ -f "${SSL_DIR}/internal_auth_token" ]; then
    export G8E_INTERNAL_AUTH_TOKEN=$(cat "${SSL_DIR}/internal_auth_token" | tr -d ' \n\r')
fi

if [ -f "${SSL_DIR}/session_encryption_key" ]; then
    export G8E_SESSION_ENCRYPTION_KEY=$(cat "${SSL_DIR}/session_encryption_key" | tr -d ' \n\r')
fi

# g8es readiness is gated by docker-compose `depends_on: g8es: service_healthy`.
# Execute the main application - bootstrap service handles secret loading
exec uvicorn app.main:app --host 0.0.0.0 --port 443 \
    --ssl-keyfile "${SSL_DIR}/server.key" \
    --ssl-certfile "${SSL_DIR}/server.crt"
