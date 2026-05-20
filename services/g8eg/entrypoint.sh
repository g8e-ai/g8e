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
. "${SCRIPT_DIR}/../../scripts/core/path_utils.sh"
G8E_PROJECT_ROOT="$(resolve_g8e_root)"
export G8E_PROJECT_ROOT

G8E_RUNTIME_DIR="${G8E_RUNTIME_DIR:-${G8E_PROJECT_ROOT:-}/.g8e}"
DATA_DIR="${OPERATOR_LISTEN_DATA_DIR:-${G8E_RUNTIME_DIR:-}/data}"
PKI_DIR="${OPERATOR_LISTEN_PKI_DIR:-${G8E_RUNTIME_DIR:-}/pki}"
SECRETS_DIR="${OPERATOR_LISTEN_SECRETS_DIR:-${G8E_RUNTIME_DIR:-}/secrets}"

GATEWAY_BIN="${G8E_PROJECT_ROOT}/services/g8eo/build/linux-amd64/g8e.gateway"

# Fallback to local build if binary not present (useful in dev/test)
if [ ! -f "$GATEWAY_BIN" ]; then
    echo "Gateway binary not found, compiling locally..."
    (cd "${G8E_PROJECT_ROOT}/services/g8eo" && make build-local)
fi

# Ensure ports match values from paths.json constants
. "${G8E_PROJECT_ROOT}/scripts/cmd/paths.sh"
HTTP_PORT="${G8E_PORT_OPERATOR_HTTP:-443}"
WSS_PORT="${G8E_PORT_OPERATOR_WSS:-443}"
BOOTSTRAP_PORT="${G8E_PORT_OPERATOR_BOOTSTRAP:-80}"
PUBLIC_PORT="${G8E_PORT_OPERATOR_PUBLIC:-443}"

echo "Starting g8eg Governance Gateway (listen mode) on ports HTTP:${HTTP_PORT}, WSS:${WSS_PORT}..."
exec "$GATEWAY_BIN" --listen \
    --data-dir "$DATA_DIR" \
    --pki-dir "$PKI_DIR" \
    --secrets-dir "$SECRETS_DIR" \
    --http-listen-port "$HTTP_PORT" \
    --wss-listen-port "$WSS_PORT" \
    --bootstrap-listen-port "$BOOTSTRAP_PORT" \
    --public-listen-port "$PUBLIC_PORT" "$@"
