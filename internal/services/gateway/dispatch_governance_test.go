// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software
// is released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// stubL2Deliberator is a minimal L2ConsensusDeliberator for the gateway
// posture-table tests. It records whether Deliberate was invoked and returns
// the envelope bytes unchanged (the caller re-decodes to inspect L2 metadata).
type stubL2Deliberator struct {
	called   bool
	envelope []byte
}

func (s *stubL2Deliberator) Deliberate(_ context.Context, envelopeBytes []byte) ([]byte, error) {
	s.called = true
	s.envelope = envelopeBytes
	return envelopeBytes, nil
}

// failingL2Deliberator always returns an error, proving the dispatch path
// fails closed when L2 deliberation fails under a posture that requires it.
type failingL2Deliberator struct{}

func (failingL2Deliberator) Deliberate(_ context.Context, _ []byte) ([]byte, error) {
	return nil, errors.New("l2 deliberation unavailable")
}

// inferencePayload builds a valid InferenceRequested proto payload.
func inferencePayload(t *testing.T) []byte {
	t.Helper()
	msg := &operatorv1.InferenceRequested{
		Role:  operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model: "gemma3:4b",
		Messages: []*operatorv1.InferenceMessage{{
			Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
			Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: "summarize the doctrine"}}},
		}},
	}
	b, err := proto.Marshal(msg)
	require.NoError(t, err)
	return b
}

// fsReadPayload builds a valid FsReadRequested proto payload (a read-only,
// non-mutation action used to exercise the L1-clean pass without L3 gating).
func fsReadPayload(t *testing.T) []byte {
	t.Helper()
	msg := &operatorv1.FsReadRequested{Path: "/tmp/short-data", ExecutionId: "exec-1"}
	b, err := proto.Marshal(msg)
	require.NoError(t, err)
	return b
}

// maliciousCommandPayload builds a CommandRequested whose command string
// triggers an L1 doctrine forbidden-pattern violation.
func maliciousCommandPayload(t *testing.T) []byte {
	t.Helper()
	msg := &operatorv1.CommandRequested{Command: "rm -rf /", ExecutionId: "exec-1", Justification: "wipe"}
	b, err := proto.Marshal(msg)
	require.NoError(t, err)
	return b
}

// baseEnvelopeParams returns a BuildEnvelopeParams populated with valid
// identifiers and the given action/payload/posture, ready for posture-table
// subtests to override individual fields.
func baseEnvelopeParams(action constants.ActionType, payload []byte, posture string, doctrine *governance.L1Doctrine) BuildEnvelopeParams {
	return BuildEnvelopeParams{
		OperatorID:        "op-001",
		OperatorSessionID: "sess-001",
		ActionType:        string(action),
		Payload:           payload,
		TargetResource:    "localhost",
		RequestorUserID:   "user-001",
		ActingAppID:       "spiffe://g8e.local/app/test-app",
		StateMerkleRoot:   "root-abc",
		Posture:           posture,
		Doctrine:          doctrine,
	}
}

func testDoctrine() *governance.L1Doctrine {
	return governance.NewL1Doctrine()
}

// TestBuildGovernanceEnvelope_DoctrineScreeningPassesForCleanPayload proves
// that under doctrine posture, a clean typed payload passes L1 screening and
// the envelope carries L1.Validated=true only because the doctrine actually
// validated it (not because the builder asserted it).
func TestBuildGovernanceEnvelope_DoctrineScreeningPassesForCleanPayload(t *testing.T) {
	env, err := BuildGovernanceEnvelope(baseEnvelopeParams(
		constants.ActionTypeFsRead, fsReadPayload(t), constants.PostureDoctrine, testDoctrine(),
	))
	require.NoError(t, err)
	require.NotNil(t, env.Governance)
	require.NotNil(t, env.Governance.L1)
	assert.True(t, env.Governance.L1.Validated, "clean payload must set L1.Validated=true after screening")
}

// TestBuildGovernanceEnvelope_NilDoctrineFailsClosed proves that a nil
// doctrine fails closed: the builder cannot assert L1.Validated=true without a
// doctrine to run screening against.
func TestBuildGovernanceEnvelope_NilDoctrineFailsClosed(t *testing.T) {
	params := baseEnvelopeParams(constants.ActionTypeFsRead, fsReadPayload(t), constants.PostureDoctrine, nil)
	_, err := BuildGovernanceEnvelope(params)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTxDoctrineMissing)
}

// TestBuildGovernanceEnvelope_DecodeFailureFailsClosed proves that a payload
// that cannot be decoded into the typed proto for its action type fails closed
// with ErrTxPayloadDecodeFailed rather than skipping L1.
func TestBuildGovernanceEnvelope_DecodeFailureFailsClosed(t *testing.T) {
	params := baseEnvelopeParams(constants.ActionTypeFsRead, []byte{0xFF, 0xFF, 0xFF}, constants.PostureDoctrine, testDoctrine())
	_, err := BuildGovernanceEnvelope(params)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTxPayloadDecodeFailed)
}

// TestBuildGovernanceEnvelope_L1ViolationFailsClosed proves that a payload
// whose content triggers a doctrine forbidden-pattern violation fails closed
// with ErrTxL1ValidationFailed rather than reaching the operator.
func TestBuildGovernanceEnvelope_L1ViolationFailsClosed(t *testing.T) {
	params := baseEnvelopeParams(constants.ActionTypeExecuteBash, maliciousCommandPayload(t), constants.PostureDoctrine, testDoctrine())
	_, err := BuildGovernanceEnvelope(params)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTxL1ValidationFailed)
}

// TestBuildGovernanceEnvelope_InferenceDecodesAndScreens proves the inference
// action type now flows through L1 screening: a clean InferenceRequested
// payload passes, and the envelope carries L1.Validated=true.
func TestBuildGovernanceEnvelope_InferenceDecodesAndScreens(t *testing.T) {
	env, err := BuildGovernanceEnvelope(baseEnvelopeParams(
		constants.ActionTypeInference, inferencePayload(t), constants.PostureDoctrine, testDoctrine(),
	))
	require.NoError(t, err)
	require.NotNil(t, env.Governance.L1)
	assert.True(t, env.Governance.L1.Validated)
}

// TestBuildGovernanceEnvelope_RatifyRejectsMutation proves that under ratify
// posture, a mutation-classified action (inference) is rejected early because
// the gateway dispatch path cannot mint L3 human proofs.
func TestBuildGovernanceEnvelope_RatifyRejectsMutation(t *testing.T) {
	params := baseEnvelopeParams(constants.ActionTypeInference, inferencePayload(t), constants.PostureRatify, testDoctrine())
	_, err := BuildGovernanceEnvelope(params)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTxL3ProofUnmintable)
}

// TestBuildGovernanceEnvelope_NotaryRejectsMutation proves that under notary
// posture, a mutation-classified action is rejected early for the same reason.
func TestBuildGovernanceEnvelope_NotaryRejectsMutation(t *testing.T) {
	params := baseEnvelopeParams(constants.ActionTypeInference, inferencePayload(t), constants.PostureNotary, testDoctrine())
	_, err := BuildGovernanceEnvelope(params)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTxL3ProofUnmintable)
}

// TestBuildGovernanceEnvelope_RatifyAllowsReadOnly proves that under ratify
// posture, a read-only (non-mutation) action still passes because L3 is not
// required for reads.
func TestBuildGovernanceEnvelope_RatifyAllowsReadOnly(t *testing.T) {
	env, err := BuildGovernanceEnvelope(baseEnvelopeParams(
		constants.ActionTypeFsRead, fsReadPayload(t), constants.PostureRatify, testDoctrine(),
	))
	require.NoError(t, err)
	assert.True(t, env.Governance.L1.Validated)
}

// TestDispatchService_PostureTable_L2Deliberation proves that the DispatchService
// invokes the L2 deliberator after envelope construction under postures that
// require L2 signatures (consensus, notary) and does not invoke it under
// postures that do not (doctrine, ratify). Under notary, the mutation is
// rejected before deliberation because the path cannot mint L3 proofs.
func TestDispatchService_PostureTable_L2Deliberation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}

	cases := []struct {
		name            string
		posture         string
		action          constants.ActionType
		payload         func(t *testing.T) []byte
		wantDeliberated bool
		wantErr         error
	}{
		{
			name:    "doctrine read does not deliberate",
			posture: constants.PostureDoctrine,
			action:  constants.ActionTypeFsRead,
			payload: fsReadPayload,
		},
		{
			name:    "ratify read does not deliberate",
			posture: constants.PostureRatify,
			action:  constants.ActionTypeFsRead,
			payload: fsReadPayload,
		},
		{
			name:            "consensus read deliberates",
			posture:         constants.PostureConsensus,
			action:          constants.ActionTypeFsRead,
			payload:         fsReadPayload,
			wantDeliberated: true,
		},
		{
			name:    "notary mutation rejected before deliberation",
			posture: constants.PostureNotary,
			action:  constants.ActionTypeInference,
			payload: inferencePayload,
			wantErr: constants.ErrTxL3ProofUnmintable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broker := NewGatewayWebSocketHandler(logger)
			deliberator := &stubL2Deliberator{}
			svc := NewDispatchService(
				logger, broker,
				&stubStateRootProvider{root: "root-abc"},
				&stubOperatorSessionValidator{op: op},
				tc.posture,
				testDoctrine(),
				deliberator,
				nil,
			)

			// Register an operator handler that publishes a result so the
			// dispatch completes (or fails before publish depending on posture).
			cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)
			resultsChannel := pubsub.ResultsChannel(op.ID, op.OperatorSessionID)
			unreg := broker.RegisterHandler(cmdChannel, func(_ string, data []byte) {
				// Echo a minimal result envelope with the same Id.
				broker.Publish(resultsChannel, data)
			})
			defer unreg()

			_, err := svc.Dispatch(context.Background(), DispatchRequest{
				TargetOperatorSessionID: op.OperatorSessionID,
				ActionType:              string(tc.action),
				Payload:                 tc.payload(t),
				RequestorUserID:         "user-001",
			})

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.False(t, deliberator.called, "deliberator must not be called when the dispatch fails before L2")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantDeliberated, deliberator.called)
		})
	}
}

// TestDispatchService_L2DeliberationFailureFailsClosed proves that when the L2
// deliberator returns an error under a posture that requires L2, the dispatch
// fails closed rather than publishing an unsigned envelope.
func TestDispatchService_L2DeliberationFailureFailsClosed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	broker := NewGatewayWebSocketHandler(logger)
	svc := NewDispatchService(
		logger, broker,
		&stubStateRootProvider{root: "root-abc"},
		&stubOperatorSessionValidator{op: op},
		constants.PostureConsensus,
		testDoctrine(),
		failingL2Deliberator{},
		nil,
	)

	published := false
	unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, _ []byte) {
		published = true
	})
	defer unreg()

	_, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: op.OperatorSessionID,
		ActionType:              string(constants.ActionTypeFsRead),
		Payload:                 fsReadPayload(t),
		RequestorUserID:         "user-001",
	})
	require.Error(t, err)
	assert.False(t, published, "envelope must not be published when L2 deliberation fails")
}
