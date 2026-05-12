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
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"google.golang.org/protobuf/proto"
)

var (
	ErrInvalidProtobuf          = errors.New("TX_INVALID_PROTOBUF: failed to decode UniversalEnvelope")
	ErrUnknownActionType        = errors.New("TX_UNKNOWN_ACTION: action type not recognized")
	ErrPayloadDecodeFailed       = errors.New("TX_PAYLOAD_DECODE: failed to decode typed payload")
	ErrTransactionHashMismatch  = errors.New("TX_HASH_MISMATCH: transaction_hash does not match computed hash")
	ErrTransactionExpired       = errors.New("TX_EXPIRED: transaction has expired")
	ErrTransactionReplay        = errors.New("TX_REPLAY: nonce already used")
	ErrStateRootMissing         = errors.New("TX_STATE_MISSING: state_merkle_root required but missing")
	ErrStateRootMismatch        = errors.New("TX_STATE_MISMATCH: state_merkle_root does not match current state")
	ErrL2SignatureMissing       = errors.New("TX_L2_SIG_MISSING: L2 tribunal_signature required but missing")
	ErrL2SignatureInvalid       = errors.New("TX_L2_SIG_INVALID: L2 tribunal_signature failed verification")
	ErrL2KeyNotConfigured       = errors.New("TX_L2_KEY_MISSING: trusted L2 signer key not configured")
	ErrL3SignatureMissing       = errors.New("TX_L3_SIG_MISSING: L3 human_signature required but missing")
	ErrL3SignatureInvalid       = errors.New("TX_L3_SIG_INVALID: L3 human_signature failed verification")
	ErrL3VerifierNotConfigured  = errors.New("TX_L3_VERIFIER_MISSING: L3 verifier required but not configured")
)

// ReplayStore defines the interface for nonce replay protection.
type ReplayStore interface {
	// CheckAndSetNonce returns true if the nonce was already used (replay detected).
	// If not used, it marks the nonce as used and returns false.
	CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error)
}

// StateRootProvider defines the interface for obtaining the current state root.
type StateRootProvider interface {
	GetCurrentStateRoot() (string, error)
}

// VerifiedTransaction represents a fully verified transaction ready for execution.
type VerifiedTransaction struct {
	Envelope     *uap.UAPEnvelope
	ActionType   string
	Payload      []byte
	DecodedPayload proto.Message
	StateRoot    string
	Nonce        string
	ExpiresAt    time.Time
}

// TransactionVerifier performs all pre-dispatch verification checks.
type TransactionVerifier struct {
	logger           *slog.Logger
	replayStore      ReplayStore
	stateRootProvider StateRootProvider
	trustedSigners   map[string]ed25519.PublicKey
	l3Verifier       L3Verifier
	knownActionTypes map[string]struct{}
}

// NewTransactionVerifier creates a new transaction verifier.
func NewTransactionVerifier(
	logger *slog.Logger,
	replayStore ReplayStore,
	stateRootProvider StateRootProvider,
	trustedSigners map[string]ed25519.PublicKey,
	l3Verifier L3Verifier,
	knownActionTypes []string,
) *TransactionVerifier {
	knownActions := make(map[string]struct{})
	for _, action := range knownActionTypes {
		knownActions[action] = struct{}{}
	}

	return &TransactionVerifier{
		logger:           logger,
		replayStore:      replayStore,
		stateRootProvider: stateRootProvider,
		trustedSigners:   trustedSigners,
		l3Verifier:       l3Verifier,
		knownActionTypes: knownActions,
	}
}

// VerifyTransaction performs all required verification checks on a serialized UniversalEnvelope.
func (tv *TransactionVerifier) VerifyTransaction(envelopeBytes []byte) (*VerifiedTransaction, error) {
	// 1. Decode Protobuf UniversalEnvelope
	envelope := &commonv1.UniversalEnvelope{}
	if err := proto.Unmarshal(envelopeBytes, envelope); err != nil {
		tv.logger.Error("Failed to decode UniversalEnvelope", "error", err)
		return nil, ErrInvalidProtobuf
	}

	// 2. Verify action type is known
	if _, ok := tv.knownActionTypes[envelope.ActionType]; !ok {
		tv.logger.Error("Unknown action type", "action_type", envelope.ActionType)
		return nil, ErrUnknownActionType
	}

	// 3. Verify transaction hash matches computed hash
	computedHash, err := tv.computeTransactionHash(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to compute transaction hash: %w", err)
	}
	if envelope.TransactionHash != "" && envelope.TransactionHash != computedHash {
		tv.logger.Error("Transaction hash mismatch",
			"provided", envelope.TransactionHash,
			"computed", computedHash)
		return nil, ErrTransactionHashMismatch
	}

	// 4. Verify transaction has not expired
	if envelope.ExpiresAt != nil {
		expiresAt := envelope.ExpiresAt.AsTime()
		if time.Now().UTC().After(expiresAt) {
			tv.logger.Error("Transaction expired", "expires_at", expiresAt)
			return nil, ErrTransactionExpired
		}
	}

	// 5. Verify nonce replay protection
	if envelope.Nonce != "" {
		expiresAt := time.Time{}
		if envelope.ExpiresAt != nil {
			expiresAt = envelope.ExpiresAt.AsTime()
		}
		replayed, err := tv.replayStore.CheckAndSetNonce(envelope.Nonce, expiresAt)
		if err != nil {
			return nil, fmt.Errorf("replay check failed: %w", err)
		}
		if replayed {
			tv.logger.Error("Transaction replay detected", "nonce", envelope.Nonce)
			return nil, ErrTransactionReplay
		}
	}

	// 6. Verify state root matches current state
	if envelope.StateMerkleRoot != "" {
		if tv.stateRootProvider == nil {
			tv.logger.Error("State root verification required but provider not configured")
			return nil, ErrStateRootMissing
		}
		currentRoot, err := tv.stateRootProvider.GetCurrentStateRoot()
		if err != nil {
			tv.logger.Error("Failed to get current state root", "error", err)
			return nil, fmt.Errorf("failed to get current state root: %w", err)
		}
		if currentRoot != envelope.StateMerkleRoot {
			tv.logger.Error("State root mismatch",
				"envelope_root", envelope.StateMerkleRoot,
				"current_root", currentRoot)
			return nil, ErrStateRootMismatch
		}
	}

	// 7. Verify L2 signature
	if envelope.Governance != nil && envelope.Governance.L2 != nil {
		if envelope.Governance.L2.TribunalSignature == "" {
			return nil, ErrL2SignatureMissing
		}
		if envelope.Governance.L2.KeyId == "" {
			return nil, ErrL2KeyNotConfigured
		}
		pubKey, ok := tv.trustedSigners[envelope.Governance.L2.KeyId]
		if !ok {
			tv.logger.Error("L2 signer key not found in trusted signers", "key_id", envelope.Governance.L2.KeyId)
			return nil, ErrL2KeyNotConfigured
		}
		if !tv.verifyL2Signature(pubKey, envelope.Governance.L2.TribunalSignature, envelope.Id, true) {
			return nil, ErrL2SignatureInvalid
		}
	}

	// 8. Verify L3 signature for mutations
	if tv.isMutation(envelope.ActionType) {
		if envelope.Governance == nil || envelope.Governance.L3 == nil || envelope.Governance.L3.HumanSignature == "" {
			return nil, ErrL3SignatureMissing
		}
		if tv.l3Verifier == nil {
			return nil, ErrL3VerifierNotConfigured
		}
		ok, err := tv.l3Verifier.VerifyL3Proof(
			envelope.OperatorId,
			envelope.Id,
			envelope.Governance.L3.HumanSignature,
			envelope.Governance.L3.PublicKey,
		)
		if err != nil || !ok {
			tv.logger.Error("L3 verification failed", "error", err)
			return nil, ErrL3SignatureInvalid
		}
	}

	// 9. Return verified transaction
	verified := &VerifiedTransaction{
		Envelope:     (*uap.UAPEnvelope)(envelope),
		ActionType:   envelope.ActionType,
		Payload:      envelope.Payload,
		StateRoot:    envelope.StateMerkleRoot,
		Nonce:        envelope.Nonce,
		ExpiresAt:    envelope.ExpiresAt.AsTime(),
	}

	return verified, nil
}

// computeTransactionHash computes the canonical transaction hash.
func (tv *TransactionVerifier) computeTransactionHash(envelope *commonv1.UniversalEnvelope) (string, error) {
	// For now, use the existing GenerateMessageID from uap package
	// This should be replaced with a proper protobuf-based hash computation
	env := (*uap.UAPEnvelope)(envelope)
	return uap.GenerateMessageID(env)
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

// isMutation returns true if the action type modifies system state.
func (tv *TransactionVerifier) isMutation(actionType string) bool {
	switch actionType {
	case "EXECUTE_BASH", "FILE_EDIT", "RESTORE_FILE", "SHUTDOWN":
		return true
	default:
		return false
	}
}
