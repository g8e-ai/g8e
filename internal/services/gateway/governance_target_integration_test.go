// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestGovernanceEnvelope_ForeignOperatorTarget_Returns403(t *testing.T) {
	svc := newTestGatewayService(t, testGatewayOpts{posture: config.PostureDoctrine})
	payload, err := proto.Marshal(&operatorv1.FsReadRequested{Path: constants.TestPathShortData, ExecutionId: "target-rejection"})
	require.NoError(t, err)
	env := &commonv1.GovernanceEnvelope{
		ProtocolVersion:   govtypes.GovernanceProtocolVersionV2,
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(time.Minute)),
		OperatorId:        "foreign-outbound-operator",
		OperatorSessionId: "session-1",
		ActionType:        string(constants.ActionTypeFsRead),
		Payload:           payload,
		Nonce:             "foreign-target-nonce",
	}
	eventType, err := constants.RequestEventForAction(constants.ActionTypeFsRead)
	require.NoError(t, err)
	env.EventType = string(eventType)
	env.StateMerkleRoot, err = svc.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	env.Id, err = govtypes.GenerateMessageID(env)
	require.NoError(t, err)
	env.TransactionHash = env.Id
	uri := "spiffe://g8e.local/operator/org-1/foreign-outbound-operator/session-1"
	body := marshalEnvelope(t, env)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader(body))
	req.TLS = identityBindingRequest(t, uri).TLS
	w := httptest.NewRecorder()
	svc.GetHTTPHandler().governanceController.handleGovernanceEnvelope(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "TX_TARGET_OPERATOR_MISMATCH")
	// A second submission must reach the same target rejection, not a replay error.
	w = httptest.NewRecorder()
	req.Body = io.NopCloser(bytes.NewReader(body))
	svc.GetHTTPHandler().governanceController.handleGovernanceEnvelope(w, req)
	require.Contains(t, w.Body.String(), "TX_TARGET_OPERATOR_MISMATCH")
}
