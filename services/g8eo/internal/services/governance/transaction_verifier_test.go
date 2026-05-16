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
	"log/slog"
	"os"
	"testing"
	"time"

	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockReplayStore struct {
	nonces map[string]bool
}

func newMockReplayStore() *mockReplayStore {
	return &mockReplayStore{nonces: make(map[string]bool)}
}

func (m *mockReplayStore) CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error) {
	if m.nonces[nonce] {
		return true, nil
	}
	m.nonces[nonce] = true
	return false, nil
}

type mockStateRootProvider struct {
	root string
}

func (m *mockStateRootProvider) GetCurrentStateRoot() (string, error) {
	return m.root, nil
}

type mockL3Verifier struct {
	shouldPass bool
}

func (m *mockL3Verifier) VerifyL3Proof(userID, transactionHash string, proof *commonv1.L3Proof) (bool, error) {
	return m.shouldPass, nil
}

func newStrictVerifier(t *testing.T, replayStore ReplayStore, stateRootProvider StateRootProvider, l3Verifier L3Verifier) (*TransactionVerifier, ed25519.PrivateKey) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate signer: %v", err)
	}
	return NewTransactionVerifier(
		slog.New(slog.NewTextHandler(os.Stdout, nil)),
		replayStore,
		stateRootProvider,
		&SimpleSignerStore{Signers: map[string]ed25519.PublicKey{"test-key": pubKey}},
		l3Verifier,
		[]string{"EXECUTE_BASH", "FS_LIST"},
	), privKey
}

func typedPayload(t *testing.T, actionType string) []byte {
	t.Helper()
	var msg proto.Message
	switch actionType {
	case "EXECUTE_BASH":
		msg = &operatorv1.CommandRequested{Command: "uptime", ExecutionId: "exec-1", Justification: "test"}
	case "FS_LIST":
		msg = &operatorv1.FsListRequested{Path: ".", ExecutionId: "exec-1"}
	default:
		t.Fatalf("unsupported action type: %s", actionType)
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	return payload
}

func signedEnvelope(t *testing.T, actionType string, payload []byte, privKey ed25519.PrivateKey) *uap.UAPEnvelope {
	t.Helper()
	env := &uap.UAPEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().UTC().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_G8EE,
		OperatorId:        "operator-1",
		OperatorSessionId: "session-1",
		ActionType:        actionType,
		TargetResource:    "localhost",
		Payload:           payload,
		StateMerkleRoot:   "root-1",
		Nonce:             "nonce-" + actionType + "-" + hex.EncodeToString(payload[:4]),
	}
	hash, err := uap.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("failed to generate transaction hash: %v", err)
	}
	env.Id = hash
	env.TransactionHash = hash
	env.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			KeyId:             "test-key",
			TribunalSignature: hex.EncodeToString(ed25519.Sign(privKey, []byte(hash+"|true"))),
		},
	}
	if actionType == "EXECUTE_BASH" {
		env.Governance.L3 = &commonv1.L3Metadata{
			Proof: &commonv1.L3Proof{
				Signature: "human-proof",
			},
		}
	}
	return env
}

func TestTransactionVerifier_AcceptsValidNonMutationUAPEnvelope(t *testing.T) {
	verifier, privKey := newStrictVerifier(t, newMockReplayStore(), &mockStateRootProvider{root: "root-1"}, &mockL3Verifier{shouldPass: true})
	env := signedEnvelope(t, "FS_LIST", typedPayload(t, "FS_LIST"), privKey)

	verified, err := verifier.VerifyEnvelope(env)
	if err != nil {
		t.Fatalf("expected verification to pass, got %v", err)
	}
	if verified.DecodedPayload == nil || verified.ActionType != "FS_LIST" {
		t.Fatalf("verified transaction missing decoded payload or action: %#v", verified)
	}
}

func TestTransactionVerifier_AcceptsValidMutationUAPEnvelopeWithL3(t *testing.T) {
	verifier, privKey := newStrictVerifier(t, newMockReplayStore(), &mockStateRootProvider{root: "root-1"}, &mockL3Verifier{shouldPass: true})
	env := signedEnvelope(t, "EXECUTE_BASH", typedPayload(t, "EXECUTE_BASH"), privKey)

	_, err := verifier.VerifyEnvelope(env)
	if err != nil {
		t.Fatalf("expected verification to pass, got %v", err)
	}
}

func TestTransactionVerifier_FailClosedProofs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*uap.UAPEnvelope)
		want   error
	}{
		{name: "missing id", mutate: func(env *uap.UAPEnvelope) { env.Id = "" }, want: ErrTransactionIDMissing},
		{name: "unknown action", mutate: func(env *uap.UAPEnvelope) { env.ActionType = "UNKNOWN" }, want: ErrUnknownActionType},
		{name: "missing payload", mutate: func(env *uap.UAPEnvelope) { env.Payload = nil }, want: ErrPayloadMissing},
		{name: "invalid typed payload", mutate: func(env *uap.UAPEnvelope) { env.Payload = []byte("not protobuf") }, want: ErrPayloadDecodeFailed},
		{name: "missing transaction hash", mutate: func(env *uap.UAPEnvelope) { env.TransactionHash = "" }, want: ErrTransactionHashMissing},
		{name: "hash mismatch", mutate: func(env *uap.UAPEnvelope) { env.TransactionHash = "wrong" }, want: ErrTransactionHashMismatch},
		{name: "expired", mutate: func(env *uap.UAPEnvelope) {
			env.ExpiresAt = timestamppb.New(time.Now().UTC().Add(-time.Minute))
			rehash(t, env)
		}, want: ErrTransactionExpired},
		{name: "missing nonce", mutate: func(env *uap.UAPEnvelope) { env.Nonce = ""; rehash(t, env) }, want: ErrNonceMissing},
		{name: "missing state root", mutate: func(env *uap.UAPEnvelope) { env.StateMerkleRoot = ""; rehash(t, env) }, want: ErrStateRootRequired},
		{name: "missing l2", mutate: func(env *uap.UAPEnvelope) { env.Governance.L2 = nil }, want: ErrL2SignatureMissing},
		{name: "missing l2 key", mutate: func(env *uap.UAPEnvelope) { env.Governance.L2.KeyId = "" }, want: ErrL2KeyNotConfigured},
		{name: "invalid l2 signature", mutate: func(env *uap.UAPEnvelope) { env.Governance.L2.TribunalSignature = "deadbeef" }, want: ErrL2SignatureInvalid},
		{name: "missing l3", mutate: func(env *uap.UAPEnvelope) { env.Governance.L3 = nil }, want: ErrL3ProofMissing},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verifier, privKey := newStrictVerifier(t, newMockReplayStore(), &mockStateRootProvider{root: "root-1"}, &mockL3Verifier{shouldPass: true})
			env := signedEnvelope(t, "EXECUTE_BASH", typedPayload(t, "EXECUTE_BASH"), privKey)
			tc.mutate(env)

			_, err := verifier.VerifyEnvelope(env)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestTransactionVerifier_ReplayAndStateRootReject(t *testing.T) {
	t.Run("replayed nonce", func(t *testing.T) {
		replayStore := newMockReplayStore()
		verifier, privKey := newStrictVerifier(t, replayStore, &mockStateRootProvider{root: "root-1"}, &mockL3Verifier{shouldPass: true})
		env := signedEnvelope(t, "FS_LIST", typedPayload(t, "FS_LIST"), privKey)
		if _, err := verifier.VerifyEnvelope(env); err != nil {
			t.Fatalf("first verification failed: %v", err)
		}
		_, err := verifier.VerifyEnvelope(env)
		if !errors.Is(err, ErrTransactionReplay) {
			t.Fatalf("expected replay rejection, got %v", err)
		}
	})

	t.Run("state root mismatch", func(t *testing.T) {
		verifier, privKey := newStrictVerifier(t, newMockReplayStore(), &mockStateRootProvider{root: "other-root"}, &mockL3Verifier{shouldPass: true})
		env := signedEnvelope(t, "FS_LIST", typedPayload(t, "FS_LIST"), privKey)
		_, err := verifier.VerifyEnvelope(env)
		if !errors.Is(err, ErrStateRootMismatch) {
			t.Fatalf("expected state root mismatch, got %v", err)
		}
	})
}

func TestTransactionVerifier_MissingVerifierDependenciesReject(t *testing.T) {
	t.Run("missing replay store", func(t *testing.T) {
		verifier, privKey := newStrictVerifier(t, nil, &mockStateRootProvider{root: "root-1"}, &mockL3Verifier{shouldPass: true})
		env := signedEnvelope(t, "FS_LIST", typedPayload(t, "FS_LIST"), privKey)
		_, err := verifier.VerifyEnvelope(env)
		if !errors.Is(err, ErrReplayStoreMissing) {
			t.Fatalf("expected replay store rejection, got %v", err)
		}
	})

	t.Run("missing state root provider", func(t *testing.T) {
		verifier, privKey := newStrictVerifier(t, newMockReplayStore(), nil, &mockL3Verifier{shouldPass: true})
		env := signedEnvelope(t, "FS_LIST", typedPayload(t, "FS_LIST"), privKey)
		_, err := verifier.VerifyEnvelope(env)
		if !errors.Is(err, ErrStateRootMissing) {
			t.Fatalf("expected state root provider rejection, got %v", err)
		}
	})

	t.Run("missing l3 verifier", func(t *testing.T) {
		verifier, privKey := newStrictVerifier(t, newMockReplayStore(), &mockStateRootProvider{root: "root-1"}, nil)
		env := signedEnvelope(t, "EXECUTE_BASH", typedPayload(t, "EXECUTE_BASH"), privKey)
		_, err := verifier.VerifyEnvelope(env)
		if !errors.Is(err, ErrL3VerifierNotConfigured) {
			t.Fatalf("expected l3 verifier rejection, got %v", err)
		}
	})
}

func rehash(t *testing.T, env *uap.UAPEnvelope) {
	t.Helper()
	hash, err := uap.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("failed to regenerate transaction hash: %v", err)
	}
	env.Id = hash
	env.TransactionHash = hash
}
