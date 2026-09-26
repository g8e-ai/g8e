// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govpkg "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	pubsubv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/pubsub/v1"
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
			name:           "app publishes to cmd: channel rejected",
			channel:        "cmd:op-001:sess-001",
			operatorID:     "op-001",
			spiffeID:       "spiffe://g8e.local/app/g8ee",
			wantErr:        true,
			wantErrIs:      constants.ErrPubSubPublishUnauthorized,
			wantErrContain: "gateway-internal only",
		},
		{
			name:           "operator publishes to cmd: channel rejected",
			channel:        "cmd:op-001:sess-001",
			operatorID:     "op-001",
			spiffeID:       "spiffe://g8e.local/operator/org-1/op-001/sess-1",
			wantErr:        true,
			wantErrIs:      constants.ErrPubSubPublishUnauthorized,
			wantErrContain: "gateway-internal only",
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
			name:       "operator publishes to own receipts channel permitted",
			channel:    "receipts:op-001:sess-001",
			operatorID: "op-001",
			spiffeID:   "spiffe://g8e.local/operator/org-1/op-001/sess-1",
			wantErr:    false,
		},
		{
			name:           "operator publishes to another operator receipts rejected",
			channel:        "receipts:op-other:sess-001",
			operatorID:     "op-001",
			spiffeID:       "spiffe://g8e.local/operator/org-1/op-001/sess-1",
			wantErr:        true,
			wantErrIs:      constants.ErrPubSubPublishUnauthorized,
			wantErrContain: "channel operator_id mismatch",
		},
		{
			name:       "ensemble app publishes to receipts channel permitted",
			channel:    "receipts:op-001:sess-001",
			operatorID: "g8ee",
			spiffeID:   "spiffe://g8e.local/app/g8ee",
			wantErr:    false,
		},
		{
			name:           "ensemble app publishes to cmd: channel rejected",
			channel:        "cmd:op-001:sess-001",
			operatorID:     "g8ee",
			spiffeID:       "spiffe://g8e.local/app/g8ee",
			wantErr:        true,
			wantErrIs:      constants.ErrPubSubPublishUnauthorized,
			wantErrContain: "gateway-internal only",
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
	payload, err := proto.Marshal(&operatorv1.FsReadRequested{Path: "/etc/hostname", ExecutionId: "exec-1"})
	require.NoError(t, err)
	env, err := BuildGovernanceEnvelope(BuildEnvelopeParams{
		OperatorID:        "op-001",
		OperatorSessionID: "sess-001",
		EventType:         string(constants.Event.Operator.FsRead.Requested),
		Payload:           payload,
		TargetResource:    "/etc/hostname",
		RequestorUserID:   "user-001",
		ActingAppID:       "spiffe://g8e.local/app/g8ee",
		StateMerkleRoot:   "root-abc",
		Posture:           "doctrine",
		Doctrine:          governance.NewL1Doctrine(),
	})
	require.NoError(t, err)
	require.NotNil(t, env)

	assert.Equal(t, govpkg.GovernanceProtocolVersionV2, env.ProtocolVersion)
	assert.Equal(t, "op-001", env.OperatorId)
	assert.Equal(t, "sess-001", env.OperatorSessionId)
	assert.Equal(t, string(constants.ActionTypeFsRead), env.ActionType)
	assert.Equal(t, "/etc/hostname", env.TargetResource)
	assert.Equal(t, payload, env.Payload)
	assert.Equal(t, "root-abc", env.StateMerkleRoot)
	assert.NotEmpty(t, env.Nonce, "envelope must have a nonce")
	assert.NotEmpty(t, env.Id, "envelope must have an Id (transaction hash)")
	assert.Equal(t, env.Id, env.TransactionHash, "Id must equal TransactionHash")
	assert.Equal(t, "user-001", env.RequestorUserId)
	assert.Equal(t, "spiffe://g8e.local/app/g8ee", env.ActingAppId)
	assert.Equal(t, "doctrine", env.Posture, "envelope must carry the gateway posture")
	require.NotNil(t, env.Governance)
	require.NotNil(t, env.Governance.L1)
	assert.True(t, env.Governance.L1.Validated, "L1 doctrine validation marker must be set after screening")
	assert.NotNil(t, env.Timestamp)
	assert.NotNil(t, env.ExpiresAt)
}

func TestBuildGovernanceEnvelope_DeterministicTxHash(t *testing.T) {
	// Same params (except nonce which is random) produce different tx hashes
	// because the nonce differs. But the envelope structure is consistent.
	payload, err := proto.Marshal(&operatorv1.FsReadRequested{Path: "/etc/hostname", ExecutionId: "exec-1"})
	require.NoError(t, err)
	params := BuildEnvelopeParams{
		OperatorID:        "op-001",
		OperatorSessionID: "sess-001",
		EventType:         string(constants.Event.Operator.FsRead.Requested),
		Payload:           payload,
		TargetResource:    "/etc/hostname",
		RequestorUserID:   "user-001",
		ActingAppID:       "spiffe://g8e.local/app/g8ee",
		StateMerkleRoot:   "root-abc",
		Posture:           "doctrine",
		Doctrine:          governance.NewL1Doctrine(),
	}

	env1, err := BuildGovernanceEnvelope(params)
	require.NoError(t, err)

	env2, err := BuildGovernanceEnvelope(params)
	require.NoError(t, err)

	// Nonces are random, so transaction hashes must differ.
	assert.NotEqual(t, env1.Id, env2.Id, "transaction hashes must differ due to random nonce")
	// But nonces must be different.
	assert.NotEqual(t, env1.Nonce, env2.Nonce, "nonces must differ")
	// Posture is identical and not part of the hash.
	assert.Equal(t, env1.Posture, env2.Posture)
	assert.Equal(t, "doctrine", env1.Posture)
}

// TestBuildGovernanceEnvelope_MissingPostureFailsClosed verifies that the
// gateway never emits an envelope without posture. An empty posture
// indicates a gateway bug (the constructor was called without wiring
// cfg.Gateway.Posture), and the operator would reject it per-transaction
// anyway — fail closed at construction time.
func TestBuildGovernanceEnvelope_MissingPostureFailsClosed(t *testing.T) {
	payload, err := proto.Marshal(&operatorv1.FsReadRequested{Path: "/etc/hostname", ExecutionId: "exec-1"})
	require.NoError(t, err)
	_, err = BuildGovernanceEnvelope(BuildEnvelopeParams{
		OperatorID:        "op-001",
		OperatorSessionID: "sess-001",
		EventType:         string(constants.Event.Operator.FsRead.Requested),
		Payload:           payload,
		TargetResource:    "/etc/hostname",
		StateMerkleRoot:   "root-abc",
		Doctrine:          governance.NewL1Doctrine(),
		// Posture intentionally omitted.
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEnvelopePostureMissing),
		"missing posture must fail closed with ErrEnvelopePostureMissing, got: %v", err)
}

// --- handlePublish integration tests ---

func newTestBroker(t *testing.T) *GatewayWebSocketHandler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	return NewGatewayWebSocketHandler(logger)
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

func TestHandlePublish_AppCommandPublishRejectedAtACL(t *testing.T) {
	broker := newTestBroker(t)
	handler := newAppSessionHandler(broker, "g8ee")

	cmdChannel := pubsub.CmdChannel("op-001", "sess-001")
	var called bool
	unregister := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		called = true
	})
	defer unregister()

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: cmdChannel,
		Data:    []byte(`{"operator_id":"op-001","operator_session_id":"sess-001"}`),
	})

	assert.False(t, called, "app publish to cmd: must be rejected at ACL; use HTTP dispatch")
}

func TestHandlePublish_OperatorPublishToCmdRejected(t *testing.T) {
	op := &models.OperatorDocumentGo{
		ID:                "op-001",
		OperatorSessionID: "sess-001",
	}
	broker := newTestBroker(t)
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
	broker := newTestBroker(t)
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
	broker := newTestBroker(t)
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

// TestHandlePublish_OperatorCrossOperatorHeartbeatRejected verifies that
// an operator cannot publish to another operator's heartbeat channel.
func TestHandlePublish_OperatorCrossOperatorHeartbeatRejected(t *testing.T) {
	broker := newTestBroker(t)
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
	assert.Empty(t, broker.posture, "posture must be empty before SetCommandRelayDeps")
	assert.Nil(t, broker.doctrine, "doctrine must be nil before SetCommandRelayDeps")

	provider := &stubStateRootProvider{root: "root-1"}
	validator := &stubOperatorSessionValidator{op: &models.OperatorDocumentGo{ID: "op-1"}}
	doctrine := governance.NewL1Doctrine()
	broker.SetCommandRelayDeps(provider, validator, "doctrine", doctrine)

	broker.mu.RLock()
	assert.NotNil(t, broker.stateRootProvider)
	assert.NotNil(t, broker.sessionValidator)
	assert.Equal(t, "doctrine", broker.posture)
	assert.Equal(t, doctrine, broker.doctrine)
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
