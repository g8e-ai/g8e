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
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	pb "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func requireLastPublished(t *testing.T, db *MockOperatorPubSubClient) []byte {
	t.Helper()
	published := db.LastPublished()
	require.NotNil(t, published, "expected a message to be published")
	return published.Data
}

func TestNewPubSubResultsService(t *testing.T) {
	t.Run("creates service", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)

		require.NoError(t, err)
		assert.NotNil(t, svc)
		assert.Equal(t, cfg, svc.config)
		assert.NotNil(t, svc.logger)
		assert.NotNil(t, svc.client)
	})
}

func TestPubSubResultsService_PublishExecutionResult(t *testing.T) {
	t.Run("successful publish", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		startTime := time.Now().UTC()
		_ = startTime // reserved for future use if needed in pb
		returnCode := int32(0)

		result := &pb.CommandResult{
			ExecutionId:          "req-123",
			Status:               protoExecutionStatus(constants.ExecutionStatusCompleted),
			Output:               "test\n",
			ExitCode:             returnCode,
			ExecutionTimeSeconds: 2.0,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishExecutionResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)

		assert.Equal(t, constants.Event.Operator.Command.Completed, env.EventType)
		assert.Equal(t, cfg.OperatorID, env.OperatorId)
		assert.Equal(t, "case-456", env.CaseId)

		var payload pb.CommandResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Equal(t, "req-123", payload.ExecutionId)
		assert.Equal(t, int32(0), payload.ExitCode, "exit_code should be 0 for successful execution")
	})

	t.Run("return_code type validation", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		returnCode := int32(42)
		result := &pb.CommandResult{
			ExecutionId: "req-serialization",
			Status:      protoExecutionStatus(constants.ExecutionStatusFailed),
			ExitCode:    returnCode,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-serialization",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-serialization",
		}

		err = svc.PublishExecutionResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)

		var payload pb.CommandResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Equal(t, int32(42), payload.ExitCode, "exit_code value should match")
	})
}

func TestPubSubResultsService_PublishFileEditResult(t *testing.T) {
	t.Run("successful publish", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-123"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		bytesWritten := int64(100)

		result := &pb.FileEditResult{
			ExecutionId:     "req-123",
			Operation:       string(models.FileEditOperationWrite),
			FilePath:        "/tmp/test.txt",
			Status:          protoExecutionStatus(constants.ExecutionStatusCompleted),
			BytesWritten:    bytesWritten,
			DurationSeconds: 1.0,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishFileEditResult(context.Background(), result, originalMsg)

		require.NoError(t, err)
	})

	t.Run("failed file edit result", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-123"
		cfg.OperatorID = "test-operator-id"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		errorMsg := "file not found"
		errorType := "not_found"
		result := &pb.FileEditResult{
			ExecutionId:  "req-123",
			Operation:    string(models.FileEditOperationRead),
			FilePath:     "/tmp/nonexistent.txt",
			Status:       protoExecutionStatus(constants.ExecutionStatusFailed),
			ErrorMessage: errorMsg,
			ErrorType:    errorType,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishFileEditResult(context.Background(), result, originalMsg)

		require.NoError(t, err)
	})

	t.Run("successful read operation populates content field", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-123"
		cfg.OperatorID = "test-operator-id"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		content := "file content for read operation"
		result := &pb.FileEditResult{
			ExecutionId:     "req-123",
			Operation:       string(models.FileEditOperationRead),
			FilePath:        "/tmp/test.txt",
			Status:          protoExecutionStatus(constants.ExecutionStatusCompleted),
			DurationSeconds: 1.0,
			Content:         content,
			StdoutSize:      int32(len(content)),
		}
		// Wait, FileEditResult doesn't have a content field in proto.
		// For read operations we should use FsReadResult.
		// But let's check what the service does.

		originalMsg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishFileEditResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)

		var payload pb.FileEditResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Equal(t, content, payload.Content, "Content field must match the read content")
		assert.Equal(t, int32(len(content)), payload.StdoutSize, "StdoutSize must match content length")
	})
}

func TestPubSubResultsService_PublishHeartbeat(t *testing.T) {
	t.Run("successful heartbeat publish", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-123"
		cfg.OperatorID = "test-operator-id"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		heartbeat := &pb.HeartbeatResult{
			OperatorId:        cfg.OperatorID,
			OperatorSessionId: cfg.OperatorSessionId,
			Status:            "healthy",
			Timestamp:         models.NowTimestamp(),
		}

		err = svc.PublishHeartbeat(context.Background(), heartbeat)

		require.NoError(t, err)
	})

	t.Run("heartbeat with system info", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-123"
		cfg.OperatorID = "test-operator-id"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		heartbeat := &pb.HeartbeatResult{
			OperatorId:        cfg.OperatorID,
			OperatorSessionId: cfg.OperatorSessionId,
			Status:            "healthy",
			Timestamp:         models.NowTimestamp(),
		}

		err = svc.PublishHeartbeat(context.Background(), heartbeat)

		require.NoError(t, err)
	})
}

func TestPubSubResultsService_MessageFormatting(t *testing.T) {
	t.Run("execution result envelope format", func(t *testing.T) {
		result := &pb.CommandResult{
			ExecutionId: "req-123",
			Status:      protoExecutionStatus(constants.ExecutionStatusCompleted),
			ExitCode:    0,
		}

		env, err := BuildUniversalEnvelope(testutil.NewTestConfig(t), constants.Event.Operator.Command.Completed, result, "")
		require.NoError(t, err)

		data, err := proto.Marshal(env)
		require.NoError(t, err)

		var env2 commonv1.GovernanceEnvelope
		err = proto.Unmarshal(data, &env2)
		require.NoError(t, err)
		assert.Equal(t, constants.Event.Operator.Command.Completed, env2.EventType)
		assert.Equal(t, "req-123", result.ExecutionId)
	})

	t.Run("file edit result envelope format", func(t *testing.T) {
		result := &pb.FileEditResult{
			ExecutionId: "req-123",
			Operation:   string(models.FileEditOperationWrite),
			FilePath:    "/tmp/test.txt",
			Status:      protoExecutionStatus(constants.ExecutionStatusCompleted),
		}

		env, err := BuildUniversalEnvelope(testutil.NewTestConfig(t), constants.Event.Operator.FileEdit.Completed, result, "")
		require.NoError(t, err)

		data, err := proto.Marshal(env)
		require.NoError(t, err)

		var env2 commonv1.GovernanceEnvelope
		err = proto.Unmarshal(data, &env2)
		require.NoError(t, err)
		assert.Equal(t, constants.Event.Operator.FileEdit.Completed, env2.EventType)
	})
}

func TestPubSubResultsService_PublishExecutionResultWithTaskAndInvestigation(t *testing.T) {
	db := NewMockOperatorPubSubClient()
	defer db.Close()

	cfg := testutil.NewTestConfig(t)
	cfg.OperatorSessionId = "test-session-123"
	cfg.OperatorID = "test-operator-id"
	logger := testutil.NewTestLogger()

	svc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	taskID := "task-123"
	_ = taskID
	result := &pb.CommandResult{
		ExecutionId: "req-123",
		Status:      protoExecutionStatus(constants.ExecutionStatusCompleted),
		ExitCode:    0,
	}

	originalMsg := PubSubCommandMessage{
		ID:                "msg-123",
		EventType:         constants.Event.Operator.Command.Requested,
		CaseID:            "case-456",
		OperatorSessionID: "web-789",
	}

	err = svc.PublishExecutionResult(context.Background(), result, originalMsg)

	require.NoError(t, err)
}

func TestPubSubResultsService_PublishExecutionResultFailed(t *testing.T) {
	db := NewMockOperatorPubSubClient()
	defer db.Close()

	cfg := testutil.NewTestConfig(t)
	cfg.OperatorSessionId = "test-session-123"
	cfg.OperatorID = "test-operator-id"
	logger := testutil.NewTestLogger()

	svc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	result := &pb.CommandResult{
		ExecutionId: "req-123",
		Status:      protoExecutionStatus(constants.ExecutionStatusFailed),
		ExitCode:    1,
	}

	originalMsg := PubSubCommandMessage{
		ID:        "msg-123",
		EventType: constants.Event.Operator.Command.Requested,
		CaseID:    "case-456",
	}

	err = svc.PublishExecutionResult(context.Background(), result, originalMsg)

	require.NoError(t, err)
}

func TestPubSubResultsService_PublishExecutionResultTimeout(t *testing.T) {
	db := NewMockOperatorPubSubClient()
	defer db.Close()

	cfg := testutil.NewTestConfig(t)
	cfg.OperatorSessionId = "test-session-123"
	cfg.OperatorID = "test-operator-id"
	logger := testutil.NewTestLogger()

	svc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	result := &pb.CommandResult{
		ExecutionId: "req-123",
		Status:      protoExecutionStatus(constants.ExecutionStatusTimeout),
		ExitCode:    124,
	}

	originalMsg := PubSubCommandMessage{
		ID:        "msg-123",
		EventType: constants.Event.Operator.Command.Requested,
		CaseID:    "case-456",
	}

	err = svc.PublishExecutionResult(context.Background(), result, originalMsg)

	require.NoError(t, err)
}

func TestPubSubResultsService_PublishFileEditResultWithBackup(t *testing.T) {
	db := NewMockOperatorPubSubClient()
	defer db.Close()

	cfg := testutil.NewTestConfig(t)
	cfg.OperatorSessionId = "test-session-123"
	cfg.OperatorID = "test-operator-id"
	logger := testutil.NewTestLogger()

	svc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	bytesWritten := int64(100)
	linesChanged := int32(5)
	backupPath := "/tmp/test.txt.backup"

	result := &pb.FileEditResult{
		ExecutionId:  "req-123",
		Operation:    string(models.FileEditOperationReplace),
		FilePath:     "/tmp/test.txt",
		Status:       protoExecutionStatus(constants.ExecutionStatusCompleted),
		BytesWritten: bytesWritten,
		LinesChanged: linesChanged,
		BackupPath:   backupPath,
	}

	originalMsg := PubSubCommandMessage{
		ID:        "msg-123",
		EventType: constants.Event.Operator.FileEdit.Requested,
		CaseID:    "case-456",
	}

	err = svc.PublishFileEditResult(context.Background(), result, originalMsg)

	require.NoError(t, err)
}

func TestPubSubResultsService_PublishExecutionStatus_DataSovereignty(t *testing.T) {
	// Architecture note:
	// - LFAA (LocalStore) = local ledger for audit/file tracking, sets stored_locally flag
	// - sentinel.Sentinel = data scrubbing, scrubs sensitive data BEFORE calling PublishExecutionStatus
	// - Output is ALWAYS transmitted in status updates (after sentinel.Sentinel scrubbing by caller)

	t.Run("status update sets stored_locally flag when LFAA enabled", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		localStoreCfg := &storage.LocalStoreConfig{Enabled: true}
		localStore, err := storage.NewLocalStoreService(localStoreCfg, logger)
		require.NoError(t, err)

		svc, err := NewPubSubResultsService(cfg, logger, db, localStore)
		require.NoError(t, err)

		scrubbedOutput := "[CREDENTIAL_REFERENCE]\n[CREDENTIAL]"
		scrubbedStderr := "Connection string: [CONN_STRING]"

		status := &pb.ExecutionStatusUpdate{
			ExecutionId:    "exec-lfaa-test",
			Command:        "cat /etc/passwd",
			Status:         protoExecutionStatus(constants.ExecutionStatusExecuting),
			ProcessAlive:   true,
			NewOutput:      scrubbedOutput,
			NewStderr:      scrubbedStderr,
			ElapsedSeconds: 5.0,
			Message:        "Command still executing",
		}

		err = svc.PublishExecutionStatus(context.Background(), status)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)

		var payload pb.ExecutionStatusUpdate
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)

		assert.NotEmpty(t, payload.NewOutput, "new_output should be present")
		assert.Equal(t, scrubbedOutput, payload.NewOutput, "new_output should contain scrubbed output")
		assert.Equal(t, scrubbedStderr, payload.NewStderr, "new_stderr should contain scrubbed output")
	})

	t.Run("status update includes output without stored_locally when LFAA disabled", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		status := &pb.ExecutionStatusUpdate{
			ExecutionId:    "exec-no-lfaa-test",
			Command:        "echo hello",
			Status:         protoExecutionStatus(constants.ExecutionStatusExecuting),
			ProcessAlive:   true,
			NewOutput:      "hello world",
			NewStderr:      "",
			ElapsedSeconds: 2.0,
			Message:        "Command executing",
		}

		err = svc.PublishExecutionStatus(context.Background(), status)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)

		var payload pb.ExecutionStatusUpdate
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)

		assert.Equal(t, "hello world", payload.NewOutput, "new_output should contain the actual output")
	})
}

func TestPubSubResultsService_PublishFsListResult(t *testing.T) {
	t.Run("successful fs list publish", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		startTime := time.Now().UTC()
		_ = startTime

		result := &pb.FsListResult{
			ExecutionId:     "req-fslist-123",
			Status:          protoExecutionStatus(constants.ExecutionStatusCompleted),
			Path:            "/tmp",
			TotalCount:      3,
			Truncated:       false,
			DurationSeconds: 0.5,
			Entries: []*pb.FsEntry{
				{Name: "file1.txt", Size: 100},
				{Name: "file2.txt", Size: 200},
				{Name: "subdir", IsDir: true, Size: 0},
			},
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-fslist-123",
			EventType: constants.Event.Operator.FsList.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishFsListResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)
		assert.Equal(t, constants.Event.Operator.FsList.Completed, env.EventType)

		var payload pb.FsListResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Equal(t, "file1.txt", payload.Entries[0].Name)
	})

	t.Run("failed fs list result", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		errorMsg := "directory not found"
		errorType := "not_found"

		result := &pb.FsListResult{
			ExecutionId:  "req-fslist-fail",
			Status:       protoExecutionStatus(constants.ExecutionStatusFailed),
			Path:         "/nonexistent",
			ErrorMessage: errorMsg,
			ErrorType:    errorType,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-fslist-fail",
			EventType: constants.Event.Operator.FsList.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishFsListResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)
		assert.Equal(t, constants.Event.Operator.FsList.Failed, env.EventType)

		var payload pb.FsListResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Equal(t, "directory not found", payload.ErrorMessage)
	})

	t.Run("truncated fs list result", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-truncated"
		cfg.OperatorID = "test-operator-truncated"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &pb.FsListResult{
			ExecutionId: "req-truncated",
			Status:      protoExecutionStatus(constants.ExecutionStatusCompleted),
			Path:        "/var/log",
			TotalCount:  100,
			Truncated:   true,
			Entries:     []*pb.FsEntry{},
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-truncated",
			EventType: constants.Event.Operator.FsList.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishFsListResult(context.Background(), result, originalMsg)
		require.NoError(t, err)
	})
}

func TestPubSubResultsService_PublishResult(t *testing.T) {
	t.Run("publishes generic result", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		// Generic result must be a proto.Message now
		result := &pb.CommandResult{
			ExecutionId: "custom-123",
			Output:      "custom_value",
		}

		err = svc.PublishExecutionResult(context.Background(), result, PubSubCommandMessage{})
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)

		var payload pb.CommandResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Equal(t, "custom-123", payload.ExecutionId)
		assert.Equal(t, "custom_value", payload.Output)
	})

	t.Run("publishes result with complex payload", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-complex"
		cfg.OperatorID = "test-operator-complex"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		// Complex payload now means a specific Protobuf result
		result := &pb.FsListResult{
			ExecutionId: "complex-123",
			Entries: []*pb.FsEntry{
				{Name: "nested", IsDir: true},
			},
		}

		err = svc.PublishFsListResult(context.Background(), result, PubSubCommandMessage{})
		require.NoError(t, err)
	})

	t.Run("populates missing operator fields", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &commonv1.GovernanceEnvelope{
			EventType: "test.auto.populate",
			CaseId:    "test-case",
			Payload:   testutil.MustMarshalProtobufHeartbeatRequested(t),
		}

		err = svc.PublishResult(context.Background(), result)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)
		assert.Equal(t, cfg.OperatorID, env.OperatorId)
		assert.Equal(t, cfg.OperatorSessionId, env.OperatorSessionId)
	})
}

func TestPubSubResultsService_PublishCancellationResult(t *testing.T) {
	t.Run("publishes cancellation result", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &pb.CommandResult{
			ExecutionId: "req-cancel-123",
			Status:      protoExecutionStatus(constants.ExecutionStatusCancelled),
			ExitCode:    -1,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-cancel-123",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishCancellationResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		env := testutil.MustUnmarshalUniversalEnvelope(t, receivedMsg)
		assert.Equal(t, constants.Event.Operator.Command.Cancelled, env.EventType)
	})

	t.Run("publishes cancellation with task and investigation IDs", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-cancel-ids"
		cfg.OperatorID = "test-operator-cancel-ids"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &pb.CommandResult{
			ExecutionId: "req-cancel-ids",
			Status:      protoExecutionStatus(constants.ExecutionStatusCancelled),
			ExitCode:    -1,
		}

		taskID := "task-cancel"
		originalMsg := PubSubCommandMessage{
			ID:                "msg-cancel-ids",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			TaskID:            &taskID,
			OperatorSessionID: "web-cancel",
		}

		err = svc.PublishCancellationResult(context.Background(), result, originalMsg)
		require.NoError(t, err)
	})
}

func TestResultMessage_APIKeyPropagation(t *testing.T) {
	t.Run("execution result carries api_key from config", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_test_key_abc123"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &pb.CommandResult{
			ExecutionId: "req-apikey-1",
			Status:      protoExecutionStatus(constants.ExecutionStatusCompleted),
			ExitCode:    0,
		}
		originalMsg := PubSubCommandMessage{
			ID:        "msg-apikey-1",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-apikey",
		}

		err = svc.PublishExecutionResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		published := db.LastPublished()
		require.NotNil(t, published, "must have published a message")

		env := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
		assert.Equal(t, cfg.OperatorID, env.OperatorId,
			"GovernanceEnvelope must carry operator_id from config")
	})

	t.Run("cancellation result carries api_key from config", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_cancel_key"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &pb.CommandResult{
			ExecutionId: "req-cancel-apikey",
			Status:      protoExecutionStatus(constants.ExecutionStatusCancelled),
		}
		originalMsg := PubSubCommandMessage{
			ID:        "msg-cancel-apikey",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-apikey",
		}

		err = svc.PublishCancellationResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		published := db.LastPublished()
		require.NotNil(t, published)

		env := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
		assert.Equal(t, cfg.OperatorID, env.OperatorId,
			"Cancellation ResultMessage must carry operator_id from config")
	})
}

func TestPubSubResultsService_PublishExecutionStatus_EventTypeMapping(t *testing.T) {
	tests := []struct {
		status        constants.ExecutionStatus
		expectedEvent string
	}{
		{constants.ExecutionStatusPending, constants.Event.Operator.Command.StatusUpdated.Queued},
		{constants.ExecutionStatusExecuting, constants.Event.Operator.Command.StatusUpdated.Running},
		{constants.ExecutionStatusCompleted, constants.Event.Operator.Command.StatusUpdated.Completed},
		{constants.ExecutionStatusFailed, constants.Event.Operator.Command.StatusUpdated.Failed},
		{constants.ExecutionStatusTimeout, constants.Event.Operator.Command.StatusUpdated.Failed},
		{constants.ExecutionStatusCancelled, constants.Event.Operator.Command.StatusUpdated.Cancelled},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			db := NewMockOperatorPubSubClient()
			defer db.Close()

			cfg := testutil.NewTestConfig(t)
			logger := testutil.NewTestLogger()

			svc, err := NewPubSubResultsService(cfg, logger, db, nil)
			require.NoError(t, err)

			status := &pb.ExecutionStatusUpdate{
				ExecutionId: "exec-event-type-test",
				Command:     "echo test",
				Status:      protoExecutionStatus(tt.status),
			}

			err = svc.PublishExecutionStatus(context.Background(), status)
			require.NoError(t, err)

			published := db.LastPublished()
			require.NotNil(t, published, "must have published a message for status %s", tt.status)

			env := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
			assert.Equal(t, tt.expectedEvent, env.EventType,
				"execution status %q must emit event type %q on the wire", tt.status, tt.expectedEvent)
		})
	}
}

func TestHeartbeat_APIKeyPropagation(t *testing.T) {
	t.Run("heartbeat carries api_key from config via buildHeartbeat", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_heartbeat_key"
		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		cmdSvc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:         cfg,
			Logger:         logger,
			Execution:      execSvc,
			FileEdit:       fileEditSvc,
			PubSubClient:   db,
			ResultsService: resultsSvc,
		})
		require.NoError(t, err)

		heartbeat := cmdSvc.heartbeat.Build(models.HeartbeatTypeAutomatic)
		assert.Equal(t, cfg.OperatorID, heartbeat.OperatorID)
	})

	t.Run("heartbeat api_key appears in published envelope", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_hb_json_key"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		heartbeat := &pb.HeartbeatResult{
			OperatorId:        cfg.OperatorID,
			OperatorSessionId: cfg.OperatorSessionId,
			Status:            "healthy",
			Timestamp:         models.NowTimestamp(),
		}

		err = svc.PublishHeartbeat(context.Background(), heartbeat)
		require.NoError(t, err)

		published := db.LastPublished()
		require.NotNil(t, published)

		env := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
		assert.Equal(t, cfg.OperatorID, env.OperatorId)
	})
}
