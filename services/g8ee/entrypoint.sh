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

# Derive project root using shared utility
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SCRIPT_DIR}/../../scripts/core/config.sh"

# Load security tokens into environment if files exist
if [ -f "${G8E_SECRETS_DIR}/session_encryption_key" ]; then
    export G8E_SESSION_ENCRYPTION_KEY=$(cat "${G8E_SECRETS_DIR}/session_encryption_key" | tr -d ' \n\r')
fi

# operator readiness is gated by docker-compose `depends_on: operator: service_healthy`.
# Execute the main application - bootstrap service handles secret loading
CERT_NAME="${G8E_PATH_G8EE_CERT_NAME:-g8ee}"
exec uvicorn app.main:app --host 0.0.0.0 --port "${G8E_G8EE_HTTP_PORT}" \
    --ssl-keyfile "${G8E_PKI_DIR}/issued/apps/${CERT_NAME}.key" \
    --ssl-certfile "${G8E_PKI_DIR}/issued/apps/${CERT_NAME}.crt"
