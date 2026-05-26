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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

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

		publishLFAATypedResponseTo(context.Background(), client, cfg, logger, msg, constants.Event.Operator.Command.Completed, payload)

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

		publishLFAATypedResponseTo(context.Background(), client, cfg, logger, msg, constants.Event.Operator.Command.Completed, payload)
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

		publishLFAAErrorTo(context.Background(), client, cfg, logger, msg, constants.Event.Operator.Command.Failed, "test error")

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.NotEmpty(t, published.Data)
	})
}
