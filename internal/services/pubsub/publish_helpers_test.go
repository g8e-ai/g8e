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
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// mockAuditStore is a simple in-memory mock for testing
type mockAuditStore struct {
	mu               sync.Mutex
	events           []*storage.Event
	recordEventError bool
}

func (m *mockAuditStore) RecordEvent(event *storage.Event) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recordEventError {
		return 0, assert.AnError
	}
	m.events = append(m.events, event)
	return int64(len(m.events)), nil
}

func (m *mockAuditStore) GetEvents() []*storage.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events
}

func (m *mockAuditStore) SetRecordEventError(err bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordEventError = err
}

func TestExecutionIDFromMessage(t *testing.T) {
	t.Run("extracts execution_id from payload", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{Command: "ls -la", ExecutionId: "exec-123"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   payload,
		}

		execID := executionIDFromMessage(msg)
		assert.Equal(t, "exec-123", execID)
	})

	t.Run("falls back to message ID when payload has no execution_id", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{Command: "ls -la"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   payload,
		}

		execID := executionIDFromMessage(msg)
		assert.Equal(t, "msg-1", execID)
	})

	t.Run("falls back to message ID on unmarshal error", func(t *testing.T) {
		t.Parallel()
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		execID := executionIDFromMessage(msg)
		assert.Equal(t, "msg-1", execID)
	})
}

func TestSetExecutionIDOnPayload(t *testing.T) {
	t.Run("sets execution_id on payload with field", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{Command: "ls -la"}
		setExecutionIDOnPayload(req, "exec-123")
		assert.Equal(t, "exec-123", req.ExecutionId)
	})

	t.Run("does nothing when execution_id is empty", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{Command: "ls -la", ExecutionId: "original"}
		setExecutionIDOnPayload(req, "")
		assert.Equal(t, "original", req.ExecutionId)
	})

	t.Run("does nothing when payload has no execution_id field", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.HeartbeatRequested{}
		setExecutionIDOnPayload(req, "exec-123")
		// Should not panic
	})
}

func TestPublishLFAATypedResponseTo(t *testing.T) {
	t.Run("publishes typed response successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-1",
			InvestigationID:   "investigation-1",
			OperatorSessionID: "session-1",
			WebSessionID:      "web-session-1",
			CLISessionID:      "cli-session-1",
			Payload: mustMarshalProto(t, &operatorv1.CommandRequested{
				Command:     "ls -la",
				ExecutionId: "exec-123",
			}),
		}

		payload := &operatorv1.CommandResult{
			ExecutionId: "exec-123",
			Status:      operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		}

		publishLFAATypedResponseTo(context.Background(), client, cfg, logger, msg, constants.Event.Operator.Command.Completed, payload, nil, nil)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.NotEmpty(t, published.Data)
	})

	t.Run("handles build envelope error gracefully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-1",
			InvestigationID:   "investigation-1",
			OperatorSessionID: "session-1",
			WebSessionID:      "web-session-1",
			CLISessionID:      "cli-session-1",
			Payload:           []byte("invalid protobuf"),
		}

		payload := &operatorv1.CommandResult{
			ExecutionId: "exec-123",
			Status:      operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		}

		publishLFAATypedResponseTo(context.Background(), client, cfg, logger, msg, constants.Event.Operator.Command.Completed, payload, nil, nil)
		// Should log error and not panic
	})

	t.Run("handles publish error gracefully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		client.SetPublishError(true)

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-1",
			InvestigationID:   "investigation-1",
			OperatorSessionID: "session-1",
			WebSessionID:      "web-session-1",
			CLISessionID:      "cli-session-1",
			Payload: mustMarshalProto(t, &operatorv1.CommandRequested{
				Command:     "ls -la",
				ExecutionId: "exec-123",
			}),
		}

		payload := &operatorv1.CommandResult{
			ExecutionId: "exec-123",
			Status:      operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		}

		publishLFAATypedResponseTo(context.Background(), client, cfg, logger, msg, constants.Event.Operator.Command.Completed, payload, nil, nil)
		// Should log error and not panic
	})
}

func TestPublishLFAAErrorTo(t *testing.T) {
	t.Run("publishes error response successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-1",
			InvestigationID:   "investigation-1",
			OperatorSessionID: "session-1",
			WebSessionID:      "web-session-1",
			CLISessionID:      "cli-session-1",
			Payload:           []byte("invalid protobuf"),
		}

		publishLFAAErrorTo(context.Background(), client, cfg, logger, msg, constants.Event.Operator.Command.Failed, "test error", nil, nil)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.NotEmpty(t, published.Data)
	})

	t.Run("handles publish error gracefully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		client.SetPublishError(true)

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-1",
			InvestigationID:   "investigation-1",
			OperatorSessionID: "session-1",
			WebSessionID:      "web-session-1",
			CLISessionID:      "cli-session-1",
			Payload:           []byte("invalid protobuf"),
		}

		publishLFAAErrorTo(context.Background(), client, cfg, logger, msg, constants.Event.Operator.Command.Failed, "test error", nil, nil)
		// Should log error and not panic
	})
}

func TestPublishObservedStateEvidence(t *testing.T) {
	t.Run("persists observed-state content for FsListResult", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		auditStore := &mockAuditStore{}

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.FsListResult{
			Entries: []*operatorv1.FsEntry{
				{Name: "file1.txt", Size: 100},
				{Name: "file2.txt", Size: 200},
			},
		}

		publishObservedStateEvidence(context.Background(), logger, msg, constants.Event.Operator.FsList.Completed, payload, auditStore, nil)

		events := auditStore.GetEvents()
		require.Len(t, events, 1)
		assert.Equal(t, constants.Event.Operator.FsList.Completed, events[0].Type)
		assert.NotEmpty(t, events[0].ContentText)
		assert.Contains(t, events[0].ContentText, "file1.txt")
	})

	t.Run("persists observed-state content for FsReadResult", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		auditStore := &mockAuditStore{}

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.FsReadResult{
			Content: "file content here",
		}

		publishObservedStateEvidence(context.Background(), logger, msg, constants.Event.Operator.FsRead.Completed, payload, auditStore, nil)

		events := auditStore.GetEvents()
		require.Len(t, events, 1)
		assert.Equal(t, constants.Event.Operator.FsRead.Completed, events[0].Type)
		assert.Equal(t, "file content here", events[0].ContentText)
	})

	t.Run("persists observed-state content for PortCheckResult", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		auditStore := &mockAuditStore{}

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.PortCheckResult{
			Results: []*operatorv1.PortCheckEntry{
				{Host: "localhost", Port: 8080, Open: true},
			},
		}

		publishObservedStateEvidence(context.Background(), logger, msg, constants.Event.Operator.PortCheck.Completed, payload, auditStore, nil)

		events := auditStore.GetEvents()
		require.Len(t, events, 1)
		assert.Equal(t, constants.Event.Operator.PortCheck.Completed, events[0].Type)
		assert.NotEmpty(t, events[0].ContentText)
	})

	t.Run("scrubs sensitive content before persisting", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		auditStore := &mockAuditStore{}
		scrubbingSvc := scrubbing.NewScrubbingService(scrubbing.DefaultConfig(), logger, nil)

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.FsReadResult{
			Content: "password=secret123 api_key=ghp_test_token",
		}

		publishObservedStateEvidence(context.Background(), logger, msg, constants.Event.Operator.FsRead.Completed, payload, auditStore, scrubbingSvc)

		events := auditStore.GetEvents()
		require.Len(t, events, 1)
		assert.Equal(t, constants.Event.Operator.FsRead.Completed, events[0].Type)
		// Content should be scrubbed
		assert.NotContains(t, events[0].ContentText, "secret123")
		assert.NotContains(t, events[0].ContentText, "ghp_test_token")
	})

	t.Run("does not persist for command events (double-recording guard)", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		auditStore := &mockAuditStore{}

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.CommandResult{
			ExecutionId: "exec-1",
			Status:      operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		}

		// Command events should not be recorded via publishObservedStateEvidence
		publishObservedStateEvidence(context.Background(), logger, msg, constants.Event.Operator.Command.Completed, payload, auditStore, nil)

		events := auditStore.GetEvents()
		assert.Empty(t, events, "command events should not be recorded via observed-state path")
	})

	t.Run("non-fatal: store errors do not prevent publish", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		auditStore := &mockAuditStore{}
		auditStore.SetRecordEventError(true) // Simulate store error

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.FsReadResult{
			Content: "file content",
		}

		// Should not panic despite store error
		assert.NotPanics(t, func() {
			publishObservedStateEvidence(context.Background(), logger, msg, constants.Event.Operator.FsRead.Completed, payload, auditStore, nil)
		})
	})

	t.Run("handles nil auditStore gracefully", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.FsReadResult{
			Content: "file content",
		}

		// Should not panic with nil auditStore
		assert.NotPanics(t, func() {
			publishObservedStateEvidence(context.Background(), logger, msg, constants.Event.Operator.FsRead.Completed, payload, nil, nil)
		})
	})

	t.Run("handles empty content gracefully", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		auditStore := &mockAuditStore{}

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.FsListResult{
			Entries: []*operatorv1.FsEntry{}, // Empty list
		}

		publishObservedStateEvidence(context.Background(), logger, msg, constants.Event.Operator.FsList.Completed, payload, auditStore, nil)

		events := auditStore.GetEvents()
		assert.Empty(t, events, "empty content should not be recorded")
	})

	t.Run("handles nil scrubbingService gracefully", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		auditStore := &mockAuditStore{}

		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			OperatorSessionID: "session-1",
		}

		payload := &operatorv1.FsReadResult{
			Content: "password=secret123",
		}

		publishObservedStateEvidence(context.Background(), logger, msg, constants.Event.Operator.FsRead.Completed, payload, auditStore, nil)

		events := auditStore.GetEvents()
		require.Len(t, events, 1)
		// Without scrubbing, content should be preserved as-is
		assert.Contains(t, events[0].ContentText, "secret123")
	})
}
