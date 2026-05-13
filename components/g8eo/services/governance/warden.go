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

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/internal/mappings"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/storage"
	operatorv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
)

type L3Verifier interface {
	VerifyL3Proof(userID, messageID, signatureHex, pubKeyHex string) (bool, error)
}

// ExecutionHandler is the interface for executing verified transactions.
// This avoids import cycles between governance and pubsub packages.
type ExecutionHandler interface {
	ExecuteVerifiedTransaction(ctx context.Context, eventType string, cmdMsg interface{}) error
}

type TransactionAuditStore interface {
	DocSet(collection, id string, data json.RawMessage) error
}

// Warden is the execution gateway. It is the final stop for all UAP envelopes.
type Warden struct {
	Logger            *slog.Logger
	TrustedNodes      map[string]ed25519.PublicKey
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
	eventType := mappings.MapActionTypeToEventType(vt.ActionType)

	w.Logger.Info("Warden executing transaction",
		"message_id", vt.Envelope.Id,
		"action_type", vt.ActionType,
		"event_type", eventType)

	// Execute through the handler
	err := w.ExecutionHandler.ExecuteVerifiedTransaction(ctx, eventType, cmdMsg)

	status := operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED
	summary := "completed"
	if err != nil {
		status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
		summary = fmt.Sprintf("failed: %v", err)
	}

	stateAfter := ""
	if w.StateRootProvider != nil {
		var err error
		stateAfter, err = w.StateRootProvider.GetCurrentStateRoot()
		if err != nil {
			w.Logger.Warn("Failed to get state root after execution", "error", err)
		}
	}

	receipt := &operatorv1.ActionReceipt{
		TransactionId:    vt.Envelope.Id,
		TransactionHash:  vt.Envelope.TransactionHash,
		Status:           status,
		ResultSummary:    summary,
		StateRootBefore:  stateBefore,
		StateRootAfter:   stateAfter,
		ExecutedAtUnixMs: time.Now().UnixMilli(),
		SignerKeyId:      w.KeyID,
	}

	// Sign the receipt
	sig, signErr := w.signReceipt(receipt)
	if signErr != nil {
		w.Logger.Error("Failed to sign action receipt", "error", signErr, "message_id", vt.Envelope.Id)
		// We still have the execution result, but the receipt is broken
	} else {
		receipt.Signature = sig
	}

	// Persist to audit
	w.LogReceipt(vt.Envelope, receipt)

	return receipt, err
}

func (w *Warden) signReceipt(r *operatorv1.ActionReceipt) (string, error) {
	if len(w.SigningKey) == 0 {
		return "", errors.New("signing key missing")
	}

	// Canonical serialization for signing (simplified for now: pipe-delimited)
	payload := fmt.Sprintf("%s|%s|%d|%s|%s|%s|%d|%s",
		r.TransactionId,
		r.TransactionHash,
		int32(r.Status),
		r.ResultSummary,
		r.StateRootBefore,
		r.StateRootAfter,
		r.ExecutedAtUnixMs,
		r.SignerKeyId,
	)

	sig := ed25519.Sign(w.SigningKey, []byte(payload))
	return hex.EncodeToString(sig), nil
}

// LogReceipt records the signed action receipt in the audit vault and console_audit.
func (w *Warden) LogReceipt(env *uap.UAPEnvelope, r *operatorv1.ActionReceipt) {
	w.logReceiptDocument(env, r)

	if w.AuditVault == nil {
		return
	}

	event := &storage.Event{
		OperatorSessionID: env.OperatorId,
		Timestamp:         time.Now(),
		Type:              storage.EventType("action_receipt"),
		ContentText:       fmt.Sprintf("ActionReceipt: %s (Status: %s, Summary: %s)", r.TransactionId, r.Status, r.ResultSummary),
		CommandRaw:        fmt.Sprintf("tx_hash: %s, state_before: %s, state_after: %s", r.TransactionHash, r.StateRootBefore, r.StateRootAfter),
	}

	if _, err := w.AuditVault.RecordEvent(event); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record ActionReceipt in audit vault", "error", err)
		}
	}
}

func (w *Warden) logReceiptDocument(env *uap.UAPEnvelope, r *operatorv1.ActionReceipt) {
	if w.AuditStore == nil || env == nil {
		return
	}

	record := models.ActionReceiptRecord{
		TransactionID:     r.TransactionId,
		TransactionHash:   r.TransactionHash,
		OperatorID:        env.OperatorId,
		OperatorSessionID: env.OperatorSessionId,
		ActionType:        env.ActionType,
		TargetResource:    env.TargetResource,
		Status:            r.Status.String(),
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
		return
	}

	if err := w.AuditStore.DocSet(string(constants.CollectionConsoleAudit), r.TransactionId, body); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record action receipt document", "error", err, "message_id", r.TransactionId)
		}
	}
}

// NoOpL3Verifier is an L3 verifier that always returns true.
// Used for outbound mode where L3 verification happens at the platform level.
type NoOpL3Verifier struct{}

func (n *NoOpL3Verifier) VerifyL3Proof(userID, messageID, signatureHex, pubKeyHex string) (bool, error) {
	return true, nil
}
