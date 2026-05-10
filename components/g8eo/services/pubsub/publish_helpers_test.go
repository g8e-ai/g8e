// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pubsub

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	pb "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionIDFromMessage_PrefersPayloadExecutionID(t *testing.T) {
	payload := testutil.MustMarshalProtobufCommandRequested(t, "ls", "exec-123", "", "", 0)
	msg := PubSubCommandMessage{
		ID:        "envelope-abc",
		EventType: constants.Event.Operator.Command.Requested,
		Payload:   payload,
	}
	assert.Equal(t, "exec-123", executionIDFromMessage(msg))
}

func TestExecutionIDFromMessage_FallsBackToEnvelopeID(t *testing.T) {
	payload := testutil.MustMarshalProtobufHeartbeatRequested(t)
	msg := PubSubCommandMessage{
		ID:        "envelope-abc",
		EventType: constants.Event.Operator.HeartbeatRequested,
		Payload:   payload,
	}
	assert.Equal(t, "envelope-abc", executionIDFromMessage(msg))
}

func TestExecutionIDFromMessage_EmptyPayload(t *testing.T) {
	msg := PubSubCommandMessage{ID: "envelope-abc"}
	assert.Equal(t, "envelope-abc", executionIDFromMessage(msg))
}

func TestExecutionIDFromMessage_MalformedPayloadFallsBack(t *testing.T) {
	msg := PubSubCommandMessage{ID: "envelope-abc", Payload: []byte("not json")}
	assert.Equal(t, "envelope-abc", executionIDFromMessage(msg))
}

func TestExecutionIDFromMessage_EmptyExecutionIDInPayloadFallsBack(t *testing.T) {
	payload := testutil.MustMarshalProtobufCommandRequested(t, "ls", "", "", "", 0)
	msg := PubSubCommandMessage{ID: "envelope-abc", Payload: payload}
	assert.Equal(t, "envelope-abc", executionIDFromMessage(msg))
}

func TestSetExecutionIDOnPayload_CommandResult(t *testing.T) {
	payload := &pb.CommandResult{
		Status: protoExecutionStatus(constants.ExecutionStatusFailed),
		Error:  "test error",
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_FileEditResult(t *testing.T) {
	payload := &pb.FileEditResult{
		ExecutionId: "original-id",
		Status:      protoExecutionStatus(constants.ExecutionStatusCompleted),
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_FsListResult(t *testing.T) {
	payload := &pb.FsListResult{
		ExecutionId: "",
		Status:      protoExecutionStatus(constants.ExecutionStatusCompleted),
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_PortCheckResult(t *testing.T) {
	payload := &pb.PortCheckResult{
		ExecutionId: "existing-id",
		Status:      protoExecutionStatus(constants.ExecutionStatusCompleted),
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_EmptyExecutionID(t *testing.T) {
	payload := &pb.CommandResult{
		Status: protoExecutionStatus(constants.ExecutionStatusFailed),
		Error:  "test error",
	}
	setExecutionIDOnPayload(payload, "")
	assert.Equal(t, "", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_UnsupportedType(t *testing.T) {
	// HeartbeatRequested does not have execution_id
	payload := &pb.HeartbeatRequested{}
	setExecutionIDOnPayload(payload, "msg-abc")
	// No panic, and no change (obviously)
}

func TestSetExecutionIDOnPayload_FetchFileDiffResult(t *testing.T) {
	payload := &pb.FetchFileDiffResult{
		Success:     true,
		ExecutionId: "original-id",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_FetchHistoryResult(t *testing.T) {
	payload := &pb.FetchHistoryResult{
		Success:     true,
		ExecutionId: "",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_FetchFileHistoryResult(t *testing.T) {
	payload := &pb.FetchFileHistoryResult{
		Success:     false,
		Error:       "test error",
		ExecutionId: "old-id",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_RestoreFileResult(t *testing.T) {
	payload := &pb.RestoreFileResult{
		Success:     true,
		ExecutionId: "",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionId)
}

// The LFAA publish path (file reads, port checks, fetch logs/history, restore)
// must use UniversalEnvelope for cross-component consistency.
func TestPublishLFAA_EnvelopeStructure(t *testing.T) {
	ctx := context.Background()
	logger := testutil.NewTestLogger()

	cmdMsg := PubSubCommandMessage{
		ID:                "cmd-1",
		EventType:         constants.Event.Operator.PortCheck.Requested,
		CaseID:            "case-lfaa",
		InvestigationID:   "inv-lfaa",
		OperatorSessionID: "opsess-lfaa",
	}

	t.Run("typed response uses UniversalEnvelope", func(t *testing.T) {
		client := NewMockOperatorPubSubClient()
		defer client.Close()
		cfg := testutil.NewTestConfig(t)

		publishLFAATypedResponseTo(ctx, client, cfg, logger, cmdMsg,
			constants.Event.Operator.PortCheck.Completed,
			&pb.PortCheckResult{Status: protoExecutionStatus(constants.ExecutionStatusCompleted)})

		published := client.LastPublished()
		require.NotNil(t, published)

		env := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
		assert.Equal(t, constants.Event.Operator.PortCheck.Completed, env.EventType)
		assert.Equal(t, cfg.OperatorID, env.OperatorId)
		assert.Equal(t, "case-lfaa", env.CaseId)
		assert.Equal(t, "inv-lfaa", env.InvestigationId)
		assert.Equal(t, "opsess-lfaa", env.OperatorSessionId)
	})

	t.Run("error response uses UniversalEnvelope", func(t *testing.T) {
		client := NewMockOperatorPubSubClient()
		defer client.Close()
		cfg := testutil.NewTestConfig(t)

		publishLFAAErrorTo(ctx, client, cfg, logger, cmdMsg,
			constants.Event.Operator.PortCheck.Failed, "boom")

		published := client.LastPublished()
		require.NotNil(t, published)

		env := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
		assert.Equal(t, constants.Event.Operator.PortCheck.Failed, env.EventType)

		var payload pb.CommandResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Equal(t, protoExecutionStatus(constants.ExecutionStatusFailed), payload.Status)
		assert.Equal(t, "boom", payload.Error)
	})
}
