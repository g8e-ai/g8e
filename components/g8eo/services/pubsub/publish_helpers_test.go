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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Wire contract: every LFAA result payload must carry an execution_id so g8ee
// can correlate operator responses back to the originating request. Typed
// payloads (ExecutionResultsPayload, FsReadResultPayload, ...) set it
// explicitly; this helper injects it for payloads that don't (LFAAErrorPayload,
// FetchFileHistoryResultPayload, FetchFileDiffResultPayload, ...).
func TestMergeExecutionID_InjectsWhenMissing(t *testing.T) {
	raw := json.RawMessage(`{"success":false,"error":"history handler not available on this operator"}`)

	merged := mergeExecutionID(raw, "msg-abc")

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(merged, &got))
	assert.Equal(t, "msg-abc", got["execution_id"])
	assert.Equal(t, false, got["success"])
	assert.Equal(t, "history handler not available on this operator", got["error"])
}

func TestMergeExecutionID_PreservesExistingValue(t *testing.T) {
	raw := json.RawMessage(`{"execution_id":"typed-value","stdout":"hi"}`)

	merged := mergeExecutionID(raw, "msg-abc")

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(merged, &got))
	assert.Equal(t, "typed-value", got["execution_id"], "typed payload's own execution_id must win")
}

func TestMergeExecutionID_OverwritesEmptyString(t *testing.T) {
	raw := json.RawMessage(`{"execution_id":"","stdout":"hi"}`)

	merged := mergeExecutionID(raw, "msg-abc")

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(merged, &got))
	assert.Equal(t, "msg-abc", got["execution_id"])
}

func TestMergeExecutionID_NoOpOnEmptyInputs(t *testing.T) {
	assert.Equal(t, json.RawMessage(nil), mergeExecutionID(nil, "msg-abc"))

	raw := json.RawMessage(`{"a":1}`)
	assert.Equal(t, raw, mergeExecutionID(raw, ""), "empty execution_id must leave payload untouched")
}

func TestMergeExecutionID_NonObjectPayloadUntouched(t *testing.T) {
	raw := json.RawMessage(`[1,2,3]`)
	assert.Equal(t, raw, mergeExecutionID(raw, "msg-abc"))
}
