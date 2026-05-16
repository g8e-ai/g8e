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
	"regexp"
	"strings"
	"time"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
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
	ErrL2SignatureMissing      = errors.New("TX_L2_SIG_MISSING: L2 tribunal_signature required but missing")
	ErrL2SignatureInvalid      = errors.New("TX_L2_SIG_INVALID: L2 tribunal_signature failed verification")
	ErrL2KeyNotConfigured      = errors.New("TX_L2_KEY_MISSING: trusted L2 signer key not configured")
	ErrL3SignatureMissing      = errors.New("TX_L3_SIG_MISSING: L3 human_signature required but missing")
	ErrL3SignatureInvalid      = errors.New("TX_L3_SIG_INVALID: L3 human_signature failed verification")
	ErrL3VerifierNotConfigured = errors.New("TX_L3_VERIFIER_MISSING: L3 verifier required but not configured")
	ErrTransactionHashMissing  = errors.New("TX_HASH_MISSING: transaction_hash required")
	ErrTransactionIDMissing    = errors.New("TX_ID_MISSING: id required")
	ErrExpiresAtMissing        = errors.New("TX_EXPIRES_AT_MISSING: expires_at required")
	ErrNonceMissing            = errors.New("TX_NONCE_MISSING: nonce required")
	ErrReplayStoreMissing      = errors.New("TX_REPLAY_STORE_MISSING: replay store required")
	ErrStateRootRequired       = errors.New("TX_STATE_REQUIRED: state_merkle_root required")
	ErrPayloadMissing          = errors.New("TX_PAYLOAD_MISSING: typed protobuf payload required")
	ErrPayloadActionMismatch   = errors.New("TX_PAYLOAD_ACTION_MISMATCH: action type does not match typed payload")
	ErrL1ValidationFailed      = errors.New("TX_L1_FAILED: typed payload violates L1 forbidden patterns")
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
	ActionType     string
	Payload        []byte
	DecodedPayload proto.Message
	StateRoot      string
	Nonce          string
	ExpiresAt      time.Time
}

// TransactionVerifier performs all pre-dispatch verification checks.
type TransactionVerifier struct {
	logger            *slog.Logger
	replayStore       ReplayStore
	stateRootProvider StateRootProvider
	trustedSigners    map[string]ed25519.PublicKey
	l3Verifier        L3Verifier
	knownActionTypes  map[string]struct{}
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
		logger:            logger,
		replayStore:       replayStore,
		stateRootProvider: stateRootProvider,
		trustedSigners:    trustedSigners,
		l3Verifier:        l3Verifier,
		knownActionTypes:  knownActions,
	}
}

// VerifyEnvelope performs all required verification checks on a decoded UAP JSON GovernanceEnvelope.
func (tv *TransactionVerifier) VerifyEnvelope(envelope *uap.UAPEnvelope) (*VerifiedTransaction, error) {
	if envelope == nil {
		return nil, ErrInvalidEnvelope
	}
	if envelope.Id == "" {
		return nil, ErrTransactionIDMissing
	}
	if _, ok := tv.knownActionTypes[envelope.ActionType]; !ok {
		tv.logger.Error("Unknown action type", "action_type", envelope.ActionType)
		return nil, ErrUnknownActionType
	}
	if len(envelope.Payload) == 0 {
		return nil, ErrPayloadMissing
	}
	decodedPayload, err := tv.decodePayloadForAction(envelope.ActionType, envelope.Payload)
	if err != nil {
		tv.logger.Error("Failed to decode typed payload", "action_type", envelope.ActionType, "error", err)
		return nil, ErrPayloadDecodeFailed
	}
	if violations := tv.validateL1Governance(decodedPayload); len(violations) > 0 {
		tv.logger.Error("L1 validation failed", "action_type", envelope.ActionType, "violations", violations)
		return nil, ErrL1ValidationFailed
	}

	computedHash, err := tv.computeTransactionHash(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to compute transaction hash: %w", err)
	}
	if envelope.TransactionHash == "" {
		return nil, ErrTransactionHashMissing
	}
	if envelope.TransactionHash != computedHash {
		tv.logger.Error("Transaction hash mismatch",
			"provided", envelope.TransactionHash,
			"computed", computedHash)
		return nil, ErrTransactionHashMismatch
	}
	if envelope.Id != computedHash {
		tv.logger.Error("Transaction id mismatch",
			"provided", envelope.Id,
			"computed", computedHash)
		return nil, ErrTransactionHashMismatch
	}

	if envelope.ExpiresAt == nil {
		return nil, ErrExpiresAtMissing
	}
	expiresAt := envelope.ExpiresAt.AsTime()
	if time.Now().UTC().After(expiresAt) {
		tv.logger.Error("Transaction expired", "expires_at", expiresAt)
		return nil, ErrTransactionExpired
	}

	if envelope.Nonce == "" {
		return nil, ErrNonceMissing
	}
	if tv.replayStore == nil {
		return nil, ErrReplayStoreMissing
	}
	replayed, err := tv.replayStore.CheckAndSetNonce(envelope.Nonce, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("replay check failed: %w", err)
	}
	if replayed {
		tv.logger.Error("Transaction replay detected", "nonce", envelope.Nonce)
		return nil, ErrTransactionReplay
	}

	if envelope.StateMerkleRoot == "" {
		return nil, ErrStateRootRequired
	}
	if tv.stateRootProvider == nil {
		tv.logger.Error("State root verification required but provider not configured")
		return nil, ErrStateRootMissing
	}
	currentRoot, err := tv.stateRootProvider.GetCurrentStateRoot()
	if err != nil {
		tv.logger.Error("Failed to get current state root", "error", err)
		return nil, fmt.Errorf("failed to get current state root: %w", err)
	}
	if currentRoot == "" {
		return nil, ErrStateRootMissing
	}
	if currentRoot != envelope.StateMerkleRoot {
		tv.logger.Error("State root mismatch",
			"envelope_root", envelope.StateMerkleRoot,
			"current_root", currentRoot)
		return nil, ErrStateRootMismatch
	}

	if envelope.Governance == nil || envelope.Governance.L2 == nil {
		return nil, ErrL2SignatureMissing
	}
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
	if !tv.verifyL2Signature(pubKey, envelope.Governance.L2.TribunalSignature, computedHash, true) {
		return nil, ErrL2SignatureInvalid
	}

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
		Envelope:       envelope,
		ActionType:     envelope.ActionType,
		Payload:        envelope.Payload,
		DecodedPayload: decodedPayload,
		StateRoot:      envelope.StateMerkleRoot,
		Nonce:          envelope.Nonce,
		ExpiresAt:      expiresAt,
	}

	return verified, nil
}

func (tv *TransactionVerifier) decodePayloadForAction(actionType string, payload []byte) (proto.Message, error) {
	var msg proto.Message
	switch actionType {
	case "EXECUTE_BASH":
		msg = &operatorv1.CommandRequested{}
	case "FILE_EDIT":
		msg = &operatorv1.FileEditRequested{}
	case "RESTORE_FILE":
		msg = &operatorv1.RestoreFileRequested{}
	case "SHUTDOWN":
		msg = &operatorv1.ShutdownRequested{}
	case "FS_LIST":
		msg = &operatorv1.FsListRequested{}
	case "FS_READ":
		msg = &operatorv1.FsReadRequested{}
	case "FS_GREP":
		msg = &operatorv1.FsGrepRequested{}
	case "PORT_CHECK":
		msg = &operatorv1.CheckPortRequested{}
	case "FETCH_LOGS":
		msg = &operatorv1.FetchLogsRequested{}
	case "FETCH_HISTORY":
		msg = &operatorv1.FetchHistoryRequested{}
	case "FETCH_FILE_HISTORY":
		msg = &operatorv1.FetchFileHistoryRequested{}
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
		patternsStr := proto.GetExtension(opts, commonv1.E_ForbiddenPatterns).(string)
		if patternsStr == "" {
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

// isMutation returns true if the action type modifies system state.
func (tv *TransactionVerifier) isMutation(actionType string) bool {
	switch actionType {
	case "EXECUTE_BASH", "FILE_EDIT", "RESTORE_FILE", "SHUTDOWN":
		return true
	default:
		return false
	}
}
