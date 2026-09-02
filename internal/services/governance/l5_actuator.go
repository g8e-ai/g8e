// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

//go:generate mockery --name ExecutionHandler --output ./mocks --dir .

// ExecutionHandler is the interface for executing verified transactions.
// This avoids import cycles between governance and pubsub packages.
type ExecutionHandler interface {
	ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg CommandMessage) (string, error)
}

// ReceiptPublisher publishes a signed ActionReceipt (wrapped in a
// GovernanceEnvelope carrying the original command's identity fields) to the
// gateway's receipts: channel after execution. The gateway intercepts the
// publish, verifies the receipt signature against the operator's actuator
// public key, records the receipt in its SQLAuditStore, and fans it out to
// subscribers. Implemented by pubsub.PubSubResultsService on the operator
// side; nil on the gateway in-process operator path (in-process receipts are
// already recorded in the gateway's SQLAuditStore directly by L5Actuator).
// Publish errors are best-effort: the receipt is already recorded locally
// and the result already published, so the gateway-side mirror is not a gate.
type ReceiptPublisher interface {
	PublishActionReceipt(ctx context.Context, env *govtypes.GovernanceEnvelope, receipt *operatorv1.ActionReceipt) error
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

	// ReceiptPublisher publishes the signed ActionReceipt to the gateway's
	// receipts: channel after execution so the gateway can record it in its
	// SQLAuditStore. Nil on the gateway in-process operator path (receipts
	// are already recorded locally). Best-effort: publish errors are logged
	// and do not fail the execution.
	ReceiptPublisher ReceiptPublisher

	// L5Actuator's own signing identity for ActionReceipts
	SigningKey        ed25519.PrivateKey
	KeyID             string
	AuditorSigningKey ed25519.PrivateKey
	AuditorKeyID      string

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
	if w.SQLAuditStore != nil {
		if len(w.AuditorSigningKey) == 0 {
			return nil, constants.ErrL5ActuatorAuditorKeyMissing
		}
		if w.AuditorKeyID == "" {
			return nil, constants.ErrL5ActuatorAuditorKeyIDMissing
		}
		if w.SQLAuditStore.CommitmentLedger() == nil {
			return nil, constants.ErrL5ActuatorCommitmentLedger
		}
	}

	eventType := constants.MapActionTypeToEventType(vt.ActionType)

	w.Logger.Info("L5Actuator preparing to execute transaction",
		"message_id", vt.Envelope.Id,
		"action_type", vt.ActionType,
		"event_type", eventType)

	receipt := w.buildInitialReceipt(vt)

	receiptPersistenceStart := governanceMonotonicNow()
	if err := w.signAndLogReceipt(vt, receipt); err != nil {
		return nil, err
	}
	receiptPersistenceStage := newDeterministicStageEvidence(
		vt.Envelope,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
		receiptPersistenceStart,
		operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
	)
	receiptPersistenceStage.SignerKeyId = receipt.SignerKeyId
	receiptPersistenceStage.ReceiptSignatureDigest = signatureDigest([]string{receipt.Signature})
	receiptPersistenceStage.AuditRecordId = receipt.TransactionId
	receipt.DeterministicStageEvidence = append(receipt.DeterministicStageEvidence, receiptPersistenceStage)
	if w.SQLAuditStore != nil {
		commitmentStart := governanceMonotonicNow()
		attestation, err := w.persistCommitment(vt, receipt)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrL5ActuatorCommitmentPersist, err)
		}
		commitmentStage := newDeterministicStageEvidence(
			vt.Envelope,
			operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
			commitmentStart,
			operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
		)
		commitmentStage.SignerKeyId = attestation.AuditorKeyId
		commitmentStage.CommitmentHash = attestation.Hash
		commitmentStage.PriorCommitmentHash = attestation.PriorCommitmentHash
		commitmentStage.L2SignatureDigest = attestation.L2SignatureDigest
		commitmentStage.L3SignatureDigest = attestation.HumanSignatureDigest
		receipt.DeterministicStageEvidence = append(receipt.DeterministicStageEvidence, commitmentStage)
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

	executionStart := governanceMonotonicNow()
	summary, execErr := w.ExecutionHandler.ExecuteVerifiedTransaction(execCtx, eventType, cmdMsg)
	executionOutcome := operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED
	if execErr != nil {
		executionOutcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
	}
	executionStage := newDeterministicStageEvidence(
		vt.Envelope,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
		executionStart,
		executionOutcome,
	)
	executionStage.StateRootBefore = receipt.StateRootBefore
	for _, stage := range receipt.DeterministicStageEvidence {
		if stage == nil {
			continue
		}
		switch stage.Kind {
		case operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
			operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
			operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND:
			stage.ParentStageId = executionStage.StageId
		}
	}
	receipt.DeterministicStageEvidence = append(receipt.DeterministicStageEvidence, executionStage)

	cap.Dissolve()
	w.Logger.Info("Dissolved JIT capability", "message_id", vt.Envelope.Id, "action_type", vt.ActionType)

	w.finalizeReceipt(receipt, summary, execErr)
	executionStage.StateRootAfter = receipt.StateRootAfter

	if err := w.signAndLogFinalReceipt(vt, receipt); err != nil {
		return receipt, err
	}

	// Publish the signed ActionReceipt to the gateway's receipts: channel
	// so the gateway can record it in its SQLAuditStore. Best-effort: the
	// receipt is already recorded locally and the result already published;
	// the gateway-side mirror is not a gate. Only attempted when a
	// ReceiptPublisher is wired (outbound operator path).
	if w.ReceiptPublisher != nil {
		if pubErr := w.ReceiptPublisher.PublishActionReceipt(ctx, vt.Envelope, receipt); pubErr != nil {
			w.Logger.Warn("Failed to publish ActionReceipt to gateway receipts channel",
				string(constants.ConnectionStateError), pubErr,
				"message_id", vt.Envelope.Id)
		}
	}

	return receipt, execErr
}

func (w *L5Actuator) RecordRejectedTransaction(ctx context.Context, vt *VerifiedTransaction, rejection error) (*operatorv1.ActionReceipt, error) {
	if vt == nil || vt.Envelope == nil {
		return nil, constants.ErrTxInvalidEnvelope
	}
	if rejection == nil {
		return nil, constants.ErrTxInvalidEnvelope
	}
	if len(w.SigningKey) == 0 {
		return nil, constants.ErrL5ActuatorSigningKeyMissing
	}

	receipt := w.buildInitialReceipt(vt)
	receipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	receipt.ResultSummary = fmt.Sprintf("rejected: %v", rejection)
	receipt.ExecutedAtUnixMs = time.Now().UnixMilli()
	if err := w.signAndLogFinalReceipt(vt, receipt); err != nil {
		return nil, err
	}
	if w.ReceiptPublisher != nil {
		if err := w.ReceiptPublisher.PublishActionReceipt(ctx, vt.Envelope, receipt); err != nil {
			w.Logger.Warn("Failed to publish rejected ActionReceipt to gateway receipts channel",
				string(constants.ConnectionStateError), err,
				"message_id", vt.Envelope.Id)
		}
	}
	return receipt, nil
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

	stages := make([]*operatorv1.DeterministicStageEvidence, len(vt.DeterministicStageEvidence))
	for index, stage := range vt.DeterministicStageEvidence {
		if stage == nil {
			continue
		}
		stages[index] = &operatorv1.DeterministicStageEvidence{}
		proto.Merge(stages[index], stage)
	}

	return &operatorv1.ActionReceipt{
		TransactionId:              vt.Envelope.Id,
		TransactionHash:            vt.Envelope.TransactionHash,
		Status:                     operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		ResultSummary:              "executing",
		StateRootBefore:            stateBefore,
		ExecutedAtUnixMs:           time.Now().UnixMilli(),
		SignerKeyId:                w.KeyID,
		L2Status:                   l2Status,
		L3Status:                   l3Status,
		DeterministicStageEvidence: stages,
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

func CanonicalizeCommitmentAttestation(attestation *operatorv1.CommitmentAttestation) ([]byte, error) {
	if attestation == nil {
		return nil, constants.ErrTxInvalidEnvelope
	}
	var payload bytes.Buffer
	for _, value := range []string{
		attestation.TransactionId,
		attestation.TransactionHash,
		attestation.PriorCommitmentHash,
		attestation.StateRootAtCommit,
		attestation.L2SignatureDigest,
		attestation.WardenIntentSignatureDigest,
		attestation.HumanSignatureDigest,
		attestation.ActionType,
		attestation.TargetResource,
	} {
		writeCanonicalString(&payload, value)
	}
	var timestamp [8]byte
	binary.BigEndian.PutUint64(timestamp[:], uint64(attestation.CommittedAtUnixMs))
	payload.Write(timestamp[:])
	writeCanonicalString(&payload, attestation.AuditorKeyId)
	return payload.Bytes(), nil
}

func writeCanonicalString(payload *bytes.Buffer, value string) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	payload.Write(length[:])
	payload.WriteString(value)
}

func (w *L5Actuator) persistCommitment(vt *VerifiedTransaction, receipt *operatorv1.ActionReceipt) (*operatorv1.CommitmentAttestation, error) {
	if len(w.AuditorSigningKey) != ed25519.PrivateKeySize {
		return nil, constants.ErrL5ActuatorAuditorKeyMissing
	}
	publicKey := w.AuditorSigningKey.Public().(ed25519.PublicKey)
	if w.AuditorKeyID != hex.EncodeToString(publicKey) {
		return nil, constants.ErrValidationFailed
	}
	var persisted *operatorv1.CommitmentAttestation
	err := w.SQLAuditStore.CommitmentLedger().AppendCommitment(func(priorHash string) ([]byte, string, error) {
		attestation := &operatorv1.CommitmentAttestation{
			TransactionId:               vt.Envelope.Id,
			TransactionHash:             vt.Envelope.TransactionHash,
			PriorCommitmentHash:         priorHash,
			StateRootAtCommit:           receipt.StateRootBefore,
			L2SignatureDigest:           l2SignatureDigest(vt.Envelope),
			WardenIntentSignatureDigest: signatureDigest([]string{receipt.Signature}),
			HumanSignatureDigest:        humanSignatureDigest(vt.Envelope),
			ActionType:                  string(vt.ActionType),
			TargetResource:              vt.Envelope.TargetResource,
			CommittedAtUnixMs:           time.Now().UnixMilli(),
			AuditorKeyId:                w.AuditorKeyID,
		}
		canonical, err := CanonicalizeCommitmentAttestation(attestation)
		if err != nil {
			return nil, "", err
		}
		hash := sha256.Sum256(canonical)
		attestation.Hash = hex.EncodeToString(hash[:])
		attestation.Signature = hex.EncodeToString(ed25519.Sign(w.AuditorSigningKey, canonical))
		payload, err := json.Marshal(attestation)
		if err != nil {
			return nil, "", fmt.Errorf("commitment attestation: marshal: %w", err)
		}
		persisted = attestation
		return payload, attestation.Hash, nil
	})
	if err != nil {
		return nil, err
	}
	return persisted, nil
}

func l2SignatureDigest(env *govtypes.GovernanceEnvelope) string {
	if env == nil || env.Governance == nil || env.Governance.L2 == nil {
		return ""
	}
	signatures := make([]string, 0, len(env.Governance.L2.Votes))
	for _, vote := range env.Governance.L2.Votes {
		if vote != nil {
			signatures = append(signatures, vote.ConsensusSignature)
		}
	}
	return signatureDigest(signatures)
}

func humanSignatureDigest(env *govtypes.GovernanceEnvelope) string {
	if env == nil || env.Governance == nil || env.Governance.L3 == nil || env.Governance.L3.Proof == nil {
		return ""
	}
	proof := env.Governance.L3.Proof
	return signatureDigest([]string{proof.Signature, proof.CliSignature})
}

func signatureDigest(signatures []string) string {
	values := make([]string, 0, len(signatures))
	for _, signature := range signatures {
		if signature != "" {
			values = append(values, signature)
		}
	}
	if len(values) == 0 {
		return ""
	}
	sort.Strings(values)
	var payload bytes.Buffer
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], uint32(len(values)))
	payload.Write(count[:])
	for _, value := range values {
		writeCanonicalString(&payload, value)
	}
	digest := sha256.Sum256(payload.Bytes())
	return hex.EncodeToString(digest[:])
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
	attestation, err := w.signReceiptPersistenceAttestation(receipt)
	if err != nil {
		return err
	}
	receipt.FinalPersistenceAttestation = attestation
	if logErr := w.LogReceipt(vt.Envelope, receipt); logErr != nil {
		w.Logger.Error("Failed to log final receipt persistence attestation", string(constants.ConnectionStateError), logErr, "message_id", vt.Envelope.Id)
		return fmt.Errorf("%w: %w", constants.ErrL5ActuatorLogReceipt, logErr)
	}
	return nil
}

func CanonicalizeReceiptPersistenceAttestation(attestation *operatorv1.ReceiptPersistenceAttestation) ([]byte, error) {
	if attestation == nil {
		return nil, constants.ErrReceiptPersistenceAttestationMissing
	}
	var payload bytes.Buffer
	writeCanonicalString(&payload, attestation.TransactionId)
	writeCanonicalString(&payload, attestation.ReceiptSignatureDigest)
	var persistedAt [8]byte
	binary.BigEndian.PutUint64(persistedAt[:], uint64(attestation.PersistedAtUnixMs))
	payload.Write(persistedAt[:])
	writeCanonicalString(&payload, attestation.AuditRecordId)
	writeCanonicalString(&payload, attestation.SignerKeyId)
	return payload.Bytes(), nil
}

func (w *L5Actuator) signReceiptPersistenceAttestation(receipt *operatorv1.ActionReceipt) (*operatorv1.ReceiptPersistenceAttestation, error) {
	attestation := &operatorv1.ReceiptPersistenceAttestation{
		TransactionId:          receipt.TransactionId,
		ReceiptSignatureDigest: signatureDigest([]string{receipt.Signature}),
		PersistedAtUnixMs:      time.Now().UnixMilli(),
		AuditRecordId:          receipt.TransactionId,
		SignerKeyId:            w.KeyID,
	}
	payload, err := CanonicalizeReceiptPersistenceAttestation(attestation)
	if err != nil {
		return nil, fmt.Errorf("%w: persistence attestation: %w", constants.ErrL5ActuatorCanonicalizeReceipt, err)
	}
	attestation.Signature = hex.EncodeToString(ed25519.Sign(w.SigningKey, payload))
	return attestation, nil
}

func VerifyActionReceiptSignature(receipt *operatorv1.ActionReceipt, publicKey ed25519.PublicKey) error {
	if receipt == nil {
		return constants.ErrActionReceiptMissing
	}
	if receipt.Signature == "" || len(publicKey) != ed25519.PublicKeySize {
		return constants.ErrActionReceiptSignatureInvalid
	}
	signature, err := hex.DecodeString(receipt.Signature)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrActionReceiptSignatureInvalid, err)
	}
	payload, err := CanonicalizeActionReceipt(receipt)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrActionReceiptSignatureInvalid, err)
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return constants.ErrActionReceiptSignatureInvalid
	}
	return nil
}

func VerifyReceiptPersistenceAttestation(receipt *operatorv1.ActionReceipt, publicKey ed25519.PublicKey) error {
	if receipt == nil || receipt.FinalPersistenceAttestation == nil {
		return constants.ErrReceiptPersistenceAttestationMissing
	}
	attestation := receipt.FinalPersistenceAttestation
	if receipt.TransactionId == "" ||
		attestation.TransactionId != receipt.TransactionId ||
		attestation.AuditRecordId != receipt.TransactionId ||
		receipt.SignerKeyId == "" ||
		attestation.SignerKeyId != receipt.SignerKeyId ||
		attestation.PersistedAtUnixMs <= 0 {
		return constants.ErrReceiptPersistenceAttestationInvalid
	}
	if receipt.Signature == "" || attestation.ReceiptSignatureDigest != signatureDigest([]string{receipt.Signature}) {
		return constants.ErrReceiptPersistenceSignatureMismatch
	}
	signature, err := hex.DecodeString(attestation.Signature)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrReceiptPersistenceAttestationInvalid, err)
	}
	payload, err := CanonicalizeReceiptPersistenceAttestation(attestation)
	if err != nil {
		return err
	}
	if len(publicKey) != ed25519.PublicKeySize || !ed25519.Verify(publicKey, payload, signature) {
		return constants.ErrReceiptPersistenceAttestationInvalid
	}
	return nil
}

// canonicalReceipt is the typed representation for ActionReceipt canonicalization.
// This ensures strict typing and deterministic JSON marshaling for signing/verification.
type canonicalReceipt struct {
	TransactionID                  string `json:"transaction_id"`
	TransactionHash                string `json:"transaction_hash"`
	Status                         int32  `json:"status"`
	ResultSummary                  string `json:"result_summary"`
	StateRootBefore                string `json:"state_root_before"`
	StateRootAfter                 string `json:"state_root_after"`
	ExecutedAtUnixMs               int64  `json:"executed_at_unix_ms"`
	SignerKeyID                    string `json:"signer_key_id"`
	L2Status                       int32  `json:"l2_status"`
	L3Status                       int32  `json:"l3_status"`
	DeterministicStageEvidenceHash string `json:"deterministic_stage_evidence_hash,omitempty"`
}

func deterministicStageEvidenceHash(stages []*operatorv1.DeterministicStageEvidence) (string, error) {
	if len(stages) == 0 {
		return "", nil
	}
	var payload bytes.Buffer
	for _, stage := range stages {
		encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(stage)
		if err != nil {
			return "", err
		}
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(encoded)))
		payload.Write(length[:])
		payload.Write(encoded)
	}
	digest := sha256.Sum256(payload.Bytes())
	return hex.EncodeToString(digest[:]), nil
}

// CanonicalizeActionReceipt produces a deterministic byte representation for signing/verification.
// This function must be used by both signing and verification to ensure consistency.
// Field order: transaction_id, transaction_hash, status, result_summary, state_root_before,
// state_root_after, executed_at_unix_ms, signer_key_id, l2_status, l3_status, and the
// deterministic stage evidence hash when stage evidence is present.
func CanonicalizeActionReceipt(r *operatorv1.ActionReceipt) ([]byte, error) {
	stageEvidenceHash, err := deterministicStageEvidenceHash(r.DeterministicStageEvidence)
	if err != nil {
		return nil, fmt.Errorf("%w: deterministic stage evidence: %w", constants.ErrL5ActuatorCanonicalizeReceipt, err)
	}
	canonical := canonicalReceipt{
		TransactionID:                  r.TransactionId,
		TransactionHash:                r.TransactionHash,
		Status:                         int32(r.Status),
		ResultSummary:                  r.ResultSummary,
		StateRootBefore:                r.StateRootBefore,
		StateRootAfter:                 r.StateRootAfter,
		ExecutedAtUnixMs:               r.ExecutedAtUnixMs,
		SignerKeyID:                    r.SignerKeyId,
		L2Status:                       int32(r.L2Status),
		L3Status:                       int32(r.L3Status),
		DeterministicStageEvidenceHash: stageEvidenceHash,
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

	record := BuildReceiptRecord(env, r)

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

	record := BuildReceiptRecord(env, r)

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
func BuildReceiptRecord(env *govtypes.GovernanceEnvelope, r *operatorv1.ActionReceipt) *models.ActionReceiptRecord {
	return &models.ActionReceiptRecord{
		TransactionID:     r.TransactionId,
		TransactionHash:   r.TransactionHash,
		InvestigationID:   env.InvestigationId,
		OperatorID:        env.OperatorId,
		OperatorSessionID: env.OperatorSessionId,
		RequestorUserID:   env.RequestorUserId,
		ActingAppID:       env.ActingAppId,
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
		ActionReceipt:     proto.Clone(r).(*operatorv1.ActionReceipt),
	}
}

// Wait blocks until all in-flight transactions have finished executing.
func (w *L5Actuator) Wait() {
	w.wg.Wait()
}
