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

# g8eg Entrypoint script - starts the Governance Gateway (g8e.gateway) in listen mode
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SCRIPT_DIR}/../../scripts/core/config.sh"

DATA_DIR="$G8E_DATA_DIR"
PKI_DIR="$G8E_PKI_DIR"
SECRETS_DIR="$G8E_SECRETS_DIR"

GATEWAY_BIN="${G8E_PROJECT_ROOT}/services/g8eo/build/linux-amd64/g8e.gateway"

# Fallback to local build if binary not present (useful in dev/test)
if [ ! -f "$GATEWAY_BIN" ]; then
    echo "Gateway binary not found, compiling locally..."
    (cd "${G8E_PROJECT_ROOT}/services/g8eo" && make build-local)
fi

# Ensure ports match values from paths.json constants
HTTP_PORT="$G8E_OPERATOR_HTTP_PORT"
WSS_PORT="$G8E_OPERATOR_WSS_PORT"
BOOTSTRAP_PORT="$G8E_OPERATOR_BOOTSTRAP_PORT"
# Public TLS surface must NOT collide with the mTLS surface (443) — sharing
# a port would force VerifyClientCertIfGiven and downgrade the mTLS gate.
PUBLIC_PORT="$G8E_OPERATOR_PUBLIC_PORT"

echo "Starting g8eg Governance Gateway (listen mode) on ports HTTP:${HTTP_PORT}..."
exec "$GATEWAY_BIN" --listen \
    --data-dir "$DATA_DIR" \
    --pki-dir "$PKI_DIR" \
    --secrets-dir "$SECRETS_DIR" \
    --http-listen-port "$HTTP_PORT" \
    --bootstrap-listen-port "$BOOTSTRAP_PORT" \
    --public-listen-port "$PUBLIC_PORT" "$@"
