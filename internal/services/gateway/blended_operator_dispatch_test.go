// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type embeddedEnvelopeProcessorFunc func(context.Context, []byte) (*operatorv1.ActionReceipt, error)

func (f embeddedEnvelopeProcessorFunc) ProcessEnvelope(ctx context.Context, wire []byte) (*operatorv1.ActionReceipt, error) {
	return f(ctx, wire)
}

func TestEmbeddedDispatchUsesGovernanceProcessorAndCorrelatesResult(t *testing.T) {
	logger := testutil.NewTestLogger()
	broker := NewGatewayWebSocketHandler(logger)
	op := &operatorv1.OperatorDocument{Id: string(constants.DocIDEmbeddedOperator), OperatorSessionId: "embedded-session", OperatorType: string(constants.OperatorTypeEmbedded), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{Roles: models.OperatorRolesToProto(constants.OperatorRoles{constants.OperatorRoleData, constants.OperatorRoleObserver})}}
	called := false
	processor := embeddedEnvelopeProcessorFunc(func(ctx context.Context, wire []byte) (*operatorv1.ActionReceipt, error) {
		called = true
		env := &commonv1.GovernanceEnvelope{}
		require.NoError(t, protojson.Unmarshal(wire, env))
		require.Equal(t, op.Id, env.OperatorId)
		require.Equal(t, op.OperatorSessionId, env.OperatorSessionId)
		result, err := proto.Marshal(&operatorv1.CommandResult{Stdout: "hello"})
		require.NoError(t, err)
		completion := &commonv1.GovernanceEnvelope{Id: env.Id, EventType: string(constants.Event.Operator.Command.Completed), ActionType: string(constants.ActionTypeExecuteBash), Payload: result}
		payload, err := protojson.Marshal(completion)
		require.NoError(t, err)
		broker.Publish(pubsub.ResultsChannel(op.Id, op.OperatorSessionId), payload)
		return &operatorv1.ActionReceipt{}, nil
	})
	svc := NewDispatchService(logger, broker, &stubStateRootProvider{root: "root"}, &stubOperatorSessionValidator{op: op}, string(constants.PostureDoctrine), governance.NewL1Doctrine(), nil, nil, processor)
	payload, err := proto.Marshal(&operatorv1.CommandRequested{Command: "echo hello", ExecutionId: "execution"})
	require.NoError(t, err)
	result, err := svc.Dispatch(t.Context(), DispatchRequest{TargetOperatorSessionID: op.OperatorSessionId, RequestorUserID: "user", EventType: string(constants.Event.Operator.Command.Requested), Payload: payload})
	require.NoError(t, err)
	require.True(t, called)
	require.NotNil(t, result)
}

func TestPublishCommandRoutesEmbeddedOperatorThroughGovernanceProcessor(t *testing.T) {
	logger := testutil.NewTestLogger()
	broker := NewGatewayWebSocketHandler(logger)
	op := &operatorv1.OperatorDocument{Id: string(constants.DocIDEmbeddedOperator), OperatorSessionId: "embedded-session", OperatorType: string(constants.OperatorTypeEmbedded)}
	var processed *commonv1.GovernanceEnvelope
	processor := embeddedEnvelopeProcessorFunc(func(ctx context.Context, wire []byte) (*operatorv1.ActionReceipt, error) {
		processed = &commonv1.GovernanceEnvelope{}
		require.NoError(t, protojson.Unmarshal(wire, processed))
		return &operatorv1.ActionReceipt{}, nil
	})
	svc := NewDispatchService(logger, broker, &stubStateRootProvider{root: "root"}, &stubOperatorSessionValidator{op: op}, string(constants.PostureDoctrine), governance.NewL1Doctrine(), nil, nil, processor)
	payload, err := proto.Marshal(&operatorv1.CommandRequested{Command: "echo hello", ExecutionId: "execution"})
	require.NoError(t, err)

	txID, err := svc.PublishCommand(t.Context(), PublishCommandRequest{
		TargetOperatorSessionID: op.OperatorSessionId,
		EventType:               string(constants.Event.Operator.Command.Requested),
		Payload:                 payload,
		RequestorUserID:         "user",
	})
	require.NoError(t, err)
	require.NotNil(t, processed)
	require.Equal(t, txID, processed.Id)

	noProcessor := NewDispatchService(logger, broker, &stubStateRootProvider{root: "root"}, &stubOperatorSessionValidator{op: op}, string(constants.PostureDoctrine), governance.NewL1Doctrine(), nil, nil, nil)
	_, err = noProcessor.PublishCommand(t.Context(), PublishCommandRequest{TargetOperatorSessionID: op.OperatorSessionId, EventType: string(constants.Event.Operator.Command.Requested), Payload: payload, RequestorUserID: "user"})
	require.ErrorIs(t, err, constants.ErrDispatchNoDelivery)
}

func TestEmbeddedInferenceDispatchDrainsProgressWhileProcessing(t *testing.T) {
	logger := testutil.NewTestLogger()
	broker := NewGatewayWebSocketHandler(logger)
	op := &operatorv1.OperatorDocument{Id: string(constants.DocIDEmbeddedOperator), OperatorSessionId: "embedded-session", OperatorType: string(constants.OperatorTypeEmbedded)}
	errProcessing := errors.New("embedded processing failed")
	processor := embeddedEnvelopeProcessorFunc(func(ctx context.Context, wire []byte) (*operatorv1.ActionReceipt, error) {
		env := &commonv1.GovernanceEnvelope{}
		require.NoError(t, protojson.Unmarshal(wire, env))
		// More events than the result buffer holds, all published before
		// ProcessEnvelope returns, as an in-process inference stream does.
		for i := 0; i < InferenceProgressResultBuffer*2; i++ {
			progress, err := proto.Marshal(&operatorv1.InferenceProgressEvent{
				ProviderAttemptId: "attempt-1",
				Sequence:          uint32(i + 1),
				Parts:             []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "x"}}},
			})
			require.NoError(t, err)
			out, err := protojson.Marshal(&commonv1.GovernanceEnvelope{Id: env.Id, EventType: string(constants.Event.Operator.Inference.ProgressUpdated), ActionType: env.ActionType, Payload: progress})
			require.NoError(t, err)
			broker.Publish(pubsub.ResultsChannel(op.Id, op.OperatorSessionId), out)
		}
		return nil, errProcessing
	})
	svc := NewDispatchService(logger, broker, &stubStateRootProvider{root: "root"}, &stubOperatorSessionValidator{op: op}, string(constants.PostureDoctrine), governance.NewL1Doctrine(), nil, nil, processor)

	var seen int
	_, err := svc.Dispatch(t.Context(), DispatchRequest{
		TargetOperatorSessionID: op.OperatorSessionId,
		RequestorUserID:         "user",
		EventType:               string(constants.Event.Operator.Inference.Requested),
		Payload:                 inferencePayload(t),
		Timeout:                 5 * time.Second,
		OnInferenceProgress:     func(*operatorv1.InferenceProgressEvent) error { seen++; return nil },
	})
	require.ErrorIs(t, err, errProcessing)
	require.NotErrorIs(t, err, constants.ErrInferenceProgressBackpressure)
}
