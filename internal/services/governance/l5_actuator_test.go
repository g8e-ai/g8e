// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	govtypes "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/internal/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
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
	require.Len(t, consoleAuditStore.DocSetCalls, 2)

	// Verify both calls were to console_audit collection
	for _, call := range consoleAuditStore.DocSetCalls {
		require.Equal(t, marshaler.CollectionName(constants.CollectionConsoleAudit), call.Collection)
		require.Equal(t, envelope.Id, call.ID)
	}

	// Verify initial receipt has EXECUTING status
	var initialRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.DocSetCalls[0].Data, &initialRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING, initialRecord.Status)

	// Verify final receipt has COMPLETED status
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.DocSetCalls[1].Data, &finalRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, finalRecord.Status)
}

func TestL5ActuatorExecuteHandlerError(t *testing.T) {
	t.Parallel()
	actuator, pubKey := newTestActuator(t)

	// Configure handler to return error
	handler := actuator.ExecutionHandler.(*mockExecutionHandler)
	handler.err = errors.New("handler execution failed")

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
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
	require.Len(t, consoleAuditStore.DocSetCalls, 2)

	// Verify final receipt has FAILED status
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.DocSetCalls[1].Data, &finalRecord)
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

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
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
	require.Error(t, err)
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

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
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
	require.Error(t, err)
	require.Nil(t, receipt)
}

func TestL5ActuatorExecuteMissingSigningKey(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)
	actuator.SigningKey = nil

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
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
	require.Contains(t, err.Error(), "L5Actuator: signing key missing")
	require.Nil(t, receipt)
}

func TestL5ActuatorExecuteMissingExecutionHandler(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)
	actuator.ExecutionHandler = nil

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
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
	require.Contains(t, err.Error(), "L5Actuator: ExecutionHandler not set")
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
	require.InEpsilon(t, float64(2), parsed["status"], 0.0) // EXECUTION_STATUS_COMPLETED = 2 (JSON marshals enums as float64)
	require.Equal(t, "test summary", parsed["result_summary"])
	require.Equal(t, "root-before", parsed["state_root_before"])
	require.Equal(t, "root-after", parsed["state_root_after"])
	require.InEpsilon(t, float64(1234567890), parsed["executed_at_unix_ms"], 0.0)
	require.Equal(t, "test-key-id", parsed["signer_key_id"])
}

// TestL5ActuatorExecuteCallsReceiptPublisherOnSuccess verifies that Execute
// calls ReceiptPublisher.PublishActionReceipt with the original command
// envelope and the final signed receipt after successful execution.
func TestL5ActuatorExecuteCallsReceiptPublisherOnSuccess(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)
	publisher := &mockReceiptPublisher{}
	actuator.ReceiptPublisher = publisher

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
		TransactionHash:   "test-hash-receipt-pub",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
		RequestorUserId:   "user-001",
		ActingAppId:       "spiffe://g8e.local/app/g8ee",
	}
	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	require.Equal(t, 1, publisher.callCount(), "PublishActionReceipt must be called exactly once after successful execution")
	require.Equal(t, envelope, publisher.envelope, "publisher must receive the original command envelope")
	require.Equal(t, receipt.TransactionId, publisher.receipt.TransactionId, "publisher must receive the final receipt")
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, publisher.receipt.Status)
	require.NotEmpty(t, publisher.receipt.Signature, "publisher must receive a signed receipt")
}

// TestL5ActuatorExecuteDoesNotCallReceiptPublisherWhenFinalReceiptFails
// verifies that Execute does not call ReceiptPublisher.PublishActionReceipt
// when signAndLogFinalReceipt fails (the receipt is not finalized).
func TestL5ActuatorExecuteDoesNotCallReceiptPublisherWhenFinalReceiptFails(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)

	// Configure the console audit store to fail on the second DocSet call
	// (the final receipt log), causing signAndLogFinalReceipt to return an
	// error before the ReceiptPublisher is reached.
	consoleAuditStore := actuator.ConsoleAuditStore.(*testutil.ConfigurableMockAuditStore)
	callCount := 0
	consoleAuditStore.DocSetFunc = func(collection, id string, data json.RawMessage) error {
		callCount++
		if callCount == 2 {
			return errors.New("final audit write failed")
		}
		return nil
	}

	publisher := &mockReceiptPublisher{}
	actuator.ReceiptPublisher = publisher

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
		TransactionHash:   "test-hash-receipt-pub-fail",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}
	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	_, err := actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err, "Execute must return an error when signAndLogFinalReceipt fails")
	require.Equal(t, 0, publisher.callCount(), "PublishActionReceipt must not be called when signAndLogFinalReceipt fails")
}

// TestL5ActuatorExecuteNilReceiptPublisherDoesNotPanic verifies that a nil
// ReceiptPublisher (the gateway in-process operator path) does not cause a
// panic and execution proceeds normally.
func TestL5ActuatorExecuteNilReceiptPublisherDoesNotPanic(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)
	// ReceiptPublisher is nil by default (not set in newTestActuator).
	require.Nil(t, actuator.ReceiptPublisher)

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
		TransactionHash:   "test-hash-nil-pub",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}
	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	assert.NotPanics(t, func() {
		receipt, err := actuator.Execute(context.Background(), vt, nil)
		require.NoError(t, err)
		require.NotNil(t, receipt)
	})
}

// TestL5ActuatorExecuteReceiptPublisherErrorDoesNotFailExecution verifies
// that a ReceiptPublisher publish error is best-effort: the execution
// succeeds and the receipt is returned, with the publish error only logged.
func TestL5ActuatorExecuteReceiptPublisherErrorDoesNotFailExecution(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)
	publisher := &mockReceiptPublisher{publishError: errors.New("publish failed")}
	actuator.ReceiptPublisher = publisher

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
		TransactionHash:   "test-hash-pub-err",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}
	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err, "publish error must not fail the execution")
	require.NotNil(t, receipt)
	require.Equal(t, 1, publisher.callCount(), "PublishActionReceipt must still be attempted")
}
