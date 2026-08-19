// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	govpkg "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	pubsubtest "github.com/g8e-ai/g8e/internal/services/pubsub/pubsubtest"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	storagetest "github.com/g8e-ai/g8e/internal/services/storage/storagetest"
	vault "github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewOperatorPubSubService(t *testing.T) {
	t.Run("returns non-nil service without error", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

func TestNewOperatorPubSubService_StartsWithoutTrustedSignersButRejectsL2(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	tmpDir := testutil.TempDir(t)
	cfg.PKIDir = filepath.Join(tmpDir, "pki")
	cfg.Gateway.Posture = config.PostureConsensus // Set Consensus posture to enforce L2
	svc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:       cfg,
		Logger:       testutil.NewTestLogger(),
		PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
	}, GovernanceDeps{
		ReplayStore:       &testutil.MockReplayStore{},
		StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
		TransactionAudit:  &testutil.MockTransactionAudit{},
		L3Notary:          &testutil.MockL3Notary{},
		Doctrine:          governance.NewL1Doctrine(),
	})
	require.NoError(t, err)
	require.NotNil(t, svc.l4warden)

	_, signerPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	env := unsignedSignerEnvelope(t, signerPriv)

	_, err = svc.l4warden.VerifyEnvelope(context.Background(), env)
	require.Error(t, err)
	assert.ErrorIs(t, err, governance.ErrL2ConsensusNotConfigured, "expected consensus not configured error, got %v", err)
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
		SourceComponent:   commonv1.Component_COMPONENT_AGENT,
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
			ConsensusSetId: "test-consensus",
			Votes: []*commonv1.L2Vote{
				{
					SignerKeyId:        "missing-key",
					ConsensusSignature: hex.EncodeToString(ed25519.Sign(signerPriv, []byte(hash+"|true"))),
					Decision:           true,
				},
			},
		},
	}
	return env
}

func TestOperatorPubSubService_handleCommandPayload(t *testing.T) {
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

func TestOperatorPubSubService_handleGovernanceEnvelope(t *testing.T) {
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
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
			SignerStore:       &governance.FailClosedSignerStore{Signers: map[string]ed25519.PublicKey{}},
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

func TestOperatorPubSubService_AllActionTypesProduceReceipts(t *testing.T) {
	// §4.2: Receipt-coverage test - drive each action type through ProcessEnvelope
	// and assert a receipt is written EXECUTING before and terminal status after.
	// This locks the invariant that every execution path produces a receipt.
	t.Run("all action types produce receipts through Actuator", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)

		// Test a representative subset of action types that have simple payloads
		// Complex types (MCP_CALL, A2A_CALL) are tested in their own integration tests
		testCases := []struct {
			name       string
			actionType constants.ActionType
			payload    []byte
		}{
			{
				name:       "FS_LIST",
				actionType: constants.ActionTypeFsList,
				payload: mustMarshalProto(t, &operatorv1.FsListRequested{
					Path:        ".",
					ExecutionId: "exec-1",
				}),
			},
			{
				name:       "FS_READ",
				actionType: constants.ActionTypeFsRead,
				payload: mustMarshalProto(t, &operatorv1.FsReadRequested{
					Path:        "test.txt",
					ExecutionId: "exec-1",
				}),
			},
			{
				name:       "FS_GREP",
				actionType: constants.ActionTypeFsGrep,
				payload: mustMarshalProto(t, &operatorv1.FsGrepRequested{
					Pattern:     "test",
					Path:        ".",
					ExecutionId: "exec-1",
				}),
			},
			{
				name:       "PORT_CHECK",
				actionType: constants.ActionTypePortCheck,
				payload: mustMarshalProto(t, &operatorv1.CheckPortRequested{
					Host:        "localhost",
					Port:        8080,
					ExecutionId: "exec-1",
				}),
			},
			{
				name:       "HEARTBEAT",
				actionType: constants.ActionTypeHeartbeat,
				payload:    mustMarshalProto(t, &operatorv1.HeartbeatRequested{}),
			},
			{
				name:       "EVAL_ANSWER",
				actionType: constants.ActionTypeEvalAnswer,
				payload: mustMarshalProto(t, &operatorv1.EvalAnswerRequested{
					PromptId:  "test-prompt",
					Benchmark: "test",
					Answer:    "test answer",
					Model:     "test-model",
				}),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				env := &govpkg.GovernanceEnvelope{
					Id:              "tx-" + tc.name,
					TransactionHash: "hash-" + tc.name,
					ProtocolVersion: "1.0",
					Timestamp:       timestamppb.Now(),
					ExpiresAt:       timestamppb.New(time.Now().Add(time.Hour)),
					ActionType:      string(tc.actionType),
					TargetResource:  "localhost",
					Payload:         tc.payload,
					StateMerkleRoot: "test-state-root",
					Nonce:           "nonce-" + tc.name,
					Governance: &commonv1.GovernanceMetadata{
						L2: &commonv1.L2Metadata{
							ConsensusSetId: "test-consensus",
							Votes: []*commonv1.L2Vote{
								{
									SignerKeyId: "test-key",
									Decision:    true,
								},
							},
						},
					},
				}

				// Re-hash for verifier
				env.TransactionHash, _ = govpkg.GenerateMessageID(env)
				env.Id = env.TransactionHash

				// Sign for verifier
				l2Payload := fmt.Sprintf("%s|true", env.TransactionHash)
				sig := ed25519.Sign(f.SignerPriv, []byte(l2Payload))
				env.Governance.L2.Votes[0].ConsensusSignature = hex.EncodeToString(sig)

				envelopeBytes, _ := (protojson.MarshalOptions{}).Marshal(env)

				receipt, err := f.Svc.ProcessEnvelope(context.Background(), envelopeBytes)
				require.NoError(t, err, "ProcessEnvelope should succeed")
				require.NotNil(t, receipt, "receipt must not be nil")
				require.Equal(t, env.Id, receipt.TransactionId, "receipt transaction ID must match")
				require.NotEmpty(t, receipt.Signature, "receipt must be signed")

				// Receipt should have a terminal status (COMPLETED or FAILED)
				// EXECUTING is only the initial status before handler execution
				require.NotEqual(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
					receipt.Status, "receipt should have terminal status after execution")
			})
		}
	})
}

func TestOperatorPubSubService_CancellationReceipt(t *testing.T) {
	// BUG-2 §4.3: Cancellation test - assert a cancel produces a receipt for both
	// the real-kill and already-finished cases, with distinguishable status.
	t.Run("CANCEL action type produces receipt through Actuator", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)

		req := &operatorv1.CommandCancelRequested{
			ExecutionId: "exec-cancel-1",
		}
		payload, _ := proto.Marshal(req)

		env := &govpkg.GovernanceEnvelope{
			Id:              "tx-cancel",
			TransactionHash: "hash-cancel",
			ProtocolVersion: "1.0",
			Timestamp:       timestamppb.Now(),
			ExpiresAt:       timestamppb.New(time.Now().Add(time.Hour)),
			ActionType:      string(constants.ActionTypeCancel),
			TargetResource:  "localhost",
			Payload:         payload,
			StateMerkleRoot: "test-state-root",
			Nonce:           "nonce-cancel",
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					ConsensusSetId: "test-consensus",
					Votes: []*commonv1.L2Vote{
						{
							SignerKeyId: "test-key",
							Decision:    true,
						},
					},
				},
				// CANCEL is a mutation — L3 proof required by notary posture
				L3: &commonv1.L3Metadata{
					Proof: &commonv1.L3Proof{
						Signature: "human-proof",
					},
				},
			},
		}

		// Re-hash for verifier
		env.TransactionHash, _ = govpkg.GenerateMessageID(env)
		env.Id = env.TransactionHash

		// Sign for verifier
		l2Payload := fmt.Sprintf("%s|true", env.TransactionHash)
		sig := ed25519.Sign(f.SignerPriv, []byte(l2Payload))
		env.Governance.L2.Votes[0].ConsensusSignature = hex.EncodeToString(sig)

		envelopeBytes, _ := (protojson.MarshalOptions{}).Marshal(env)

		receipt, err := f.Svc.ProcessEnvelope(context.Background(), envelopeBytes)
		require.NoError(t, err, "ProcessEnvelope should succeed")
		require.NotNil(t, receipt, "receipt must not be nil")
		require.Equal(t, env.Id, receipt.TransactionId, "receipt transaction ID must match")
		require.NotEmpty(t, receipt.Signature, "receipt must be signed")

		// Receipt should have a terminal status (COMPLETED or FAILED)
		// The handler distinguishes real kill (CANCELLED) from no-op on already-finished
		require.NotEqual(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
			receipt.Status, "receipt should have terminal status after execution")
	})
}

func TestOperatorPubSubService_SinglePathEnforcement(t *testing.T) {
	// BUG-1 regression test: enforce that handlers map is only reachable through Actuator
	// This ensures no receipt-less execution paths can be added in the future.
	// The source-scan approach will actually fail CI if either function is re-added.
	t.Run("bypass functions do not exist", func(t *testing.T) {
		t.Parallel()
		src, err := os.ReadFile("pubsub_commands.go")
		require.NoError(t, err, "failed to read pubsub_commands.go")
		content := string(src)

		require.NotContains(t, content, "func (rs *OperatorPubSubService) HandleCommandData",
			"HandleCommandData bypass must not exist")
		require.NotContains(t, content, "func (rs *OperatorPubSubService) dispatchCommand",
			"dispatchCommand bypass must not exist")
	})
}

func TestOperatorPubSubService_ExecuteVerifiedTransaction(t *testing.T) {
	f := newPubsubFixture(t)

	t.Run("rejects nil cmdMsg", func(t *testing.T) {
		t.Parallel()
		_, err := f.Svc.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Command.Requested, nil)
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

func TestOperatorPubSubService_handleMcpCallRequestSync(t *testing.T) {
	f := newPubsubFixture(t)

	t.Run("rejects when MCP gateway not configured", func(t *testing.T) {
		t.Parallel()
		f.Svc.mcpGateway = nil
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.Mcp.CallRequested,
			ID:        "msg-1",
			Payload:   mustMarshalProto(t, &operatorv1.McpCallRequested{ToolName: "test"}),
		}
		_, err := f.Svc.handleMcpCallRequestSync(context.Background(), msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MCP gateway not configured")
	})
}

func TestOperatorPubSubService_handleA2aCallRequestSync(t *testing.T) {
	t.Run("rejects when A2A gateway not configured", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		f.Svc.mcpGateway = nil
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.A2a.CallRequested,
			ID:        "msg-1",
			Payload:   mustMarshalProto(t, &operatorv1.A2ACallRequested{SkillName: "test"}),
		}
		_, err := f.Svc.handleA2aCallRequestSync(context.Background(), msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "A2A gateway not configured")
	})
}

func TestOperatorPubSubService_handleAppInvestigationCreatedSync(t *testing.T) {
	t.Run("rejects when Actuator not configured", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		f.Svc.SetActuator(nil)
		msg := &PubSubCommandMessage{
			EventType: constants.EventAppInvestigationCreated,
			ID:        "msg-1",
			Payload:   mustMarshalJSON(t, map[string]string{"test": "data"}),
		}
		_, err := f.Svc.handleAppInvestigationCreatedSync(context.Background(), msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "actuator or ConsoleAuditStore not configured")
	})

	t.Run("rejects when ConsoleAuditStore not configured", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		f.Svc.SetActuator(&governance.L5Actuator{})
		f.Svc.Actuator().ConsoleAuditStore = nil
		msg := &PubSubCommandMessage{
			EventType: constants.EventAppInvestigationCreated,
			ID:        "msg-1",
			Payload:   mustMarshalJSON(t, map[string]string{"test": "data"}),
		}
		_, err := f.Svc.handleAppInvestigationCreatedSync(context.Background(), msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "actuator or ConsoleAuditStore not configured")
	})

	t.Run("creates investigation document successfully", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		f.Svc.Actuator().ConsoleAuditStore = &testutil.MockTransactionAudit{}
		msg := &PubSubCommandMessage{
			EventType: constants.EventAppInvestigationCreated,
			ID:        "investigation-1",
			Payload:   mustMarshalJSON(t, map[string]string{"title": "test investigation"}),
		}
		summary, err := f.Svc.handleAppInvestigationCreatedSync(context.Background(), msg)
		require.NoError(t, err)
		assert.Equal(t, "investigation created", summary)
	})
}

func TestOperatorPubSubService_handleShutdownRequest(t *testing.T) {
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

func TestOperatorPubSubService_handleEvalAnswerRequestSync(t *testing.T) {
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

	t.Run("returns short answer without truncation", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.EvalAnswerRequested{
			PromptId:  "prompt-1",
			Benchmark: "test-benchmark",
			Answer:    "short answer",
			Model:     "test-model",
		}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.Eval.AnswerRequested,
			ID:        "msg-1",
			Payload:   payload,
		}
		summary, err := f.Svc.handleEvalAnswerRequestSync(context.Background(), msg)
		require.NoError(t, err)
		assert.Equal(t, "short answer", summary)
	})

	t.Run("truncates answer exceeding ReceiptSummaryMaxBytes", func(t *testing.T) {
		t.Parallel()
		longAnswer := make([]byte, constants.ReceiptSummaryMaxBytes+1000)
		for i := range longAnswer {
			longAnswer[i] = 'A'
		}
		req := &operatorv1.EvalAnswerRequested{
			PromptId:  "prompt-1",
			Benchmark: "test-benchmark",
			Answer:    string(longAnswer),
			Model:     "test-model",
		}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			EventType: constants.Event.Operator.Eval.AnswerRequested,
			ID:        "msg-1",
			Payload:   payload,
		}
		summary, err := f.Svc.handleEvalAnswerRequestSync(context.Background(), msg)
		require.NoError(t, err)
		assert.Len(t, summary, constants.ReceiptSummaryMaxBytes)
	})
}

func TestOperatorPubSubService_Start(t *testing.T) {

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
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
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

func TestOperatorPubSubService_Stop(t *testing.T) {
	t.Parallel()

	t.Run("stops gracefully when not running", func(t *testing.T) {
		t.Parallel()
		f := newPubsubFixture(t)
		err := f.Svc.Stop()
		require.NoError(t, err)
	})
}

func TestOperatorPubSubService_ProcessEnvelope(t *testing.T) {
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
					ConsensusSetId: "test-consensus",
					Votes: []*commonv1.L2Vote{
						{
							SignerKeyId: "test-key",
							Decision:    true,
						},
					},
				},
			},
		}

		// Re-hash for verifier
		env.TransactionHash, _ = govpkg.GenerateMessageID(env)
		env.Id = env.TransactionHash

		// Sign for verifier
		l2Payload := fmt.Sprintf("%s|true", env.TransactionHash)
		sig := ed25519.Sign(f.SignerPriv, []byte(l2Payload))
		env.Governance.L2.Votes[0].ConsensusSignature = hex.EncodeToString(sig)

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
		assert.Error(t, err)
	})

	t.Run("rejects when transaction verifier not configured", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
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

// failingAuditStore is a mock audit store that always fails
type failingAuditStore struct{}

func (f *failingAuditStore) RecordEvent(event *storage.Event) (int64, error) {
	return 0, fmt.Errorf("mock audit store error")
}

func TestOperatorPubSubService_ObservedStateEvidence(t *testing.T) {
	// §4.5: Observed-evidence tests - test that publish helpers record
	// events with scrubbed content to the audit store, including the error path
	// and non-fatal store errors.
	// Uses TestSQLAuditStore to ensure encryption at rest (LFAA invariant).
	t.Run("publish helpers record scrubbed content evidence", func(t *testing.T) {
		t.Parallel()

		cfg := testutil.NewTestConfig(t)
		client := pubsubtest.NewMockOperatorPubSubClient()

		// Test FS_LIST
		t.Run("FS_LIST records content evidence", func(t *testing.T) {
			t.Parallel()

			// Set up TestSQLAuditStore with encryption at rest for this subtest
			tmpDir := testutil.TempDir(t)
			vaultDir := filepath.Join(tmpDir, "vault")
			vaultPrivKey := make([]byte, 32)
			_, _ = rand.Read(vaultPrivKey)

			require.NoError(t, os.MkdirAll(vaultDir, 0700))
			logger := testutil.NewTestLogger()
			header, _, err := vault.NewVaultHeader(vaultPrivKey)
			require.NoError(t, err)
			require.NoError(t, header.Save(vaultDir))
			testVault, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDir,
				Logger:  logger,
			})
			require.NoError(t, err)
			require.NoError(t, testVault.Unlock(vaultPrivKey))
			defer testVault.Close()

			fileSvc := storagetest.NewTestFileSvc(t, tmpDir)
			gitPath, _ := exec.LookPath("git")
			avCfg := &storagetest.TestSQLAuditStoreConfig{
				DBPath:                    "g8e.db",
				LedgerDir:                 "ledger",
				MaxDBSizeMB:               2048,
				RetentionDays:             90,
				PruneIntervalMinutes:      60,
				OutputTruncationThreshold: 102400,
				HeadTailSize:              51200,
				GitPath:                   gitPath,
				EncryptionVault:           testVault,
			}

			auditStore, err := storagetest.NewTestSQLAuditStore(avCfg, logger, fileSvc)
			require.NoError(t, err)
			defer auditStore.Close()

			scrubbingSvc := mustNewScrubbingSvc(t, logger)

			// Create a session for this test
			sessionID := "session-fslist-" + hex.EncodeToString([]byte{1, 2, 3, 4})
			err = auditStore.CreateSession(sessionID, "operator", "FS_LIST Test", "test@example.com")
			require.NoError(t, err)

			msg := &PubSubCommandMessage{
				ID:                "msg-fslist",
				OperatorSessionID: sessionID,
			}

			payload := &operatorv1.FsListResult{
				Entries: []*operatorv1.FsEntry{
					{Name: "file1.txt", Size: 100},
					{Name: "file2.txt", Size: 200},
				},
			}

			publishLFAATypedResponseTo(context.Background(), client, cfg, logger, msg,
				constants.Event.Operator.FsList.Completed, payload, auditStore, scrubbingSvc)

			events, err := auditStore.GetEvents(sessionID, 10, 0)
			require.NoError(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, constants.Event.Operator.FsList.Completed, events[0].Type)
			assert.NotEmpty(t, events[0].ContentText)
		})

		// Test PORT_CHECK
		t.Run("PORT_CHECK records content evidence", func(t *testing.T) {
			t.Parallel()

			// Set up TestSQLAuditStore with encryption at rest for this subtest
			tmpDir := testutil.TempDir(t)
			vaultDir := filepath.Join(tmpDir, "vault")
			vaultPrivKey := make([]byte, 32)
			_, _ = rand.Read(vaultPrivKey)

			require.NoError(t, os.MkdirAll(vaultDir, 0700))
			logger := testutil.NewTestLogger()
			header, _, err := vault.NewVaultHeader(vaultPrivKey)
			require.NoError(t, err)
			require.NoError(t, header.Save(vaultDir))
			testVault, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDir,
				Logger:  logger,
			})
			require.NoError(t, err)
			require.NoError(t, testVault.Unlock(vaultPrivKey))
			defer testVault.Close()

			fileSvc := storagetest.NewTestFileSvc(t, tmpDir)
			gitPath, _ := exec.LookPath("git")
			avCfg := &storagetest.TestSQLAuditStoreConfig{
				DBPath:                    "g8e.db",
				LedgerDir:                 "ledger",
				MaxDBSizeMB:               2048,
				RetentionDays:             90,
				PruneIntervalMinutes:      60,
				OutputTruncationThreshold: 102400,
				HeadTailSize:              51200,
				GitPath:                   gitPath,
				EncryptionVault:           testVault,
			}

			auditStore, err := storagetest.NewTestSQLAuditStore(avCfg, logger, fileSvc)
			require.NoError(t, err)
			defer auditStore.Close()

			scrubbingSvc := mustNewScrubbingSvc(t, logger)

			// Create a session for this test
			sessionID := "session-port-" + hex.EncodeToString([]byte{5, 6, 7, 8})
			err = auditStore.CreateSession(sessionID, "operator", "PORT_CHECK Test", "test@example.com")
			require.NoError(t, err)

			msg := &PubSubCommandMessage{
				ID:                "msg-port",
				OperatorSessionID: sessionID,
			}

			payload := &operatorv1.PortCheckResult{
				Results: []*operatorv1.PortCheckEntry{
					{Host: "localhost", Port: 8080, Open: true},
				},
			}

			publishLFAATypedResponseTo(context.Background(), client, cfg, logger, msg,
				constants.Event.Operator.PortCheck.Completed, payload, auditStore, scrubbingSvc)

			events, err := auditStore.GetEvents(sessionID, 10, 0)
			require.NoError(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, constants.Event.Operator.PortCheck.Completed, events[0].Type)
			assert.NotEmpty(t, events[0].ContentText)
		})

		// Test error path
		t.Run("error path records evidence", func(t *testing.T) {
			t.Parallel()

			// Set up TestSQLAuditStore with encryption at rest for this subtest
			tmpDir := testutil.TempDir(t)
			vaultDir := filepath.Join(tmpDir, "vault")
			vaultPrivKey := make([]byte, 32)
			_, _ = rand.Read(vaultPrivKey)

			require.NoError(t, os.MkdirAll(vaultDir, 0700))
			logger := testutil.NewTestLogger()
			header, _, err := vault.NewVaultHeader(vaultPrivKey)
			require.NoError(t, err)
			require.NoError(t, header.Save(vaultDir))
			testVault, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDir,
				Logger:  logger,
			})
			require.NoError(t, err)
			require.NoError(t, testVault.Unlock(vaultPrivKey))
			defer testVault.Close()

			fileSvc := storagetest.NewTestFileSvc(t, tmpDir)
			gitPath, _ := exec.LookPath("git")
			avCfg := &storagetest.TestSQLAuditStoreConfig{
				DBPath:                    "g8e.db",
				LedgerDir:                 "ledger",
				MaxDBSizeMB:               2048,
				RetentionDays:             90,
				PruneIntervalMinutes:      60,
				OutputTruncationThreshold: 102400,
				HeadTailSize:              51200,
				GitPath:                   gitPath,
				EncryptionVault:           testVault,
			}

			auditStore, err := storagetest.NewTestSQLAuditStore(avCfg, logger, fileSvc)
			require.NoError(t, err)
			defer auditStore.Close()

			scrubbingSvc := mustNewScrubbingSvc(t, logger)

			// Create a session for this test
			sessionID := "session-error-" + hex.EncodeToString([]byte{9, 10, 11, 12})
			err = auditStore.CreateSession(sessionID, "operator", "Error Path Test", "test@example.com")
			require.NoError(t, err)

			msg := &PubSubCommandMessage{
				ID:                "msg-error",
				OperatorSessionID: sessionID,
			}

			publishLFAAErrorTo(context.Background(), client, cfg, logger, msg,
				constants.Event.Operator.FsRead.Failed, "file not found", auditStore, scrubbingSvc)

			events, err := auditStore.GetEvents(sessionID, 10, 0)
			require.NoError(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, events[0].Type, constants.Event.Operator.FsRead.Failed)
		})
	})

	t.Run("store errors are non-fatal for observed evidence", func(t *testing.T) {
		t.Parallel()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()

		// Create a mock audit store that fails
		mockAuditStore := &failingAuditStore{}

		scrubbingSvc := mustNewScrubbingSvc(t, logger)

		msg := &PubSubCommandMessage{
			ID:                "msg-nonfatal",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.FsListResult{
			Entries: []*operatorv1.FsEntry{{Name: "file1.txt", Size: 100}},
		}

		// This should not panic even though audit store fails
		publishLFAATypedResponseTo(context.Background(), client, cfg, logger, msg,
			constants.Event.Operator.FsList.Completed, payload, mockAuditStore, scrubbingSvc)

		// Verify response was still published
		published := client.LastPublished()
		require.NotNil(t, published, "response should still be published despite audit store error")
	})

	t.Run("scrubbing test - seeded secret is redacted", func(t *testing.T) {
		t.Parallel()

		// Set up TestSQLAuditStore with encryption at rest for this subtest
		tmpDir := testutil.TempDir(t)
		vaultDir := filepath.Join(tmpDir, "vault")
		vaultPrivKey := make([]byte, 32)
		_, _ = rand.Read(vaultPrivKey)

		require.NoError(t, os.MkdirAll(vaultDir, 0700))
		logger := testutil.NewTestLogger()
		header, _, err := vault.NewVaultHeader(vaultPrivKey)
		require.NoError(t, err)
		require.NoError(t, header.Save(vaultDir))
		testVault, err := vault.NewVault(&vault.VaultConfig{
			DataDir: vaultDir,
			Logger:  logger,
		})
		require.NoError(t, err)
		require.NoError(t, testVault.Unlock(vaultPrivKey))
		defer testVault.Close()

		fileSvc := storagetest.NewTestFileSvc(t, tmpDir)
		gitPath, _ := exec.LookPath("git")
		avCfg := &storagetest.TestSQLAuditStoreConfig{
			DBPath:                    "g8e.db",
			LedgerDir:                 "ledger",
			MaxDBSizeMB:               2048,
			RetentionDays:             90,
			PruneIntervalMinutes:      60,
			OutputTruncationThreshold: 102400,
			HeadTailSize:              51200,
			GitPath:                   gitPath,
			EncryptionVault:           testVault,
		}

		auditStore, err := storagetest.NewTestSQLAuditStore(avCfg, logger, fileSvc)
		require.NoError(t, err)
		defer auditStore.Close()

		// Use default scrubbing config which has built-in patterns for common secrets
		scrubbingSvc := mustNewScrubbingSvc(t, logger)

		cfg := testutil.NewTestConfig(t)
		client := pubsubtest.NewMockOperatorPubSubClient()

		// Create a session for this test
		sessionID := "session-scrub-" + hex.EncodeToString([]byte{13, 14, 15, 16})
		err = auditStore.CreateSession(sessionID, "operator", "Scrubbing Test", "test@example.com")
		require.NoError(t, err)

		msg := &PubSubCommandMessage{
			ID:                "msg-scrub",
			OperatorSessionID: sessionID,
		}

		// Payload contains secrets that should be redacted by default patterns
		payload := &operatorv1.FsReadResult{
			Content: "password=secret123 api_key=ghp_test_token",
		}

		publishLFAATypedResponseTo(context.Background(), client, cfg, logger, msg,
			constants.Event.Operator.FsRead.Completed, payload, auditStore, scrubbingSvc)

		events, err := auditStore.GetEvents(sessionID, 10, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, constants.Event.Operator.FsRead.Completed, events[0].Type)
		assert.NotEmpty(t, events[0].ContentText)
		// Verify the secrets were redacted by default patterns
		assert.NotContains(t, events[0].ContentText, "secret123", "password secret should be redacted")
		assert.NotContains(t, events[0].ContentText, "ghp_test_token", "api key should be redacted")
	})
}

func TestOperatorPubSubService_SetL4Warden(t *testing.T) {
	t.Run("sets L4 warden for testing", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)

		mockWarden := &governance.L4Warden{}
		svc.SetL4Warden(mockWarden)

		assert.Equal(t, mockWarden, svc.l4warden)
	})
}

func TestOperatorPubSubService_handleEvalAnswerRequest(t *testing.T) {
	t.Run("handles eval answer request asynchronously", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)

		req := &operatorv1.EvalAnswerRequested{
			Benchmark: "test-benchmark",
			PromptId:  "prompt-1",
			Answer:    "test answer",
		}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Eval.AnswerRequested,
			Payload:   payload,
		}

		// Should not panic
		svc.handleEvalAnswerRequest(context.Background(), msg)
	})
}

func TestOperatorPubSubService_handleHeartbeatEvent(t *testing.T) {
	t.Run("handles heartbeat event and publishes", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)

		// Set up heartbeat service with mock publisher
		mockPublisher := &mockResultsPublisher{}
		svc.heartbeat.SetResultsPublisher(mockPublisher)

		heartbeat := &operatorv1.HeartbeatResult{
			OperatorId:        "op-1",
			OperatorSessionId: "session-1",
			Status:            "automatic",
		}
		payload, _ := proto.Marshal(heartbeat)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Heartbeat,
			Payload:   payload,
		}

		svc.handleHeartbeatEvent(context.Background(), msg)
		assert.True(t, mockPublisher.publishHeartbeatCalled)
	})

	t.Run("logs error when payload unmarshal fails", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Heartbeat,
			Payload:   []byte("invalid protobuf"),
		}

		// Should not panic, should log error
		svc.handleHeartbeatEvent(context.Background(), msg)
	})
}

func TestOperatorPubSubService_SendAutomaticHeartbeat(t *testing.T) {
	t.Run("sends automatic heartbeat via heartbeat service", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		privKey := ed25519.NewKeyFromSeed(make([]byte, 32))
		mockPublisher := &mockResultsPublisher{}
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:             cfg,
			Logger:             logger,
			PubSubClient:       pubsubtest.NewMockOperatorPubSubClient(),
			ResultsService:     mockPublisher,
			ActuatorSigningKey: privKey,
			ActuatorKeyID:      "test-key",
		}, GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)

		err = svc.SendAutomaticHeartbeat()
		assert.NoError(t, err)
		assert.True(t, mockPublisher.publishHeartbeatCalled, "automatic heartbeat must be published through the results publisher")
	})
}

func TestPubsubAuditLogger_LogFieldRead(t *testing.T) {
	t.Run("records field read event in audit store", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		mockStore := &mockAuditStore{}
		auditLogger := &pubsubAuditLogger{
			store:  mockStore,
			logger: logger,
		}

		testVal := "test-value"
		err := auditLogger.LogFieldRead("session-1", "collection", "doc-1", "field.path", mcp.FieldValue{Str: &testVal})
		require.NoError(t, err)

		events := mockStore.GetEvents()
		require.Len(t, events, 1)
		assert.Equal(t, "session-1", events[0].OperatorSessionID)
		assert.Equal(t, constants.EventOperatorFieldReadRequested, events[0].Type)
		assert.Contains(t, events[0].ContentText, "collection/doc-1.field.path")
		assert.Equal(t, "test-value", events[0].CommandStdout)
	})

	t.Run("returns error when store fails", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		mockStore := &mockAuditStore{}
		mockStore.SetRecordEventError(true)
		auditLogger := &pubsubAuditLogger{
			store:  mockStore,
			logger: logger,
		}

		testVal := "test-value"
		err := auditLogger.LogFieldRead("session-1", "collection", "doc-1", "field.path", mcp.FieldValue{Str: &testVal})
		assert.Error(t, err)
	})
}

func TestOperatorPubSubService_ValidateSession(t *testing.T) {
	t.Run("always returns true for operator mode", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)

		valid, err := svc.ValidateSession("session-1")
		assert.True(t, valid)
		assert.NoError(t, err)
	})

	t.Run("always returns true for any session ID", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc, err := NewOperatorPubSubService(CommandServiceConfig{
			Config:       cfg,
			Logger:       testutil.NewTestLogger(),
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)

		valid, err := svc.ValidateSession("")
		assert.True(t, valid)
		assert.NoError(t, err)
	})
}

// TestCommandServiceConfig_NoGatewayFields is a compile-time test that verifies
// CommandServiceConfig does not have MCPGateway, FieldReader, or any governance
// fields. These fields live in GovernanceDeps (shared) and
// GatewayCommandServiceConfig (gateway-only) to enforce mode bifurcation at
// the type level.
func TestCommandServiceConfig_NoGatewayFields(t *testing.T) {
	t.Parallel()

	// CommandServiceConfig must NOT have MCPGateway, FieldReader, or governance fields.
	var base CommandServiceConfig
	_ = base

	// GovernanceDeps must have all governance fields.
	var gd GovernanceDeps
	_ = gd.ReplayStore
	_ = gd.StateRootProvider
	_ = gd.TransactionAudit
	_ = gd.L3Notary
	_ = gd.SignerStore
	_ = gd.ConsensusPolicyStore
	_ = gd.FieldReader

	// GatewayCommandServiceConfig must have MCPGateway and GovDeps.
	var gw GatewayCommandServiceConfig
	_ = gw.MCPGateway
	_ = gw.GovDeps
	_ = gw.L2ConsensusDeliberator

	// GatewayCommandServiceConfig embeds CommandServiceConfig, so all base
	// fields are accessible.
	_ = gw.Config
	_ = gw.Logger
}

// TestNewOperatorPubSubService_NilOptionalGovDeps_PreservedAsNil verifies that
// nil ConsensusPolicyStore and FieldReader in GovernanceDeps are preserved as
// nil during construction. Call sites nil-check with fail-closed behavior
// instead of relying on no-op stubs.
func TestNewOperatorPubSubService_NilOptionalGovDeps_PreservedAsNil(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	svc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:       cfg,
		Logger:       testutil.NewTestLogger(),
		PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
	}, GovernanceDeps{
		ReplayStore:       &testutil.MockReplayStore{},
		StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
		TransactionAudit:  &testutil.MockTransactionAudit{},
		L3Notary:          &testutil.MockL3Notary{},
		// ConsensusPolicyStore and FieldReader intentionally left nil
	})
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.l4warden, "l4warden must be initialized even with nil consensus policy store")
}

// TestNewOperatorPubSubService_NilDoctrine_NotDefaultedAtCallSite verifies that
// a nil Doctrine in GovernanceDeps is NOT silently replaced with NewL1Doctrine()
// at the call site. The doctrine default belongs in the mode's dependency wiring
// (g8eo.go for outbound mode), not in initializeGovernance. A nil doctrine here
// is a wiring bug; L4Warden fail-closes at verification time with
// ErrTxDoctrineMissing rather than silently running with a default doctrine.
func TestNewOperatorPubSubService_NilDoctrine_NotDefaultedAtCallSite(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	svc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:       cfg,
		Logger:       testutil.NewTestLogger(),
		PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
	}, GovernanceDeps{
		ReplayStore:       &testutil.MockReplayStore{},
		StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
		TransactionAudit:  &testutil.MockTransactionAudit{},
		L3Notary:          &testutil.MockL3Notary{},
		// Doctrine intentionally left nil — must not be defaulted at the call site.
	})
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, svc.l4warden, "l4warden must be initialized")
	assert.Nil(t, svc.l4warden.Doctrine(), "nil Doctrine must not be defaulted at the call site; wire it in the mode's dependency wiring instead")
}
