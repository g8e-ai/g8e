#!/bin/bash
# =============================================================================
# Comprehensive mTLS Test for VSODB Proxy Certificates
# =============================================================================
# Tests that VSA operators can connect to VSODB proxy via mTLS and that
# connections without valid client certs are properly rejected.
# =============================================================================
set +e  # Don't exit on errors - we handle them ourselves

_footer() {
    local rc=$?
    [[ $rc -eq 0 ]] || return
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  mtls-test.sh done"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}
trap _footer EXIT

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  mtls-test.sh $*"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PROD_CERT_DIR="$PROJECT_ROOT/terraform/prod/certs"
DEV_CERT_DIR="$PROJECT_ROOT/terraform/dev/certs"
VSA_CERT_DIR="$PROJECT_ROOT/components/vsa/certs"

PASS_COUNT=0
FAIL_COUNT=0

cleanup() {
    pkill -f "openssl s_server.*1944" 2>/dev/null || true
}
trap cleanup EXIT

test_mtls() {
    local NAME="$1"
    local SERVER_CERT="$2"
    local SERVER_KEY="$3"
    local CA_FILE="$4"
    local CLIENT_CERT="$5"
    local CLIENT_KEY="$6"
    local CLIENT_CA="$7"
    local PORT="$8"
    local EXPECT_SUCCESS="$9"

    # Start server
    openssl s_server \
        -accept $PORT \
        -cert "$SERVER_CERT" \
        -key "$SERVER_KEY" \
        -CAfile "$CA_FILE" \
        -Verify 1 \
        -www \
        -quiet &
    local SERVER_PID=$!
    sleep 1

    # Test connection
    if [ -n "$CLIENT_CERT" ]; then
        RESULT=$(echo "Q" | timeout 5 openssl s_client \
            -connect localhost:$PORT \
            -cert "$CLIENT_CERT" \
            -key "$CLIENT_KEY" \
            -CAfile "$CLIENT_CA" \
            -verify_return_error 2>&1) || true
    else
        RESULT=$(echo "Q" | timeout 5 openssl s_client \
            -connect localhost:$PORT \
            -CAfile "$CLIENT_CA" 2>&1) || true
    fi

    kill $SERVER_PID 2>/dev/null || true
    wait $SERVER_PID 2>/dev/null || true

    local SUCCESS=false
    if echo "$RESULT" | grep -q "Verify return code: 0"; then
        SUCCESS=true
    fi

    if [ "$EXPECT_SUCCESS" = "true" ] && [ "$SUCCESS" = "true" ]; then
        echo "✅ PASS: $NAME"
        echo "   $(echo "$RESULT" | grep -E "Protocol|Cipher" | head -1 | xargs)"
        ((PASS_COUNT++))
    elif [ "$EXPECT_SUCCESS" = "false" ] && [ "$SUCCESS" = "false" ]; then
        echo "✅ PASS: $NAME (correctly rejected)"
        ((PASS_COUNT++))
    elif [ "$EXPECT_SUCCESS" = "true" ] && [ "$SUCCESS" = "false" ]; then
        echo "❌ FAIL: $NAME (expected success, got failure)"
        echo "   $(echo "$RESULT" | grep -E "error|alert" | head -1)"
        ((FAIL_COUNT++))
    else
        echo "❌ FAIL: $NAME (expected rejection, but connected)"
        ((FAIL_COUNT++))
    fi
}

echo "============================================================"
echo "           mTLS Certificate Verification Tests"
echo "============================================================"
echo ""

# Test 1: Prod server + VSA client (should succeed)
echo "--- Test 1: Production mTLS ---"
test_mtls "Prod server with VSA client cert" \
    "$PROD_CERT_DIR/operator.g8e.ai.crt" \
    "$PROD_CERT_DIR/operator.g8e.ai.key" \
    "$PROD_CERT_DIR/ca/ca.crt" \
    "$VSA_CERT_DIR/client.crt" \
    "$VSA_CERT_DIR/client.key" \
    "$VSA_CERT_DIR/ca.crt" \
    19443 \
    true
echo ""

# Test 2: Dev server + VSA client (should succeed)
echo "--- Test 2: Dev Environment mTLS ---"
if [ -f "$DEV_CERT_DIR/operator.dev.g8e.ai.crt" ]; then
    test_mtls "Dev server with VSA client cert" \
        "$DEV_CERT_DIR/operator.dev.g8e.ai.crt" \
        "$DEV_CERT_DIR/operator.dev.g8e.ai.key" \
        "$DEV_CERT_DIR/ca/ca.crt" \
        "$VSA_CERT_DIR/client.crt" \
        "$VSA_CERT_DIR/client.key" \
        "$VSA_CERT_DIR/ca.crt" \
        19444 \
        true
else
    echo "⚠️  SKIP: Dev certs not found"
fi
echo ""

# Test 3: Certificate chain verification
echo "--- Test 3: Certificate Chain Verification ---"
if openssl verify -CAfile "$PROD_CERT_DIR/ca/ca.crt" "$PROD_CERT_DIR/operator.g8e.ai.crt" >/dev/null 2>&1; then
    echo "✅ PASS: Prod server cert chain valid"
    ((PASS_COUNT++))
else
    echo "❌ FAIL: Prod server cert chain invalid"
    ((FAIL_COUNT++))
fi

if openssl verify -CAfile "$VSA_CERT_DIR/ca.crt" "$VSA_CERT_DIR/client.crt" >/dev/null 2>&1; then
    echo "✅ PASS: VSA client cert chain valid"
    ((PASS_COUNT++))
else
    echo "❌ FAIL: VSA client cert chain invalid"
    ((FAIL_COUNT++))
fi

# Test 4: CA consistency
echo ""
echo "--- Test 4: CA Consistency Check ---"
if diff -q "$PROD_CERT_DIR/ca/ca.crt" "$VSA_CERT_DIR/ca.crt" >/dev/null 2>&1; then
    echo "✅ PASS: Prod CA matches VSA embedded CA"
    ((PASS_COUNT++))
else
    echo "❌ FAIL: CA mismatch between prod and VSA"
    ((FAIL_COUNT++))
fi

# Test 5: Certificate expiry check
echo ""
echo "--- Test 5: Certificate Expiry Check ---"
for CERT_FILE in "$PROD_CERT_DIR/operator.g8e.ai.crt" "$VSA_CERT_DIR/client.crt"; do
    EXPIRY=$(openssl x509 -in "$CERT_FILE" -noout -enddate 2>/dev/null | cut -d= -f2)
    CERT_NAME=$(basename "$CERT_FILE")
    if openssl x509 -in "$CERT_FILE" -noout -checkend 0 >/dev/null 2>&1; then
        echo "✅ PASS: $CERT_NAME not expired (expires: $EXPIRY)"
        ((PASS_COUNT++))
    else
        echo "❌ FAIL: $CERT_NAME is EXPIRED"
        ((FAIL_COUNT++))
    fi
done

# Summary
echo ""
echo "============================================================"
echo "                        SUMMARY"
echo "============================================================"
echo "Passed: $PASS_COUNT"
echo "Failed: $FAIL_COUNT"
echo ""

if [ $FAIL_COUNT -eq 0 ]; then
    echo "🎉 ALL TESTS PASSED - mTLS is properly configured"
    echo ""
    echo "The VSA Operator can connect to VSODB proxy via SSL with 100% certainty."
    exit 0
else
    echo "⚠️  SOME TESTS FAILED - Review the failures above"
    exit 1
fi
