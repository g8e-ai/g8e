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
	"encoding/json"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/services/system"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireLastPublished(t *testing.T, db *MockG8esPubSubClient) []byte {
	t.Helper()
	published := db.LastPublished()
	require.NotNil(t, published, "expected a message to be published")
	return published.Data
}

func TestNewPubSubResultsService(t *testing.T) {
	t.Run("creates service", func(t *testing.T) {
		db := NewMockG8esPubSubClient()

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
		db := NewMockG8esPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		startTime := time.Now().UTC()
		endTime := startTime.Add(2 * time.Second)
		returnCode := 0

		result := &models.ExecutionResultsPayload{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			Command:         "echo",
			Args:            []string{"test"},
			Status:          constants.ExecutionStatusCompleted,
			ReturnCode:      &returnCode,
			Stdout:          "test\n",
			Stderr:          "",
			StartTime:       &startTime,
			EndTime:         &endTime,
			DurationSeconds: 2.0,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishExecutionResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		assert.Contains(t, string(receivedMsg), "req-123")
		assert.Contains(t, string(receivedMsg), constants.Event.Operator.Command.Completed)

		// CRITICAL: operator_id must be config.OperatorID, not hostname
		assert.Contains(t, string(receivedMsg), cfg.OperatorID,
			"Result must contain config.OperatorID, not hostname")
		assert.NotContains(t, string(receivedMsg), system.GetHostname(),
			"CRITICAL BUG: Result contains hostname instead of operator_id")

		// CRITICAL: return_code must be an integer, not a pointer address string
		var parsedMsg models.G8eMessage
		require.NoError(t, json.Unmarshal(receivedMsg, &parsedMsg), "Published message must be valid JSON")

		var payload models.ExecutionResultsPayload
		require.NoError(t, json.Unmarshal(parsedMsg.Payload, &payload), "Payload must unmarshal into ExecutionResultsPayload")
		require.NotNil(t, payload.ReturnCode, "Payload must contain return_code field")
		assert.Equal(t, 0, *payload.ReturnCode, "return_code should be 0 for successful execution")
	})

	t.Run("return_code type validation", func(t *testing.T) {
		db := NewMockG8esPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		returnCode := 42
		result := &models.ExecutionResultsPayload{
			ExecutionID: "req-serialization",
			CaseID:      "case-serialization",
			Command:     "exit 42",
			Status:      constants.ExecutionStatusFailed,
			ReturnCode:  &returnCode,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-serialization",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-serialization",
		}

		err = svc.PublishExecutionResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)

		var parsedMsg models.G8eMessage
		require.NoError(t, json.Unmarshal(receivedMsg, &parsedMsg), "Published message must be valid JSON")

		var payload models.ExecutionResultsPayload
		require.NoError(t, json.Unmarshal(parsedMsg.Payload, &payload), "Message must have valid ExecutionResultsPayload")
		require.NotNil(t, payload.ReturnCode, "Payload must contain return_code field")
		// CRITICAL: must be a number, not a pointer address string
		assert.Equal(t, 42, *payload.ReturnCode, "return_code value should match")
	})
}

func TestPubSubResultsService_PublishFileEditResult(t *testing.T) {
	t.Run("successful publish", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-123"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		startTime := time.Now().UTC()
		endTime := startTime.Add(1 * time.Second)
		bytesWritten := int64(100)

		result := &models.FileEditResult{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			Operation:       models.FileEditOperationWrite,
			FilePath:        "/tmp/test.txt",
			Status:          constants.ExecutionStatusCompleted,
			BytesWritten:    &bytesWritten,
			StartTime:       &startTime,
			EndTime:         &endTime,
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
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-123"
		cfg.OperatorID = "test-operator-id"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		errorMsg := "file not found"
		errorType := "not_found"
		result := &models.FileEditResult{
			ExecutionID:  "req-123",
			CaseID:       "case-456",
			Operation:    models.FileEditOperationRead,
			FilePath:     "/tmp/nonexistent.txt",
			Status:       constants.ExecutionStatusFailed,
			ErrorMessage: &errorMsg,
			ErrorType:    &errorType,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishFileEditResult(context.Background(), result, originalMsg)

		require.NoError(t, err)
	})
}

func TestPubSubResultsService_PublishHeartbeat(t *testing.T) {
	t.Run("successful heartbeat publish", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-123"
		cfg.OperatorID = "test-operator-id"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		heartbeat := &models.Heartbeat{
			EventType:       constants.Event.Operator.Heartbeat,
			SourceComponent: "g8eo",
			Timestamp:       models.NowTimestamp(),
			HeartbeatType:   models.HeartbeatTypeAutomatic,
		}

		err = svc.PublishHeartbeat(context.Background(), heartbeat)

		require.NoError(t, err)
	})

	t.Run("heartbeat with system info", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-123"
		cfg.OperatorID = "test-operator-id"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		heartbeat := &models.Heartbeat{
			EventType:       constants.Event.Operator.Heartbeat,
			SourceComponent: "g8eo",
			HeartbeatType:   models.HeartbeatTypeAutomatic,
			Timestamp:       models.NowTimestamp(),
			SystemIdentity: models.HeartbeatSystemIdentity{
				Hostname: "test-host",
				OS:       "linux",
			},
		}

		err = svc.PublishHeartbeat(context.Background(), heartbeat)

		require.NoError(t, err)
	})
}

func TestPubSubResultsService_MessageFormatting(t *testing.T) {
	t.Run("execution result envelope format", func(t *testing.T) {
		returnCode := 0
		result := &models.ExecutionResultsPayload{
			ExecutionID: "req-123",
			CaseID:      "case-456",
			Command:     "echo",
			Status:      constants.ExecutionStatusCompleted,
			ReturnCode:  &returnCode,
		}

		// Simulate what would be published
		envelope, err := models.NewG8eMessage(
			constants.Event.Operator.Command.Completed, result.CaseID,
			"", "", "", result,
		)
		require.NoError(t, err)

		data, err := json.Marshal(envelope)
		require.NoError(t, err)
		assert.Contains(t, string(data), constants.Event.Operator.Command.Completed)
		assert.Contains(t, string(data), "req-123")
	})

	t.Run("file edit result envelope format", func(t *testing.T) {
		result := &models.FileEditResult{
			ExecutionID: "req-123",
			CaseID:      "case-456",
			Operation:   models.FileEditOperationWrite,
			FilePath:    "/tmp/test.txt",
			Status:      constants.ExecutionStatusCompleted,
		}

		envelope, err := models.NewG8eMessage(
			constants.Event.Operator.FileEdit.Completed, result.CaseID,
			"", "", "", result,
		)
		require.NoError(t, err)

		data, err := json.Marshal(envelope)
		require.NoError(t, err)
		assert.Contains(t, string(data), constants.Event.Operator.FileEdit.Completed)
		assert.Contains(t, string(data), "req-123")
	})
}

func TestPubSubResultsService_PublishExecutionResultWithTaskAndInvestigation(t *testing.T) {
	db := NewMockG8esPubSubClient()
	defer db.Close()

	cfg := testutil.NewTestConfig(t)
	cfg.OperatorSessionId = "test-session-123"
	cfg.OperatorID = "test-operator-id"
	logger := testutil.NewTestLogger()

	svc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	taskID := "task-123"
	operatorSessionID := "web-789"
	returnCode := 0

	result := &models.ExecutionResultsPayload{
		ExecutionID:     "req-123",
		CaseID:          "case-456",
		TaskID:          &taskID,
		InvestigationID: "inv-456",
		Command:         "ls",
		Status:          constants.ExecutionStatusCompleted,
		ReturnCode:      &returnCode,
	}

	originalMsg := PubSubCommandMessage{
		ID:                "msg-123",
		EventType:         constants.Event.Operator.Command.Requested,
		CaseID:            "case-456",
		OperatorSessionID: operatorSessionID,
	}

	err = svc.PublishExecutionResult(context.Background(), result, originalMsg)

	require.NoError(t, err)
}

func TestPubSubResultsService_PublishExecutionResultFailed(t *testing.T) {
	db := NewMockG8esPubSubClient()
	defer db.Close()

	cfg := testutil.NewTestConfig(t)
	cfg.OperatorSessionId = "test-session-123"
	cfg.OperatorID = "test-operator-id"
	logger := testutil.NewTestLogger()

	svc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	returnCode := 1
	result := &models.ExecutionResultsPayload{
		ExecutionID: "req-123",
		CaseID:      "case-456",
		Command:     "false",
		Status:      constants.ExecutionStatusFailed,
		ReturnCode:  &returnCode,
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
	db := NewMockG8esPubSubClient()
	defer db.Close()

	cfg := testutil.NewTestConfig(t)
	cfg.OperatorSessionId = "test-session-123"
	cfg.OperatorID = "test-operator-id"
	logger := testutil.NewTestLogger()

	svc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	returnCode := 124
	result := &models.ExecutionResultsPayload{
		ExecutionID: "req-123",
		CaseID:      "case-456",
		Command:     "sleep 100",
		Status:      constants.ExecutionStatusTimeout,
		ReturnCode:  &returnCode,
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
	db := NewMockG8esPubSubClient()
	defer db.Close()

	cfg := testutil.NewTestConfig(t)
	cfg.OperatorSessionId = "test-session-123"
	cfg.OperatorID = "test-operator-id"
	logger := testutil.NewTestLogger()

	svc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	bytesWritten := int64(100)
	linesChanged := 5
	backupPath := "/tmp/test.txt.backup"
	taskID := "task-123"

	result := &models.FileEditResult{
		ExecutionID:     "req-123",
		CaseID:          "case-456",
		TaskID:          &taskID,
		InvestigationID: "inv-456",
		Operation:       models.FileEditOperationReplace,
		FilePath:        "/tmp/test.txt",
		Status:          constants.ExecutionStatusCompleted,
		BytesWritten:    &bytesWritten,
		LinesChanged:    &linesChanged,
		BackupPath:      &backupPath,
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
		db := NewMockG8esPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		localStoreCfg := &storage.LocalStoreConfig{Enabled: true}
		localStore, err := storage.NewLocalStoreService(localStoreCfg, logger)
		require.NoError(t, err)

		svc, err := NewPubSubResultsService(cfg, logger, db, localStore)
		require.NoError(t, err)

		scrubbedOutput := "[CREDENTIAL_REFERENCE]\n[CREDENTIAL]"
		scrubbedStderr := "Connection string: [CONN_STRING]"

		status := &ExecutionStatusUpdate{
			ExecutionID:    "exec-lfaa-test",
			CaseID:         "case-lfaa",
			Command:        "cat /etc/passwd",
			Status:         constants.ExecutionStatusExecuting,
			ProcessAlive:   true,
			NewOutput:      scrubbedOutput,
			NewStderr:      scrubbedStderr,
			ElapsedSeconds: 5.0,
			Message:        "Command still executing",
		}

		err = svc.PublishExecutionStatus(context.Background(), status)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)

		var parsedMsg models.G8eMessage
		require.NoError(t, json.Unmarshal(receivedMsg, &parsedMsg))

		var payload models.ExecutionStatusPayload
		require.NoError(t, json.Unmarshal(parsedMsg.Payload, &payload), "Message must have valid ExecutionStatusPayload")

		assert.True(t, payload.StoredLocally, "stored_locally should be true when LFAA enabled")
		assert.NotEmpty(t, payload.NewOutput, "new_output should be present")
		assert.Equal(t, scrubbedOutput, payload.NewOutput, "new_output should contain scrubbed output")
		assert.Equal(t, scrubbedStderr, payload.NewStderr, "new_stderr should contain scrubbed output")
		assert.Contains(t, string(receivedMsg), "[CREDENTIAL_REFERENCE]")
		assert.Contains(t, string(receivedMsg), "[CONN_STRING]")
	})

	t.Run("status update includes output without stored_locally when LFAA disabled", func(t *testing.T) {
		db := NewMockG8esPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		status := &ExecutionStatusUpdate{
			ExecutionID:    "exec-no-lfaa-test",
			CaseID:         "case-no-lfaa",
			Command:        "echo hello",
			Status:         constants.ExecutionStatusExecuting,
			ProcessAlive:   true,
			NewOutput:      "hello world",
			NewStderr:      "",
			ElapsedSeconds: 2.0,
			Message:        "Command executing",
		}

		err = svc.PublishExecutionStatus(context.Background(), status)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)

		var parsedMsg models.G8eMessage
		require.NoError(t, json.Unmarshal(receivedMsg, &parsedMsg))

		var payload models.ExecutionStatusPayload
		require.NoError(t, json.Unmarshal(parsedMsg.Payload, &payload), "Message must have valid ExecutionStatusPayload")

		assert.Equal(t, "hello world", payload.NewOutput, "new_output should contain the actual output")
		assert.False(t, payload.StoredLocally, "stored_locally should be false when LFAA disabled")
	})
}

func TestPubSubResultsService_PublishFsListResult(t *testing.T) {
	t.Run("successful fs list publish", func(t *testing.T) {
		db := NewMockG8esPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		taskID := "task-123"
		startTime := time.Now().UTC()
		endTime := startTime.Add(500 * time.Millisecond)

		result := &models.FsListResult{
			ExecutionID:     "req-fslist-123",
			CaseID:          "case-456",
			TaskID:          &taskID,
			Status:          constants.ExecutionStatusCompleted,
			Path:            "/tmp",
			TotalCount:      3,
			Truncated:       false,
			StartTime:       &startTime,
			EndTime:         &endTime,
			DurationSeconds: 0.5,
			Entries: []models.FsListEntry{
				{Name: "file1.txt", Path: "/tmp/file1.txt", IsDir: false, Size: 100},
				{Name: "file2.txt", Path: "/tmp/file2.txt", IsDir: false, Size: 200},
				{Name: "subdir", Path: "/tmp/subdir", IsDir: true, Size: 0},
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
		assert.Contains(t, string(receivedMsg), constants.Event.Operator.FsList.Completed)
		assert.Contains(t, string(receivedMsg), "file1.txt")
	})

	t.Run("failed fs list result", func(t *testing.T) {
		db := NewMockG8esPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		errorMsg := "directory not found"
		errorType := "not_found"

		result := &models.FsListResult{
			ExecutionID:  "req-fslist-fail",
			CaseID:       "case-456",
			Status:       constants.ExecutionStatusFailed,
			Path:         "/nonexistent",
			ErrorMessage: &errorMsg,
			ErrorType:    &errorType,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-fslist-fail",
			EventType: constants.Event.Operator.FsList.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishFsListResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		assert.Contains(t, string(receivedMsg), constants.Event.Operator.FsList.Failed)
		assert.Contains(t, string(receivedMsg), "directory not found")
	})

	t.Run("truncated fs list result", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-truncated"
		cfg.OperatorID = "test-operator-truncated"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &models.FsListResult{
			ExecutionID: "req-truncated",
			CaseID:      "case-456",
			Status:      constants.ExecutionStatusCompleted,
			Path:        "/var/log",
			TotalCount:  100,
			Truncated:   true,
			Entries:     []models.FsListEntry{},
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
		db := NewMockG8esPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		type customPayload struct {
			CustomField string `json:"custom_field"`
			Count       int    `json:"count"`
		}
		result, err := models.NewG8eMessage(
			"custom.event.type", "test-case",
			cfg.OperatorID, cfg.OperatorSessionId, "",
			customPayload{CustomField: "custom_value", Count: 42},
		)
		require.NoError(t, err)

		err = svc.PublishResult(context.Background(), result)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		assert.Contains(t, string(receivedMsg), "custom.event.type")
		assert.Contains(t, string(receivedMsg), "custom_value")
	})

	t.Run("publishes result with complex payload", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-complex"
		cfg.OperatorID = "test-operator-complex"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		type nestedPayload struct {
			Nested map[string]interface{} `json:"nested"`
			Array  []string               `json:"array"`
		}
		result, err := models.NewG8eMessage(
			"nested.event", "test-case",
			"test-operator-complex", "test-session-complex", "",
			nestedPayload{
				Nested: map[string]interface{}{"key1": "value1", "key2": 123},
				Array:  []string{"a", "b", "c"},
			},
		)
		require.NoError(t, err)

		err = svc.PublishResult(context.Background(), result)
		require.NoError(t, err)
	})

	t.Run("populates missing operator fields", func(t *testing.T) {
		db := NewMockG8esPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result, err := models.NewG8eMessage(
			"test.auto.populate", "test-case",
			"", "", "",
			struct{}{},
		)
		require.NoError(t, err)

		err = svc.PublishResult(context.Background(), result)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		assert.Contains(t, string(receivedMsg), cfg.OperatorID)
		assert.Contains(t, string(receivedMsg), cfg.OperatorSessionId)
		assert.Contains(t, string(receivedMsg), "g8eo")
	})
}

func TestPubSubResultsService_PublishCancellationResult(t *testing.T) {
	t.Run("publishes cancellation result", func(t *testing.T) {
		db := NewMockG8esPubSubClient()

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		startTime := time.Now().UTC()
		endTime := startTime.Add(1 * time.Second)
		returnCode := -1

		result := &models.ExecutionResultsPayload{
			ExecutionID:     "req-cancel-123",
			CaseID:          "case-456",
			Command:         "sleep 100",
			Status:          constants.ExecutionStatusCancelled,
			ReturnCode:      &returnCode,
			StartTime:       &startTime,
			EndTime:         &endTime,
			DurationSeconds: 1.0,
		}

		originalMsg := PubSubCommandMessage{
			ID:        "msg-cancel-123",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-456",
		}

		err = svc.PublishCancellationResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublished(t, db)
		assert.Contains(t, string(receivedMsg), constants.Event.Operator.Command.Cancelled)
	})

	t.Run("publishes cancellation with task and investigation IDs", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.OperatorSessionId = "test-session-cancel-ids"
		cfg.OperatorID = "test-operator-cancel-ids"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		taskID := "task-cancel"
		operatorSessionID := "web-cancel"
		returnCode := -1

		result := &models.ExecutionResultsPayload{
			ExecutionID:     "req-cancel-ids",
			CaseID:          "case-456",
			TaskID:          &taskID,
			InvestigationID: "inv-cancel",
			Command:         "sleep 100",
			Status:          constants.ExecutionStatusCancelled,
			ReturnCode:      &returnCode,
		}

		originalMsg := PubSubCommandMessage{
			ID:                "msg-cancel-ids",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			TaskID:            &taskID,
			OperatorSessionID: operatorSessionID,
		}

		err = svc.PublishCancellationResult(context.Background(), result, originalMsg)
		require.NoError(t, err)
	})
}

func TestResultMessage_APIKeyPropagation(t *testing.T) {
	t.Run("execution result carries api_key from config", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_test_key_abc123"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		returnCode := 0
		result := &models.ExecutionResultsPayload{
			ExecutionID: "req-apikey-1",
			CaseID:      "case-apikey",
			Command:     "echo",
			Status:      constants.ExecutionStatusCompleted,
			ReturnCode:  &returnCode,
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

		var msg models.G8eMessage
		require.NoError(t, json.Unmarshal(published.Data, &msg))
		assert.Equal(t, "g8e_test_key_abc123", msg.APIKey,
			"ResultMessage must carry api_key from config")
	})

	t.Run("cancellation result carries api_key from config", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_cancel_key"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &models.ExecutionResultsPayload{
			ExecutionID: "req-cancel-apikey",
			CaseID:      "case-apikey",
			Command:     "kill",
			Status:      constants.ExecutionStatusCancelled,
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

		var msg models.G8eMessage
		require.NoError(t, json.Unmarshal(published.Data, &msg))
		assert.Equal(t, "g8e_cancel_key", msg.APIKey,
			"Cancellation ResultMessage must carry api_key from config")
	})

	t.Run("empty api_key is omitted from json", func(t *testing.T) {
		msg := &models.G8eMessage{
			ID:        "msg-no-key",
			EventType: "test.event",
		}
		data, err := msg.Marshal()
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"api_key"`,
			"api_key with empty value must be omitted from JSON (omitempty)")
	})

	t.Run("non-empty api_key is present in json", func(t *testing.T) {
		msg := &models.G8eMessage{
			ID:        "msg-with-key",
			EventType: "test.event",
			APIKey:    "g8e_present_key",
		}
		data, err := msg.Marshal()
		require.NoError(t, err)
		assert.Contains(t, string(data), `"api_key":"g8e_present_key"`,
			"api_key must appear in JSON when non-empty")
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
			db := NewMockG8esPubSubClient()
			defer db.Close()

			cfg := testutil.NewTestConfig(t)
			logger := testutil.NewTestLogger()

			svc, err := NewPubSubResultsService(cfg, logger, db, nil)
			require.NoError(t, err)

			status := &ExecutionStatusUpdate{
				ExecutionID: "exec-event-type-test",
				CaseID:      "case-event-type",
				Command:     "echo test",
				Status:      tt.status,
			}

			err = svc.PublishExecutionStatus(context.Background(), status)
			require.NoError(t, err)

			published := db.LastPublished()
			require.NotNil(t, published, "must have published a message for status %s", tt.status)

			var msg models.G8eMessage
			require.NoError(t, json.Unmarshal(published.Data, &msg))
			assert.Equal(t, tt.expectedEvent, msg.EventType,
				"execution status %q must emit event type %q on the wire", tt.status, tt.expectedEvent)
		})
	}
}

func TestHeartbeat_APIKeyPropagation(t *testing.T) {
	t.Run("heartbeat carries api_key from config via buildHeartbeat", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
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
		assert.Equal(t, "g8e_heartbeat_key", heartbeat.APIKey,
			"Heartbeat must carry APIKey from config")
	})

	t.Run("heartbeat api_key appears in published json", func(t *testing.T) {
		db := NewMockG8esPubSubClient()
		defer db.Close()

		cfg := testutil.NewTestConfig(t)
		cfg.APIKey = "g8e_hb_json_key"
		logger := testutil.NewTestLogger()

		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		heartbeat := &models.Heartbeat{
			EventType:     constants.Event.Operator.Heartbeat,
			HeartbeatType: models.HeartbeatTypeAutomatic,
			Timestamp:     models.NowTimestamp(),
			APIKey:        "g8e_hb_json_key",
		}

		err = svc.PublishHeartbeat(context.Background(), heartbeat)
		require.NoError(t, err)

		published := db.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), `"api_key":"g8e_hb_json_key"`,
			"Published heartbeat JSON must contain api_key")
	})

	t.Run("heartbeat without api_key omits field from json", func(t *testing.T) {
		heartbeat := &models.Heartbeat{
			EventType:     constants.Event.Operator.Heartbeat,
			HeartbeatType: models.HeartbeatTypeAutomatic,
		}
		data, err := json.Marshal(heartbeat)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"api_key"`,
			"Empty api_key must be omitted from heartbeat JSON (omitempty)")
	})
}
