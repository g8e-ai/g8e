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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/system"
	govtypes "github.com/g8e-ai/g8e/internal/governance"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/proto"
)

//go:generate mockery --name ReplayStore --output ./mocks --dir .

// ReplayStore defines the interface for nonce replay protection.
type ReplayStore interface {
	// ReserveNonce atomically reserves a nonce for early replay protection.
	// Returns true if the nonce was already reserved/used (replay detected).
	// If not used, it reserves the nonce and returns false.
	// This allows early durable commitment before expensive L2/L3 checks.
	ReserveNonce(nonce string, expiresAt time.Time) (bool, error)

	// FinalizeNonce marks a reserved nonce as fully consumed.
	// This is called after successful execution to prevent reuse.
	FinalizeNonce(nonce string) error

	// ReleaseNonce removes a reservation for a failed transaction.
	// This allows the nonce to be reused for retry.
	ReleaseNonce(nonce string) error

	// Close shuts down the replay store and releases resources.
	Close() error
}

//go:generate mockery --name StateRootProvider --output ./mocks --dir .

// StateRootProvider defines the interface for obtaining the current state root.
type StateRootProvider interface {
	GetCurrentStateRoot() (string, error)
}

// SignerStore defines the interface for loading trusted L2 signers.
type SignerStore interface {
	GetTrustedSigner(keyID string) (ed25519.PublicKey, error)
}

// TribunalStore defines the interface for loading TribunalPolicy by ID.
type TribunalStore interface {
	GetTribunal(id string) (*models.TribunalPolicy, error)
}

// AppPolicyStore defines the interface for loading AppPolicies for external apps.
type AppPolicyStore interface {
	GetAppPolicy(appID string) (*models.AppPolicy, error)
}

// SimpleAppPolicyStore implements AppPolicyStore using a static map.
type SimpleAppPolicyStore struct {
	Policies map[string]*models.AppPolicy
}

func (s *SimpleAppPolicyStore) GetAppPolicy(appID string) (*models.AppPolicy, error) {
	if s.Policies == nil {
		return nil, nil
	}
	policy, ok := s.Policies[appID]
	if !ok {
		return nil, nil
	}
	return policy, nil
}

// SimpleSignerStore implements SignerStore using a static map.
type SimpleSignerStore struct {
	Signers map[string]ed25519.PublicKey
}

func (s *SimpleSignerStore) GetTrustedSigner(keyID string) (ed25519.PublicKey, error) {
	if s.Signers == nil {
		return nil, nil
	}
	pubKey, ok := s.Signers[keyID]
	if !ok {
		return nil, nil
	}
	return pubKey, nil
}

// FilesystemSignerStore implements SignerStore by loading public keys from
// .pub files in a directory. Each file's basename is the keyID, and the file
// content is a hex-encoded ED25519 public key.
type FilesystemSignerStore struct {
	signers map[string]ed25519.PublicKey
	logger  *slog.Logger
}

// NewFilesystemSignerStore loads all .pub files from the specified directory.
// The directory must exist and contain .pub files with hex-encoded public keys.
// Returns an error if the directory does not exist or if any file is malformed.
func NewFilesystemSignerStore(dir string, logger *slog.Logger) (*FilesystemSignerStore, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read trusted signers directory %s: %w", dir, err)
	}

	signers := make(map[string]ed25519.PublicKey)
	loadedCount := 0
	failureCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".pub") {
			continue
		}

		keyID := strings.TrimSuffix(entry.Name(), ".pub")
		filePath := filepath.Join(dir, entry.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			if logger != nil {
				logger.Warn("Failed to read trusted signer file", "path", filePath, "error", err)
			}
			failureCount++
			continue
		}

		hexKey := strings.TrimSpace(string(data))
		pubKeyBytes, err := hex.DecodeString(hexKey)
		if err != nil {
			if logger != nil {
				logger.Warn("Failed to decode hex public key", "path", filePath, "error", err)
			}
			failureCount++
			continue
		}

		if len(pubKeyBytes) != ed25519.PublicKeySize {
			if logger != nil {
				logger.Warn("Invalid public key size", "path", filePath, "size", len(pubKeyBytes), "expected", ed25519.PublicKeySize)
			}
			failureCount++
			continue
		}

		signers[keyID] = ed25519.PublicKey(pubKeyBytes)
		loadedCount++
	}

	if logger != nil {
		logger.Info("Loaded trusted signers from filesystem",
			"directory", dir,
			"loaded", loadedCount,
			"failed", failureCount)
	}

	return &FilesystemSignerStore{
		signers: signers,
		logger:  logger,
	}, nil
}

func (s *FilesystemSignerStore) GetTrustedSigner(keyID string) (ed25519.PublicKey, error) {
	if s.signers == nil {
		return nil, nil
	}
	pubKey, ok := s.signers[keyID]
	if !ok {
		return nil, nil
	}
	return pubKey, nil
}

// SimpleTribunalStore implements TribunalStore using a static map.
type SimpleTribunalStore struct {
	Tribunals map[string]*models.TribunalPolicy
}

func (s *SimpleTribunalStore) GetTribunal(id string) (*models.TribunalPolicy, error) {
	if s.Tribunals == nil {
		return nil, nil
	}
	tribunal, ok := s.Tribunals[id]
	if !ok {
		return nil, nil
	}
	return tribunal, nil
}

// SimpleStateRootProvider returns a fixed root set at construction time.
// Root must be non-empty; a missing root is a misconfiguration that returns an
// error so callers fail closed rather than silently accepting any state root.
type SimpleStateRootProvider struct {
	Root string
}

func (s *SimpleStateRootProvider) GetCurrentStateRoot() (string, error) {
	if s.Root == "" {
		return "", constants.ErrTxProviderMisconfigured
	}
	return s.Root, nil
}

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
	logger            *slog.Logger
	replayStore       ReplayStore
	stateRootProvider StateRootProvider
	signerStore       SignerStore
	tribunalStore     TribunalStore
	appPolicyStore    AppPolicyStore
	l3Notary          L3Notary
	doctrine          *L1Doctrine
	knownActionTypes  map[constants.ActionType]struct{}
	posture           GovernancePosture // Governance posture: doctrine, consensus, or notary
	clock             system.Clock      // Injectable time source for deterministic testing

	inFlight sync.Map // Concurrent-safe tracking of in-flight nonces
}

// NewL4Warden creates a new L4 Warden.
func NewL4Warden(
	logger *slog.Logger,
	replayStore ReplayStore,
	stateRootProvider StateRootProvider,
	signerStore SignerStore,
	tribunalStore TribunalStore,
	appPolicyStore AppPolicyStore,
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

	// Default to protobuf doctrine validator if not provided
	if doctrine == nil {
		doctrine = NewL1Doctrine()
	}

	return &L4Warden{
		logger:            logger,
		replayStore:       replayStore,
		stateRootProvider: stateRootProvider,
		signerStore:       signerStore,
		tribunalStore:     tribunalStore,
		appPolicyStore:    appPolicyStore,
		l3Notary:          l3Notary,
		doctrine:          doctrine,
		knownActionTypes:  knownActions,
		posture:           NewGovernancePosture(posture),
		clock:             clock,
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
		return nil, fmt.Errorf("nonce reservation failed: %w", err)
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
		_ = tv.replayStore.ReleaseNonce(envelope.Nonce)
		return nil, err
	}

	// 3. Stateful Validation (excluding nonce, which is already reserved)
	_, err = tv.verifyStateful(envelope)
	if err != nil {
		tv.logger.Error("Transaction rejected: STATEFUL_VALIDATION_FAILED",
			"nonce", envelope.Nonce,
			"tx_id", envelope.Id,
			string(constants.ConnectionStateError), err)
		// Release nonce reservation on stateful validation failure
		_ = tv.replayStore.ReleaseNonce(envelope.Nonce)
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
		_ = tv.replayStore.ReleaseNonce(envelope.Nonce)
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

// isMutation returns true if the action type modifies system state.
// Uses the strongly-typed intrinsic property from the action definition.
// Mutation classification is defined in protocol/constants/status.json via the _mutation field.
// Actions marked as mutations require L3 Notary (human-presence) verification.
func (tv *L4Warden) isMutation(actionType constants.ActionType) bool {
	return actionType.IsMutation()
}

// verifyStateless performs basic structural, hash, and L1 Doctrine checks.
func (tv *L4Warden) verifyStateless(envelope *govtypes.GovernanceEnvelope) (proto.Message, string, error) {
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
		return nil, "", fmt.Errorf("failed to compute transaction hash: %w", err)
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
func (tv *L4Warden) verifyStateful(envelope *govtypes.GovernanceEnvelope) (time.Time, error) {
	if envelope.StateMerkleRoot == "" {
		return time.Time{}, constants.ErrTxStateRootRequired
	}

	if tv.stateRootProvider == nil {
		tv.logger.Error("State root verification required but provider not configured")
		return time.Time{}, constants.ErrTxStateRootMissing
	}

	currentRoot, err := tv.stateRootProvider.GetCurrentStateRoot()
	if err != nil {
		tv.logger.Error("Failed to get current state root", string(constants.ConnectionStateError), err)
		return time.Time{}, fmt.Errorf("failed to get current state root: %w", err)
	}

	if currentRoot == "" {
		return time.Time{}, constants.ErrTxStateRootMissing
	}

	if currentRoot != envelope.StateMerkleRoot {
		tv.logger.Error("State root mismatch",
			"envelope_root", envelope.StateMerkleRoot,
			"current_root", currentRoot)
		return time.Time{}, constants.ErrTxStateRootMismatch
	}

	return time.Time{}, nil
}

// verifyPosture performs governance posture-aware checks for L2 and L3.
// L2 (machine consensus) is verified first. Only if L2 passes does L3
// (human-presence) run. This preserves the architectural invariant: the
// human's approval bond is spent only on transactions that have already
// cleared tribunal consensus. A human should never be asked to authorize
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

func (tv *L4Warden) verifyL2Posture(envelope *govtypes.GovernanceEnvelope, computedHash string) (bool, error) {
	if envelope.Governance == nil || envelope.Governance.L2 == nil || len(envelope.Governance.L2.Votes) == 0 {
		if tv.posture.RequiresL2Signature() {
			tv.logger.Error("L2 votes missing but required by posture", "posture", tv.posture.Name())
			return false, constants.ErrTxL2SignatureMissing
		}
		return false, nil
	}

	l2 := envelope.Governance.L2

	if tv.signerStore == nil {
		if tv.posture.RequiresL2Signature() {
			tv.logger.Error("Signer store not configured but required by posture", "posture", tv.posture.Name())
			return false, constants.ErrTxL2SignerStoreNotConfigured
		}
		return false, nil
	}

	if tv.tribunalStore == nil {
		if tv.posture.RequiresL2Signature() {
			tv.logger.Error("Tribunal store not configured but required by posture", "posture", tv.posture.Name())
			return false, constants.ErrTxL2TribunalNotConfigured
		}
		return false, nil
	}

	policy, err := tv.tribunalStore.GetTribunal(l2.TribunalId)
	if err != nil {
		if tv.posture.RequiresL2Signature() {
			tv.logger.Error("Failed to load tribunal policy", "tribunal_id", l2.TribunalId, string(constants.ConnectionStateError), err)
			return false, fmt.Errorf("l4 warden: verify L2 posture: %w", err)
		}
		return false, nil
	}
	if policy == nil || !policy.Enabled {
		if tv.posture.RequiresL2Signature() {
			tv.logger.Error("Tribunal policy not found or disabled", "tribunal_id", l2.TribunalId)
			return false, constants.ErrTxL2TribunalNotConfigured
		}
		return false, nil
	}

	members := make(map[string]bool, len(policy.MemberAppIDs))
	for _, m := range policy.MemberAppIDs {
		members[m] = true
	}

	seen := make(map[string]bool)
	affirmative := 0

	for _, vote := range l2.Votes {
		if !members[vote.SignerKeyId] {
			continue
		}
		if seen[vote.SignerKeyId] {
			if policy.RequireDistinct {
				tv.logger.Error("Duplicate signer in vote set with require_distinct", "key_id", vote.SignerKeyId)
				return false, constants.ErrTxL2DuplicateSigner
			}
			continue
		}
		pubKey, err := tv.signerStore.GetTrustedSigner(vote.SignerKeyId)
		if err != nil {
			tv.logger.Error("Failed to load trusted signer", "key_id", vote.SignerKeyId, string(constants.ConnectionStateError), err)
			continue
		}
		if pubKey == nil {
			tv.logger.Error("Consensus (L2) signer key not found in trusted signers", "key_id", vote.SignerKeyId)
			continue
		}
		if !tv.verifyL2Signature(pubKey, vote.ConsensusSignature, computedHash, vote.Decision) {
			tv.logger.Error("L2 signature verification failed", "key_id", vote.SignerKeyId)
			continue
		}
		seen[vote.SignerKeyId] = true
		if vote.Decision {
			affirmative++
		}
	}

	if affirmative < policy.Quorum {
		if tv.posture.RequiresL2Signature() {
			tv.logger.Error("L2 quorum not met", "affirmative", affirmative, "quorum", policy.Quorum, "posture", tv.posture.Name())
			return false, constants.ErrTxL2QuorumNotMet
		}
		return false, nil
	}

	return true, nil
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

	default:
		return nil, constants.ErrTxUnknownActionType
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

// verifyL2Signature verifies an L2 ED25519 signature.
func (tv *L4Warden) verifyL2Signature(pubKey ed25519.PublicKey, signature, messageID string, decision bool) bool {
	if signature == "" || signature == "UNSIGNED" {
		return false
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	payload := fmt.Sprintf("%s|%v", messageID, decision)
	return ed25519.Verify(pubKey, []byte(payload), sigBytes)
}

// Posture returns the current governance posture.
func (tv *L4Warden) Posture() GovernancePosture {
	return tv.posture
}

// Doctrine returns the current L1 doctrine validator.
func (tv *L4Warden) Doctrine() *L1Doctrine {
	return tv.doctrine
}
