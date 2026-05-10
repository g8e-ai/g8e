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

//go:build integration

package pubsub

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAIAgentServicesMessageFormat_CommandRequest validates that g8eo can unmarshal the EXACT message format that AI Agent Services sends
func TestAIAgentServicesMessageFormat_CommandRequest(t *testing.T) {
	t.Run("validates AI Agent Services command request format with ISO timestamp string", func(t *testing.T) {
		// This is the EXACT format AI Agent Services sends - timestamp is an ISO string, not a time.Time!
		aiServicesMessageJSON := fmt.Sprintf(`{
			"id": "exec_48b6ba793ffe_1764019469",
			"event_type": "%s",`, constants.Event.Operator.Command.Requested) + `
			"case_id": "ec9d560b-ee88-47f3-94e9-c96c7fd0847f",
			"task_id": "ai.command",
			"investigation_id": "2190729a-2909-4199-a342-e8a291cd5929",
			"operator_session_id": "session_1764019197657_e9aa6cb4-2c1f-4ba3-badb-7b115f285cbf",
			"operator_session_id": "session_1764019456527_088d6ea3-336a-4afc-b9c9-0f044c3e0d43",
			"operator_id": "28a08c8f-6bc3-4ddb-b429-adb35691adc7_operator_1_1764019197447_d2ajn",
			"payload": {
				"execution_id": "exec_48b6ba793ffe_1764019469",
				"command": "ping -c 4 google.com",
				"justification": "Pinging google.com to check for network connectivity.",
				"timeout_seconds": 300,
				"requested_at": "2025-11-24T21:24:29.361000+00:00",
				"source": "ai.tool.call",
				"user_id": "28a08c8f-6bc3-4ddb-b429-adb35691adc7"
			},
			"timestamp": "2025-11-24T21:24:33.42443+00:00"
		}`

		var msg PubSubCommandMessage
		err := json.Unmarshal([]byte(aiServicesMessageJSON), &msg)

		// THIS TEST WILL FAIL if g8eo can't parse AI Agent Services message format!
		require.NoError(t, err, "g8eo MUST be able to unmarshal the exact message format that AI Agent Services sends")

		// Validate all fields were parsed correctly
		assert.Equal(t, "exec_48b6ba793ffe_1764019469", msg.ID)
		assert.Equal(t, constants.Event.Operator.Command.Requested, msg.EventType)
		assert.Equal(t, "ec9d560b-ee88-47f3-94e9-c96c7fd0847f", msg.CaseID)
		assert.NotNil(t, msg.TaskID)
		assert.Equal(t, constants.Status.AITaskID.Command, *msg.TaskID)
		assert.Equal(t, "2190729a-2909-4199-a342-e8a291cd5929", msg.InvestigationID)
		assert.NotEmpty(t, msg.OperatorSessionID)
		assert.NotNil(t, msg.OperatorID)

		// Validate payload
		assert.NotNil(t, msg.Payload)
		var payload models.CommandPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &payload))
		assert.Equal(t, "exec_48b6ba793ffe_1764019469", payload.ExecutionID)
		assert.Equal(t, "ping -c 4 google.com", payload.Command)
		assert.Equal(t, "Pinging google.com to check for network connectivity.", payload.Justification)

		// Validate timestamp was parsed (this is the critical part!)
		assert.False(t, msg.Timestamp.IsZero(), "Timestamp MUST be parsed from AI Agent Services ISO string format")

		// Verify the timestamp is approximately correct (within 1 year for sanity)
		now := time.Now()
		timeDiff := now.Sub(msg.Timestamp)
		assert.True(t, timeDiff < 365*24*time.Hour && timeDiff > -365*24*time.Hour,
			"Parsed timestamp should be reasonable: got %v, now is %v", msg.Timestamp, now)
	})
}

// TestAIAgentServicesMessageFormat_FileEditRequest validates file edit requests
func TestAIAgentServicesMessageFormat_FileEditRequest(t *testing.T) {
	t.Run("validates AI Agent Services file edit request format with ISO timestamp string", func(t *testing.T) {
		aiServicesMessageJSON := fmt.Sprintf(`{
			"id": "edit_123abc_1764019469",
			"event_type": "%s",`, constants.Event.Operator.FileEdit.Requested) + `
			"case_id": "case-789",
			"investigation_id": "inv-456",
			"operator_session_id": "session_123",
			"operator_session_id": "session_456",
			"operator_id": "operator_789",
			"payload": {
				"file_path": "/tmp/test.txt",
				"operation": "write",
				"content": "test content"
			},
			"timestamp": "2025-11-24T21:24:33.42443+00:00"
		}`

		var msg PubSubCommandMessage
		err := json.Unmarshal([]byte(aiServicesMessageJSON), &msg)

		require.NoError(t, err, "g8eo MUST be able to unmarshal AI Agent Services file edit messages")
		assert.Equal(t, constants.Event.Operator.FileEdit.Requested, msg.EventType)
		assert.False(t, msg.Timestamp.IsZero())
	})
}

// TestAIAgentServicesMessageFormat_AllTimestampFormats tests various ISO timestamp formats AI Agent Services might send
func TestAIAgentServicesMessageFormat_AllTimestampFormats(t *testing.T) {
	testCases := []struct {
		name      string
		timestamp string
		shouldOK  bool
	}{
		{
			name:      "Python isoformat with microseconds",
			timestamp: "2025-11-24T21:24:33.42443+00:00",
			shouldOK:  true,
		},
		{
			name:      "Python isoformat without microseconds",
			timestamp: "2025-11-24T21:24:33+00:00",
			shouldOK:  true,
		},
		{
			name:      "RFC3339 with Z",
			timestamp: "2025-11-24T21:24:33Z",
			shouldOK:  true,
		},
		{
			name:      "RFC3339 with timezone",
			timestamp: "2025-11-24T21:24:33-07:00",
			shouldOK:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonMsg := fmt.Sprintf(`{
				"id": "test-123",
				"event_type": "%s",
				"case_id": "case-789",
				"payload": {},
				"timestamp": "%s"
			}`, constants.Event.Operator.Command.Requested, tc.timestamp)

			var msg PubSubCommandMessage
			err := json.Unmarshal([]byte(jsonMsg), &msg)

			if tc.shouldOK {
				assert.NoError(t, err, "Should parse timestamp format: %s", tc.name)
				assert.False(t, msg.Timestamp.IsZero(), "Timestamp should not be zero")
			} else {
				assert.Error(t, err, "Should reject invalid timestamp format: %s", tc.name)
			}
		})
	}
}

// TestPubSubCommandMessage_RoundTrip ensures g8eo can receive, process, and send messages
func TestPubSubCommandMessage_RoundTrip(t *testing.T) {
	t.Run("unmarshal AI Agent Services message, process, and check result format", func(t *testing.T) {
		// Simulate AI Agent Services sending a message
		aiServicesJSON := fmt.Sprintf(`{
			"id": "exec_test_123",
			"event_type": "%s",
			"case_id": "case-xyz",
			"payload": {
				"command": "echo test",
				"justification": "testing"
			},
			"timestamp": "2025-11-24T21:24:33.42443+00:00"
		}`, constants.Event.Operator.Command.Requested)

		var receivedMsg PubSubCommandMessage
		err := json.Unmarshal([]byte(aiServicesJSON), &receivedMsg)
		require.NoError(t, err)

		// Verify we can extract command from payload
		var payload models.CommandPayload
		require.NoError(t, json.Unmarshal(receivedMsg.Payload, &payload))
		assert.Equal(t, "echo test", payload.Command)
		assert.Equal(t, "testing", payload.Justification)
	})
}
