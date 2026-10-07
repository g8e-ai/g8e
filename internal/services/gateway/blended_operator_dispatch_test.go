// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"testing"

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
	op := &models.OperatorDocumentGo{ID: string(constants.DocIDEmbeddedOperator), OperatorSessionID: "embedded-session", OperatorType: constants.OperatorTypeEmbedded, RuntimeConfig: &models.RuntimeConfig{Roles: constants.OperatorRoles{constants.OperatorRoleData, constants.OperatorRoleObserver}}}
	called := false
	processor := embeddedEnvelopeProcessorFunc(func(ctx context.Context, wire []byte) (*operatorv1.ActionReceipt, error) {
		called = true
		env := &commonv1.GovernanceEnvelope{}
		require.NoError(t, protojson.Unmarshal(wire, env))
		require.Equal(t, op.ID, env.OperatorId)
		require.Equal(t, op.OperatorSessionID, env.OperatorSessionId)
		result, err := proto.Marshal(&operatorv1.CommandResult{Stdout: "hello"})
		require.NoError(t, err)
		completion := &commonv1.GovernanceEnvelope{Id: env.Id, EventType: string(constants.Event.Operator.Command.Completed), ActionType: string(constants.ActionTypeExecuteBash), Payload: result}
		payload, err := protojson.Marshal(completion)
		require.NoError(t, err)
		broker.Publish(pubsub.ResultsChannel(op.ID, op.OperatorSessionID), payload)
		return &operatorv1.ActionReceipt{}, nil
	})
	svc := NewDispatchService(logger, broker, &stubStateRootProvider{root: "root"}, &stubOperatorSessionValidator{op: op}, string(constants.PostureDoctrine), governance.NewL1Doctrine(), nil, nil, processor)
	payload, err := proto.Marshal(&operatorv1.CommandRequested{Command: "echo hello", ExecutionId: "execution"})
	require.NoError(t, err)
	result, err := svc.Dispatch(t.Context(), DispatchRequest{TargetOperatorSessionID: op.OperatorSessionID, RequestorUserID: "user", EventType: string(constants.Event.Operator.Command.Requested), Payload: payload})
	require.NoError(t, err)
	require.True(t, called)
	require.NotNil(t, result)
}
