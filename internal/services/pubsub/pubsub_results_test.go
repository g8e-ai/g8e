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
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	pb "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func requireLastPublishedUniversal(t *testing.T, db *MockOperatorPubSubClient) []byte {
	t.Helper()
	published := db.LastPublished()
	require.NotNil(t, published, "expected a message to be published")
	return published.Data
}

func mustUnmarshalGovernanceEnvelope(t *testing.T, data []byte) *commonv1.GovernanceEnvelope {
	t.Helper()
	// Decode using protojson — the canonical wire codec the publisher uses
	// (protojson.Marshal). encoding/json cannot parse protojson output
	// (RFC3339 timestamps, camelCase proto field names).
	var env commonv1.GovernanceEnvelope
	err := protojson.Unmarshal(data, &env)
	require.NoError(t, err, "failed to unmarshal GovernanceEnvelope")
	return &env
}

func TestNewPubSubResultsService(t *testing.T) {
	t.Run("creates service", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

func TestPubSubResultsService_PublishHeartbeat(t *testing.T) {
	t.Run("successful heartbeat publish", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		heartbeat := &pb.HeartbeatResult{
			OperatorId:        cfg.OperatorID,
			OperatorSessionId: cfg.OperatorSessionId,
			Status:            "healthy",
			Timestamp:         models.NowTimestamp(),
		}

		err = svc.PublishHeartbeat(context.Background(), heartbeat)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.Heartbeat), env.EventType)
	})
}

func TestPubSubResultsService_PublishCancellationResult(t *testing.T) {
	t.Run("successful cancellation publish", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.CommandResult{
			ExecutionId: "req-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_CANCELLED,
			Stdout:      "cancelled",
			ReturnCode:  130,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.CancelRequested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishCancellationResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.Command.Cancelled), env.EventType)
	})
}

func TestPubSubResultsService_PublishFsListResult(t *testing.T) {
	t.Run("successful fs list publish", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.FsListResult{
			ExecutionId: "req-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Entries:     []*pb.FsEntry{{Name: "test.txt", Size: 100}},
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.FsList.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishFsListResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.FsList.Completed), env.EventType)
	})

	t.Run("publishes failed status on error", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.FsListResult{
			ExecutionId:  "req-123",
			Status:       pb.ExecutionStatus_EXECUTION_STATUS_FAILED,
			ErrorMessage: "permission denied",
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.FsList.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishFsListResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.FsList.Failed), env.EventType)
	})
}

func TestPubSubResultsService_PublishFsGrepResult(t *testing.T) {
	t.Run("successful fs grep publish", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.FsGrepResult{
			ExecutionId: "req-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Matches:     []*pb.FsGrepMatch{{Path: filepath.Join(tmpDir, "test.txt"), LineNumber: 1, Content: "match"}},
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.FsGrep.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishFsGrepResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.FsGrep.Completed), env.EventType)
	})

	t.Run("publishes failed status on error", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.FsGrepResult{
			ExecutionId:  "req-123",
			Status:       pb.ExecutionStatus_EXECUTION_STATUS_FAILED,
			ErrorMessage: "pattern error",
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.FsGrep.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishFsGrepResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.FsGrep.Failed), env.EventType)
	})
}

func TestPubSubResultsService_PublishExecutionResult(t *testing.T) {
	t.Run("successful publish", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.CommandResult{
			ExecutionId:          "req-123",
			Status:               pb.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Stdout:               "test\n",
			ReturnCode:           0,
			ExecutionTimeSeconds: 2.0,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishExecutionResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)

		assert.Equal(t, string(constants.Event.Operator.Command.Completed), env.EventType)
		assert.Equal(t, "EXECUTE_BASH_RESULT", env.ActionType)
		assert.Equal(t, "case-456", env.CaseId)
		assert.Equal(t, "msg-123", env.Id)

		// Verify payload_type was injected into IntentData for agent Pydantic
		require.NotNil(t, env.IntentData)
		assert.Equal(t, "execution_result", env.IntentData.Fields["payload_type"].GetStringValue())

		var payload pb.CommandResult
		err = proto.Unmarshal(env.Payload, &payload)
		require.NoError(t, err)
		assert.Equal(t, "req-123", payload.ExecutionId)
	})

	t.Run("publishes failed status on error", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.CommandResult{
			ExecutionId: "req-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_FAILED,
			Stderr:      "error occurred",
			ReturnCode:  1,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishExecutionResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.Command.Failed), env.EventType)
	})

	t.Run("publishes timeout status on timeout", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.CommandResult{
			ExecutionId: "req-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_TIMEOUT,
			Stderr:      "timeout",
			ReturnCode:  124,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishExecutionResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.Command.Failed), env.EventType)
	})
}

func TestPubSubResultsService_PublishFileEditResult(t *testing.T) {
	t.Run("successful publish", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.FileEditResult{
			ExecutionId: "req-123",
			Operation:   "write",
			FilePath:    filepath.Join(tmpDir, "test.txt"),
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.FileEdit.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishFileEditResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.FileEdit.Completed), env.EventType)
	})

	t.Run("publishes failed status on error", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		result := &pb.FileEditResult{
			ExecutionId:  "req-123",
			Operation:    "write",
			FilePath:     filepath.Join(tmpDir, "test.txt"),
			Status:       pb.ExecutionStatus_EXECUTION_STATUS_FAILED,
			ErrorMessage: "permission denied",
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.FileEdit.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishFileEditResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.FileEdit.Failed), env.EventType)
	})
}

func TestPubSubResultsService_PublishExecutionStatus(t *testing.T) {
	t.Run("publishes running status", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		status := &pb.CommandResult{
			ExecutionId: "exec-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		}

		taskID := "task-101"
		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			InvestigationID:   "invest-789",
			TaskID:            &taskID,
			WebSessionID:      "web-session-123",
			CLISessionID:      "cli-session-456",
			OperatorSessionID: "op-session-789",
		}

		err = svc.PublishExecutionStatus(context.Background(), status, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.Command.StatusUpdated.Running), env.EventType)
	})

	t.Run("publishes completed status", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		status := &pb.CommandResult{
			ExecutionId: "exec-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "op-session-789",
		}

		err = svc.PublishExecutionStatus(context.Background(), status, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.Command.StatusUpdated.Completed), env.EventType)
	})

	t.Run("publishes failed status", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		status := &pb.CommandResult{
			ExecutionId: "exec-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_FAILED,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "op-session-789",
		}

		err = svc.PublishExecutionStatus(context.Background(), status, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.Command.StatusUpdated.Failed), env.EventType)
	})

	t.Run("publishes cancelled status", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		status := &pb.CommandResult{
			ExecutionId: "exec-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_CANCELLED,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "op-session-789",
		}

		err = svc.PublishExecutionStatus(context.Background(), status, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.Command.StatusUpdated.Cancelled), env.EventType)
	})

	t.Run("publishes queued status for unspecified", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		status := &pb.CommandResult{
			ExecutionId: "exec-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "op-session-789",
		}

		err = svc.PublishExecutionStatus(context.Background(), status, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, string(constants.Event.Operator.Command.StatusUpdated.Queued), env.EventType)
	})

	t.Run("uses original message ID for correlation", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		status := &pb.CommandResult{
			ExecutionId: "exec-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "op-session-789",
		}

		err = svc.PublishExecutionStatus(context.Background(), status, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, "msg-123", env.Id)
	})

	t.Run("uses custom operator ID when provided", func(t *testing.T) {
		t.Parallel()
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		customOpID := "custom-operator-123"
		status := &pb.CommandResult{
			ExecutionId: "exec-123",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		}

		originalMsg := &PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "op-session-789",
			OperatorID:        &customOpID,
		}

		err = svc.PublishExecutionStatus(context.Background(), status, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, customOpID, env.OperatorId)
	})
}
