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
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionIDFromMessage_PrefersPayloadExecutionID(t *testing.T) {
	payload, err := json.Marshal(map[string]string{"execution_id": "exec-123"})
	assert.NoError(t, err)
	msg := PubSubCommandMessage{ID: "envelope-abc", Payload: payload}
	assert.Equal(t, "exec-123", executionIDFromMessage(msg))
}

func TestExecutionIDFromMessage_FallsBackToEnvelopeID(t *testing.T) {
	payload, err := json.Marshal(map[string]string{"other": "value"})
	assert.NoError(t, err)
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
	payload, err := json.Marshal(map[string]string{"execution_id": ""})
	assert.NoError(t, err)
	msg := PubSubCommandMessage{ID: "envelope-abc", Payload: payload}
	assert.Equal(t, "envelope-abc", executionIDFromMessage(msg))
}

// For MCP-translated envelopes, msg.ID has been rewritten to the JSON-RPC
// request id by handleMCPToolsCall and is NOT the execution_id. The fallback
// must be suppressed so callers don't stamp results with a wrong id.
func TestExecutionIDFromMessage_MCPEnvelopeDoesNotFallBack(t *testing.T) {
	payload, err := json.Marshal(map[string]string{"other": "value"})
	assert.NoError(t, err)
	msg := PubSubCommandMessage{
		ID:        "jsonrpc-req-id",
		EventType: constants.Event.Operator.MCP.ToolsCall,
		Payload:   payload,
	}
	assert.Equal(t, "", executionIDFromMessage(msg))
}

func TestExecutionIDFromMessage_MCPEnvelopePrefersPayloadExecutionID(t *testing.T) {
	payload, err := json.Marshal(map[string]string{"execution_id": "cmd_abc_123"})
	assert.NoError(t, err)
	msg := PubSubCommandMessage{
		ID:        "jsonrpc-req-id",
		EventType: constants.Event.Operator.MCP.ToolsCall,
		Payload:   payload,
	}
	assert.Equal(t, "cmd_abc_123", executionIDFromMessage(msg))
}

func TestSetExecutionIDOnPayload_LFAAErrorPayload(t *testing.T) {
	payload := &models.LFAAErrorPayload{
		Success: false,
		Error:   "test error",
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_FileEditResultPayload(t *testing.T) {
	payload := &models.FileEditResultPayload{
		ExecutionID: "original-id",
		Status:      constants.ExecutionStatusCompleted,
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_FsListResultPayload(t *testing.T) {
	payload := &models.FsListResultPayload{
		ExecutionID: "",
		Status:      constants.ExecutionStatusCompleted,
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_PortCheckResultPayload(t *testing.T) {
	payload := &models.PortCheckResultPayload{
		ExecutionID: "existing-id",
		Status:      constants.ExecutionStatusCompleted,
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_EmptyExecutionID(t *testing.T) {
	payload := &models.LFAAErrorPayload{
		Success: false,
		Error:   "test error",
	}
	setExecutionIDOnPayload(payload, "")
	assert.Equal(t, "", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_UnsupportedType(t *testing.T) {
	type unsupportedPayload struct {
		Field string
	}
	payload := &unsupportedPayload{Field: "value"}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "value", payload.Field)
}

func TestSetExecutionIDOnPayload_FetchFileDiffResultPayload(t *testing.T) {
	payload := &models.FetchFileDiffResultPayload{
		Success:     true,
		Error:       nil,
		ExecutionID: "original-id",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_FetchHistoryResultPayload(t *testing.T) {
	payload := &models.FetchHistoryResultPayload{
		Success:     true,
		ExecutionID: "",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_FetchFileHistoryResultPayload(t *testing.T) {
	payload := &models.FetchFileHistoryResultPayload{
		Success:     false,
		Error:       "test error",
		ExecutionID: "old-id",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_RestoreFileResultPayload(t *testing.T) {
	payload := &models.RestoreFileResultPayload{
		Success:     true,
		ExecutionID: "",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionID)
}

// The LFAA publish path (file reads, port checks, fetch logs/history, restore)
// must stamp the configured operator APIKey onto the outbound G8eMessage for
// identity continuity, matching the PublishResultEnvelope path in
// pubsub_results.go. Regression for the asymmetry flagged in the
// mcp-pubsub-bug-fix branch status synthesis.
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
			&models.PortCheckResultPayload{Status: constants.ExecutionStatusCompleted})

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
