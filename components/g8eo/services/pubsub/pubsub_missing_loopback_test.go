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
	"fmt"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
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
		expected  string
	}{
		{
			name:      "FetchLogs",
			eventType: constants.Event.Operator.FetchLogs.Requested,
			expected:  constants.Event.Operator.FetchLogs.Failed,
		},
		{
			name:      "FetchHistory",
			eventType: constants.Event.Operator.FetchHistory.Requested,
			expected:  constants.Event.Operator.FetchHistory.Failed,
		},
		{
			name:      "FetchFileHistory",
			eventType: constants.Event.Operator.FetchFileHistory.Requested,
			expected:  constants.Event.Operator.FetchFileHistory.Failed,
		},
		{
			name:      "FetchFileDiff",
			eventType: constants.Event.Operator.FetchFileDiff.Requested,
			expected:  constants.Event.Operator.FetchFileDiff.Failed,
		},
		{
			name:      "RestoreFile",
			eventType: constants.Event.Operator.RestoreFile.Requested,
			expected:  constants.Event.Operator.RestoreFile.Failed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payloadBytes []byte
			switch tt.eventType {
			case constants.Event.Operator.FetchLogs.Requested:
				payloadBytes = testutil.MustMarshalProtobufFetchLogsRequested(t, "exec-1")
			case constants.Event.Operator.FetchHistory.Requested:
				payloadBytes = testutil.MustMarshalProtobufFetchHistoryRequested(t, "exec-1", svc.config.OperatorSessionId, 50, 0)
			case constants.Event.Operator.FetchFileHistory.Requested:
				payloadBytes = testutil.MustMarshalProtobufFetchFileHistoryRequested(t, "exec-1", "test.txt", 50)
			case constants.Event.Operator.FetchFileDiff.Requested:
				payloadBytes = testutil.MustMarshalProtobufFetchFileDiffRequested(t, "exec-1", "test.txt")
			case constants.Event.Operator.RestoreFile.Requested:
				payloadBytes = testutil.MustMarshalProtobufRestoreFileRequested(t, "exec-1", "test.txt", "abc", svc.config.OperatorSessionId)
			}

			envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, fmt.Sprintf("id-%s", tt.name), tt.eventType, payloadBytes, "", svc.config.OperatorID, "case-1", "inv-1", svc.config.OperatorSessionId)
			injectCmdProtobuf(t, f, svc, envelopeBytes)

			msg := drainOne(t, resultsSub)
			envelope := testutil.MustUnmarshalUniversalEnvelope(t, msg)
			assert.Equal(t, tt.expected, envelope.EventType)
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
			envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, fmt.Sprintf("audit-%s", tt.name), tt.eventType, []byte("{}"), "", svc.config.OperatorID, "case-1", "inv-1", svc.config.OperatorSessionId)
			injectCmdProtobuf(t, f, svc, envelopeBytes)
			// Audit events are fire-and-forget in the dispatcher,
			// they don't necessarily publish a "Completed" event back to results
			// unless the specific handler does so. We verify no panic.
		}
	})
}
