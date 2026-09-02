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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
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

func TestL5ActuatorRecordRejectedTransactionSignsFailedStageEvidence(t *testing.T) {
	t.Parallel()
	actuator, publicKey := newTestActuator(t)
	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
		TransactionHash:   "test-rejected-hash",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
	}
	failedStage := newDeterministicStageEvidence(
		envelope,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
		governanceMonotonicNow(),
		operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED,
	)
	rejected := &VerifiedTransaction{
		Envelope:                   envelope,
		ActionType:                 constants.ActionTypeExecuteBash,
		DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{failedStage},
	}

	receipt, err := actuator.RecordRejectedTransaction(context.Background(), rejected, constants.ErrTxStateRootMismatch)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, receipt.Status)
	require.Contains(t, receipt.ResultSummary, constants.ErrTxStateRootMismatch.Error())
	require.Len(t, receipt.DeterministicStageEvidence, 1)
	assert.Equal(t, operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED, receipt.DeterministicStageEvidence[0].Outcome)
	assert.False(t, actuator.ExecutionHandler.(*mockExecutionHandler).executed)

	canonical, err := CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	signature, err := hex.DecodeString(receipt.Signature)
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(publicKey, canonical, signature))
	require.NotNil(t, receipt.FinalPersistenceAttestation)
	assert.NoError(t, VerifyReceiptPersistenceAttestation(receipt, publicKey))
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
	require.Len(t, receipt.DeterministicStageEvidence, 2)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE, receipt.DeterministicStageEvidence[0].Kind)
	assert.NotEmpty(t, receipt.DeterministicStageEvidence[0].ReceiptSignatureDigest)
	assert.Equal(t, envelope.Id, receipt.DeterministicStageEvidence[0].AuditRecordId)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION, receipt.DeterministicStageEvidence[1].Kind)
	assert.Equal(t, receipt.DeterministicStageEvidence[1].StageId, receipt.DeterministicStageEvidence[0].ParentStageId)
	assert.Equal(t, operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, receipt.DeterministicStageEvidence[1].Outcome)
	assert.Equal(t, receipt.StateRootBefore, receipt.DeterministicStageEvidence[1].StateRootBefore)
	assert.Equal(t, receipt.StateRootAfter, receipt.DeterministicStageEvidence[1].StateRootAfter)

	// Verify signature
	canonical, err := CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)

	sigBytes, err := hex.DecodeString(receipt.Signature)
	require.NoError(t, err)

	valid := ed25519.Verify(pubKey, canonical, sigBytes)
	require.True(t, valid, "Receipt signature should verify against L5Actuator public key")

	tampered := proto.Clone(receipt).(*operatorv1.ActionReceipt)
	tampered.DeterministicStageEvidence[1].StateRootAfter = "tampered-root"
	tamperedCanonical, err := CanonicalizeActionReceipt(tampered)
	require.NoError(t, err)
	assert.False(t, ed25519.Verify(pubKey, tamperedCanonical, sigBytes))

	// Verify audit store recorded the initial receipt, final receipt, and persistence attestation
	consoleAuditStore := actuator.ConsoleAuditStore.(*testutil.ConfigurableMockAuditStore)
	require.Len(t, consoleAuditStore.DocSetCalls, 3)

	// Verify all calls were to console_audit collection
	for _, call := range consoleAuditStore.DocSetCalls {
		require.Equal(t, marshaler.CollectionName(constants.CollectionConsoleAudit), call.Collection)
		require.Equal(t, envelope.Id, call.ID)
	}

	// Verify initial receipt has EXECUTING status
	var initialRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.DocSetCalls[0].Data, &initialRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING, initialRecord.Status)

	// Verify persisted final receipt has COMPLETED status and an attestation
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.DocSetCalls[2].Data, &finalRecord)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, finalRecord.Status)
	require.NotNil(t, receipt.FinalPersistenceAttestation)
	require.NoError(t, VerifyReceiptPersistenceAttestation(receipt, pubKey))
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

	// Verify console audit store recorded the final persistence attestation
	consoleAuditStore := actuator.ConsoleAuditStore.(*testutil.ConfigurableMockAuditStore)
	require.Len(t, consoleAuditStore.DocSetCalls, 3)

	// Verify final receipt has FAILED status
	var finalRecord models.ActionReceiptRecord
	err = json.Unmarshal(consoleAuditStore.DocSetCalls[2].Data, &finalRecord)
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

func TestL5ActuatorExecuteFinalPersistenceAttestationWriteFailure(t *testing.T) {
	t.Parallel()
	actuator, _ := newTestActuator(t)
	consoleAuditStore := actuator.ConsoleAuditStore.(*testutil.ConfigurableMockAuditStore)
	writeErr := errors.New("final persistence attestation write failed")
	callCount := 0
	consoleAuditStore.DocSetFunc = func(collection, id string, data json.RawMessage) error {
		callCount++
		if callCount == 3 {
			return writeErr
		}
		return nil
	}
	vt := &VerifiedTransaction{
		Envelope: &govtypes.GovernanceEnvelope{
			Id:                uuid.NewString(),
			TransactionHash:   "test-final-persistence-write-failure",
			OperatorId:        "test-operator",
			OperatorSessionId: "test-operator-session",
			ActionType:        string(constants.ActionTypeExecuteBash),
			TargetResource:    "localhost",
		},
		ActionType: constants.ActionTypeExecuteBash,
	}

	receipt, err := actuator.Execute(context.Background(), vt, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrL5ActuatorLogReceipt)
	assert.ErrorIs(t, err, writeErr)
	require.NotNil(t, receipt)
	assert.NotNil(t, receipt.FinalPersistenceAttestation)
	assert.Equal(t, 3, callCount)
}

func TestVerifyActionReceiptSignatureFailsClosedOnInvalidEvidence(t *testing.T) {
	actuator, publicKey := newTestActuator(t)
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    "test-transaction",
		TransactionHash:  "test-transaction-hash",
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:    "completed",
		ExecutedAtUnixMs: 1,
		SignerKeyId:      actuator.KeyID,
	}
	signature, err := actuator.signReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = signature
	wrongPublicKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	tests := []struct {
		name      string
		receipt   *operatorv1.ActionReceipt
		publicKey ed25519.PublicKey
		wantErr   error
	}{
		{name: "valid signature", receipt: receipt, publicKey: publicKey},
		{name: "missing receipt", publicKey: publicKey, wantErr: constants.ErrActionReceiptMissing},
		{name: "missing signature", receipt: func() *operatorv1.ActionReceipt {
			candidate := proto.Clone(receipt).(*operatorv1.ActionReceipt)
			candidate.Signature = ""
			return candidate
		}(), publicKey: publicKey, wantErr: constants.ErrActionReceiptSignatureInvalid},
		{name: "malformed signature", receipt: func() *operatorv1.ActionReceipt {
			candidate := proto.Clone(receipt).(*operatorv1.ActionReceipt)
			candidate.Signature = "not-hex"
			return candidate
		}(), publicKey: publicKey, wantErr: constants.ErrActionReceiptSignatureInvalid},
		{name: "tampered receipt", receipt: func() *operatorv1.ActionReceipt {
			candidate := proto.Clone(receipt).(*operatorv1.ActionReceipt)
			candidate.ResultSummary = "tampered"
			return candidate
		}(), publicKey: publicKey, wantErr: constants.ErrActionReceiptSignatureInvalid},
		{name: "wrong public key", receipt: receipt, publicKey: wrongPublicKey, wantErr: constants.ErrActionReceiptSignatureInvalid},
		{name: "malformed public key", receipt: receipt, publicKey: ed25519.PublicKey("short"), wantErr: constants.ErrActionReceiptSignatureInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyActionReceiptSignature(tt.receipt, tt.publicKey)
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestVerifyReceiptPersistenceAttestationRejectsSemanticallyUnboundFields(t *testing.T) {
	t.Parallel()
	actuator, publicKey := newTestActuator(t)
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "test-transaction",
		SignerKeyId:   actuator.KeyID,
		Signature:     "abcd",
	}
	tests := []struct {
		name   string
		mutate func(*operatorv1.ReceiptPersistenceAttestation)
	}{
		{name: "transaction mismatch", mutate: func(attestation *operatorv1.ReceiptPersistenceAttestation) {
			attestation.TransactionId = "other-transaction"
		}},
		{name: "audit record mismatch", mutate: func(attestation *operatorv1.ReceiptPersistenceAttestation) {
			attestation.AuditRecordId = "other-record"
		}},
		{name: "signer mismatch", mutate: func(attestation *operatorv1.ReceiptPersistenceAttestation) {
			attestation.SignerKeyId = "other-signer"
		}},
		{name: "missing persistence timestamp", mutate: func(attestation *operatorv1.ReceiptPersistenceAttestation) {
			attestation.PersistedAtUnixMs = 0
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attestation, err := actuator.signReceiptPersistenceAttestation(receipt)
			require.NoError(t, err)
			tt.mutate(attestation)
			canonical, err := CanonicalizeReceiptPersistenceAttestation(attestation)
			require.NoError(t, err)
			attestation.Signature = hex.EncodeToString(ed25519.Sign(actuator.SigningKey, canonical))
			receipt.FinalPersistenceAttestation = attestation

			assert.ErrorIs(t, VerifyReceiptPersistenceAttestation(receipt, publicKey), constants.ErrReceiptPersistenceAttestationInvalid)
		})
	}
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

func TestCanonicalizeCommitmentAttestation_ProtocolVector(t *testing.T) {
	t.Parallel()
	attestation := &operatorv1.CommitmentAttestation{
		TransactionId:               "tx-1",
		TransactionHash:             "abc123",
		StateRootAtCommit:           "state-1",
		L2SignatureDigest:           "l2",
		WardenIntentSignatureDigest: "warden",
		HumanSignatureDigest:        "human",
		ActionType:                  "FILE_EDIT",
		TargetResource:              "/tmp/example",
		CommittedAtUnixMs:           1700000000123,
		AuditorKeyId:                "0123",
		Signature:                   "excluded",
		Hash:                        "excluded",
	}

	canonical, err := CanonicalizeCommitmentAttestation(attestation)
	require.NoError(t, err)
	assert.Equal(t, "0000000474782d3100000006616263313233000000000000000773746174652d31000000026c320000000677617264656e0000000568756d616e0000000946494c455f454449540000000c2f746d702f6578616d706c650000018bcfe5687b0000000430313233", hex.EncodeToString(canonical))
}

func TestSignatureDigest_ProtocolVectors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		signatures []string
		expected   string
	}{
		{name: "no signatures", signatures: nil, expected: ""},
		{name: "single signature", signatures: []string{"aa"}, expected: "e9e797104fd871e7e240d637b2bba89f78211d032586b6ce20080025db015bec"},
		{name: "sorted nonempty signatures", signatures: []string{"bb", "", "aa"}, expected: "ee23674743e070fa0543cbf985228c7e34e18e6f64652da9cb1bec62887d9518"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := append([]string(nil), tt.signatures...)
			assert.Equal(t, tt.expected, SignatureDigest(input))
			assert.Equal(t, tt.signatures, input)
		})
	}
}
