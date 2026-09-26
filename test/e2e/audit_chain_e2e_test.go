// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// TestAuditChain_VerifyOK proves the Gateway audit hash chain verifies end to end.
func TestAuditChain_VerifyOK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := e2eClient.VerifyAuditChain(ctx, 0)
	require.NoError(t, err, "audit verify endpoint must succeed")
	assert.True(t, resp.OK, "audit chain must verify")
	assert.Greater(t, resp.HeadSeq, int64(0), "audit chain head seq must be positive")
	assert.NotEmpty(t, resp.HeadHash, "audit chain head hash must be present")
	t.Logf("audit chain OK: verified_from=%d head_seq=%d head_hash=%s",
		resp.VerifiedFromSeq, resp.HeadSeq, resp.HeadHash)
}

// TestDispatch_UnknownEventRejected proves unregistered governed events fail closed at dispatch ingress.
func TestDispatch_UnknownEventRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err)
	require.True(t, operators.Success)
	target := findLiveActiveRemoteOperator(operators.Operators)
	require.NotNil(t, target, "a live active remote operator must exist")
	require.NotEmpty(t, target.OperatorSessionID)

	fsReadReq := &operatorv1.FsReadRequested{Path: constants.PathEtcHostname}
	payload, err := proto.Marshal(fsReadReq)
	require.NoError(t, err)

	reqBody := dispatchRequestJSON{
		TargetOperatorSessionID: target.OperatorSessionID,
		EventType:               "g8e.v1.not.registered.event",
		Payload:                 payload,
		TargetResource:          constants.PathEtcHostname,
	}

	status, body, err := e2eClient.DispatchCommandExpectStatus(ctx, reqBody, 400)
	require.NoError(t, err, "dispatch with unknown event must return 400, not transport error")
	assert.Equal(t, 400, status, "unknown event must be rejected at gateway ingress: %s", string(body))
}

// TestLFAA_AuditIngestAckAndVerify proves LFAA ingest returns chain metadata
// and the Gateway audit chain still verifies afterwards.
func TestLFAA_AuditIngestAckAndVerify(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err)
	require.True(t, operators.Success)
	target := findLiveActiveRemoteOperator(operators.Operators)
	require.NotNil(t, target, "a live active remote operator must exist")
	require.NotEmpty(t, target.OperatorSessionID)

	ack, err := e2eClient.IngestAuditRecord(ctx, models.AuditRecordIngestRequest{
		EventType:         string(constants.EventOperatorAuditDirectCommandRecordRequested),
		OperatorID:        target.ID,
		OperatorSessionID: target.OperatorSessionID,
		IdempotencyKey:    fmt.Sprintf("e2e-lfaa-%d", time.Now().UnixNano()),
		Payload:           []byte(`{"command":"e2e-lfaa-proof"}`),
	})
	require.NoError(t, err, "LFAA ingest must be acknowledged")
	assert.Greater(t, ack.Seq, int64(0), "ingest ack must include chain seq")
	assert.NotEmpty(t, ack.Hash, "ingest ack must include chain hash")

	resp, err := e2eClient.VerifyAuditChain(ctx, 0)
	require.NoError(t, err, "audit verify after LFAA ingest must succeed")
	assert.True(t, resp.OK, "audit chain must verify after LFAA ingest: %s", resp.Error)
}
