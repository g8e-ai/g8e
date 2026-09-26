// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestBuildExecuteBashDispatchRequest(t *testing.T) {
	request, err := BuildExecuteBashDispatchRequest(
		"4881d566-90a9-44c9-9e3e-c6bb51e07f5c",
		"echo hello",
		"exec-1",
		"cli-1",
	)
	require.NoError(t, err)
	assert.Equal(t, "4881d566-90a9-44c9-9e3e-c6bb51e07f5c", request.TargetOperatorSessionID)
	assert.Equal(t, string(constants.Event.Operator.Command.Requested), request.EventType)
	assert.Equal(t, "cli-1", request.CliSessionID)

	var payload operatorv1.CommandRequested
	require.NoError(t, proto.Unmarshal(request.Payload, &payload))
	assert.Equal(t, "echo hello", payload.Command)
	assert.Equal(t, "exec-1", payload.ExecutionId)
}

func TestBuildExecuteBashDispatchRequest_MissingFields(t *testing.T) {
	_, err := BuildExecuteBashDispatchRequest("", "echo", "exec-1", "cli-1")
	require.Error(t, err)
}

func TestParseCommandResult_EmptyPayload(t *testing.T) {
	_, err := ParseCommandResult(&DispatchResponse{
		Success:   true,
		EventType: "g8e.v1.operator.command.status.updated.running",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty command result payload")
}

func TestParseCommandResult_Success(t *testing.T) {
	payload, err := proto.Marshal(&operatorv1.CommandResult{
		Stdout:     "hello\n",
		ReturnCode: 0,
	})
	require.NoError(t, err)

	result, err := ParseCommandResult(&DispatchResponse{
		Success:       true,
		EventType:     "g8e.v1.operator.command.completed",
		ActionType:    string(constants.ActionTypeExecuteBash),
		ResultPayload: payload,
	})
	require.NoError(t, err)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.Equal(t, int32(0), result.ReturnCode)
}
