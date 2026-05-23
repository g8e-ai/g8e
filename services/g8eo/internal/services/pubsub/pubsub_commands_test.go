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

package pubsub

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewPubSubCommandService(t *testing.T) {
	t.Parallel()
	t.Run("creates service successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:            cfg,
			Logger:            testutil.NewTestLogger(),
			PubSubClient:      NewMockOperatorPubSubClient(),
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

func TestNewPubSubCommandService_StartsWithoutTrustedSignersButRejectsL2(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	cfg.PKIDir = filepath.Join(t.TempDir(), "pki")
	cfg.Gateway.Posture = config.PostureConsensus // Set Consensus posture to enforce L2
	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:            cfg,
		Logger:            testutil.NewTestLogger(),
		PubSubClient:      NewMockOperatorPubSubClient(),
		ReplayStore:       &testutil.MockReplayStore{},
		StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
		TransactionAudit:  &testutil.MockTransactionAudit{},
		L3Notary:          &testutil.MockL3Notary{},
	})
	require.NoError(t, err)
	require.NotNil(t, svc.transactionVerifier)

	_, signerPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	env := unsignedSignerEnvelope(t, signerPriv)

	_, err = svc.transactionVerifier.VerifyEnvelope(env)
	require.Error(t, err)
	assert.True(t, errors.Is(err, governance.ErrL2KeyNotConfigured), "expected missing L2 key error, got %v", err)
}

func unsignedSignerEnvelope(t *testing.T, signerPriv ed25519.PrivateKey) *uap.UAPEnvelope {
	t.Helper()
	req := &operatorv1.FsListRequested{Path: ".", ExecutionId: "exec-1"}
	payload, err := proto.Marshal(req)
	require.NoError(t, err)
	env := &uap.UAPEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().UTC().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_G8EE,
		OperatorId:        "operator-1",
		OperatorSessionId: "session-1",
		ActionType:        string(constants.ActionTypeFsList),
		TargetResource:    "localhost",
		Payload:           payload,
		StateMerkleRoot:   "test-state-root",
		Nonce:             "nonce-missing-signer",
	}
	hash, err := uap.GenerateMessageID(env)
	require.NoError(t, err)
	env.Id = hash
	env.TransactionHash = hash
	env.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			KeyId:             "missing-key",
			TribunalSignature: hex.EncodeToString(ed25519.Sign(signerPriv, []byte(hash+"|true"))),
		},
	}
	return env
}

func TestPubSubCommandService_ProcessEnvelope(t *testing.T) {
	t.Parallel()
	f := newPubsubFixture(t)

	t.Run("successful synchronous processing", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FsListRequested{Path: ".", ExecutionId: "exec-sync"}
		payload, _ := proto.Marshal(req)

		env := &commonv1.GovernanceEnvelope{
			Id:              "tx-sync",
			TransactionHash: "hash-sync",
			ProtocolVersion: "1.0",
			Timestamp:       timestamppb.Now(),
			ExpiresAt:       timestamppb.New(time.Now().Add(time.Hour)),
			ActionType:      string(constants.ActionTypeFsList),
			TargetResource:  "localhost",
			Payload:         payload,
			StateMerkleRoot: "test-state-root",
			Nonce:           "nonce-sync",
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					KeyId: "test-key",
				},
			},
		}

		// Re-hash for verifier
		env.TransactionHash, _ = uap.GenerateMessageID(env)
		env.Id = env.TransactionHash

		// Sign for verifier
		l2Payload := fmt.Sprintf("%s|true", env.TransactionHash)
		sig := ed25519.Sign(f.SignerPriv, []byte(l2Payload))
		env.Governance.L2.TribunalSignature = hex.EncodeToString(sig)

		uapBytes, _ := (protojson.MarshalOptions{}).Marshal(env)

		receipt, err := f.Svc.ProcessEnvelope(context.Background(), uapBytes)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		require.Equal(t, env.Id, receipt.TransactionId)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
	})
}
