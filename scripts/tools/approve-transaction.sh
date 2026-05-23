#!/usr/bin/env bash
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

set -e
source "$(dirname "${BASH_SOURCE[0]}")/../cmd/common.sh"

SUB="${1:-}"
TX_HASH="${2:-}"

_banner "approve ${SUB}"

case "$SUB" in
    -h|--help|"")
        echo "Usage: ./g8e approve <command> [args]"
        echo ""
        echo "Commands:"
        echo "  list                    List all suspended transactions awaiting approval"
        echo "  <tx_hash>              Approve a specific suspended transaction by hash"
        echo ""
        echo "Examples:"
        echo "  ./g8e approve list"
        echo "  ./g8e approve abc123def456..."
        exit 0 ;;

    list)
        _ensure_operator
        _require_authenticated

        echo "Fetching suspended transactions..."
        response=$(_operator_curl GET "/api/suspended-transactions" "")

        if [[ -z "$response" ]]; then
            echo "[g8e] No suspended transactions found"
            exit 0
        fi

        echo "$response" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    if not data or 'transactions' not in data or not data['transactions']:
        print('[g8e] No suspended transactions found')
        sys.exit(0)
    
    print(f\"Found {len(data['transactions'])} suspended transaction(s):\")
    for tx in data['transactions']:
        hash_prefix = tx['transaction_hash'][:12] + '...'
        print(f\"  - {hash_prefix}\")
        print(f\"    Tool: {tx.get('tool_name', 'N/A')}\")
        print(f\"    Created: {tx.get('created_at', 'N/A')}\")
        print(f\"    Expires: {tx.get('expires_at', 'N/A')}\")
        print(f\"    User ID: {tx.get('user_id', 'N/A')}\")
        print()
except Exception as e:
    print(f'[g8e] Failed to parse response: {e}', file=sys.stderr)
    sys.exit(1)
"
        ;;

    *)
        # Treat SUB as a transaction hash
        TX_HASH="$SUB"
        if [[ -z "$TX_HASH" ]]; then
            echo "[g8e] transaction hash required" >&2
            echo "Usage: ./g8e approve <tx_hash>" >&2
            exit 1
        fi

        _ensure_operator
        _require_authenticated

        echo "Approving transaction: ${TX_HASH:0:12}..."

        # Extract mTLS cert fingerprint from the CLI certificate
        if [[ ! -f "$G8E_CLI_CERT_FILE" ]]; then
            echo "[g8e] CLI certificate not found at $G8E_CLI_CERT_FILE" >&2
            echo "Run: ./g8e login" >&2
            exit 1
        fi

        # Calculate SHA-256 fingerprint of the certificate
        cert_fingerprint=$(openssl x509 -in "$G8E_CLI_CERT_FILE" -noout -fingerprint -sha256 | sed 's/SHA256 Fingerprint=//g' | tr -d ':')
        if [[ -z "$cert_fingerprint" ]]; then
            echo "[g8e] Failed to calculate certificate fingerprint" >&2
            exit 1
        fi

        echo "Using mTLS cert fingerprint: ${cert_fingerprint:0:16}..."

        # Build L3 proof with mtls_cert_fingerprint
        proof_body=$(python3 -c "import json; print(json.dumps({
    'mtls_cert_fingerprint': '$cert_fingerprint'
}))")

        # Call the approval endpoint
        response=$(_operator_curl POST "/api/approve/$TX_HASH" "$proof_body")

        if [[ -z "$response" ]]; then
            echo "[g8e] Approval failed: no response from server" >&2
            exit 1
        fi

        # Parse response
        echo "$response" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    if 'error' in data:
        print(f'[g8e] Approval failed: {data[\"error\"]}', file=sys.stderr)
        sys.exit(1)
    
    print('[g8e] Transaction approved successfully')
    if 'transaction_hash' in data:
        print(f\"  Transaction: {data['transaction_hash'][:12]}...\")
    if 'status' in data:
        print(f\"  Status: {data['status']}\")
    if 'result_summary' in data:
        print(f\"  Result: {data['result_summary']}\")
except Exception as e:
    print(f'[g8e] Failed to parse response: {e}', file=sys.stderr)
    print(f'Response: {sys.stdin.read()}', file=sys.stderr)
    sys.exit(1)
"
        ;;
esac
