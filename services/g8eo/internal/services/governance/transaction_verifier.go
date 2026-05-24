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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sentinel"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	ErrInvalidEnvelope         = errors.New("TX_INVALID_ENVELOPE: failed to decode UAP JSON GovernanceEnvelope")
	ErrUnknownActionType       = errors.New("TX_UNKNOWN_ACTION: action type not recognized")
	ErrPayloadDecodeFailed     = errors.New("TX_PAYLOAD_DECODE: failed to decode typed payload")
	ErrTransactionHashMismatch = errors.New("TX_HASH_MISMATCH: transaction_hash does not match computed hash")
	ErrTransactionExpired      = errors.New("TX_EXPIRED: transaction has expired")
	ErrTransactionReplay       = errors.New("TX_REPLAY: nonce already used")
	ErrStateRootMissing        = errors.New("TX_STATE_MISSING: state_merkle_root required but missing")
	ErrStateRootMismatch       = errors.New("TX_STATE_MISMATCH: state_merkle_root does not match current state")
	ErrL2SignatureMissing      = errors.New("TX_QUORUM_L2_SIG_MISSING: Quorum (L2Consensus) tribunal_signature required but missing")
	ErrL2SignatureInvalid      = errors.New("TX_QUORUM_L2_SIG_INVALID: Quorum (L2Consensus) tribunal_signature failed verification")
	ErrL2KeyNotConfigured      = errors.New("TX_QUORUM_L2_KEY_MISSING: trusted Quorum (L2Consensus) signer key not configured")
	ErrL3ProofMissing          = errors.New("TX_NOTARY_L3_PROOF_MISSING: Notary (L3Notary) WebAuthn proof required but missing")
	ErrL3ProofInvalid          = errors.New("TX_NOTARY_L3_PROOF_INVALID: Notary (L3Notary) WebAuthn proof failed verification")
	ErrL3NotaryNotConfigured   = errors.New("TX_NOTARY_L3_NOTARY_MISSING: Notary (L3Notary) required but not configured")
	ErrTransactionHashMissing  = errors.New("TX_HASH_MISSING: transaction_hash required")
	ErrTransactionIDMissing    = errors.New("TX_ID_MISSING: id required")
	ErrExpiresAtMissing        = errors.New("TX_EXPIRES_AT_MISSING: expires_at required")
	ErrNonceMissing            = errors.New("TX_NONCE_MISSING: nonce required")
	ErrReplayStoreMissing      = errors.New("TX_REPLAY_STORE_MISSING: replay store required")
	ErrStateRootRequired       = errors.New("TX_STATE_REQUIRED: state_merkle_root required")
	ErrPayloadMissing          = errors.New("TX_PAYLOAD_MISSING: typed protobuf payload required")
	ErrPayloadActionMismatch   = errors.New("TX_PAYLOAD_ACTION_MISMATCH: action type does not match typed payload")
	ErrL1ValidationFailed      = errors.New("TX_DOCTRINE_L1_FAILED: typed payload violates Doctrine (L1Doctrine) forbidden patterns")
	ErrTxInFlight              = errors.New("TX_IN_FLIGHT: transaction with same nonce already in-flight")
)

//go:generate mockery --name ReplayStore --output ./mocks --dir .

// ReplayStore defines the interface for nonce replay protection.
type ReplayStore interface {
	// CheckAndSetNonce returns true if the nonce was already used (replay detected).
	// If not used, it marks the nonce as used and returns false.
	CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error)

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
}

//go:generate mockery --name StateRootProvider --output ./mocks --dir .

// StateRootProvider defines the interface for obtaining the current state root.
type StateRootProvider interface {
	GetCurrentStateRoot() (string, error)
}

//go:generate mockery --name GovernancePosture --output ./mocks --dir .

// GovernancePosture defines the interface for posture-aware governance checks.
// Different postures (doctrine, consensus, notary) have different requirements
// for L2 and L3 proofs. This interface allows adding new postures without
// modifying the core verification logic (Open-Closed Principle).
type GovernancePosture interface {
	// Name returns the posture name (e.g., "doctrine", "consensus", "notary").
	Name() string

	// RequiresL2Signature returns true if this posture requires L2 signatures.
	RequiresL2Signature() bool

	// RequiresL3Proof returns true if this posture requires L3 proofs for mutations.
	RequiresL3Proof() bool
}

// DoctrinePosture implements the doctrine governance posture.
// Doctrine is the minimal posture requiring only L1 (Doctrine) validation.
type DoctrinePosture struct{}

func (p *DoctrinePosture) Name() string              { return "doctrine" }
func (p *DoctrinePosture) RequiresL2Signature() bool { return true }
func (p *DoctrinePosture) RequiresL3Proof() bool     { return true }

// ConsensusPosture implements the consensus governance posture.
// Consensus requires L1 (Doctrine) and L2 (Consensus) validation.
type ConsensusPosture struct{}

func (p *ConsensusPosture) Name() string              { return "consensus" }
func (p *ConsensusPosture) RequiresL2Signature() bool { return true }
func (p *ConsensusPosture) RequiresL3Proof() bool     { return false }

// NotaryPosture implements the notary governance posture.
// Notary requires L1 (Doctrine), L2 (Consensus), and L3 (Notary) validation.
type NotaryPosture struct{}

func (p *NotaryPosture) Name() string              { return "notary" }
func (p *NotaryPosture) RequiresL2Signature() bool { return true }
func (p *NotaryPosture) RequiresL3Proof() bool     { return true }

// NewGovernancePosture creates a GovernancePosture from a string name.
// Panics on invalid posture to fail-closed at startup rather than silently defaulting.
func NewGovernancePosture(posture string) GovernancePosture {
	switch posture {
	case "doctrine":
		return &DoctrinePosture{}
	case "consensus":
		return &ConsensusPosture{}
	case "notary":
		return &NotaryPosture{}
	default:
		panic(fmt.Sprintf("invalid governance posture: %s (must be one of: doctrine, consensus, notary)", posture))
	}
}

// SignerStore defines the interface for loading trusted L2Consensus signers.
type SignerStore interface {
	GetTrustedSigner(keyID string) (ed25519.PublicKey, error)
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

		var pubKey ed25519.PublicKey
		copy(pubKey, pubKeyBytes)
		signers[keyID] = pubKey
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

// SimpleStateRootProvider returns a fixed root set at construction time.
// Root must be non-empty; a missing root is a misconfiguration that returns an
// error so callers fail closed rather than silently accepting any state root.
type SimpleStateRootProvider struct {
	Root string
}

func (s *SimpleStateRootProvider) GetCurrentStateRoot() (string, error) {
	if s.Root == "" {
		return "", errors.New("PROVIDER_MISCONFIGURED: state root is empty")
	}
	return s.Root, nil
}

// VerifiedTransaction represents a fully verified transaction ready for execution.
type VerifiedTransaction struct {
	Envelope       *uap.UAPEnvelope
	ActionType     constants.ActionType
	Payload        []byte
	DecodedPayload proto.Message
	StateRoot      string
	Nonce          string
	ExpiresAt      time.Time
	L2Valid        bool // Whether L2 signature was valid (may be false in Doctrine posture)
	L3Valid        bool // Whether L3 proof was valid (may be false in Doctrine/Consensus posture)
}

// TransactionVerifier performs all pre-dispatch verification checks.
type TransactionVerifier struct {
	logger            *slog.Logger
	replayStore       ReplayStore
	stateRootProvider StateRootProvider
	signerStore       SignerStore
	appPolicyStore    AppPolicyStore
	l3Notary          L3Notary
	sentinel          *sentinel.Sentinel // L1 threat detection for MCP arguments
	knownActionTypes  map[constants.ActionType]struct{}
	posture           GovernancePosture // Governance posture: doctrine, consensus, or notary

	inFlight sync.Map // Concurrent-safe tracking of in-flight nonces
}

// NewTransactionVerifier creates a new transaction verifier.
func NewTransactionVerifier(
	logger *slog.Logger,
	replayStore ReplayStore,
	stateRootProvider StateRootProvider,
	signerStore SignerStore,
	appPolicyStore AppPolicyStore,
	l3Notary L3Notary,
	sentinel *sentinel.Sentinel,
	knownActionTypes []constants.ActionType,
	posture string,
) *TransactionVerifier {
	knownActions := make(map[constants.ActionType]struct{})
	for _, action := range knownActionTypes {
		knownActions[action] = struct{}{}
	}

	return &TransactionVerifier{
		logger:            logger,
		replayStore:       replayStore,
		stateRootProvider: stateRootProvider,
		signerStore:       signerStore,
		appPolicyStore:    appPolicyStore,
		l3Notary:          l3Notary,
		sentinel:          sentinel,
		knownActionTypes:  knownActions,
		posture:           NewGovernancePosture(posture),
	}
}

// VerifyEnvelope performs all required verification checks on a decoded UAP JSON GovernanceEnvelope.
// It is decomposed into three discrete validation stages:
// 1. Stateless: Basic structural, hash, and L1 Doctrine checks that don't require external state.
// 2. Stateful: Checks requiring external state (expiry, state root, and early nonce reservation).
// 3. Posture: Governance posture-aware checks (L2 Consensus and L3 Notary proofs).
func (tv *TransactionVerifier) VerifyEnvelope(ctx context.Context, envelope *uap.UAPEnvelope) (*VerifiedTransaction, error) {
	if envelope == nil {
		return nil, ErrInvalidEnvelope
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
		return nil, ErrReplayStoreMissing
	}
	if envelope.ExpiresAt == nil {
		tv.releaseInFlight(envelope.Nonce)
		return nil, ErrExpiresAtMissing
	}
	expiresAt := envelope.ExpiresAt.AsTime()
	if time.Now().UTC().After(expiresAt) {
		tv.logger.Error("Transaction rejected: EXPIRED",
			"nonce", envelope.Nonce,
			"expires_at", expiresAt,
			"now", time.Now().UTC())
		tv.releaseInFlight(envelope.Nonce)
		return nil, ErrTransactionExpired
	}
	if envelope.Nonce == "" {
		tv.logger.Error("Transaction rejected: NONCE_MISSING")
		tv.releaseInFlight(envelope.Nonce)
		return nil, ErrNonceMissing
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
		return nil, ErrTransactionReplay
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
	}, nil
}

func (tv *TransactionVerifier) trackInFlight(nonce string) error {
	if nonce == "" {
		return nil
	}
	_, loaded := tv.inFlight.LoadOrStore(nonce, true)
	if loaded {
		tv.logger.Warn("Transaction with same nonce already in-flight", "nonce", nonce)
		return ErrTxInFlight
	}
	return nil
}

func (tv *TransactionVerifier) releaseInFlight(nonce string) {
	tv.inFlight.Delete(nonce)
}

// isMutation returns true if the action type modifies system state.
// Uses the strongly-typed intrinsic property from the action definition.
// Mutation classification is defined in protocol/constants/status.json via the _mutation field.
// Actions marked as mutations require L3 Notary (human-presence) verification.
func (tv *TransactionVerifier) isMutation(actionType constants.ActionType) bool {
	return constants.IsMutation(actionType)
}

// verifyStateless performs basic structural, hash, and L1 Doctrine checks.
func (tv *TransactionVerifier) verifyStateless(envelope *uap.UAPEnvelope) (proto.Message, string, error) {
	if envelope.Id == "" {
		return nil, "", ErrTransactionIDMissing
	}

	actionType := constants.ActionType(envelope.ActionType)
	if _, ok := tv.knownActionTypes[actionType]; !ok {
		tv.logger.Error("Unknown action type", "action_type", envelope.ActionType)
		return nil, "", ErrUnknownActionType
	}

	if len(envelope.Payload) == 0 {
		return nil, "", ErrPayloadMissing
	}

	decodedPayload, err := tv.decodePayloadForAction(actionType, envelope.Payload)
	if err != nil {
		tv.logger.Error("Failed to decode typed payload", "action_type", envelope.ActionType, string(constants.ConnectionStateError), err)
		return nil, "", ErrPayloadDecodeFailed
	}

	// INVESTIGATION_CREATE has no typed payload (returns nil), skip L1 validation
	if decodedPayload != nil {
		if violations := tv.validateL1Governance(decodedPayload); len(violations) > 0 {
			tv.logger.Error("Doctrine (L1Doctrine) validation failed", "action_type", envelope.ActionType, "violations", violations)
			return nil, "", fmt.Errorf("%w: %s", ErrL1ValidationFailed, strings.Join(violations, ", "))
		}
	}

	computedHash, err := tv.computeTransactionHash(envelope)
	if err != nil {
		return nil, "", fmt.Errorf("failed to compute transaction hash: %w", err)
	}

	if envelope.TransactionHash == "" {
		return nil, "", ErrTransactionHashMissing
	}

	if envelope.TransactionHash != computedHash {
		tv.logger.Error("Transaction hash mismatch",
			"provided", envelope.TransactionHash,
			"computed", computedHash)
		return nil, "", ErrTransactionHashMismatch
	}

	if envelope.Id != computedHash {
		tv.logger.Error("Transaction id mismatch",
			"provided", envelope.Id,
			"computed", computedHash)
		return nil, "", ErrTransactionHashMismatch
	}

	return decodedPayload, computedHash, nil
}

// verifyStateful checks state root. Nonce and expiry are checked earlier in VerifyEnvelope.
func (tv *TransactionVerifier) verifyStateful(envelope *uap.UAPEnvelope) (time.Time, error) {
	if envelope.StateMerkleRoot == "" {
		return time.Time{}, ErrStateRootRequired
	}

	if tv.stateRootProvider == nil {
		tv.logger.Error("State root verification required but provider not configured")
		return time.Time{}, ErrStateRootMissing
	}

	currentRoot, err := tv.stateRootProvider.GetCurrentStateRoot()
	if err != nil {
		tv.logger.Error("Failed to get current state root", string(constants.ConnectionStateError), err)
		return time.Time{}, fmt.Errorf("failed to get current state root: %w", err)
	}

	if currentRoot == "" {
		return time.Time{}, ErrStateRootMissing
	}

	if currentRoot != envelope.StateMerkleRoot {
		tv.logger.Error("State root mismatch",
			"envelope_root", envelope.StateMerkleRoot,
			"current_root", currentRoot)
		return time.Time{}, ErrStateRootMismatch
	}

	return time.Time{}, nil
}

// verifyPosture performs governance posture-aware checks for L2 and L3.
func (tv *TransactionVerifier) verifyPosture(ctx context.Context, envelope *uap.UAPEnvelope, computedHash string) (bool, bool, error) {
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

func (tv *TransactionVerifier) verifyL2Posture(envelope *uap.UAPEnvelope, computedHash string) (bool, error) {
	if !tv.posture.RequiresL2Signature() {
		return true, nil
	}

	if envelope.Governance == nil || envelope.Governance.L2 == nil {
		tv.logger.Error("L2 signature missing but required by posture", "posture", tv.posture.Name())
		return false, ErrL2SignatureMissing
	}

	l2 := envelope.Governance.L2
	if l2.TribunalSignature == "" {
		tv.logger.Error("L2 signature empty but required by posture", "posture", tv.posture.Name())
		return false, ErrL2SignatureMissing
	}

	if l2.KeyId == "" {
		tv.logger.Error("L2 key ID missing but required by posture", "posture", tv.posture.Name())
		return false, ErrL2KeyNotConfigured
	}

	if tv.signerStore == nil {
		tv.logger.Error("Signer store not configured but required by posture", "posture", tv.posture.Name())
		return false, ErrL2KeyNotConfigured
	}

	pubKey, err := tv.signerStore.GetTrustedSigner(l2.KeyId)
	if err != nil {
		tv.logger.Error("Failed to load trusted signer", "key_id", l2.KeyId, string(constants.ConnectionStateError), err)
		return false, ErrL2KeyNotConfigured
	}

	if pubKey == nil {
		tv.logger.Error("Quorum (L2Consensus) signer key not found in trusted signers", "key_id", l2.KeyId)
		return false, ErrL2KeyNotConfigured
	}

	if tv.verifyL2Signature(pubKey, l2.TribunalSignature, computedHash, true) {
		return true, nil
	}

	tv.logger.Error("L2 signature verification failed but required by posture", "posture", tv.posture.Name())
	return false, ErrL2SignatureInvalid
}

func (tv *TransactionVerifier) verifyL3Posture(ctx context.Context, envelope *uap.UAPEnvelope) (bool, error) {
	actionType := constants.ActionType(envelope.ActionType)
	if !tv.isMutation(actionType) {
		return true, nil
	}

	if !tv.posture.RequiresL3Proof() {
		return true, nil
	}

	// Check if this is an external app transaction that can bypass L3 via policy
	if envelope.Governance != nil && envelope.Governance.L2 != nil && envelope.Governance.L2.KeyId != "" {
		if tv.appPolicyStore != nil {
			appID := envelope.Governance.L2.KeyId
			policy, err := tv.appPolicyStore.GetAppPolicy(appID)
			if err != nil {
				tv.logger.Warn("Failed to retrieve app policy for L3 bypass check", "app_id", appID, string(constants.ConnectionStateError), err)
				// Fail-closed: if policy lookup fails, require standard L3
			} else if policy != nil {
				// Check if this action type is in AutoApproveIntents
				actionStr := string(actionType)
				for _, autoApproveIntent := range policy.AutoApproveIntents {
					if autoApproveIntent == actionStr {
						tv.logger.Info("L3 bypassed via app policy", "app_id", appID, "action_type", actionStr)
						return true, nil
					}
				}
			}
		}
	}

	if envelope.Governance == nil || envelope.Governance.L3 == nil || envelope.Governance.L3.Proof == nil {
		tv.logger.Error("L3 proof missing but required by posture", "posture", tv.posture.Name())
		return false, ErrL3ProofMissing
	}

	if tv.l3Notary == nil {
		tv.logger.Error("L3 notary not configured but required by posture", "posture", tv.posture.Name())
		return false, ErrL3NotaryNotConfigured
	}

	ok, err := tv.l3Notary.VerifyL3Proof(
		envelope.OperatorId,
		envelope.TransactionHash,
		envelope.CliSessionId,
		envelope.Governance.L3.Proof,
	)

	if err != nil || !ok {
		tv.logger.Error("Notary (L3Notary) verification failed but required by posture", string(constants.ConnectionStateError), err)
		return false, ErrL3ProofInvalid
	}

	return true, nil
}

func (tv *TransactionVerifier) decodePayloadForAction(actionType constants.ActionType, payload []byte) (proto.Message, error) {
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
	case constants.ActionTypeGrantIntent:
		msg = &operatorv1.GrantIntentRequested{}
	case constants.ActionTypeRevokeIntent:
		msg = &operatorv1.RevokeIntentRequested{}
	case constants.ActionTypeHeartbeat:
		msg = &operatorv1.HeartbeatRequested{}
	case constants.ActionTypeInvestigationCreate:
		// No typed payload for investigation create, it uses raw bytes
		return nil, nil
	default:
		return nil, ErrUnknownActionType
	}
	if err := proto.Unmarshal(payload, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (tv *TransactionVerifier) validateL1Governance(msg proto.Message) []string {
	var violations []string
	md := msg.ProtoReflect().Descriptor()
	fields := md.Fields()

	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		opts := fd.Options()
		if opts == nil || !proto.HasExtension(opts, commonv1.E_ForbiddenPatterns) {
			continue
		}
		patternsStr, ok := proto.GetExtension(opts, commonv1.E_ForbiddenPatterns).(string)
		if !ok || patternsStr == "" {
			continue
		}
		val := msg.ProtoReflect().Get(fd)
		if fd.Kind() != protoreflect.StringKind {
			continue
		}
		strVal := val.String()
		for _, p := range strings.Split(patternsStr, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			matched, err := regexp.MatchString(p, strVal)
			if err == nil && matched {
				violations = append(violations, fmt.Sprintf("field %s violates pattern %s", fd.Name(), p))
			}
		}
	}

	// Extended L1 validation: recursively analyze MCP/A2A tool arguments
	if tv.sentinel != nil {
		switch m := msg.(type) {
		case *operatorv1.McpCallRequested:
			if m.ArgumentsJson != "" {
				analysis := tv.sentinel.AnalyzeMCPArguments(m.ArgumentsJson)
				if !analysis.Safe {
					violations = append(violations, fmt.Sprintf("MCP arguments violate L1 governance: %s", analysis.BlockReason))
				}
				// Also add elevated threat signals as informational violations
				if analysis.RequiresApproval {
					for _, sig := range analysis.ThreatSignals {
						violations = append(violations, fmt.Sprintf("MCP argument threat detected in %s: %s (confidence: %.2f)", sig.Context, sig.Indicator, sig.Confidence))
					}
				}
			}
		case *operatorv1.A2ACallRequested:
			if m.PayloadJson != "" {
				analysis := tv.sentinel.AnalyzeMCPArguments(m.PayloadJson) // Reuse same recursive logic
				if !analysis.Safe {
					violations = append(violations, fmt.Sprintf("A2A payload violates L1 governance: %s", analysis.BlockReason))
				}
				if analysis.RequiresApproval {
					for _, sig := range analysis.ThreatSignals {
						violations = append(violations, fmt.Sprintf("A2A payload threat detected in %s: %s (confidence: %.2f)", sig.Context, sig.Indicator, sig.Confidence))
					}
				}
			}
		}
	}

	return violations
}

// computeTransactionHash computes the canonical transaction hash.
func (tv *TransactionVerifier) computeTransactionHash(envelope *uap.UAPEnvelope) (string, error) {
	return uap.GenerateMessageID(envelope)
}

// verifyL2Signature verifies an L2 ED25519 signature.
func (tv *TransactionVerifier) verifyL2Signature(pubKey ed25519.PublicKey, signature, messageID string, decision bool) bool {
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
