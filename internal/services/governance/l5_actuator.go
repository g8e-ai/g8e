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
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	execution "github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/pkg/governance"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

//go:generate mockery --name ExecutionHandler --output ./mocks --dir .

// ExecutionHandler is the interface for executing verified transactions.
// This avoids import cycles between governance and pubsub packages.
type ExecutionHandler interface {
	ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error)
}

//go:generate mockery --name TransactionAuditStore --output ./mocks --dir .

type TransactionAuditStore interface {
	DocSet(collection, id string, data json.RawMessage) error
}

// L5Actuator is the execution gateway. It is the final stop for all GovernanceEnvelope envelopes.
type L5Actuator struct {
	Logger            *slog.Logger
	SignerStore       SignerStore
	Execution         *execution.ExecutionService
	SQLAuditStore     *storage.SQLAuditStore
	AuditStore        TransactionAuditStore
	L3Notary          L3Notary
	StateRootProvider StateRootProvider
	Ctx               context.Context
	ExecutionHandler  ExecutionHandler
	Scrubbing         *scrubbing.ScrubbingService
	Posture           GovernancePosture

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
func (w *L5Actuator) Execute(ctx context.Context, vt *VerifiedTransaction, cmdMsg interface{}) (*operatorv1.ActionReceipt, error) {
	w.wg.Add(1)
	defer w.wg.Done()

	if w.ExecutionHandler == nil {
		return nil, errors.New("L5Actuator ExecutionHandler not set")
	}
	if len(w.SigningKey) == 0 {
		return nil, errors.New("L5Actuator signing key missing - cannot execute mutations")
	}

	stateBefore := ""
	if w.StateRootProvider != nil {
		var err error
		stateBefore, err = w.StateRootProvider.GetCurrentStateRoot()
		if err != nil {
			w.Logger.Warn("Failed to get state root before execution", string(constants.ConnectionStateError), err)
		}
	}

	// Map action type to event type for handler lookup
	eventType := constants.MapActionTypeToEventType(vt.ActionType)

	w.Logger.Info("L5Actuator preparing to execute transaction",
		"message_id", vt.Envelope.Id,
		"action_type", vt.ActionType,
		"event_type", eventType)

	// 1. Prepare initial receipt
	// Receipt is signed by the L5Actuator (Authorized Dispatch) identity.
	gatewaySigned := false
	if vt.Envelope.Governance != nil {
		gatewaySigned = vt.Envelope.Governance.GatewaySigned
	}

	// Determine L2 status based on posture and verification result
	l2Status := operatorv1.L2Status_L2_STATUS_NOT_REQUIRED
	if vt.Posture != nil && vt.Posture.RequiresL2Signature() {
		if vt.L2Valid {
			l2Status = operatorv1.L2Status_L2_STATUS_REQUIRED_VALID
		} else {
			l2Status = operatorv1.L2Status_L2_STATUS_REQUIRED_FAILED
		}
	}

	// Determine L3 status based on posture and verification result
	l3Status := operatorv1.L3Status_L3_STATUS_NOT_REQUIRED
	if vt.Posture != nil && vt.Posture.RequiresL3Proof() {
		if vt.L3Valid {
			l3Status = operatorv1.L3Status_L3_STATUS_REQUIRED_VALID
		} else {
			l3Status = operatorv1.L3Status_L3_STATUS_REQUIRED_FAILED
		}
	}

	receipt := &operatorv1.ActionReceipt{
		TransactionId:    vt.Envelope.Id,
		TransactionHash:  vt.Envelope.TransactionHash,
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		ResultSummary:    "executing",
		StateRootBefore:  stateBefore,
		ExecutedAtUnixMs: time.Now().UnixMilli(),
		SignerKeyId:      w.KeyID,
		GatewaySigned:    gatewaySigned,
		L2Status:         l2Status,
		L3Status:         l3Status,
	}

	// 2. Sign the initial receipt (intent to execute)
	sig, signErr := w.signReceipt(receipt)
	if signErr != nil {
		w.Logger.Error("Fail-closed: Failed to sign initial action receipt", string(constants.ConnectionStateError), signErr, "message_id", vt.Envelope.Id)
		return nil, fmt.Errorf("failed to sign initial action receipt: %w", signErr)
	}
	receipt.Signature = sig

	// 3. Log intent to execute (Audit before execution)
	if err := w.LogReceipt(vt.Envelope, receipt); err != nil {
		w.Logger.Error("Fail-closed: Failed to log initial action receipt", string(constants.ConnectionStateError), err, "message_id", vt.Envelope.Id)
		return nil, fmt.Errorf("failed to log initial action receipt: %w", err)
	}

	// 3.5. Rehydrate payload if Scrubbing is available
	if w.Scrubbing != nil && cmdMsg != nil {
		if rehydratable, ok := cmdMsg.(interface {
			GetPayload() []byte
			SetPayload([]byte)
		}); ok {
			p := rehydratable.GetPayload()
			if len(p) > 0 {
				rehydrated, rehydrateErr := w.Scrubbing.RehydratePayload(p)
				if rehydrateErr == nil {
					rehydratable.SetPayload(rehydrated)
				} else {
					w.Logger.Warn("Failed to rehydrate payload", string(constants.ConnectionStateError), rehydrateErr, "message_id", vt.Envelope.Id)
				}
			}
		}
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
			w.Logger.Warn("Failed to get state root after execution", string(constants.ConnectionStateError), stateErr)
		}
	}

	receipt.Status = status
	receipt.ResultSummary = summary
	receipt.StateRootAfter = stateAfter
	receipt.ExecutedAtUnixMs = time.Now().UnixMilli()

	// 6. Sign the final receipt
	finalSig, signErr := w.signReceipt(receipt)
	if signErr != nil {
		w.Logger.Error("Failed to sign final action receipt - returning EXECUTING receipt as evidence", string(constants.ConnectionStateError), signErr, "message_id", vt.Envelope.Id)
		// Return the EXECUTING receipt with signature from step 2 as evidence
		// The mutation already executed, so we must preserve evidence of execution attempt
		return receipt, fmt.Errorf("execution completed but final receipt signing failed: %w", signErr)
	}
	receipt.Signature = finalSig

	// 7. Log final result (best-effort - mutation already executed)
	if logErr := w.LogReceipt(vt.Envelope, receipt); logErr != nil {
		w.Logger.Error("Failed to log final action receipt - mutation already executed", string(constants.ConnectionStateError), logErr, "message_id", vt.Envelope.Id)
		// Return receipt anyway - mutation already happened, evidence must be preserved
		return receipt, fmt.Errorf("execution completed but final audit logging failed: %w", logErr)
	}

	return receipt, err
}

// CanonicalizeActionReceipt produces a deterministic byte representation for signing/verification.
// This function must be used by both signing and verification to ensure consistency.
// Field order: transaction_id, transaction_hash, status, result_summary, state_root_before,
// state_root_after, executed_at_unix_ms, signer_key_id, gateway_signed, l2_status, l3_status.
// All fields are included in the canonical form.
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
		"gateway_signed":      r.GatewaySigned,
		"l2_status":           int32(r.L2Status),
		"l3_status":           int32(r.L3Status),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal receipt for canonicalization: %w", err)
	}
	return payload, nil
}

func (w *L5Actuator) signReceipt(r *operatorv1.ActionReceipt) (string, error) {
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
func (w *L5Actuator) LogReceipt(env *governance.GovernanceEnvelope, r *operatorv1.ActionReceipt) error {
	docErr := w.logReceiptDocument(env, r)

	if w.SQLAuditStore == nil {
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
		GatewaySigned:     r.GatewaySigned,
		L2Valid:           r.L2Status == operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
		L3Valid:           r.L3Status == operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
		Timestamp:         time.Now().UTC(),
	}

	if err := w.SQLAuditStore.RecordActionReceipt(&record); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record ActionReceipt in audit vault", string(constants.ConnectionStateError), err)
		}
		if docErr != nil {
			return fmt.Errorf("audit vault error: %v, doc store error: %v", err, docErr)
		}
		return err
	}

	return docErr
}

func (w *L5Actuator) logReceiptDocument(env *governance.GovernanceEnvelope, r *operatorv1.ActionReceipt) error {
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
		GatewaySigned:     r.GatewaySigned,
		L2Valid:           r.L2Status == operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
		L3Valid:           r.L3Status == operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
		Timestamp:         time.Now().UTC(),
	}

	body, err := json.Marshal(record)
	if err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to marshal action receipt record", string(constants.ConnectionStateError), err, "message_id", r.TransactionId)
		}
		return err
	}

	if err := w.AuditStore.DocSet(marshaler.CollectionName(constants.CollectionConsoleAudit), r.TransactionId, body); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record action receipt document", string(constants.ConnectionStateError), err, "message_id", r.TransactionId)
		}
		return err
	}
	return nil
}

// Wait blocks until all in-flight transactions have finished executing.
func (w *L5Actuator) Wait() {
	w.wg.Wait()
}
