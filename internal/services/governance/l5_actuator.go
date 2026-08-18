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
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	govtypes "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/services/storage"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

//go:generate mockery --name ExecutionHandler --output ./mocks --dir .

// ExecutionHandler is the interface for executing verified transactions.
// This avoids import cycles between governance and pubsub packages.
type ExecutionHandler interface {
	ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg CommandMessage) (string, error)
}

// CommandMessage is the typed command message passed through the L5 execution
// boundary. It supports sovereignty-preserving payload rehydration via
// GetPayload/SetPayload. Implemented by pubsub.PubSubCommandMessage.
type CommandMessage interface {
	GetPayload() []byte
	SetPayload([]byte)
}

// L5Actuator is the execution gateway. It is the final stop for all GovernanceEnvelope envelopes.
//
// Defense-in-depth note: L5Actuator does NOT re-verify L2 or L3 proofs. By design,
// L4Warden performs all pre-dispatch verification (L1 doctrine, L2 consensus, L3 notary)
// and embeds the results in VerifiedTransaction. L5 trusts that VerifiedTransaction,
// records the L2/L3 status in the ActionReceipt for audit, and focuses on execution
// safety: fail-closed receipt signing, JIT capability minting, and audit logging.
// The separation between L4 (verification) and L5 (execution) is the defense-in-depth
// boundary — two independent components with distinct responsibilities.
type L5Actuator struct {
	Logger            *slog.Logger
	SQLAuditStore     *storage.SQLAuditStore
	ConsoleAuditStore TransactionAuditStore
	StateRootProvider StateRootProvider
	ExecutionHandler  ExecutionHandler
	Scrubbing         *scrubbing.ScrubbingService

	// L5Actuator's own signing identity for ActionReceipts
	SigningKey ed25519.PrivateKey
	KeyID      string

	wg sync.WaitGroup
}

// Execute is the single execution boundary for all verified transactions.
// It dispatches to the registered handler, captures status, writes a console_audit row,
// signs and persists an ActionReceipt, and returns it.
//
// Fail-closed: if receipt signing or initial audit logging fails, the handler is NOT executed.
func (w *L5Actuator) Execute(ctx context.Context, vt *VerifiedTransaction, cmdMsg CommandMessage) (*operatorv1.ActionReceipt, error) {
	w.wg.Add(1)
	defer w.wg.Done()

	if w.ExecutionHandler == nil {
		return nil, constants.ErrL5ActuatorExecutionHandlerNotSet
	}
	if len(w.SigningKey) == 0 {
		return nil, constants.ErrL5ActuatorSigningKeyMissing
	}

	eventType := constants.MapActionTypeToEventType(vt.ActionType)

	w.Logger.Info("L5Actuator preparing to execute transaction",
		"message_id", vt.Envelope.Id,
		"action_type", vt.ActionType,
		"event_type", eventType)

	receipt := w.buildInitialReceipt(vt)

	if err := w.signAndLogReceipt(vt, receipt); err != nil {
		return nil, err
	}

	if err := w.rehydratePayload(ctx, vt, cmdMsg); err != nil {
		return nil, err
	}

	cap, err := MintCapability(vt, w.SigningKey, w.KeyID)
	if err != nil {
		w.Logger.Error("Fail-closed: Failed to mint execution capability", string(constants.ConnectionStateError), err, "message_id", vt.Envelope.Id)
		return nil, fmt.Errorf("%w: %w", constants.ErrL5ActuatorCapabilityMint, err)
	}
	w.Logger.Info("Minted JIT capability",
		"message_id", vt.Envelope.Id,
		"action_type", vt.ActionType,
		"target_resource", vt.Envelope.TargetResource,
		"expires_at", cap.ExpiresAt.UTC().Format(time.RFC3339))

	execCtx := ContextWithCapability(ctx, cap)

	summary, execErr := w.ExecutionHandler.ExecuteVerifiedTransaction(execCtx, eventType, cmdMsg)

	cap.Dissolve()
	w.Logger.Info("Dissolved JIT capability", "message_id", vt.Envelope.Id, "action_type", vt.ActionType)

	w.finalizeReceipt(receipt, summary, execErr)

	if err := w.signAndLogFinalReceipt(vt, receipt); err != nil {
		return receipt, err
	}

	return receipt, execErr
}

// buildInitialReceipt constructs the EXECUTING-status receipt with state root and L2/L3 status.
func (w *L5Actuator) buildInitialReceipt(vt *VerifiedTransaction) *operatorv1.ActionReceipt {
	stateBefore := ""
	if w.StateRootProvider != nil {
		var err error
		stateBefore, err = w.StateRootProvider.GetCurrentStateRoot()
		if err != nil {
			w.Logger.Warn("Failed to get state root before execution", string(constants.ConnectionStateError), err)
		}
	}

	l2Status := operatorv1.L2Status_L2_STATUS_NOT_REQUIRED
	if vt.Posture != nil && vt.Posture.RequiresL2Signature() {
		if vt.L2Valid {
			l2Status = operatorv1.L2Status_L2_STATUS_REQUIRED_VALID
		} else {
			l2Status = operatorv1.L2Status_L2_STATUS_REQUIRED_FAILED
		}
	}

	l3Status := operatorv1.L3Status_L3_STATUS_NOT_REQUIRED
	if vt.Posture != nil && vt.Posture.RequiresL3Proof() {
		if vt.L3Valid {
			l3Status = operatorv1.L3Status_L3_STATUS_REQUIRED_VALID
		} else {
			l3Status = operatorv1.L3Status_L3_STATUS_REQUIRED_FAILED
		}
	}

	return &operatorv1.ActionReceipt{
		TransactionId:    vt.Envelope.Id,
		TransactionHash:  vt.Envelope.TransactionHash,
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		ResultSummary:    "executing",
		StateRootBefore:  stateBefore,
		ExecutedAtUnixMs: time.Now().UnixMilli(),
		SignerKeyId:      w.KeyID,
		L2Status:         l2Status,
		L3Status:         l3Status,
	}
}

// signAndLogReceipt signs the initial receipt and logs it. Fail-closed: returns error if either step fails.
func (w *L5Actuator) signAndLogReceipt(vt *VerifiedTransaction, receipt *operatorv1.ActionReceipt) error {
	sig, err := w.signReceipt(receipt)
	if err != nil {
		w.Logger.Error("Fail-closed: Failed to sign initial action receipt", string(constants.ConnectionStateError), err, "message_id", vt.Envelope.Id)
		return fmt.Errorf("%w: %w", constants.ErrL5ActuatorSignReceipt, err)
	}
	receipt.Signature = sig

	if err := w.LogReceipt(vt.Envelope, receipt); err != nil {
		w.Logger.Error("Fail-closed: Failed to log initial action receipt", string(constants.ConnectionStateError), err, "message_id", vt.Envelope.Id)
		return fmt.Errorf("%w: %w", constants.ErrL5ActuatorLogReceipt, err)
	}
	return nil
}

// rehydratePayload rehydrates the command message payload in place. Fail-closed: returns error on rehydration failure.
func (w *L5Actuator) rehydratePayload(ctx context.Context, vt *VerifiedTransaction, cmdMsg CommandMessage) error {
	if w.Scrubbing == nil || cmdMsg == nil {
		return nil
	}
	p := cmdMsg.GetPayload()
	if len(p) == 0 {
		return nil
	}
	rehydrated, err := w.Scrubbing.RehydratePayload(ctx, p)
	if err != nil {
		w.Logger.Error("Fail-closed: Failed to rehydrate payload", string(constants.ConnectionStateError), err, "message_id", vt.Envelope.Id)
		return fmt.Errorf("%w: %w", constants.ErrL5ActuatorRehydrate, err)
	}
	cmdMsg.SetPayload(rehydrated)
	return nil
}

// finalizeReceipt updates the receipt with execution result, state root after, and timestamp.
func (w *L5Actuator) finalizeReceipt(receipt *operatorv1.ActionReceipt, summary string, execErr error) {
	status := operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED
	if summary == "" {
		summary = "completed"
	}
	if execErr != nil {
		status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
		summary = fmt.Errorf("failed: %w", execErr).Error()
	}

	stateAfter := ""
	if w.StateRootProvider != nil {
		var stateErr error
		stateAfter, stateErr = w.StateRootProvider.GetCurrentStateRoot()
		if stateErr != nil {
			w.Logger.Warn("Failed to get state root after execution", string(constants.ConnectionStateError), stateErr)
		}
	}

	receipt.Status = status
	receipt.ResultSummary = summary
	receipt.StateRootAfter = stateAfter
	receipt.ExecutedAtUnixMs = time.Now().UnixMilli()
}

// signAndLogFinalReceipt signs and logs the final receipt. Best-effort: returns error but receipt is still returned by caller.
func (w *L5Actuator) signAndLogFinalReceipt(vt *VerifiedTransaction, receipt *operatorv1.ActionReceipt) error {
	finalSig, err := w.signReceipt(receipt)
	if err != nil {
		w.Logger.Error("Failed to sign final action receipt - returning EXECUTING receipt as evidence", string(constants.ConnectionStateError), err, "message_id", vt.Envelope.Id)
		return fmt.Errorf("%w: %w", constants.ErrL5ActuatorSignReceipt, err)
	}
	receipt.Signature = finalSig

	if logErr := w.LogReceipt(vt.Envelope, receipt); logErr != nil {
		w.Logger.Error("Failed to log final action receipt - mutation already executed", string(constants.ConnectionStateError), logErr, "message_id", vt.Envelope.Id)
		return fmt.Errorf("%w: %w", constants.ErrL5ActuatorLogReceipt, logErr)
	}
	return nil
}

// canonicalReceipt is the typed representation for ActionReceipt canonicalization.
// This ensures strict typing and deterministic JSON marshaling for signing/verification.
type canonicalReceipt struct {
	TransactionID    string `json:"transaction_id"`
	TransactionHash  string `json:"transaction_hash"`
	Status           int32  `json:"status"`
	ResultSummary    string `json:"result_summary"`
	StateRootBefore  string `json:"state_root_before"`
	StateRootAfter   string `json:"state_root_after"`
	ExecutedAtUnixMs int64  `json:"executed_at_unix_ms"`
	SignerKeyID      string `json:"signer_key_id"`
	L2Status         int32  `json:"l2_status"`
	L3Status         int32  `json:"l3_status"`
}

// CanonicalizeActionReceipt produces a deterministic byte representation for signing/verification.
// This function must be used by both signing and verification to ensure consistency.
// Field order: transaction_id, transaction_hash, status, result_summary, state_root_before,
// state_root_after, executed_at_unix_ms, signer_key_id, l2_status, l3_status.
// All fields are included in the canonical form.
func CanonicalizeActionReceipt(r *operatorv1.ActionReceipt) ([]byte, error) {
	canonical := canonicalReceipt{
		TransactionID:    r.TransactionId,
		TransactionHash:  r.TransactionHash,
		Status:           int32(r.Status),
		ResultSummary:    r.ResultSummary,
		StateRootBefore:  r.StateRootBefore,
		StateRootAfter:   r.StateRootAfter,
		ExecutedAtUnixMs: r.ExecutedAtUnixMs,
		SignerKeyID:      r.SignerKeyId,
		L2Status:         int32(r.L2Status),
		L3Status:         int32(r.L3Status),
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrL5ActuatorMarshalReceipt, err)
	}
	return payload, nil
}

func (w *L5Actuator) signReceipt(r *operatorv1.ActionReceipt) (string, error) {
	if len(w.SigningKey) == 0 {
		return "", constants.ErrL5ActuatorSigningKeyMissing
	}

	// Use canonical serialization for signing - shared with verification
	payload, err := CanonicalizeActionReceipt(r)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrL5ActuatorCanonicalizeReceipt, err)
	}

	sig := ed25519.Sign(w.SigningKey, payload)
	return hex.EncodeToString(sig), nil
}

// LogReceipt records the signed action receipt in the audit store and console_audit.
func (w *L5Actuator) LogReceipt(env *govtypes.GovernanceEnvelope, r *operatorv1.ActionReceipt) error {
	docErr := w.logReceiptDocument(env, r)

	if w.SQLAuditStore == nil {
		return docErr
	}

	record := buildReceiptRecord(env, r)

	if err := w.SQLAuditStore.RecordActionReceipt(record); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record ActionReceipt in audit store", string(constants.ConnectionStateError), err)
		}
		if docErr != nil {
			return fmt.Errorf("%w: %v, doc store error: %v", constants.ErrL5ActuatorAuditStore, err, docErr)
		}
		return fmt.Errorf("%w: %w", constants.ErrL5ActuatorAuditStore, err)
	}

	return docErr
}

func (w *L5Actuator) logReceiptDocument(env *govtypes.GovernanceEnvelope, r *operatorv1.ActionReceipt) error {
	if w.ConsoleAuditStore == nil || env == nil {
		return nil
	}

	record := buildReceiptRecord(env, r)

	body, err := json.Marshal(record)
	if err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to marshal action receipt record", string(constants.ConnectionStateError), err, "message_id", r.TransactionId)
		}
		return fmt.Errorf("%w: %w", constants.ErrL5ActuatorMarshalReceipt, err)
	}

	if err := w.ConsoleAuditStore.DocSet(marshaler.CollectionName(constants.CollectionConsoleAudit), r.TransactionId, body); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record action receipt document", string(constants.ConnectionStateError), err, "message_id", r.TransactionId)
		}
		return fmt.Errorf("%w: %w", constants.ErrL5ActuatorDocStore, err)
	}
	return nil
}

// buildReceiptRecord constructs an ActionReceiptRecord from a GovernanceEnvelope and ActionReceipt.
// This is the single source of truth for record construction, used by both LogReceipt and logReceiptDocument.
func buildReceiptRecord(env *govtypes.GovernanceEnvelope, r *operatorv1.ActionReceipt) *models.ActionReceiptRecord {
	return &models.ActionReceiptRecord{
		TransactionID:     r.TransactionId,
		TransactionHash:   r.TransactionHash,
		OperatorID:        env.OperatorId,
		OperatorSessionID: env.OperatorSessionId,
		ActionType:        constants.ActionType(env.ActionType),
		TargetResource:    env.TargetResource,
		Status:            r.Status,
		ResultSummary:     r.ResultSummary,
		StateRootBefore:   r.StateRootBefore,
		StateRootAfter:    r.StateRootAfter,
		ExecutedAt:        time.UnixMilli(r.ExecutedAtUnixMs),
		SignerKeyID:       r.SignerKeyId,
		Signature:         r.Signature,
		L2Valid:           r.L2Status == operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
		L3Valid:           r.L3Status == operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
		Timestamp:         time.Now().UTC(),
	}
}

// Wait blocks until all in-flight transactions have finished executing.
func (w *L5Actuator) Wait() {
	w.wg.Wait()
}
