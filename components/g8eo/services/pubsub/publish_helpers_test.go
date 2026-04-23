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
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/stretchr/testify/assert"
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
