// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration

package scenario

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/governance"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// TestReceiptVerification tests receipt signature verification as a separate axis,
// independent of inbound envelope submission. Receipts are outputs, not inputs.
func TestReceiptVerification(t *testing.T) {
	// Generate Actuator signing key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := hex.EncodeToString(pubKey)

	// Create a valid receipt
	validReceipt := &operatorv1.ActionReceipt{
		TransactionId:    "test-tx-id-123",
		TransactionHash:  "test-hash-abc456",
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:    "test execution succeeded",
		StateRootBefore:  "root-before-123",
		StateRootAfter:   "root-after-456",
		ExecutedAtUnixMs: 1716624000000,
		SignerKeyId:      keyID,
		GatewaySigned:    false,
		L2Status:         operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
		L3Status:         operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
	}

	// Sign the valid receipt
	canonical, err := governance.CanonicalizeActionReceipt(validReceipt)
	require.NoError(t, err)
	sig := ed25519.Sign(privKey, canonical)
	validReceipt.Signature = hex.EncodeToString(sig)

	t.Run("valid_receipt_verifies", func(t *testing.T) {
		// Verify signature with correct public key
		canonical, err := governance.CanonicalizeActionReceipt(validReceipt)
		require.NoError(t, err)

		sigBytes, err := hex.DecodeString(validReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(pubKey, canonical, sigBytes)
		require.True(t, valid, "Valid receipt signature should verify")
	})

	t.Run("flipped_signature_byte_fails", func(t *testing.T) {
		// Flip a byte in the signature
		tamperedReceipt := proto.Clone(validReceipt).(*operatorv1.ActionReceipt)
		sigBytes, err := hex.DecodeString(tamperedReceipt.Signature)
		require.NoError(t, err)

		// Flip first byte
		sigBytes[0] = sigBytes[0] ^ 0xff
		tamperedReceipt.Signature = hex.EncodeToString(sigBytes)

		// Verify should fail
		canonical, err := governance.CanonicalizeActionReceipt(tamperedReceipt)
		require.NoError(t, err)

		tamperedSigBytes, err := hex.DecodeString(tamperedReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(pubKey, canonical, tamperedSigBytes)
		require.False(t, valid, "Tampered signature should fail verification")
	})

	t.Run("flipped_state_root_after_fails", func(t *testing.T) {
		// Flip state_root_after field
		tamperedReceipt := proto.Clone(validReceipt).(*operatorv1.ActionReceipt)
		tamperedReceipt.StateRootAfter = "tampered-root-after"

		// Verify should fail (signature doesn't match tampered data)
		canonical, err := governance.CanonicalizeActionReceipt(tamperedReceipt)
		require.NoError(t, err)

		sigBytes, err := hex.DecodeString(tamperedReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(pubKey, canonical, sigBytes)
		require.False(t, valid, "Tampered state_root_after should fail verification")
	})

	t.Run("flipped_executed_at_fails", func(t *testing.T) {
		// Flip executed_at timestamp
		tamperedReceipt := proto.Clone(validReceipt).(*operatorv1.ActionReceipt)
		tamperedReceipt.ExecutedAtUnixMs = 9999999999999

		// Verify should fail
		canonical, err := governance.CanonicalizeActionReceipt(tamperedReceipt)
		require.NoError(t, err)

		sigBytes, err := hex.DecodeString(tamperedReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(pubKey, canonical, sigBytes)
		require.False(t, valid, "Tampered executed_at should fail verification")
	})

	t.Run("wrong_public_key_fails", func(t *testing.T) {
		// Generate a different key pair
		wrongPubKey, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		// Verify with wrong public key
		canonical, err := governance.CanonicalizeActionReceipt(validReceipt)
		require.NoError(t, err)

		sigBytes, err := hex.DecodeString(validReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(wrongPubKey, canonical, sigBytes)
		require.False(t, valid, "Wrong public key should fail verification")
	})

	t.Run("flipped_status_fails", func(t *testing.T) {
		// Flip status field
		tamperedReceipt := proto.Clone(validReceipt).(*operatorv1.ActionReceipt)
		tamperedReceipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED

		// Verify should fail
		canonical, err := governance.CanonicalizeActionReceipt(tamperedReceipt)
		require.NoError(t, err)

		sigBytes, err := hex.DecodeString(tamperedReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(pubKey, canonical, sigBytes)
		require.False(t, valid, "Tampered status should fail verification")
	})

	t.Run("flipped_transaction_hash_fails", func(t *testing.T) {
		// Flip transaction_hash field
		tamperedReceipt := proto.Clone(validReceipt).(*operatorv1.ActionReceipt)
		tamperedReceipt.TransactionHash = "tampered-hash"

		// Verify should fail
		canonical, err := governance.CanonicalizeActionReceipt(tamperedReceipt)
		require.NoError(t, err)

		sigBytes, err := hex.DecodeString(tamperedReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(pubKey, canonical, sigBytes)
		require.False(t, valid, "Tampered transaction_hash should fail verification")
	})

	t.Run("flipped_l2_status_fails", func(t *testing.T) {
		// Flip l2_status field
		tamperedReceipt := proto.Clone(validReceipt).(*operatorv1.ActionReceipt)
		tamperedReceipt.L2Status = operatorv1.L2Status_L2_STATUS_REQUIRED_FAILED

		// Verify should fail
		canonical, err := governance.CanonicalizeActionReceipt(tamperedReceipt)
		require.NoError(t, err)

		sigBytes, err := hex.DecodeString(tamperedReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(pubKey, canonical, sigBytes)
		require.False(t, valid, "Tampered l2_status should fail verification")
	})

	t.Run("flipped_l3_status_fails", func(t *testing.T) {
		// Flip l3_status field
		tamperedReceipt := proto.Clone(validReceipt).(*operatorv1.ActionReceipt)
		tamperedReceipt.L3Status = operatorv1.L3Status_L3_STATUS_REQUIRED_FAILED

		// Verify should fail
		canonical, err := governance.CanonicalizeActionReceipt(tamperedReceipt)
		require.NoError(t, err)

		sigBytes, err := hex.DecodeString(tamperedReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(pubKey, canonical, sigBytes)
		require.False(t, valid, "Tampered l3_status should fail verification")
	})

	t.Run("empty_signature_fails", func(t *testing.T) {
		// Empty signature
		tamperedReceipt := proto.Clone(validReceipt).(*operatorv1.ActionReceipt)
		tamperedReceipt.Signature = ""

		// Verify should fail
		canonical, err := governance.CanonicalizeActionReceipt(tamperedReceipt)
		require.NoError(t, err)

		sigBytes, err := hex.DecodeString(tamperedReceipt.Signature)
		require.NoError(t, err)

		valid := ed25519.Verify(pubKey, canonical, sigBytes)
		require.False(t, valid, "Empty signature should fail verification")
	})
}
