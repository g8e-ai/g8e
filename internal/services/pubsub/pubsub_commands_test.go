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

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/testutil"
	govpkg "github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewPubSubCommandService(t *testing.T) {
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
	require.NotNil(t, svc.l4warden)

	_, signerPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	env := unsignedSignerEnvelope(t, signerPriv)

	_, err = svc.l4warden.VerifyEnvelope(context.Background(), env)
	require.Error(t, err)
	assert.True(t, errors.Is(err, governance.ErrL2KeyNotConfigured), "expected missing L2 key error, got %v", err)
}

func unsignedSignerEnvelope(t *testing.T, signerPriv ed25519.PrivateKey) *govpkg.GovernanceEnvelope {
	t.Helper()
	req := &operatorv1.FsListRequested{Path: ".", ExecutionId: "exec-1"}
	payload, err := proto.Marshal(req)
	require.NoError(t, err)
	env := &govpkg.GovernanceEnvelope{
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
	hash, err := govpkg.GenerateMessageID(env)
	require.NoError(t, err)
	env.Id = hash
	env.TransactionHash = hash
	env.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			KeyId:              "missing-key",
			ConsensusSignature: hex.EncodeToString(ed25519.Sign(signerPriv, []byte(hash+"|true"))),
		},
	}
	return env
}

func TestPubSubCommandService_handleCommandPayload(t *testing.T) {
	t.Run("rejects oversized payload", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		largePayload := make([]byte, MaxPayloadSize+1)
		f.Svc.handleCommandPayload(largePayload)
		// Should log error and return without panic
	})

	t.Run("rejects non-JSON payload", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		binaryPayload := []byte{0x00, 0x01, 0x02}
		f.Svc.handleCommandPayload(binaryPayload)
		// Should log error and return without panic
	})
}

func TestPubSubCommandService_handleGovernanceEnvelope(t *testing.T) {
	t.Run("rejects envelope with missing payload", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		env := &govpkg.GovernanceEnvelope{
			ProtocolVersion: "1.0",
			Timestamp:       timestamppb.Now(),
			ExpiresAt:       timestamppb.New(time.Now().Add(time.Hour)),
			ActionType:      string(constants.ActionTypeFsList),
			TargetResource:  "localhost",
			Payload:         nil,
			StateMerkleRoot: "test-state-root",
			Nonce:           "nonce-1",
		}
		f.Svc.handleGovernanceEnvelope(env)
		// Should log error and return without panic
	})

	t.Run("rejects when Actuator missing", func(t *testing.T) {
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
			SignerStore:       &governance.SimpleSignerStore{Signers: map[string]ed25519.PublicKey{}},
		})
		require.NoError(t, err)
		svc.SetActuator(nil)

		req := &operatorv1.FsListRequested{Path: ".", ExecutionId: "exec-1"}
		payload, _ := proto.Marshal(req)
		env := &govpkg.GovernanceEnvelope{
			ProtocolVersion: "1.0",
			Timestamp:       timestamppb.Now(),
			ExpiresAt:       timestamppb.New(time.Now().Add(time.Hour)),
			ActionType:      string(constants.ActionTypeFsList),
			TargetResource:  "localhost",
			Payload:         payload,
			StateMerkleRoot: "test-state-root",
			Nonce:           "nonce-1",
		}
		svc.handleGovernanceEnvelope(env)
		// Should log error and return without panic
	})
}

func TestPubSubCommandService_dispatchCommand(t *testing.T) {
	t.Run("warns on unknown event type", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		msg := &PubSubCommandMessage{
			EventType: "UNKNOWN_EVENT_TYPE",
			ID:        "msg-1",
		}
		f.Svc.dispatchCommand(msg)
		// Should log warning and return without panic
	})
}

func TestPubSubCommandService_ExecuteVerifiedTransaction(t *testing.T) {
	f := newPubsubFixture(t)

	t.Run("rejects invalid cmdMsg type", func(t *testing.T) {
		t.Parallel()
		_, err := f.Svc.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Command.Requested, "invalid type")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cmdMsg type")
	})

	t.Run("rejects when no handler registered", func(t *testing.T) {
		t.Parallel()
		msg := &PubSubCommandMessage{
			EventType: "NONEXISTENT_EVENT",
			ID:        "msg-1",
		}
		_, err := f.Svc.ExecuteVerifiedTransaction(context.Background(), msg.EventType, msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no handler for event type")
	})
}

func TestPubSubCommandService_handleMcpCallRequestSync(t *testing.T) {
	f := newPubsubFixture(t)

	t.Run("rejects when MCP gateway not configured", func(t *testing.T) {
		t.Parallel()
		f.Svc.mcpGateway = nil
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.Mcp.CallRequested,
			ID:        "msg-1",
			Payload:   mustMarshalJSON(t, map[string]interface{}{"tool_name": "test"}),
		}
		_, err := f.Svc.handleMcpCallRequestSync(context.Background(), msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MCP gateway not configured")
	})
}

func TestPubSubCommandService_handleA2aCallRequestSync(t *testing.T) {
	t.Run("rejects when A2A gateway not configured", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		f.Svc.mcpGateway = nil
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.A2a.CallRequested,
			ID:        "msg-1",
			Payload:   mustMarshalJSON(t, map[string]interface{}{"skill_name": "test"}),
		}
		_, err := f.Svc.handleA2aCallRequestSync(context.Background(), msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "A2A gateway not configured")
	})
}

func TestPubSubCommandService_handleAppInvestigationCreatedSync(t *testing.T) {
	t.Run("rejects when Actuator not configured", func(t *testing.T) {
		f := newPubsubFixture(t)
		f.Svc.SetActuator(nil)
		msg := &PubSubCommandMessage{
			EventType: constants.EventAppInvestigationCreated,
			ID:        "msg-1",
			Payload:   mustMarshalJSON(t, map[string]string{"test": "data"}),
		}
		_, err := f.Svc.handleAppInvestigationCreatedSync(context.Background(), msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "actuator or AuditStore not configured")
	})

	t.Run("rejects when AuditStore not configured", func(t *testing.T) {
		f := newPubsubFixture(t)
		f.Svc.SetActuator(&governance.L5Actuator{})
		f.Svc.Actuator().AuditStore = nil
		msg := &PubSubCommandMessage{
			EventType: constants.EventAppInvestigationCreated,
			ID:        "msg-1",
			Payload:   mustMarshalJSON(t, map[string]string{"test": "data"}),
		}
		_, err := f.Svc.handleAppInvestigationCreatedSync(context.Background(), msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "actuator or AuditStore not configured")
	})
}

func TestPubSubCommandService_handleShutdownRequest(t *testing.T) {
	f := newPubsubFixture(t)

	t.Run("rejects unmarshal error", func(t *testing.T) {
		t.Parallel()
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.ShutdownRequested,
			ID:        "msg-1",
			Payload:   []byte("invalid json"),
		}
		f.Svc.handleShutdownRequest(msg)
		// Should log error and return without panic
	})

	t.Run("rejects invalid payload type", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FsListRequested{Path: "."}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.ShutdownRequested,
			ID:        "msg-1",
			Payload:   payload,
		}
		f.Svc.handleShutdownRequest(msg)
		// Should log error and return without panic
	})

	t.Run("handles shutdown with reason", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.ShutdownRequested{Reason: "test shutdown"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.ShutdownRequested,
			ID:        "msg-1",
			Payload:   payload,
		}
		// Drain channel in goroutine to prevent blocking
		go func() {
			<-f.Svc.ShutdownChan
		}()
		f.Svc.handleShutdownRequest(msg)
	})

	t.Run("handles shutdown without reason", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.ShutdownRequested{Reason: ""}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.ShutdownRequested,
			ID:        "msg-1",
			Payload:   payload,
		}
		// Drain channel in goroutine to prevent blocking
		go func() {
			<-f.Svc.ShutdownChan
		}()
		f.Svc.handleShutdownRequest(msg)
	})
}

func TestPubSubCommandService_handleEvalAnswerRequestSync(t *testing.T) {
	f := newPubsubFixture(t)

	t.Run("rejects unmarshal error", func(t *testing.T) {
		t.Parallel()
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.Eval.AnswerRequested,
			ID:        "msg-1",
			Payload:   []byte("invalid json"),
		}
		_, err := f.Svc.handleEvalAnswerRequestSync(context.Background(), msg)
		require.Error(t, err)
	})
}

func TestPubSubCommandService_Start(t *testing.T) {

	t.Run("rejects start when already running", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		ctx := context.Background()
		err := f.Svc.Start(ctx)
		require.NoError(t, err)

		err = f.Svc.Start(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already running")

		f.Svc.Stop()
	})

	t.Run("starts in gateway mode without pub/sub subscription", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		cfg.OperatorID = ""
		cfg.OperatorSessionId = ""
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

		err = svc.Start(context.Background())
		require.NoError(t, err)
		svc.Stop()
	})
}

func TestPubSubCommandService_Stop(t *testing.T) {
	t.Parallel()

	t.Run("stops gracefully when not running", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		err := f.Svc.Stop()
		require.NoError(t, err)
	})
}

func TestPubSubCommandService_ProcessEnvelope(t *testing.T) {
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
		env.TransactionHash, _ = govpkg.GenerateMessageID(env)
		env.Id = env.TransactionHash

		// Sign for verifier
		l2Payload := fmt.Sprintf("%s|true", env.TransactionHash)
		sig := ed25519.Sign(f.SignerPriv, []byte(l2Payload))
		env.Governance.L2.ConsensusSignature = hex.EncodeToString(sig)

		envelopeBytes, _ := (protojson.MarshalOptions{}).Marshal(env)

		receipt, err := f.Svc.ProcessEnvelope(context.Background(), envelopeBytes)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		require.Equal(t, env.Id, receipt.TransactionId)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
	})

	t.Run("rejects empty payload", func(t *testing.T) {
		t.Parallel()
		_, err := f.Svc.ProcessEnvelope(context.Background(), []byte{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty payload")
	})

	t.Run("rejects oversized payload", func(t *testing.T) {
		t.Parallel()
		largePayload := make([]byte, MaxPayloadSize+1)
		_, err := f.Svc.ProcessEnvelope(context.Background(), largePayload)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds")
	})

	t.Run("rejects invalid JSON envelope", func(t *testing.T) {
		t.Parallel()
		invalidJSON := []byte("{invalid json}")
		_, err := f.Svc.ProcessEnvelope(context.Background(), invalidJSON)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid GovernanceEnvelope")
	})

	t.Run("rejects when transaction verifier not configured", func(t *testing.T) {
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
		svc.l4warden = nil

		req := &operatorv1.FsListRequested{Path: ".", ExecutionId: "exec-1"}
		payload, _ := proto.Marshal(req)
		env := &govpkg.GovernanceEnvelope{
			ProtocolVersion: "1.0",
			Timestamp:       timestamppb.Now(),
			ExpiresAt:       timestamppb.New(time.Now().Add(time.Hour)),
			ActionType:      string(constants.ActionTypeFsList),
			TargetResource:  "localhost",
			Payload:         payload,
			StateMerkleRoot: "test-state-root",
			Nonce:           "nonce-1",
		}
		envelopeBytes, _ := protojson.Marshal(env)

		_, err = svc.ProcessEnvelope(context.Background(), envelopeBytes)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transaction verifier not configured")
	})

}
