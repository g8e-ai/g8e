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
	"log/slog"
	"os"
	"testing"
	"time"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockReplayStore struct {
	nonces map[string]bool
}

func newMockReplayStore() *mockReplayStore {
	return &mockReplayStore{
		nonces: make(map[string]bool),
	}
}

func (m *mockReplayStore) CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error) {
	if m.nonces[nonce] {
		return true, nil // replay detected
	}
	m.nonces[nonce] = true
	return false, nil
}

type mockStateRootProvider struct {
	root string
}

func newMockStateRootProvider(root string) *mockStateRootProvider {
	return &mockStateRootProvider{root: root}
}

func (m *mockStateRootProvider) GetCurrentStateRoot() (string, error) {
	return m.root, nil
}

type mockL3Verifier struct {
	shouldPass bool
}

func (m *mockL3Verifier) VerifyL3Proof(userID, messageID, signatureHex, pubKeyHex string) (bool, error) {
	return m.shouldPass, nil
}

func TestTransactionVerifier_InvalidProtobuf(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Test with invalid protobuf bytes
	_, err := verifier.VerifyTransaction([]byte("invalid protobuf data"))
	if err != ErrInvalidProtobuf {
		t.Errorf("Expected ErrInvalidProtobuf, got %v", err)
	}
}

func TestTransactionVerifier_UnknownActionType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create a valid protobuf envelope with unknown action type
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "UNKNOWN_ACTION",
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		Governance:        &commonv1.GovernanceMetadata{},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrUnknownActionType {
		t.Errorf("Expected ErrUnknownActionType, got %v", err)
	}
}

func TestTransactionVerifier_TransactionExpired(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create an envelope with expired timestamp
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "EXECUTE_BASH",
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(-1 * time.Hour)), // expired
		Nonce:             "test-nonce",
		Governance:        &commonv1.GovernanceMetadata{},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrTransactionExpired {
		t.Errorf("Expected ErrTransactionExpired, got %v", err)
	}
}

func TestTransactionValidator_TransactionReplay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH", "FS_LIST"} // FS_LIST is not a mutation

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create a valid envelope
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "FS_LIST",
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance:        &commonv1.GovernanceMetadata{},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	// First call should succeed
	_, err = verifier.VerifyTransaction(bytes)
	if err != nil {
		t.Fatalf("First verification failed: %v", err)
	}

	// Second call with same nonce should fail (replay)
	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrTransactionReplay {
		t.Errorf("Expected ErrTransactionReplay, got %v", err)
	}
}

func TestTransactionVerifier_StateRootMismatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("current-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH", "FS_LIST"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create an envelope with mismatched state root
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "FS_LIST",
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "wrong-root", // mismatched
		Governance:        &commonv1.GovernanceMetadata{},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrStateRootMismatch {
		t.Errorf("Expected ErrStateRootMismatch, got %v", err)
	}
}

func TestTransactionVerifier_StateRootMissing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH", "FS_LIST"}

	// Test with nil state root provider (should fail when state root is required)
	verifier := NewTransactionVerifier(logger, replayStore, nil, trustedSigners, l3Verifier, knownActionTypes)

	// Create an envelope with state root but no provider
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "FS_LIST",
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "some-root",
		Governance:        &commonv1.GovernanceMetadata{},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrStateRootMissing {
		t.Errorf("Expected ErrStateRootMissing, got %v", err)
	}
}

func TestTransactionVerifier_L2SignatureMissing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH", "FS_LIST"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create an envelope with L2 governance but missing signature
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "FS_LIST",
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				TribunalSignature: "", // missing
				KeyId:             "test-key",
			},
		},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrL2SignatureMissing {
		t.Errorf("Expected ErrL2SignatureMissing, got %v", err)
	}
}

func TestTransactionVerifier_L2KeyNotConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey) // empty
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH", "FS_LIST"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create an envelope with L2 signature but key not in trusted signers
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "FS_LIST",
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				TribunalSignature: "test-sig",
				KeyId:             "unknown-key", // not in trusted signers
			},
		},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrL2KeyNotConfigured {
		t.Errorf("Expected ErrL2KeyNotConfigured, got %v", err)
	}
}

func TestTransactionVerifier_L3SignatureMissing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create an envelope for a mutation without L3 signature
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "EXECUTE_BASH", // mutation
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance: &commonv1.GovernanceMetadata{
			L3: &commonv1.L3Metadata{
				HumanSignature: "", // missing
				PublicKey:      "",
			},
		},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrL3SignatureMissing {
		t.Errorf("Expected ErrL3SignatureMissing, got %v", err)
	}
}

func TestTransactionVerifier_L3VerifierNotConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	var l3Verifier L3Verifier = nil // not configured
	knownActionTypes := []string{"EXECUTE_BASH"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create an envelope for a mutation with L3 signature but no verifier
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "EXECUTE_BASH", // mutation
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance: &commonv1.GovernanceMetadata{
			L3: &commonv1.L3Metadata{
				HumanSignature: "test-sig",
				PublicKey:      "test-pubkey",
			},
		},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrL3VerifierNotConfigured {
		t.Errorf("Expected ErrL3VerifierNotConfigured, got %v", err)
	}
}

func TestTransactionVerifier_L3SignatureInvalid(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: false} // will fail verification
	knownActionTypes := []string{"EXECUTE_BASH"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create an envelope for a mutation with L3 signature that will fail verification
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "EXECUTE_BASH", // mutation
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance: &commonv1.GovernanceMetadata{
			L3: &commonv1.L3Metadata{
				HumanSignature: "test-sig",
				PublicKey:      "test-pubkey",
			},
		},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrL3SignatureInvalid {
		t.Errorf("Expected ErrL3SignatureInvalid, got %v", err)
	}
}

func TestTransactionVerifier_ValidNonMutation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"FS_LIST"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create a valid envelope for a non-mutation (no L3 required)
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "FS_LIST", // not a mutation
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance:        &commonv1.GovernanceMetadata{},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	// Should succeed
	_, err = verifier.VerifyTransaction(bytes)
	if err != nil {
		t.Errorf("Expected verification to succeed for valid non-mutation, got %v", err)
	}
}

func TestTransactionVerifier_ValidMutationWithL3(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")
	trustedSigners := make(map[string]ed25519.PublicKey)
	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"EXECUTE_BASH"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create a valid envelope for a mutation with L3 signature
	envelope := &commonv1.UniversalEnvelope{
		Id:                "test-id",
		ActionType:        "EXECUTE_BASH", // mutation
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance: &commonv1.GovernanceMetadata{
			L3: &commonv1.L3Metadata{
				HumanSignature: "test-sig",
				PublicKey:      "test-pubkey",
			},
		},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	// Should succeed
	_, err = verifier.VerifyTransaction(bytes)
	if err != nil {
		t.Errorf("Expected verification to succeed for valid mutation with L3, got %v", err)
	}
}

func TestTransactionVerifier_ValidL2Signature(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")

	// Create a valid ED25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	trustedSigners := map[string]ed25519.PublicKey{
		"test-key": pubKey,
	}

	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"FS_LIST"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create a valid envelope with L2 signature
	messageID := "test-id"
	payload := messageID + "|true"
	signature := ed25519.Sign(privKey, []byte(payload))
	sigHex := hex.EncodeToString(signature)

	envelope := &commonv1.UniversalEnvelope{
		Id:                messageID,
		ActionType:        "FS_LIST",
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				TribunalSignature: sigHex,
				KeyId:             "test-key",
			},
		},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	// Should succeed
	_, err = verifier.VerifyTransaction(bytes)
	if err != nil {
		t.Errorf("Expected verification to succeed with valid L2 signature, got %v", err)
	}
}

func TestTransactionVerifier_L2SignatureInvalid(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	replayStore := newMockReplayStore()
	stateRootProvider := newMockStateRootProvider("test-root")

	// Create a valid ED25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	trustedSigners := map[string]ed25519.PublicKey{
		"test-key": pubKey,
	}

	l3Verifier := &mockL3Verifier{shouldPass: true}
	knownActionTypes := []string{"FS_LIST"}

	verifier := NewTransactionVerifier(logger, replayStore, stateRootProvider, trustedSigners, l3Verifier, knownActionTypes)

	// Create an envelope with invalid L2 signature (wrong payload)
	messageID := "test-id"
	wrongPayload := "wrong-payload|true"
	signature := ed25519.Sign(privKey, []byte(wrongPayload))
	sigHex := hex.EncodeToString(signature)

	envelope := &commonv1.UniversalEnvelope{
		Id:                messageID,
		ActionType:        "FS_LIST",
		Payload:           []byte("test payload"),
		OperatorId:        "test-operator",
		OperatorSessionId: "test-session",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(1 * time.Hour)),
		Nonce:             "test-nonce",
		StateMerkleRoot:   "test-root",
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				TribunalSignature: sigHex,
				KeyId:             "test-key",
			},
		},
	}

	bytes, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal envelope: %v", err)
	}

	_, err = verifier.VerifyTransaction(bytes)
	if err != ErrL2SignatureInvalid {
		t.Errorf("Expected ErrL2SignatureInvalid, got %v", err)
	}
}

// testLogger is a minimal logger implementation for testing
type testLogger struct{}

func newTestLogger() *testLogger {
	return &testLogger{}
}

func (l *testLogger) Log(msg string, args ...interface{})   {}
func (l *testLogger) Debug(msg string, args ...interface{}) {}
func (l *testLogger) Info(msg string, args ...interface{})  {}
func (l *testLogger) Warn(msg string, args ...interface{})  {}
func (l *testLogger) Error(msg string, args ...interface{}) {}
