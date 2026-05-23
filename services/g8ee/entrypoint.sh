#!/bin/sh
# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# g8ee Entrypoint script - waits for operator then starts the application

set -e

# Derive project root
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
G8E_PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
export G8E_PROJECT_ROOT

# Set runtime defaults
export G8E_RUNTIME_DIR="${G8E_RUNTIME_DIR:-$G8E_PROJECT_ROOT/.g8e}"
export G8E_DATA_DIR="${G8E_DATA_DIR:-$G8E_RUNTIME_DIR/data}"
export G8E_PKI_DIR="${G8E_PKI_DIR:-$G8E_RUNTIME_DIR/pki}"
export G8E_SECRETS_DIR="${G8E_SECRETS_DIR:-$G8E_RUNTIME_DIR/secrets}"
export G8E_PID_DIR="${G8E_PID_DIR:-$G8E_RUNTIME_DIR/pids}"
export G8E_LOG_DIR="${G8E_LOG_DIR:-$G8E_RUNTIME_DIR/logs}"

# Port defaults
export G8E_OPERATOR_HTTPS_PORT="${G8E_OPERATOR_HTTPS_PORT:-8440}"
export G8E_OPERATOR_PUBLIC_HTTPS_PORT="${G8E_OPERATOR_PUBLIC_HTTPS_PORT:-8442}"
export G8E_G8EE_HTTPS_PORT="${G8E_G8EE_HTTPS_PORT:-8443}"

# Load environment file if it exists
G8E_ENV_FILE="$G8E_PROJECT_ROOT/.g8e/.env"
if [[ -f "$G8E_ENV_FILE" ]]; then
    set -a
    source "$G8E_ENV_FILE"
    set +a
fi

# Load security tokens into environment if files exist
if [ -f "${G8E_SECRETS_DIR}/session_encryption_key" ]; then
    export G8E_SESSION_ENCRYPTION_KEY=$(cat "${G8E_SECRETS_DIR}/session_encryption_key" | tr -d ' \n\r')
fi

# operator readiness is gated by docker-compose `depends_on: operator: service_healthy`.
# Execute the main application - bootstrap service handles secret loading
CERT_NAME="${G8E_PATH_G8EE_CERT_NAME:-g8ee}"
exec uvicorn app.main:app --host 0.0.0.0 --port "${G8E_G8EE_HTTPS_PORT}" \
    --ssl-keyfile "${G8E_PKI_DIR}/issued/apps/${CERT_NAME}.key" \
    --ssl-certfile "${G8E_PKI_DIR}/issued/apps/${CERT_NAME}.crt"
