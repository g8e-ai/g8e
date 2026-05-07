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
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	pb "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionIDFromMessage_PrefersPayloadExecutionID(t *testing.T) {
	payload := testutil.MustMarshalProtobufCommandRequested(t, "ls", "exec-123", "", "", 0)
	msg := PubSubCommandMessage{ID: "envelope-abc", Payload: payload}
	assert.Equal(t, "exec-123", executionIDFromMessage(msg))
}

func TestExecutionIDFromMessage_FallsBackToEnvelopeID(t *testing.T) {
	payload := testutil.MustMarshalProtobufHeartbeatRequested(t)
	msg := PubSubCommandMessage{ID: "envelope-abc", Payload: payload}
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
		Status: string(constants.ExecutionStatusFailed),
		Error:  "test error",
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_FileEditResult(t *testing.T) {
	payload := &pb.FileEditResult{
		ExecutionId: "original-id",
		Status:      string(constants.ExecutionStatusCompleted),
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_FsListResult(t *testing.T) {
	payload := &pb.FsListResult{
		ExecutionId: "",
		Status:      string(constants.ExecutionStatusCompleted),
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_PortCheckResult(t *testing.T) {
	payload := &pb.PortCheckResult{
		ExecutionId: "existing-id",
		Status:      string(constants.ExecutionStatusCompleted),
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionId)
}

func TestSetExecutionIDOnPayload_EmptyExecutionID(t *testing.T) {
	payload := &pb.CommandResult{
		Status: string(constants.ExecutionStatusFailed),
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
// must stamp the configured operator APIKey onto the outbound G8eMessage for
// identity continuity, matching the PublishResultEnvelope path in
// pubsub_results.go.
func TestPublishLFAA_StampsAPIKeyFromConfig(t *testing.T) {
	ctx := context.Background()
	logger := testutil.NewTestLogger()

	cmdMsg := PubSubCommandMessage{
		ID:                "cmd-1",
		EventType:         constants.Event.Operator.PortCheck.Requested,
		CaseID:            "case-lfaa",
		InvestigationID:   "inv-lfaa",
		OperatorSessionID: "opsess-lfaa",
	}

	decode := func(t *testing.T, data []byte) models.G8eMessage {
		t.Helper()
		var m models.G8eMessage
		require.NoError(t, json.Unmarshal(data, &m))
		return m
	}

	t.Run("typed response stamps APIKey", func(t *testing.T) {
		client := NewMockG8esPubSubClient()
		defer client.Close()
		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_typed_key"

		publishLFAATypedResponseTo(ctx, client, cfg, logger, cmdMsg,
			constants.Event.Operator.PortCheck.Completed,
			&pb.PortCheckResult{Status: string(constants.ExecutionStatusCompleted)})

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Equal(t, "g8e_typed_key", decode(t, published.Data).APIKey)
	})

	t.Run("error response stamps APIKey", func(t *testing.T) {
		client := NewMockG8esPubSubClient()
		defer client.Close()
		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_err_key"

		publishLFAAErrorTo(ctx, client, cfg, logger, cmdMsg,
			constants.Event.Operator.PortCheck.Failed, "boom")

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Equal(t, "g8e_err_key", decode(t, published.Data).APIKey)
	})

	t.Run("raw response stamps APIKey", func(t *testing.T) {
		client := NewMockG8esPubSubClient()
		defer client.Close()
		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_raw_key"

		publishLFAAResponseTo(ctx, client, cfg, logger, cmdMsg,
			constants.Event.Operator.FetchHistory.Completed,
			[]byte(`{"success":true,"events":[]}`))

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Equal(t, "g8e_raw_key", decode(t, published.Data).APIKey)
	})
}
