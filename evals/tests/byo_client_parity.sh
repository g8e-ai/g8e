#!/usr/bin/env bash
# Prove BYO-client parity end-to-end using raw curl commands.
# This test drives the entire bootstrap -> chat send -> SSE poll flow.
set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$PROJECT_ROOT/scripts/cmd/common.sh"

# 1. Ensure platform is running
if ! _operator_running; then
    echo "Operator is not running. Start it first: ./g8e platform start"
    exit 1
fi

if ! _g8ee_running; then
    echo "g8ee is not running. Start it first: ./g8e apps start g8ee"
    exit 1
fi

_banner "Testing BYO Client Parity (raw curl flow)"

# 2. Setup temporary credentials for this test
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

TEST_EMAIL="byo-test@g8e.local"
TEST_NAME="BYO Test User"
TRUST_BUNDLE="$G8E_PKI_DIR_HOST/trust/hub-bundle.pem"
OPERATOR_URL="https://localhost:9003"
G8EE_URL="https://localhost:8443"

# 3. Generate CSRs
_generate_workload_csrs "$TMP_DIR"
OP_CSR_PEM="$_op_csr_pem"
CLI_CSR_PEM="$_cli_csr_pem"
CLI_KEY_FILE="$_cli_key_file"
FINGERPRINT=$(echo "g8e-byo-test" | sha256sum | awk '{print $1}')

# 4. Bootstrap
_banner "Step 1: Bootstrap over loopback"
BOOTSTRAP_BODY=$(jq -n \
    --arg email "$TEST_EMAIL" \
    --arg name "$TEST_NAME" \
    --arg op_csr "$OP_CSR_PEM" \
    --arg cli_csr "$CLI_CSR_PEM" \
    --arg fingerprint "$FINGERPRINT" \
    '{email: $email, name: $name, csr_pem: $op_csr, cli_csr_pem: $cli_csr, system_fingerprint: $fingerprint}')

RESP=$(curl -sS --cacert "$TRUST_BUNDLE" \
    -X POST -H "Content-Type: application/json" \
    -d "$BOOTSTRAP_BODY" \
    "$OPERATOR_URL/api/auth/bootstrap")

SUCCESS=$(echo "$RESP" | jq -r '.success')
if [[ "$SUCCESS" != "true" ]]; then
    echo "Bootstrap failed: $RESP"
    exit 1
fi

OPERATOR_SESSION_ID=$(echo "$RESP" | jq -r '.operator_session_id')
CLI_SESSION_ID=$(echo "$RESP" | jq -r '.cli_session_id')
USER_ID=$(echo "$RESP" | jq -r '.user.id')
CLI_CERT=$(echo "$RESP" | jq -r '.cli_cert_chain // .cli_cert')

echo "Bootstrap successful!"
echo "User ID: $USER_ID"
echo "Operator Session ID: $OPERATOR_SESSION_ID"
echo "CLI Session ID: $CLI_SESSION_ID"

# Save CLI cert for curl
CLI_CERT_FILE="$TMP_DIR/cli.crt"
echo "$CLI_CERT" > "$CLI_CERT_FILE"

# 5. Chat Send
_banner "Step 2: Send chat message to g8ee via raw curl"
# Note: We must use the CLI cert and provide the session headers
# g8ee expects Authorization: Bearer <token> for auth and RequestContext in body for routing.

CONTEXT_JSON=$(jq -n \
    --arg cli_sid "$CLI_SESSION_ID" \
    --arg uid "$USER_ID" \
    '{cli_session_id: $cli_sid, user_id: $uid, source_component: "client"}')

CHAT_BODY=$(jq -n \
    --argjson context "$CONTEXT_JSON" \
    --arg msg "Hello from BYO client test" \
    '{context: $context, message: $msg, sentinel_mode: true, resource_creation: {create_case: true}}')

# We use the CLI cert which is bound to the operator session
CHAT_RESP=$(curl -sS --cacert "$TRUST_BUNDLE" \
    --cert "$CLI_CERT_FILE" --key "$CLI_KEY_FILE" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $OPERATOR_SESSION_ID" \
    -H "X-G8E-CLI-Session-ID: $CLI_SESSION_ID" \
    -H "X-G8E-Source-Component: client" \
    -d "$CHAT_BODY" \
    "$G8EE_URL/api/internal/chat")

CHAT_SUCCESS=$(echo "$CHAT_RESP" | jq -r '.success')
if [[ "$CHAT_SUCCESS" != "true" ]]; then
    echo "Chat send failed: $CHAT_RESP"
    exit 1
fi

INVESTIGATION_ID=$(echo "$CHAT_RESP" | jq -r '.investigation_id')
echo "Chat sent! Investigation ID: $INVESTIGATION_ID"

# 6. Poll SSE
_banner "Step 3: Poll SSE events from operator via raw curl"

# We wait a bit for AI to process
sleep 2

SSE_RESP=$(curl -sS --cacert "$TRUST_BUNDLE" \
    --cert "$CLI_CERT_FILE" --key "$CLI_KEY_FILE" \
    -H "Authorization: Bearer $OPERATOR_SESSION_ID" \
    "$OPERATOR_URL/api/internal/sse/events?cli_session_id=$CLI_SESSION_ID&since_id=0&limit=10")

EVENT_COUNT=$(echo "$SSE_RESP" | jq -r '.count')
if [[ "$EVENT_COUNT" -eq 0 ]]; then
    echo "No SSE events found: $SSE_RESP"
    exit 1
fi

echo "Found $EVENT_COUNT SSE events!"
echo "$SSE_RESP" | jq -c '.events[].event_type'

_banner "BYO Client Parity Test PASSED"
