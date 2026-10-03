// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestGatewayService_LocalEnvelopeExecutionTarget(t *testing.T) {
	for _, action := range []constants.ActionType{constants.ActionTypeMcpCall, constants.ActionTypeA2aCall} {
		t.Run(string(action), func(t *testing.T) {
			g := newTestGatewayService(t)
			ctx := context.WithValue(context.Background(), constants.ContextKeyAppID, "calling-app")
			ctx = context.WithValue(ctx, constants.ContextKeyUserID, "requesting-user")
			ctx = context.WithValue(ctx, constants.ContextKeyOperatorID, "foreign-operator")
			ctx = context.WithValue(ctx, constants.ContextKeyOperatorSessionID, "foreign-session")
			eventType, err := constants.RequestEventForAction(action)
			require.NoError(t, err)
			_, wire, _, err := g.processGatewayTransaction(ctx, processGatewayOptions{eventType: eventType, targetResource: "local-tool", payloadBytes: []byte("payload")})
			require.NoError(t, err)
			env := &commonv1.GovernanceEnvelope{}
			require.NoError(t, protojson.Unmarshal(wire, env))
			require.Equal(t, string(constants.DocIDEmbeddedOperator), env.OperatorId)
			require.Equal(t, "calling-app", env.ActingAppId)
			require.Equal(t, "requesting-user", env.RequestorUserId)
			require.Empty(t, env.OperatorSessionId, "app credentials are not Operator sessions")
		})
	}
}

func TestGatewayService_ResumePreservesEnvelopeTargetAndHash(t *testing.T) {
	proc := &fakeEnvelopeProcessor{receipt: &operatorv1.ActionReceipt{}}
	store := &fakeSuspendedStore{}
	g := newTestGatewayService(t, withEnvProc(proc), withSuspendedStore(store))
	ctx := context.WithValue(context.Background(), constants.ContextKeyAppID, "calling-app")
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, "requesting-user")
	hash, wire, _, err := g.processGatewayTransaction(ctx, processGatewayOptions{eventType: constants.Event.Operator.Mcp.CallRequested, targetResource: "tool", payloadBytes: []byte("payload")})
	require.NoError(t, err)
	before := &commonv1.GovernanceEnvelope{}
	require.NoError(t, protojson.Unmarshal(wire, before))
	g.StoreSuspendedTransaction(ctx, hash, wire, "tool", json.RawMessage(`{}`), "requesting-user", "calling-app", "")
	_, err = g.ResumeWithL3Proof(ctx, hash, "requesting-user", &commonv1.L3Proof{CredentialId: "credential-1"})
	require.NoError(t, err)
	after := &commonv1.GovernanceEnvelope{}
	require.NoError(t, protojson.Unmarshal(proc.gotPayload, after))
	require.Equal(t, before.OperatorId, after.OperatorId)
	require.True(t, proto.Equal(before.Governance.GetL2(), after.Governance.GetL2()))
	computed, err := govtypes.GenerateMessageID(after)
	require.NoError(t, err)
	require.Equal(t, hash, computed, "adding L3 proof must not modify any hashed field")
	require.Equal(t, "requesting-user", after.RequestorUserId)
	require.Equal(t, "calling-app", after.ActingAppId)
}
