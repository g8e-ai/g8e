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
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// mockAuditStore is a test implementation of TransactionAuditStore.
type mockAuditStore struct {
	docSetFunc func(collection, id string, data json.RawMessage) error
	calls      []struct {
		collection string
		id         string
		data       json.RawMessage
	}
}

func (m *mockAuditStore) DocSet(collection, id string, data json.RawMessage) error {
	m.calls = append(m.calls, struct {
		collection string
		id         string
		data       json.RawMessage
	}{collection, id, data})
	if m.docSetFunc != nil {
		return m.docSetFunc(collection, id, data)
	}
	return nil
}

func newTestWarden(t *testing.T) (*Warden, ed25519.PublicKey) {
	t.Helper()

	// Generate Warden signing key
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create mock dependencies
	mockHandler := &mockExecutionHandler{err: nil}
	mockAuditStore := &mockAuditStore{}
	mockStateRoot := &mockStateRootProvider{root: "test-state-root-123"}

	logger := slog.Default()

	warden := &Warden{
		Logger:            logger,
		SigningKey:        privKey,
		KeyID:             "test-warden-key",
		ExecutionHandler:  mockHandler,
		AuditStore:        mockAuditStore,
		StateRootProvider: mockStateRoot,
	}

	return warden, pubKey
}

func TestWardenExecuteHappyPath(t *testing.T) {
	warden, pubKey := newTestWarden(t)

	// Configure handler to succeed (already set in newTestWarden)

	// Create verified transaction
	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute
	receipt, err := warden.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify receipt fields
	require.Equal(t, envelope.Id, receipt.TransactionId)
	require.Equal(t, envelope.TransactionHash, receipt.TransactionHash)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
	require.Equal(t, "completed", receipt.ResultSummary)
	require.Equal(t, "test-state-root-123", receipt.StateRootBefore)
	require.Equal(t, "test-state-root-123", receipt.StateRootAfter)
	require.Equal(t, "test-warden-key", receipt.SignerKeyId)
	require.NotEmpty(t, receipt.Signature)

	// Verify signature
	canonical, err := CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)

	sigBytes, err := hex.DecodeString(receipt.Signature)
	require.NoError(t, err)

	valid := ed25519.Verify(pubKey, canonical, sigBytes)
	require.True(t, valid, "Receipt signature should verify against Warden public key")

	// Verify audit store was called twice (initial EXECUTING receipt + final COMPLETED receipt)
	auditStore := warden.AuditStore.(*mockAuditStore)
	require.Len(t, auditStore.calls, 2)

	// Verify both calls were to console_audit collection
	for _, call := range auditStore.calls {
		require.Equal(t, marshaler.CollectionName(constants.CollectionConsoleAudit), call.collection)
		require.Equal(t, envelope.Id, call.id)
	}

	// Verify initial receipt has EXECUTING status
	var initialRecord models.ActionReceiptRecord
	err = json.Unmarshal(auditStore.calls[0].data, &initialRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING, initialRecord.Status)

	// Verify final receipt has COMPLETED status
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(auditStore.calls[1].data, &finalRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, finalRecord.Status)
}

func TestWardenExecuteHandlerError(t *testing.T) {
	warden, pubKey := newTestWarden(t)

	// Configure handler to return error
	handler := warden.ExecutionHandler.(*mockExecutionHandler)
	handler.err = errors.New("handler execution failed")

	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute
	receipt, err := warden.Execute(context.Background(), vt, nil)
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
	auditStore := warden.AuditStore.(*mockAuditStore)
	require.Len(t, auditStore.calls, 2)

	// Verify final receipt has FAILED status
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(auditStore.calls[1].data, &finalRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, finalRecord.Status)
}

func TestWardenExecuteAuditWriteFailInitial(t *testing.T) {
	warden, _ := newTestWarden(t)

	// Configure audit store to fail on first call (initial receipt)
	auditStore := warden.AuditStore.(*mockAuditStore)
	callCount := 0
	auditStore.docSetFunc = func(collection, id string, data json.RawMessage) error {
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
		OperatorSessionId: "test-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute - should fail before handler is invoked
	receipt, err := warden.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to log initial action receipt")
	require.Nil(t, receipt)

	// Verify handler was never called (only initial audit write was attempted)
	require.Equal(t, 1, callCount)
}

func TestWardenExecuteReceiptPersistFail(t *testing.T) {
	warden, _ := newTestWarden(t)

	// Configure audit store to fail on DocSet
	auditStore := warden.AuditStore.(*mockAuditStore)
	auditStore.docSetFunc = func(collection, id string, data json.RawMessage) error {
		return errors.New("doc set failed")
	}

	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute - should fail before handler is invoked
	receipt, err := warden.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to log initial action receipt")
	require.Nil(t, receipt)
}

func TestWardenExecuteMissingSigningKey(t *testing.T) {
	warden, _ := newTestWarden(t)
	warden.SigningKey = nil

	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute - should fail immediately
	receipt, err := warden.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Warden signing key missing")
	require.Nil(t, receipt)
}

func TestWardenExecuteMissingExecutionHandler(t *testing.T) {
	warden, _ := newTestWarden(t)
	warden.ExecutionHandler = nil

	envelope := &uap.UAPEnvelope{
		Id:                uuid.New().String(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute - should fail immediately
	receipt, err := warden.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Warden ExecutionHandler not set")
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
