// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	pubsubtest "github.com/g8e-ai/g8e/v2/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	pb "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func requireLastPublishedUniversal(t *testing.T, db *pubsubtest.MockOperatorPubSubClient) []byte {
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
	t.Run("returns non-nil service without error", func(t *testing.T) {
		t.Parallel()
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		heartbeat := &pb.HeartbeatResult{
			OperatorId:        cfg.OperatorID,
			OperatorSessionId: cfg.OperatorSessionId,
			Status:            "healthy",
			Timestamp:         timesvc.NowTimestamp(),
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		tmpDir := testutil.TempDir(t)
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		tmpDir := testutil.TempDir(t)
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		tmpDir := testutil.TempDir(t)
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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
		db := pubsubtest.NewMockOperatorPubSubClient()
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

func TestPubSubResultsService_PublishActionReceipt(t *testing.T) {
	t.Run("publishes signed receipt envelope to receipts channel with identity propagation", func(t *testing.T) {
		t.Parallel()
		db := pubsubtest.NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		// Build the original command envelope (the one the operator received).
		cmdEnv := &commonv1.GovernanceEnvelope{
			ProtocolVersion:   "1.0",
			OperatorId:        "op-001",
			OperatorSessionId: "sess-001",
			ActionType:        string(constants.ActionTypeFileEdit),
			TargetResource:    "/tmp/test.txt",
			RequestorUserId:   "user-001",
			ActingAppId:       "spiffe://g8e.local/app/g8ee",
			CaseId:            "case-1",
			InvestigationId:   "inv-1",
			TaskId:            "task-1",
			WebSessionId:      "web-1",
			CliSessionId:      "cli-1",
			Posture:           "doctrine",
		}

		receipt := &pb.ActionReceipt{
			TransactionId:    "tx-001",
			TransactionHash:  "hash-001",
			Status:           pb.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:    "completed",
			StateRootBefore:  "root-before",
			StateRootAfter:   "root-after",
			ExecutedAtUnixMs: 1700000000000,
			SignerKeyId:      "actuator-key-1",
			Signature:        "deadbeef",
		}

		err = svc.PublishActionReceipt(context.Background(), cmdEnv, receipt)
		require.NoError(t, err)

		published := db.LastPublished()
		require.NotNil(t, published, "expected a message to be published")
		assert.Equal(t, ReceiptsChannel("op-001", "sess-001"), published.Channel, "must publish to the receipts channel")

		env := mustUnmarshalGovernanceEnvelope(t, published.Data)
		assert.Equal(t, string(constants.Event.Operator.Receipt.Recorded), env.EventType, "event_type must be the receipt recorded event")
		assert.Equal(t, commonv1.Component_COMPONENT_G8EO, env.SourceComponent, "source_component must be G8EO")
		assert.Equal(t, "op-001", env.OperatorId, "operator_id must propagate from the command envelope")
		assert.Equal(t, "sess-001", env.OperatorSessionId, "operator_session_id must propagate")
		assert.Equal(t, string(constants.ActionTypeFileEdit), env.ActionType, "action_type must propagate")
		assert.Equal(t, "/tmp/test.txt", env.TargetResource, "target_resource must propagate")
		assert.Equal(t, "user-001", env.RequestorUserId, "requestor_user_id must propagate")
		assert.Equal(t, "spiffe://g8e.local/app/g8ee", env.ActingAppId, "acting_app_id must propagate")
		assert.Equal(t, "case-1", env.CaseId, "case_id must propagate")
		assert.Equal(t, "inv-1", env.InvestigationId, "investigation_id must propagate")
		assert.Equal(t, "task-1", env.TaskId, "task_id must propagate")
		assert.Equal(t, "web-1", env.WebSessionId, "web_session_id must propagate")
		assert.Equal(t, "cli-1", env.CliSessionId, "cli_session_id must propagate")
		assert.Equal(t, "doctrine", env.Posture, "posture must propagate")

		// The payload must be the marshaled ActionReceipt.
		var decoded pb.ActionReceipt
		err = proto.Unmarshal(env.Payload, &decoded)
		require.NoError(t, err, "payload must be a valid ActionReceipt")
		assert.Equal(t, "tx-001", decoded.TransactionId)
		assert.Equal(t, "hash-001", decoded.TransactionHash)
		assert.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_COMPLETED, decoded.Status)
		assert.Equal(t, "actuator-key-1", decoded.SignerKeyId)
		assert.Equal(t, "deadbeef", decoded.Signature)
	})

	t.Run("nil envelope returns error", func(t *testing.T) {
		t.Parallel()
		db := pubsubtest.NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		err = svc.PublishActionReceipt(context.Background(), nil, &pb.ActionReceipt{})
		require.Error(t, err)
	})

	t.Run("nil receipt returns error", func(t *testing.T) {
		t.Parallel()
		db := pubsubtest.NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db)
		require.NoError(t, err)

		err = svc.PublishActionReceipt(context.Background(), &commonv1.GovernanceEnvelope{}, nil)
		require.Error(t, err)
	})
}
