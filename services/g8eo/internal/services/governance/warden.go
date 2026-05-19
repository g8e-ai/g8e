package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	execution "github.com/g8e-ai/g8e/services/g8eo/internal/services/execution"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/storage"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
)

type L3Verifier interface {
	VerifyL3Proof(userID, transactionHash string, proof *commonv1.L3Proof) (bool, error)
}

// ExecutionHandler is the interface for executing verified transactions.
// This avoids import cycles between governance and pubsub packages.
type ExecutionHandler interface {
	ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error)
}

type TransactionAuditStore interface {
	DocSet(collection, id string, data json.RawMessage) error
}

// Warden is the execution gateway. It is the final stop for all UAP envelopes.
type Warden struct {
	Logger            *slog.Logger
	SignerStore       SignerStore
	Execution         *execution.ExecutionService
	AuditVault        *storage.AuditVaultService
	AuditStore        TransactionAuditStore
	L3Verifier        L3Verifier
	StateRootProvider StateRootProvider
	Ctx               context.Context
	ExecutionHandler  ExecutionHandler

	// Warden's own signing identity for ActionReceipts
	SigningKey ed25519.PrivateKey
	KeyID      string
}

// Execute is the single execution boundary for all verified transactions.
// It dispatches to the registered handler, captures status, writes a console_audit row,
// signs and persists an ActionReceipt, and returns it.
//
// Fail-closed: if receipt signing or initial audit logging fails, the handler is NOT executed.
func (w *Warden) Execute(ctx context.Context, vt *VerifiedTransaction, cmdMsg interface{}) (*operatorv1.ActionReceipt, error) {
	if w.ExecutionHandler == nil {
		return nil, errors.New("Warden ExecutionHandler not set")
	}
	if len(w.SigningKey) == 0 {
		return nil, errors.New("Warden signing key missing - cannot execute mutations")
	}

	stateBefore := ""
	if w.StateRootProvider != nil {
		var err error
		stateBefore, err = w.StateRootProvider.GetCurrentStateRoot()
		if err != nil {
			w.Logger.Warn("Failed to get state root before execution", "error", err)
		}
	}

	// Map action type to event type for handler lookup
	eventType := constants.MapActionTypeToEventType(vt.ActionType)

	w.Logger.Info("Warden preparing to execute transaction",
		"message_id", vt.Envelope.Id,
		"action_type", vt.ActionType,
		"event_type", eventType)

	// 1. Prepare initial receipt
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    vt.Envelope.Id,
		TransactionHash:  vt.Envelope.TransactionHash,
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		ResultSummary:    "executing",
		StateRootBefore:  stateBefore,
		ExecutedAtUnixMs: time.Now().UnixMilli(),
		SignerKeyId:      w.KeyID,
	}

	// 2. Sign the initial receipt (intent to execute)
	sig, signErr := w.signReceipt(receipt)
	if signErr != nil {
		w.Logger.Error("Fail-closed: Failed to sign initial action receipt", "error", signErr, "message_id", vt.Envelope.Id)
		return nil, fmt.Errorf("failed to sign initial action receipt: %w", signErr)
	}
	receipt.Signature = sig

	// 3. Log intent to execute (Audit before execution)
	if err := w.LogReceipt(vt.Envelope, receipt); err != nil {
		w.Logger.Error("Fail-closed: Failed to log initial action receipt", "error", err, "message_id", vt.Envelope.Id)
		return nil, fmt.Errorf("failed to log initial action receipt: %w", err)
	}

	// 4. Execute through the handler
	summary, err := w.ExecutionHandler.ExecuteVerifiedTransaction(ctx, eventType, cmdMsg)

	// 5. Update receipt with final result
	status := operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED
	if summary == "" {
		summary = "completed"
	}
	if err != nil {
		status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
		summary = fmt.Sprintf("failed: %v", err)
	}

	stateAfter := ""
	if w.StateRootProvider != nil {
		var stateErr error
		stateAfter, stateErr = w.StateRootProvider.GetCurrentStateRoot()
		if stateErr != nil {
			w.Logger.Warn("Failed to get state root after execution", "error", stateErr)
		}
	}

	receipt.Status = status
	receipt.ResultSummary = summary
	receipt.StateRootAfter = stateAfter
	receipt.ExecutedAtUnixMs = time.Now().UnixMilli()

	// 6. Sign the final receipt
	finalSig, signErr := w.signReceipt(receipt)
	if signErr != nil {
		w.Logger.Error("Failed to sign final action receipt - returning EXECUTING receipt as evidence", "error", signErr, "message_id", vt.Envelope.Id)
		// Return the EXECUTING receipt with signature from step 2 as evidence
		// The mutation already executed, so we must preserve evidence of execution attempt
		return receipt, fmt.Errorf("execution completed but final receipt signing failed: %w", signErr)
	}
	receipt.Signature = finalSig

	// 7. Log final result (best-effort - mutation already executed)
	if logErr := w.LogReceipt(vt.Envelope, receipt); logErr != nil {
		w.Logger.Error("Failed to log final action receipt - mutation already executed", "error", logErr, "message_id", vt.Envelope.Id)
		// Return receipt anyway - mutation already happened, evidence must be preserved
		return receipt, fmt.Errorf("execution completed but final audit logging failed: %w", logErr)
	}

	return receipt, err
}

// CanonicalizeActionReceipt produces a deterministic byte representation for signing/verification.
// This function must be used by both signing and verification to ensure consistency.
// Field order: transaction_id, transaction_hash, status, result_summary, state_root_before,
// state_root_after, executed_at_unix_ms, signer_key_id. All fields are included in the canonical form.
func CanonicalizeActionReceipt(r *operatorv1.ActionReceipt) ([]byte, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"transaction_id":      r.TransactionId,
		"transaction_hash":    r.TransactionHash,
		"status":              int32(r.Status),
		"result_summary":      r.ResultSummary,
		"state_root_before":   r.StateRootBefore,
		"state_root_after":    r.StateRootAfter,
		"executed_at_unix_ms": r.ExecutedAtUnixMs,
		"signer_key_id":       r.SignerKeyId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal receipt for canonicalization: %w", err)
	}
	return payload, nil
}

func (w *Warden) signReceipt(r *operatorv1.ActionReceipt) (string, error) {
	if len(w.SigningKey) == 0 {
		return "", errors.New("signing key missing")
	}

	// Use canonical serialization for signing - shared with verification
	payload, err := CanonicalizeActionReceipt(r)
	if err != nil {
		return "", fmt.Errorf("failed to canonicalize receipt for signing: %w", err)
	}

	sig := ed25519.Sign(w.SigningKey, payload)
	return hex.EncodeToString(sig), nil
}

// LogReceipt records the signed action receipt in the audit vault and console_audit.
func (w *Warden) LogReceipt(env *uap.UAPEnvelope, r *operatorv1.ActionReceipt) error {
	docErr := w.logReceiptDocument(env, r)

	if w.AuditVault == nil {
		return docErr
	}

	record := models.ActionReceiptRecord{
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
		Timestamp:         time.Now().UTC(),
	}

	if err := w.AuditVault.RecordActionReceipt(&record); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record ActionReceipt in audit vault", "error", err)
		}
		if docErr != nil {
			return fmt.Errorf("audit vault error: %v, doc store error: %v", err, docErr)
		}
		return err
	}

	return docErr
}

func (w *Warden) logReceiptDocument(env *uap.UAPEnvelope, r *operatorv1.ActionReceipt) error {
	if w.AuditStore == nil || env == nil {
		return nil
	}

	record := models.ActionReceiptRecord{
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
		Timestamp:         time.Now().UTC(),
	}

	body, err := json.Marshal(record)
	if err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to marshal action receipt record", "error", err, "message_id", r.TransactionId)
		}
		return err
	}

	if err := w.AuditStore.DocSet(marshaler.CollectionName(constants.CollectionConsoleAudit), r.TransactionId, body); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record action receipt document", "error", err, "message_id", r.TransactionId)
		}
		return err
	}
	return nil
}
