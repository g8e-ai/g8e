// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/internal/constants"
	govtypes "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/services/system"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// VerifiedTransaction represents a fully verified transaction ready for execution.
type VerifiedTransaction struct {
	Envelope       *govtypes.GovernanceEnvelope
	ActionType     constants.ActionType
	Payload        []byte
	DecodedPayload proto.Message
	StateRoot      string
	Nonce          string
	ExpiresAt      time.Time
	L2Valid        bool // Whether L2 signature was valid (may be false in Doctrine posture)
	L3Valid        bool // Whether L3 proof was valid (may be false in Doctrine/Consensus posture)
	Posture        GovernancePosture
}

// L4Warden performs all pre-dispatch verification checks.
type L4Warden struct {
	logger               *slog.Logger
	replayStore          ReplayStore
	stateRootProvider    StateRootProvider
	signerStore          SignerStore
	consensusPolicyStore L2ConsensusPolicyStore
	l3Notary             L3Notary
	doctrine             *L1Doctrine
	knownActionTypes     map[constants.ActionType]struct{}
	posture              GovernancePosture // Governance posture: doctrine, consensus, or notary
	clock                system.Clock      // Injectable time source for deterministic testing

	inFlight sync.Map // Concurrent-safe tracking of in-flight nonces
}

// NewL4Warden creates a new L4 Warden.
func NewL4Warden(
	logger *slog.Logger,
	replayStore ReplayStore,
	stateRootProvider StateRootProvider,
	signerStore SignerStore,
	consensusPolicyStore L2ConsensusPolicyStore,
	l3Notary L3Notary,
	doctrine *L1Doctrine,
	knownActionTypes []constants.ActionType,
	posture string,
	clock system.Clock,
) *L4Warden {
	knownActions := make(map[constants.ActionType]struct{})
	for _, action := range knownActionTypes {
		knownActions[action] = struct{}{}
	}

	// Default to real clock if not provided
	if clock == nil {
		clock = &system.RealClock{}
	}

	return &L4Warden{
		logger:               logger,
		replayStore:          replayStore,
		stateRootProvider:    stateRootProvider,
		signerStore:          signerStore,
		consensusPolicyStore: consensusPolicyStore,
		l3Notary:             l3Notary,
		doctrine:             doctrine,
		knownActionTypes:     knownActions,
		posture:              NewGovernancePosture(posture),
		clock:                clock,
	}
}

// VerifyEnvelope performs all required verification checks on a decoded GovernanceEnvelope JSON GovernanceEnvelope.
// It is decomposed into three discrete validation stages:
// 1. Stateless: Basic structural, hash, and L1 Doctrine checks that don't require external state.
// 2. Stateful: Checks requiring external state (expiry, state root, and early nonce reservation).
// 3. Posture: Governance posture-aware checks (L2 Consensus and L3 Notary proofs).
func (tv *L4Warden) VerifyEnvelope(ctx context.Context, envelope *govtypes.GovernanceEnvelope) (*VerifiedTransaction, error) {
	if envelope == nil {
		return nil, constants.ErrTxInvalidEnvelope
	}

	// 0. Early trackInFlight check to save expensive DB/cryptography operations.
	// The critical section must extend through nonce reservation to prevent race conditions.
	if err := tv.trackInFlight(envelope.Nonce); err != nil {
		return nil, err
	}

	// 1. Early nonce reservation for durable replay protection.
	// This prevents replay attacks if the Operator crashes mid-execution.
	// The nonce is reserved early and finalized after successful execution.
	if tv.replayStore == nil {
		tv.releaseInFlight(envelope.Nonce)
		return nil, constants.ErrTxReplayStoreMissing
	}
	if envelope.ExpiresAt == nil {
		tv.releaseInFlight(envelope.Nonce)
		return nil, constants.ErrTxExpiresAtMissing
	}
	expiresAt := envelope.ExpiresAt.AsTime()
	if tv.clock.Now().After(expiresAt) {
		tv.logger.Error("Transaction rejected: EXPIRED",
			"nonce", envelope.Nonce,
			"expires_at", expiresAt,
			"now", tv.clock.Now())
		tv.releaseInFlight(envelope.Nonce)
		return nil, constants.ErrTxTransactionExpired
	}
	if envelope.Nonce == "" {
		tv.logger.Error("Transaction rejected: NONCE_MISSING")
		tv.releaseInFlight(envelope.Nonce)
		return nil, constants.ErrTxNonceMissing
	}
	replayed, err := tv.replayStore.ReserveNonce(envelope.Nonce, expiresAt)
	if err != nil {
		tv.logger.Error("Transaction rejected: REPLAY_CHECK_FAILED",
			"nonce", envelope.Nonce,
			string(constants.ConnectionStateError), err)
		tv.releaseInFlight(envelope.Nonce)
		return nil, fmt.Errorf("l4 warden: reserve nonce: %w", err)
	}
	if replayed {
		tv.logger.Error("Transaction rejected: REPLAY_DETECTED", "nonce", envelope.Nonce)
		tv.releaseInFlight(envelope.Nonce)
		return nil, constants.ErrTxTransactionReplay
	}

	// Nonce is now durably reserved in SQLite, safe to release in-flight lock
	tv.releaseInFlight(envelope.Nonce)

	// 2. Stateless Validation
	decodedPayload, computedHash, err := tv.verifyStateless(envelope)
	if err != nil {
		tv.logger.Error("Transaction rejected: STATELESS_VALIDATION_FAILED",
			"nonce", envelope.Nonce,
			"action_type", envelope.ActionType,
			string(constants.ConnectionStateError), err)
		// Release nonce reservation on stateless validation failure
		tv.releaseNonceReservation(envelope.Nonce)
		return nil, err
	}

	// 3. Stateful Validation (excluding nonce, which is already reserved)
	err = tv.verifyStateful(envelope)
	if err != nil {
		tv.logger.Error("Transaction rejected: STATEFUL_VALIDATION_FAILED",
			"nonce", envelope.Nonce,
			"tx_id", envelope.Id,
			string(constants.ConnectionStateError), err)
		// Release nonce reservation on stateful validation failure
		tv.releaseNonceReservation(envelope.Nonce)
		return nil, err
	}

	// 4. Posture Validation (L2/L3)
	l2Valid, l3Valid, err := tv.verifyPosture(ctx, envelope, computedHash)
	if err != nil {
		tv.logger.Error("Transaction rejected: POSTURE_VALIDATION_FAILED",
			"nonce", envelope.Nonce,
			"tx_id", envelope.Id,
			"posture", tv.posture.Name(),
			string(constants.ConnectionStateError), err)
		// Release nonce reservation on posture validation failure
		tv.releaseNonceReservation(envelope.Nonce)
		return nil, err
	}

	// 5. Return verified transaction (nonce remains reserved, will be finalized after execution)
	return &VerifiedTransaction{
		Envelope:       envelope,
		ActionType:     constants.ActionType(envelope.ActionType),
		Payload:        envelope.Payload,
		DecodedPayload: decodedPayload,
		StateRoot:      envelope.StateMerkleRoot,
		Nonce:          envelope.Nonce,
		ExpiresAt:      expiresAt,
		L2Valid:        l2Valid,
		L3Valid:        l3Valid,
		Posture:        tv.posture,
	}, nil
}

func (tv *L4Warden) trackInFlight(nonce string) error {
	if nonce == "" {
		return nil
	}
	_, loaded := tv.inFlight.LoadOrStore(nonce, true)
	if loaded {
		tv.logger.Warn("Transaction with same nonce already in-flight", "nonce", nonce)
		return constants.ErrTxInFlight
	}
	return nil
}

func (tv *L4Warden) releaseInFlight(nonce string) {
	tv.inFlight.Delete(nonce)
}

func (tv *L4Warden) releaseNonceReservation(nonce string) {
	if err := tv.replayStore.ReleaseNonce(nonce); err != nil {
		tv.logger.Warn("Failed to release nonce reservation",
			"nonce", nonce,
			string(constants.ConnectionStateError), err)
	}
}

// isMutation returns true if the action type modifies system state.
// Uses the strongly-typed intrinsic property from the action definition.
// Mutation classification is defined in protocol/constants/status.json via the _mutation field.
// Actions marked as mutations require L3 Notary (human-presence) verification.
func (tv *L4Warden) isMutation(actionType constants.ActionType) bool {
	return actionType.IsMutation()
}

// verifyStateless performs basic structural, hash, and L1 Doctrine checks.
func (tv *L4Warden) verifyStateless(envelope *govtypes.GovernanceEnvelope) (proto.Message, string, error) {
	if tv.doctrine == nil {
		tv.logger.Error("L1Doctrine not configured")
		return nil, "", constants.ErrTxDoctrineMissing
	}

	if envelope.Id == "" {
		return nil, "", constants.ErrTxTransactionIDMissing
	}

	actionType := constants.ActionType(envelope.ActionType)
	if _, ok := tv.knownActionTypes[actionType]; !ok {
		tv.logger.Error("Unknown action type", "action_type", envelope.ActionType)
		return nil, "", constants.ErrTxUnknownActionType
	}

	// ActionTypeHeartbeat uses HeartbeatRequested{} which has no fields and marshals
	// to zero bytes — this is a valid empty proto, not a missing payload.
	if len(envelope.Payload) == 0 && actionType != constants.ActionTypeHeartbeat {
		return nil, "", constants.ErrTxPayloadMissing
	}

	decodedPayload, err := tv.decodePayloadForAction(actionType, envelope.Payload)
	if err != nil {
		tv.logger.Error("Failed to decode typed payload", "action_type", envelope.ActionType, string(constants.ConnectionStateError), err)
		return nil, "", constants.ErrTxPayloadDecodeFailed
	}

	// INVESTIGATION_CREATE has no typed payload (returns nil), skip L1 validation
	if decodedPayload != nil {
		if violations := tv.doctrine.ValidatePayload(decodedPayload); len(violations) > 0 {
			tv.logger.Error("Doctrine (L1Doctrine) validation failed", "action_type", envelope.ActionType, "violations", violations)
			return nil, "", fmt.Errorf("%w: %s", constants.ErrTxL1ValidationFailed, strings.Join(violations, ", "))
		}
	}

	computedHash, err := tv.computeTransactionHash(envelope)
	if err != nil {
		return nil, "", fmt.Errorf("l4 warden: compute transaction hash: %w", err)
	}

	if envelope.TransactionHash == "" {
		return nil, "", constants.ErrTxTransactionHashMissing
	}

	if envelope.TransactionHash != computedHash {
		tv.logger.Error("Transaction hash mismatch",
			"provided", envelope.TransactionHash,
			"computed", computedHash)
		return nil, "", constants.ErrTxTransactionHashMismatch
	}

	if envelope.Id != computedHash {
		tv.logger.Error("Transaction id mismatch",
			"provided", envelope.Id,
			"computed", computedHash)
		return nil, "", constants.ErrTxTransactionIDMismatch
	}

	return decodedPayload, computedHash, nil
}

// verifyStateful checks state root. Nonce and expiry are checked earlier in VerifyEnvelope.
func (tv *L4Warden) verifyStateful(envelope *govtypes.GovernanceEnvelope) error {
	if envelope.StateMerkleRoot == "" {
		return constants.ErrTxStateRootRequired
	}

	if tv.stateRootProvider == nil {
		tv.logger.Error("State root verification required but provider not configured")
		return constants.ErrTxStateRootMissing
	}

	currentRoot, err := tv.stateRootProvider.GetCurrentStateRoot()
	if err != nil {
		tv.logger.Error("Failed to get current state root", string(constants.ConnectionStateError), err)
		return fmt.Errorf("l4 warden: get current state root: %w", err)
	}

	if currentRoot == "" {
		return constants.ErrTxStateRootMissing
	}

	if currentRoot != envelope.StateMerkleRoot {
		tv.logger.Error("State root mismatch",
			"envelope_root", envelope.StateMerkleRoot,
			"current_root", currentRoot)
		return constants.ErrTxStateRootMismatch
	}

	return nil
}

// verifyPosture performs governance posture-aware checks for L2 and L3.
// L2 (machine consensus) is verified first. Only if L2 passes does L3
// (human-presence) run. This preserves the architectural invariant: the
// human's approval bond is spent only on transactions that have already
// cleared L2 consensus. A human should never be asked to authorize
// content the machines have not yet vetted.
func (tv *L4Warden) verifyPosture(ctx context.Context, envelope *govtypes.GovernanceEnvelope, computedHash string) (bool, bool, error) {
	l2Valid, err := tv.verifyL2Posture(envelope, computedHash)
	if err != nil {
		return false, false, err
	}

	l3Valid, err := tv.verifyL3Posture(ctx, envelope)
	if err != nil {
		return l2Valid, false, err
	}

	return l2Valid, l3Valid, nil
}

func (tv *L4Warden) verifyL3Posture(ctx context.Context, envelope *govtypes.GovernanceEnvelope) (bool, error) {
	actionType := constants.ActionType(envelope.ActionType)

	hasProof := envelope.Governance != nil && envelope.Governance.L3 != nil && envelope.Governance.L3.Proof != nil

	if !hasProof {
		if tv.isMutation(actionType) && tv.posture.RequiresL3Proof() {
			tv.logger.Error("L3 proof missing but required by posture", "posture", tv.posture.Name())
			return false, constants.ErrTxL3ProofMissing
		}
		return false, nil
	}

	if tv.l3Notary == nil {
		if tv.isMutation(actionType) && tv.posture.RequiresL3Proof() {
			tv.logger.Error("L3 notary not configured but required by posture", "posture", tv.posture.Name())
			return false, constants.ErrTxL3NotaryNotConfigured
		}
		return false, nil
	}

	ok, err := tv.l3Notary.VerifyL3Proof(
		ctx,
		envelope.OperatorId,
		envelope.TransactionHash,
		envelope.CliSessionId,
		envelope.Governance.L3.Proof,
	)

	if (err != nil || !ok) && tv.isMutation(actionType) && tv.posture.RequiresL3Proof() {
		tv.logger.Error("Notary (L3Notary) verification failed but required by posture", string(constants.ConnectionStateError), err)
		return false, constants.ErrTxL3ProofInvalid
	}

	return ok && err == nil, nil
}

func (tv *L4Warden) decodePayloadForAction(actionType constants.ActionType, payload []byte) (proto.Message, error) {
	var msg proto.Message
	switch actionType {
	case constants.ActionTypeExecuteBash:
		msg = &operatorv1.CommandRequested{}
	case constants.ActionTypeFileEdit:
		msg = &operatorv1.FileEditRequested{}
	case constants.ActionTypeRestoreFile:
		msg = &operatorv1.RestoreFileRequested{}
	case constants.ActionTypeShutdown:
		msg = &operatorv1.ShutdownRequested{}
	case constants.ActionTypeFsList:
		msg = &operatorv1.FsListRequested{}
	case constants.ActionTypeFsRead:
		msg = &operatorv1.FsReadRequested{}
	case constants.ActionTypeFsGrep:
		msg = &operatorv1.FsGrepRequested{}
	case constants.ActionTypePortCheck:
		msg = &operatorv1.CheckPortRequested{}
	case constants.ActionTypeFetchLogs:
		msg = &operatorv1.FetchLogsRequested{}
	case constants.ActionTypeFetchHistory:
		msg = &operatorv1.FetchHistoryRequested{}
	case constants.ActionTypeFetchFileHistory:
		msg = &operatorv1.FetchFileHistoryRequested{}
	case constants.ActionTypeEvalAnswer:
		msg = &operatorv1.EvalAnswerRequested{}
	case constants.ActionTypeMcpCall:
		msg = &operatorv1.McpCallRequested{}
	case constants.ActionTypeA2aCall:
		msg = &operatorv1.A2ACallRequested{}
	case constants.ActionTypeMcpResourceRead:
		msg = &operatorv1.McpResourceReadRequested{}
	case constants.ActionTypeMcpPromptGet:
		msg = &operatorv1.McpPromptGetRequested{}
	case constants.ActionTypeFetchFileDiff:
		msg = &operatorv1.FetchFileDiffRequested{}
	case constants.ActionTypeMcpResourceList:
		msg = &operatorv1.McpResourceListRequested{}
	case constants.ActionTypeMcpPromptList:
		msg = &operatorv1.McpPromptListRequested{}
	case constants.ActionTypeHeartbeat:
		msg = &operatorv1.HeartbeatRequested{}
	case constants.ActionTypeCancel:
		msg = &operatorv1.CommandCancelRequested{}
	case constants.ActionTypeInvestigationCreate:
		// No typed payload for investigation create, it uses raw bytes
		return nil, nil

	case constants.ActionTypePlatformEnrollmentCreate,
		constants.ActionTypePlatformEnrollmentDecide,
		constants.ActionTypePlatformEnrollmentIssue,
		constants.ActionTypePlatformEnrollmentPersistPolicy,
		constants.ActionTypePlatformEnrollmentCreateSession:
		// All five platform enrollment actions share the same
		// PlatformEnrollmentGovernancePayload proto. The action field
		// inside the payload distinguishes them; L1 doctrine validates
		// the payload shape and the handler layer enforces per-action
		// semantics. CSR PEM, token hashes, and private keys are never
		// placed in the audited envelope payload.
		msg = &commonv1.PlatformEnrollmentGovernancePayload{}

	default:
		// Known action type without a typed proto decode case (e.g.
		// adapter-specific action types). Treat payload as raw bytes,
		// same as INVESTIGATION_CREATE. Unknown action types are
		// rejected before reaching this function (knownActionTypes check).
		return nil, nil
	}
	if err := proto.Unmarshal(payload, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// computeTransactionHash computes the canonical transaction hash.
func (tv *L4Warden) computeTransactionHash(envelope *govtypes.GovernanceEnvelope) (string, error) {
	return govtypes.GenerateMessageID(envelope)
}

// Posture returns the current governance posture.
func (tv *L4Warden) Posture() GovernancePosture {
	return tv.posture
}

// Doctrine returns the current L1 doctrine validator.
func (tv *L4Warden) Doctrine() *L1Doctrine {
	return tv.doctrine
}
