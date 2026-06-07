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
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

import (
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

func newTestActuator(t *testing.T) (*L5Actuator, ed25519.PublicKey) {
	t.Helper()

	// Generate L5Actuator signing key
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create mock dependencies
	mockHandler := &mockExecutionHandler{err: nil}
	mockAuditStore := testutil.NewConfigurableMockAuditStore(nil)
	mockStateRoot := testutil.NewMockStateRootProvider("test-state-root-123")

	logger := slog.Default()

	actuator := &L5Actuator{
		Logger:            logger,
		SigningKey:        privKey,
		KeyID:             "test-Actuator-key",
		ExecutionHandler:  mockHandler,
		ConsoleAuditStore: mockAuditStore,
		StateRootProvider: mockStateRoot,
	}

	return actuator, pubKey
}

func TestL5ActuatorExecuteHappyPath(t *testing.T) {
	t.Parallel()
	actuator, pubKey := newTestActuator(t)

	// Configure handler to succeed (already set in newTestActuator)

	// Create verified transaction
	envelope := &governance.GovernanceEnvelope{
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
	receipt, err := actuator.Execute(context.Background(), vt, nil)
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
	require.True(t, valid, "Receipt signature should verify against L5Actuator public key")

	// Verify audit store was called twice (initial EXECUTING receipt + final COMPLETED receipt)
	consoleAuditStore := actuator.ConsoleAuditStore.(*testutil.ConfigurableMockAuditStore)
	require.Len(t, consoleAuditStore.Calls, 2)

	// Verify both calls were to console_audit collection
	for _, call := range consoleAuditStore.Calls {
		require.Equal(t, marshaler.CollectionName(constants.CollectionConsoleAudit), call.Collection)
		require.Equal(t, envelope.Id, call.ID)
	}

	// Verify initial receipt has EXECUTING status
	var initialRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.Calls[0].Data, &initialRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING, initialRecord.Status)

	// Verify final receipt has COMPLETED status
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.Calls[1].Data, &finalRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, finalRecord.Status)
}

func TestL5ActuatorExecuteHandlerError(t *testing.T) {
	t.Parallel()
	actuator, pubKey := newTestActuator(t)

	// Configure handler to return error
	handler := actuator.ExecutionHandler.(*mockExecutionHandler)
	handler.err = errors.New("handler execution failed")

	envelope := &governance.GovernanceEnvelope{
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
	receipt, err := actuator.Execute(context.Background(), vt, nil)
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

	// Verify console audit store was called twice
	consoleAuditStore := actuator.ConsoleAuditStore.(*testutil.ConfigurableMockAuditStore)
	require.Len(t, consoleAuditStore.Calls, 2)

	// Verify final receipt has FAILED status
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.Calls[1].Data, &finalRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, finalRecord.Status)
}

func TestL5ActuatorExecuteAuditWriteFailInitial(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)

	// Configure console audit store to fail on first call (initial receipt)
	consoleAuditStore := actuator.ConsoleAuditStore.(*testutil.ConfigurableMockAuditStore)
	callCount := 0
	consoleAuditStore.DocSetFunc = func(collection, id string, data json.RawMessage) error {
		callCount++
		if callCount == 1 {
			return errors.New("audit write failed")
		}
		return nil
	}

	envelope := &governance.GovernanceEnvelope{
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
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to log initial action receipt")
	require.Nil(t, receipt)

	// Verify handler was never called (only initial audit write was attempted)
	require.Equal(t, 1, callCount)
}

func TestL5ActuatorExecuteReceiptPersistFail(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)

	// Configure console audit store to fail on DocSet
	consoleAuditStore := actuator.ConsoleAuditStore.(*testutil.ConfigurableMockAuditStore)
	consoleAuditStore.DocSetFunc = func(collection, id string, data json.RawMessage) error {
		return errors.New("doc set failed")
	}

	envelope := &governance.GovernanceEnvelope{
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
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to log initial action receipt")
	require.Nil(t, receipt)
}

func TestL5ActuatorExecuteMissingSigningKey(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)
	actuator.SigningKey = nil

	envelope := &governance.GovernanceEnvelope{
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
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "L5Actuator signing key missing")
	require.Nil(t, receipt)
}

func TestL5ActuatorExecuteMissingExecutionHandler(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)
	actuator.ExecutionHandler = nil

	envelope := &governance.GovernanceEnvelope{
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
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "L5Actuator ExecutionHandler not set")
	require.Nil(t, receipt)
}

func TestCanonicalizeActionReceipt(t *testing.T) {
	t.Parallel()
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

func TestL5ActuatorGatewaySignedPropagation(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)

	// Create envelope with GatewaySigned=true
	envelope := &governance.GovernanceEnvelope{
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
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify GatewaySigned is propagated to receipt
	require.True(t, receipt.GatewaySigned, "GatewaySigned should be propagated from envelope to receipt")

	// Verify audit record also has GatewaySigned
	consoleAuditStore := actuator.ConsoleAuditStore.(*testutil.ConfigurableMockAuditStore)
	require.Len(t, consoleAuditStore.Calls, 2)

	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.Calls[1].Data, &finalRecord)
	require.NoError(t, err)
	require.True(t, finalRecord.GatewaySigned, "Audit record should have GatewaySigned=true")
}

func TestL5ActuatorGatewaySignedFalse(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)

	// Create envelope with GatewaySigned=false (normal L2Consensus path)
	envelope := &governance.GovernanceEnvelope{
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
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify GatewaySigned is false in receipt
	require.False(t, receipt.GatewaySigned, "GatewaySigned should be false for L2Consensus-signed transactions")
}

func TestL5ActuatorRecordActionReceiptCalled(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)

	// Create a real AuditVault with test database
	tempDir := t.TempDir()

	// Create vault for encryption
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))
	testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: slog.Default()})
	require.NoError(t, err)
	require.NoError(t, testVault.Unlock(privKey))
	defer testVault.Close()

	auditConfig := &storage.AuditStoreConfig{
		DataDir:         tempDir,
		DBPath:          "test.db",
		MaxDBSizeMB:     100,
		RetentionDays:   1,
		Enabled:         true,
		EncryptionVault: testVault,
	}

	auditStore, err := storage.NewSQLAuditStore(auditConfig, slog.Default())
	require.NoError(t, err)
	defer auditStore.Close()

	actuator.SQLAuditStore = auditStore

	// Create the Operator session first (required for fail-closed audit)
	err = auditStore.CreateSession("test-operator-session", "operator", "Test Session", "test-user")
	require.NoError(t, err)

	envelope := &governance.GovernanceEnvelope{
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
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify RecordActionReceipt was called by querying the audit store
	persistedReceipt, err := auditStore.GetActionReceipt(envelope.Id)
	require.NoError(t, err)
	require.NotNil(t, persistedReceipt, "Receipt should be persisted in audit store")

	// Verify persisted receipt matches the returned receipt
	require.Equal(t, envelope.Id, persistedReceipt.TransactionID)
	require.Equal(t, envelope.TransactionHash, persistedReceipt.TransactionHash)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, persistedReceipt.Status)
	require.Equal(t, "completed", persistedReceipt.ResultSummary)
	require.NotEmpty(t, persistedReceipt.Signature, "Persisted receipt should have signature")
}
