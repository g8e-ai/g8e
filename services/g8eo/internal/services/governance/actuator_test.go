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

package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func newTestActuator(t *testing.T) (*Actuator, ed25519.PublicKey) {
	t.Helper()

	// Generate Actuator signing key
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create mock dependencies
	mockHandler := &mockExecutionHandler{err: nil}
	mockAuditStore := testutil.NewConfigurableMockAuditStore(nil)
	mockStateRoot := testutil.NewMockStateRootProvider("test-state-root-123")

	logger := slog.Default()

	Actuator := &Actuator{
		Logger:            logger,
		SigningKey:        privKey,
		KeyID:             "test-Actuator-key",
		ExecutionHandler:  mockHandler,
		AuditStore:        mockAuditStore,
		StateRootProvider: mockStateRoot,
	}

	return Actuator, pubKey
}

func TestActuatorExecuteHappyPath(t *testing.T) {
	Actuator, pubKey := newTestActuator(t)

	// Configure handler to succeed (already set in newTestActuator)

	// Create verified transaction
	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute
	receipt, err := Actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify receipt fields
	require.Equal(t, envelope.Id, receipt.TransactionId)
	require.Equal(t, envelope.TransactionHash, receipt.TransactionHash)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
	require.Equal(t, "completed", receipt.ResultSummary)
	require.Equal(t, "test-state-root-123", receipt.StateRootBefore)
	require.Equal(t, "test-state-root-123", receipt.StateRootAfter)
	require.Equal(t, "test-Actuator-key", receipt.SignerKeyId)
	require.NotEmpty(t, receipt.Signature)

	// Verify signature
	canonical, err := CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)

	sigBytes, err := hex.DecodeString(receipt.Signature)
	require.NoError(t, err)

	valid := ed25519.Verify(pubKey, canonical, sigBytes)
	require.True(t, valid, "Receipt signature should verify against Actuator public key")

	// Verify audit store was called twice (initial EXECUTING receipt + final COMPLETED receipt)
	auditStore := Actuator.AuditStore.(*testutil.ConfigurableMockAuditStore)
	require.Len(t, auditStore.Calls, 2)

	// Verify both calls were to console_audit collection
	for _, call := range auditStore.Calls {
		require.Equal(t, marshaler.CollectionName(constants.CollectionConsoleAudit), call.Collection)
		require.Equal(t, envelope.Id, call.ID)
	}

	// Verify initial receipt has EXECUTING status
	var initialRecord models.ActionReceiptRecord
	err = json.Unmarshal(auditStore.Calls[0].Data, &initialRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING, initialRecord.Status)

	// Verify final receipt has COMPLETED status
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(auditStore.Calls[1].Data, &finalRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, finalRecord.Status)
}

func TestActuatorExecuteHandlerError(t *testing.T) {
	Actuator, pubKey := newTestActuator(t)

	// Configure handler to return error
	handler := Actuator.ExecutionHandler.(*mockExecutionHandler)
	handler.err = errors.New("handler execution failed")

	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute
	receipt, err := Actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "handler execution failed")
	require.NotNil(t, receipt)

	// Verify receipt has FAILED status
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, receipt.Status)
	require.Contains(t, receipt.ResultSummary, "handler execution failed")

	// Verify signature is still valid
	canonical, err := CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)

	sigBytes, err := hex.DecodeString(receipt.Signature)
	require.NoError(t, err)

	valid := ed25519.Verify(pubKey, canonical, sigBytes)
	require.True(t, valid, "Receipt signature should verify even when handler fails")

	// Verify audit store was called twice
	auditStore := Actuator.AuditStore.(*testutil.ConfigurableMockAuditStore)
	require.Len(t, auditStore.Calls, 2)

	// Verify final receipt has FAILED status
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(auditStore.Calls[1].Data, &finalRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, finalRecord.Status)
}

func TestActuatorExecuteAuditWriteFailInitial(t *testing.T) {
	Actuator, _ := newTestActuator(t)

	// Configure audit store to fail on first call (initial receipt)
	auditStore := Actuator.AuditStore.(*testutil.ConfigurableMockAuditStore)
	callCount := 0
	auditStore.DocSetFunc = func(collection, id string, data json.RawMessage) error {
		callCount++
		if callCount == 1 {
			return errors.New("audit write failed")
		}
		return nil
	}

	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute - should fail before handler is invoked
	receipt, err := Actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to log initial action receipt")
	require.Nil(t, receipt)

	// Verify handler was never called (only initial audit write was attempted)
	require.Equal(t, 1, callCount)
}

func TestActuatorExecuteReceiptPersistFail(t *testing.T) {
	Actuator, _ := newTestActuator(t)

	// Configure audit store to fail on DocSet
	auditStore := Actuator.AuditStore.(*testutil.ConfigurableMockAuditStore)
	auditStore.DocSetFunc = func(collection, id string, data json.RawMessage) error {
		return errors.New("doc set failed")
	}

	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute - should fail before handler is invoked
	receipt, err := Actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to log initial action receipt")
	require.Nil(t, receipt)
}

func TestActuatorExecuteMissingSigningKey(t *testing.T) {
	Actuator, _ := newTestActuator(t)
	Actuator.SigningKey = nil

	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute - should fail immediately
	receipt, err := Actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Actuator signing key missing")
	require.Nil(t, receipt)
}

func TestActuatorExecuteMissingExecutionHandler(t *testing.T) {
	Actuator, _ := newTestActuator(t)
	Actuator.ExecutionHandler = nil

	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute - should fail immediately
	receipt, err := Actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Actuator ExecutionHandler not set")
	require.Nil(t, receipt)
}

func TestCanonicalizeActionReceipt(t *testing.T) {
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    "test-tx-id",
		TransactionHash:  "test-hash",
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:    "test summary",
		StateRootBefore:  "root-before",
		StateRootAfter:   "root-after",
		ExecutedAtUnixMs: 1234567890,
		SignerKeyId:      "test-key-id",
	}

	// Canonicalize twice and verify results are identical
	bytes1, err := CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)

	bytes2, err := CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)

	require.Equal(t, bytes1, bytes2, "Canonicalization should be deterministic")

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(bytes1, &parsed)
	require.NoError(t, err)

	// Verify all expected fields are present
	require.Equal(t, "test-tx-id", parsed["transaction_id"])
	require.Equal(t, "test-hash", parsed["transaction_hash"])
	require.Equal(t, float64(2), parsed["status"]) // EXECUTION_STATUS_COMPLETED = 2 (JSON marshals enums as float64)
	require.Equal(t, "test summary", parsed["result_summary"])
	require.Equal(t, "root-before", parsed["state_root_before"])
	require.Equal(t, "root-after", parsed["state_root_after"])
	require.Equal(t, float64(1234567890), parsed["executed_at_unix_ms"])
	require.Equal(t, "test-key-id", parsed["signer_key_id"])
}

func TestActuatorGatewaySignedPropagation(t *testing.T) {
	Actuator, _ := newTestActuator(t)

	// Create envelope with GatewaySigned=true
	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
		Governance: &commonv1.GovernanceMetadata{
			GatewaySigned: true,
		},
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute
	receipt, err := Actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify GatewaySigned is propagated to receipt
	require.True(t, receipt.GatewaySigned, "GatewaySigned should be propagated from envelope to receipt")

	// Verify audit record also has GatewaySigned
	auditStore := Actuator.AuditStore.(*testutil.ConfigurableMockAuditStore)
	require.Len(t, auditStore.Calls, 2)

	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(auditStore.Calls[1].Data, &finalRecord)
	require.NoError(t, err)
	require.True(t, finalRecord.GatewaySigned, "Audit record should have GatewaySigned=true")
}

func TestActuatorGatewaySignedFalse(t *testing.T) {
	Actuator, _ := newTestActuator(t)

	// Create envelope with GatewaySigned=false (normal Tribunal path)
	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
		Governance: &commonv1.GovernanceMetadata{
			GatewaySigned: false,
		},
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute
	receipt, err := Actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify GatewaySigned is false in receipt
	require.False(t, receipt.GatewaySigned, "GatewaySigned should be false for Tribunal-signed transactions")
}
