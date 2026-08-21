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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// TestCommandRoundtrip_FsRead proves the full gateway to operator command
// round-trip end to end against the running platform: the gateway constructs
// a signed GovernanceEnvelope carrying an FS_READ action, publishes it to the
// operator's cmd channel over the WS pub/sub broker, the operator verifies it
// through the L4Warden gauntlet, executes the fs.read via the L5Actuator, and
// publishes the FsReadResult on its results channel. The gateway correlates
// the result by transaction ID and returns it in the HTTP response.
//
// The target operator is discovered through the owner-authenticated operator
// list endpoint (not from container logs or environment variables). The
// dispatch request is sent with the owner CLI identity through the
// E2EClient's strict mTLS client. The result payload is decoded as a typed
// FsReadResult protobuf message.
func TestCommandRoundtrip_FsRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resp := e2eClient.dispatchFsRead(t, ctx)

	// The response must carry the transaction ID and the completed event
	// type. The result envelope's EventType is the completed event — proof
	// the operator processed the command through the full L4/L5 chain and
	// published a typed result.
	assert.NotEmpty(t, resp.TransactionID, "response must carry the transaction ID")
	assert.Equal(t, string(constants.Event.Operator.FsRead.Completed), resp.EventType,
		"response event type must be the fs.read completed event")
	assert.NotEmpty(t, resp.ResultPayload, "response must carry the operator result payload")

	// Decode the result payload as FsReadResult and assert the operator read
	// /etc/hostname successfully. The content is the file's bytes inside the
	// operator — proving the command executed on the operator, not in the
	// gateway process.
	var fsReadResult operatorv1.FsReadResult
	require.NoError(t, proto.Unmarshal(resp.ResultPayload, &fsReadResult),
		"unmarshal FsReadResult from response payload")
	assert.Equal(t, constants.PathEtcHostname, fsReadResult.Path,
		"result path must match the requested path")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, fsReadResult.Status,
		"fs.read must complete successfully")
	assert.NotEmpty(t, fsReadResult.Content, "fs.read result content must not be empty")
	assert.Greater(t, fsReadResult.SizeBytes, int64(0), "fs.read result size must be positive")
	t.Logf("command roundtrip succeeded: txn=%s content_size=%d",
		resp.TransactionID, fsReadResult.SizeBytes)
}
