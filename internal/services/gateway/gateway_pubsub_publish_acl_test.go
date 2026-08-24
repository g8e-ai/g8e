// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	pubsubv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/pubsub/v1"
)

// --- verifyPublishACL table-driven tests ---

func TestVerifyPublishACL(t *testing.T) {
	tests := []struct {
		name           string
		channel        string
		operatorID     string
		spiffeID       string
		wantErr        bool
		wantErrIs      error
		wantErrContain string
	}{
		{
			name:       "app publishes to cmd: channel permitted",
			channel:    "cmd:op-001:sess-001",
			operatorID: "op-001",
			spiffeID:   "spiffe://g8e.local/app/g8ee",
			wantErr:    false,
		},
		{
			name:           "operator publishes to cmd: channel rejected",
			channel:        "cmd:op-001:sess-001",
			operatorID:     "op-001",
			spiffeID:       "spiffe://g8e.local/operator/org-1/op-001/sess-1",
			wantErr:        true,
			wantErrIs:      constants.ErrPubSubPublishUnauthorized,
			wantErrContain: "operators cannot publish to cmd:",
		},
		{
			name:       "operator publishes to own heartbeat channel permitted",
			channel:    "heartbeat:op-001:sess-001",
			operatorID: "op-001",
			spiffeID:   "spiffe://g8e.local/operator/org-1/op-001/sess-1",
			wantErr:    false,
		},
		{
			name:       "operator publishes to own results channel permitted",
			channel:    "results:op-001:sess-001",
			operatorID: "op-001",
			spiffeID:   "spiffe://g8e.local/operator/org-1/op-001/sess-1",
			wantErr:    false,
		},
		{
			name:           "operator publishes to another operator heartbeat rejected",
			channel:        "heartbeat:op-other:sess-001",
			operatorID:     "op-001",
			spiffeID:       "spiffe://g8e.local/operator/org-1/op-001/sess-1",
			wantErr:        true,
			wantErrIs:      constants.ErrPubSubPublishUnauthorized,
			wantErrContain: "channel operator_id mismatch",
		},
		{
			name:           "operator publishes to another operator results rejected",
			channel:        "results:op-other:sess-001",
			operatorID:     "op-001",
			spiffeID:       "spiffe://g8e.local/operator/org-1/op-001/sess-1",
			wantErr:        true,
			wantErrIs:      constants.ErrPubSubPublishUnauthorized,
			wantErrContain: "channel operator_id mismatch",
		},
		{
			name:       "ensemble app publishes to results channel permitted",
			channel:    "results:op-001:sess-001",
			operatorID: "g8ee",
			spiffeID:   "spiffe://g8e.local/app/g8ee",
			wantErr:    false,
		},
		{
			name:       "ensemble app publishes to heartbeat channel permitted",
			channel:    "heartbeat:op-001:sess-001",
			operatorID: "g8ee",
			spiffeID:   "spiffe://g8e.local/app/g8ee",
			wantErr:    false,
		},
		{
			name:       "ensemble app publishes to cmd: channel permitted",
			channel:    "cmd:op-001:sess-001",
			operatorID: "g8ee",
			spiffeID:   "spiffe://g8e.local/app/g8ee",
			wantErr:    false,
		},
		{
			name:           "unknown channel prefix rejected",
			channel:        "unknown:op-001:sess-001",
			operatorID:     "op-001",
			spiffeID:       "spiffe://g8e.local/app/g8ee",
			wantErr:        true,
			wantErrIs:      constants.ErrPubSubPublishUnauthorized,
			wantErrContain: "unknown channel prefix",
		},
		{
			name:       "malformed channel rejected",
			channel:    "cmd",
			operatorID: "op-001",
			spiffeID:   "spiffe://g8e.local/app/g8ee",
			wantErr:    true,
			wantErrIs:  constants.ErrPubSubInvalidChannelFormat,
		},
		{
			name:       "operator with missing operator_id publishing to heartbeat rejected",
			channel:    "heartbeat:op-001:sess-001",
			operatorID: "",
			spiffeID:   "spiffe://g8e.local/operator/org-1/op-001/sess-1",
			wantErr:    true,
			wantErrIs:  constants.ErrPubSubCertificateMissingOperatorID,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := verifyPublishACL(tt.channel, tt.operatorID, tt.spiffeID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}

// --- BuildGovernanceEnvelope unit tests ---

func TestBuildGovernanceEnvelope_Success(t *testing.T) {
	env, err := BuildGovernanceEnvelope(BuildEnvelopeParams{
		OperatorID:        "op-001",
		OperatorSessionID: "sess-001",
		ActionType:        string(constants.ActionTypeFileEdit),
		Payload:           []byte("file-edit-payload"),
		TargetResource:    "/etc/hostname",
		RequestorUserID:   "user-001",
		ActingAppID:       "spiffe://g8e.local/app/g8ee",
		StateMerkleRoot:   "root-abc",
	})
	require.NoError(t, err)
	require.NotNil(t, env)

	assert.Equal(t, "1.0", env.ProtocolVersion)
	assert.Equal(t, "op-001", env.OperatorId)
	assert.Equal(t, "sess-001", env.OperatorSessionId)
	assert.Equal(t, string(constants.ActionTypeFileEdit), env.ActionType)
	assert.Equal(t, "/etc/hostname", env.TargetResource)
	assert.Equal(t, []byte("file-edit-payload"), env.Payload)
	assert.Equal(t, "root-abc", env.StateMerkleRoot)
	assert.NotEmpty(t, env.Nonce, "envelope must have a nonce")
	assert.NotEmpty(t, env.Id, "envelope must have an Id (transaction hash)")
	assert.Equal(t, env.Id, env.TransactionHash, "Id must equal TransactionHash")
	assert.Equal(t, "user-001", env.RequestorUserId)
	assert.Equal(t, "spiffe://g8e.local/app/g8ee", env.ActingAppId)
	require.NotNil(t, env.Governance)
	require.NotNil(t, env.Governance.L1)
	assert.True(t, env.Governance.L1.Validated, "L1 doctrine validation marker must be set")
	assert.NotNil(t, env.Timestamp)
	assert.NotNil(t, env.ExpiresAt)
}

func TestBuildGovernanceEnvelope_DeterministicTxHash(t *testing.T) {
	// Same params (except nonce which is random) produce different tx hashes
	// because the nonce differs. But the envelope structure is consistent.
	params := BuildEnvelopeParams{
		OperatorID:        "op-001",
		OperatorSessionID: "sess-001",
		ActionType:        string(constants.ActionTypeFsRead),
		Payload:           []byte("read-payload"),
		TargetResource:    "/etc/hostname",
		RequestorUserID:   "user-001",
		ActingAppID:       "spiffe://g8e.local/app/g8ee",
		StateMerkleRoot:   "root-abc",
	}

	env1, err := BuildGovernanceEnvelope(params)
	require.NoError(t, err)

	env2, err := BuildGovernanceEnvelope(params)
	require.NoError(t, err)

	// Nonces are random, so transaction hashes must differ.
	assert.NotEqual(t, env1.Id, env2.Id, "transaction hashes must differ due to random nonce")
	// But nonces must be different.
	assert.NotEqual(t, env1.Nonce, env2.Nonce, "nonces must differ")
}

// --- Command intent relay unit tests ---

// newRelayTestBroker creates a GatewayWebSocketHandler wired with stub
// state root provider and session validator for command intent relay
// unit tests. Returns the broker and the stub validator so tests can
// override the validator's return values.
func newRelayTestBroker(t *testing.T, stateRoot string, op *models.OperatorDocumentGo) (*GatewayWebSocketHandler, *stubOperatorSessionValidator) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)
	validator := &stubOperatorSessionValidator{op: op}
	broker.SetCommandRelayDeps(&stubStateRootProvider{root: stateRoot}, validator)
	return broker, validator
}

// newAppSessionHandler creates a pubSubSessionHandler for an app workload
// (spiffe://g8e.local/app/g8ee) with the given operator_id.
func newAppSessionHandler(broker *GatewayWebSocketHandler, operatorID string) *pubSubSessionHandler {
	return &pubSubSessionHandler{
		broker: broker,
		sub: &wsSubscriber{
			buf:              newDropOldestBuf(64),
			done:             make(chan struct{}),
			identitySPIFFEID: "spiffe://g8e.local/app/g8ee",
			operatorID:       operatorID,
		},
	}
}

// newOperatorSessionHandler creates a pubSubSessionHandler for an operator
// workload (spiffe://g8e.local/operator/...).
func newOperatorSessionHandler(broker *GatewayWebSocketHandler, operatorID string) *pubSubSessionHandler {
	return &pubSubSessionHandler{
		broker: broker,
		sub: &wsSubscriber{
			buf:              newDropOldestBuf(64),
			done:             make(chan struct{}),
			identitySPIFFEID: "spiffe://g8e.local/operator/org-1/" + operatorID + "/sess-1",
			operatorID:       operatorID,
		},
	}
}

func TestHandlePublish_AppCommandIntentTransformedToGovernanceEnvelope(t *testing.T) {
	op := &models.OperatorDocumentGo{
		ID:                "op-001",
		OperatorSessionID: "sess-001",
	}
	broker, _ := newRelayTestBroker(t, "root-abc", op)
	handler := newAppSessionHandler(broker, "g8ee")

	// Register an in-process handler on the cmd channel to capture the
	// transformed envelope (simulating the operator subscriber).
	cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)
	var capturedData []byte
	unregister := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		capturedData = data
	})
	defer unregister()

	intent := commandIntent{
		OperatorID:        op.ID,
		OperatorSessionID: op.OperatorSessionID,
		ActionType:        string(constants.ActionTypeFileEdit),
		Payload:           []byte("file-edit-payload"),
		TargetResource:    "/etc/hostname",
		RequestorUserID:   "user-001",
	}
	intentJSON, err := json.Marshal(intent)
	require.NoError(t, err)

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: cmdChannel,
		Data:    intentJSON,
	})

	require.NotNil(t, capturedData, "transformed envelope must be fanned out to cmd channel")

	// Verify the fanned-out envelope is a valid GovernanceEnvelope with
	// the gateway's state root and a valid transaction hash.
	env := &commonv1.GovernanceEnvelope{}
	err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(capturedData, env)
	require.NoError(t, err, "fanned-out frame must be a valid protojson GovernanceEnvelope")

	assert.Equal(t, "root-abc", env.StateMerkleRoot, "envelope must carry the gateway's state root")
	assert.Equal(t, op.ID, env.OperatorId)
	assert.Equal(t, op.OperatorSessionID, env.OperatorSessionId)
	assert.Equal(t, string(constants.ActionTypeFileEdit), env.ActionType)
	assert.Equal(t, "/etc/hostname", env.TargetResource)
	assert.Equal(t, []byte("file-edit-payload"), env.Payload)
	assert.Equal(t, "user-001", env.RequestorUserId)
	assert.Equal(t, "spiffe://g8e.local/app/g8ee", env.ActingAppId)
	assert.NotEmpty(t, env.Nonce)
	assert.NotEmpty(t, env.Id)
	assert.Equal(t, env.Id, env.TransactionHash)
	require.NotNil(t, env.Governance)
	require.NotNil(t, env.Governance.L1)
	assert.True(t, env.Governance.L1.Validated)
}

func TestHandlePublish_OperatorPublishToCmdRejected(t *testing.T) {
	op := &models.OperatorDocumentGo{
		ID:                "op-001",
		OperatorSessionID: "sess-001",
	}
	broker, _ := newRelayTestBroker(t, "root-abc", op)
	handler := newOperatorSessionHandler(broker, "op-001")

	cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)

	// Register a handler to verify nothing is fanned out.
	var called bool
	unregister := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		called = true
	})
	defer unregister()

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: cmdChannel,
		Data:    []byte("should-not-be-relayed"),
	})

	assert.False(t, called, "operator publish to cmd: must be rejected by publish ACL, no fan-out")
}

func TestHandlePublish_OperatorPublishToHeartbeatPermitted(t *testing.T) {
	broker, _ := newRelayTestBroker(t, "root-abc", nil)
	handler := newOperatorSessionHandler(broker, "op-001")

	heartbeatChannel := pubsub.HeartbeatChannel("op-001", "sess-001")

	// Register a handler to verify the publish is fanned out verbatim.
	var capturedData []byte
	unregister := broker.RegisterHandler(heartbeatChannel, func(channel string, data []byte) {
		capturedData = data
	})
	defer unregister()

	payload := []byte("heartbeat-payload")
	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: heartbeatChannel,
		Data:    payload,
	})

	assert.Equal(t, payload, capturedData, "operator publish to heartbeat: must be fanned out verbatim")
}

func TestHandlePublish_OperatorPublishToResultsPermitted(t *testing.T) {
	broker, _ := newRelayTestBroker(t, "root-abc", nil)
	handler := newOperatorSessionHandler(broker, "op-001")

	resultsChannel := pubsub.ResultsChannel("op-001", "sess-001")

	var capturedData []byte
	unregister := broker.RegisterHandler(resultsChannel, func(channel string, data []byte) {
		capturedData = data
	})
	defer unregister()

	payload := []byte("result-payload")
	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: resultsChannel,
		Data:    payload,
	})

	assert.Equal(t, payload, capturedData, "operator publish to results: must be fanned out verbatim")
}

func TestHandlePublish_MalformedCommandIntentDroppedFailClosed(t *testing.T) {
	op := &models.OperatorDocumentGo{
		ID:                "op-001",
		OperatorSessionID: "sess-001",
	}
	broker, _ := newRelayTestBroker(t, "root-abc", op)
	handler := newAppSessionHandler(broker, "g8ee")

	cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)

	var called bool
	unregister := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		called = true
	})
	defer unregister()

	// Publish malformed JSON (not a valid command intent).
	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: cmdChannel,
		Data:    []byte("{invalid json"),
	})

	assert.False(t, called, "malformed command intent must be dropped fail-closed, no fan-out")
}

func TestHandlePublish_InvalidOperatorSessionDroppedFailClosed(t *testing.T) {
	broker, validator := newRelayTestBroker(t, "root-abc", nil)
	validator.err = errors.New("session not found")
	handler := newAppSessionHandler(broker, "g8ee")

	cmdChannel := pubsub.CmdChannel("op-001", "sess-001")

	var called bool
	unregister := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		called = true
	})
	defer unregister()

	intent := commandIntent{
		OperatorID:        "op-001",
		OperatorSessionID: "sess-001",
		ActionType:        string(constants.ActionTypeFileEdit),
		Payload:           []byte("payload"),
		TargetResource:    "/etc/hostname",
		RequestorUserID:   "user-001",
	}
	intentJSON, err := json.Marshal(intent)
	require.NoError(t, err)

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: cmdChannel,
		Data:    intentJSON,
	})

	assert.False(t, called, "invalid operator session must be dropped fail-closed, no fan-out")
}

func TestHandlePublish_StateRootErrorDroppedFailClosed(t *testing.T) {
	op := &models.OperatorDocumentGo{
		ID:                "op-001",
		OperatorSessionID: "sess-001",
	}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)
	broker.SetCommandRelayDeps(
		&stubStateRootProvider{err: errors.New("state root unavailable")},
		&stubOperatorSessionValidator{op: op},
	)
	handler := newAppSessionHandler(broker, "g8ee")

	cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)

	var called bool
	unregister := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		called = true
	})
	defer unregister()

	intent := commandIntent{
		OperatorID:        op.ID,
		OperatorSessionID: op.OperatorSessionID,
		ActionType:        string(constants.ActionTypeFileEdit),
		Payload:           []byte("payload"),
		TargetResource:    "/etc/hostname",
		RequestorUserID:   "user-001",
	}
	intentJSON, err := json.Marshal(intent)
	require.NoError(t, err)

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: cmdChannel,
		Data:    intentJSON,
	})

	assert.False(t, called, "state root error must be dropped fail-closed, no fan-out")
}

func TestHandlePublish_CommandIntentMissingOperatorIDDropped(t *testing.T) {
	op := &models.OperatorDocumentGo{
		ID:                "op-001",
		OperatorSessionID: "sess-001",
	}
	broker, _ := newRelayTestBroker(t, "root-abc", op)
	handler := newAppSessionHandler(broker, "g8ee")

	cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)

	var called bool
	unregister := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		called = true
	})
	defer unregister()

	// Intent with missing operator_id.
	intent := commandIntent{
		OperatorSessionID: "sess-001",
		ActionType:        string(constants.ActionTypeFileEdit),
		Payload:           []byte("payload"),
		RequestorUserID:   "user-001",
	}
	intentJSON, err := json.Marshal(intent)
	require.NoError(t, err)

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: cmdChannel,
		Data:    intentJSON,
	})

	assert.False(t, called, "command intent missing operator_id must be dropped fail-closed")
}

func TestHandlePublish_CommandIntentChannelMismatchDropped(t *testing.T) {
	op := &models.OperatorDocumentGo{
		ID:                "op-001",
		OperatorSessionID: "sess-001",
	}
	broker, _ := newRelayTestBroker(t, "root-abc", op)
	handler := newAppSessionHandler(broker, "g8ee")

	// Publish to op-001's channel but the intent targets op-002.
	cmdChannel := pubsub.CmdChannel("op-001", "sess-001")

	var called bool
	unregister := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		called = true
	})
	defer unregister()

	intent := commandIntent{
		OperatorID:        "op-002",
		OperatorSessionID: "sess-002",
		ActionType:        string(constants.ActionTypeFileEdit),
		Payload:           []byte("payload"),
		RequestorUserID:   "user-001",
	}
	intentJSON, err := json.Marshal(intent)
	require.NoError(t, err)

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: cmdChannel,
		Data:    intentJSON,
	})

	assert.False(t, called, "command intent channel mismatch must be dropped fail-closed")
}

func TestHandlePublish_RelayDisabledWhenDepsNotConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)
	// Do NOT call SetCommandRelayDeps — relay is disabled.
	handler := newAppSessionHandler(broker, "g8ee")

	cmdChannel := pubsub.CmdChannel("op-001", "sess-001")

	var called bool
	unregister := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		called = true
	})
	defer unregister()

	intent := commandIntent{
		OperatorID:        "op-001",
		OperatorSessionID: "sess-001",
		ActionType:        string(constants.ActionTypeFileEdit),
		Payload:           []byte("payload"),
		RequestorUserID:   "user-001",
	}
	intentJSON, err := json.Marshal(intent)
	require.NoError(t, err)

	// Should not panic, should not fan out, should be silently dropped.
	assert.NotPanics(t, func() {
		handler.handleAction(&pubsubv1.PubSubMessage{
			Action:  constants.PubSubActionPublish,
			Channel: cmdChannel,
			Data:    intentJSON,
		})
	})
	assert.False(t, called, "cmd: relay must be dropped when deps are not configured")
}

// TestHandlePublish_OperatorCrossOperatorHeartbeatRejected verifies that
// an operator cannot publish to another operator's heartbeat channel.
func TestHandlePublish_OperatorCrossOperatorHeartbeatRejected(t *testing.T) {
	broker, _ := newRelayTestBroker(t, "root-abc", nil)
	handler := newOperatorSessionHandler(broker, "op-001")

	heartbeatChannel := pubsub.HeartbeatChannel("op-other", "sess-001")

	var called bool
	unregister := broker.RegisterHandler(heartbeatChannel, func(channel string, data []byte) {
		called = true
	})
	defer unregister()

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: heartbeatChannel,
		Data:    []byte("heartbeat"),
	})

	assert.False(t, called, "operator publishing to another operator's heartbeat must be rejected")
}

// TestSetCommandRelayDeps verifies that SetCommandRelayDeps stores the
// dependencies and they are retrievable for the relay path.
func TestSetCommandRelayDeps(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	assert.Nil(t, broker.stateRootProvider, "state root provider must be nil before SetCommandRelayDeps")
	assert.Nil(t, broker.sessionValidator, "session validator must be nil before SetCommandRelayDeps")

	provider := &stubStateRootProvider{root: "root-1"}
	validator := &stubOperatorSessionValidator{op: &models.OperatorDocumentGo{ID: "op-1"}}
	broker.SetCommandRelayDeps(provider, validator)

	broker.mu.RLock()
	assert.NotNil(t, broker.stateRootProvider)
	assert.NotNil(t, broker.sessionValidator)
	broker.mu.RUnlock()
}

// TestHandlePublish_LogsOnACLViolation verifies that ACL violations are
// logged at WARN level for observability.
func TestHandlePublish_LogsOnACLViolation(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	broker := NewGatewayWebSocketHandler(logger)
	handler := newOperatorSessionHandler(broker, "op-001")

	cmdChannel := pubsub.CmdChannel("op-001", "sess-001")
	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: cmdChannel,
		Data:    []byte("payload"),
	})

	logs := logBuf.String()
	assert.Contains(t, logs, "ACL violation", "ACL violation must be logged")
	assert.Contains(t, logs, "level=WARN", "ACL violation must be logged at WARN level")
	assert.True(t, strings.Contains(logs, "cmd:op-001:sess-001"), "log must include the rejected channel")
}
