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
	"fmt"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoopback_CommandDispatch_HistoryAndAudit tests execution requests that were
// previously missing from loopback coverage: FetchLogs, FetchHistory, FetchFileHistory,
// FetchFileDiff, RestoreFile, and Audit events.
func TestLoopback_CommandDispatch_HistoryAndAudit(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub) // Consume automatic heartbeat

	tests := []struct {
		name      string
		eventType string
		payload   interface{}
		expected  string
	}{
		{
			name:      "FetchLogs",
			eventType: constants.Event.Operator.FetchLogs.Requested,
			payload:   map[string]interface{}{"execution_id": "exec-1"},
			expected:  constants.Event.Operator.FetchLogs.Failed,
		},
		{
			name:      "FetchHistory",
			eventType: constants.Event.Operator.FetchHistory.Requested,
			payload:   map[string]interface{}{"limit": 10},
			expected:  constants.Event.Operator.FetchHistory.Failed,
		},
		{
			name:      "FetchFileHistory",
			eventType: constants.Event.Operator.FetchFileHistory.Requested,
			payload:   map[string]interface{}{"file_path": "test.txt"},
			expected:  constants.Event.Operator.FetchFileHistory.Failed,
		},
		{
			name:      "FetchFileDiff",
			eventType: constants.Event.Operator.FetchFileDiff.Requested,
			payload:   map[string]interface{}{"file_path": "test.txt"},
			expected:  constants.Event.Operator.FetchFileDiff.Failed,
		},
		{
			name:      "RestoreFile",
			eventType: constants.Event.Operator.RestoreFile.Requested,
			payload:   map[string]interface{}{"file_path": "test.txt", "commit_hash": "abc"},
			expected:  constants.Event.Operator.RestoreFile.Failed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloadBytes, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			injectCmd(t, f, svc, PubSubCommandMessage{
				ID:              fmt.Sprintf("id-%s", tt.name),
				EventType:       tt.eventType,
				CaseID:          "case-1",
				InvestigationID: "inv-1",
				Payload:         payloadBytes,
				Timestamp:       time.Now().UTC(),
			})

			msg := drainOne(t, resultsSub)
			assert.Contains(t, string(msg), tt.expected, "expected event type %s in result", tt.expected)
		})
	}

	t.Run("AuditEvents", func(t *testing.T) {
		auditTests := []struct {
			name      string
			eventType string
		}{
			{"UserMsg", constants.Event.Operator.Audit.UserMsg},
			{"AIMsg", constants.Event.Operator.Audit.AIMsg},
			{"DirectCmd", constants.Event.Operator.Audit.DirectCmd},
			{"DirectCmdResult", constants.Event.Operator.Audit.DirectCmdResult},
		}

		for _, tt := range auditTests {
			injectCmd(t, f, svc, PubSubCommandMessage{
				ID:        fmt.Sprintf("audit-%s", tt.name),
				EventType: tt.eventType,
				CaseID:    "case-1",
				Payload:   json.RawMessage(`{}`),
				Timestamp: time.Now().UTC(),
			})
			// Audit events are fire-and-forget in the dispatcher,
			// they don't necessarily publish a "Completed" event back to results
			// unless the specific handler does so. We verify no panic.
		}
	})
}
