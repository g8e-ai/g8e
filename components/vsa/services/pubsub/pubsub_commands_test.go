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

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	execution "github.com/g8e-ai/g8e/components/vsa/services/execution"
	storage "github.com/g8e-ai/g8e/components/vsa/services/storage"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewPubSubCommandService(t *testing.T) {
	t.Run("creates service successfully", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:       cfg,
			Logger:       logger,
			Execution:    execSvc,
			FileEdit:     fileEditSvc,
			PubSubClient: NewMockVSODBPubSubClient(),
		})

		require.NoError(t, err)
		assert.NotNil(t, svc)
		assert.Equal(t, cfg, svc.config)
		assert.NotNil(t, svc.logger)
	})
}

// ---------------------------------------------------------------------------
// SetResultsService
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SetResultsService(t *testing.T) {
	f := newPubsubFixture(t)

	resultsSvc, err := NewPubSubResultsService(f.Cfg, f.Logger, f.DB, nil)
	require.NoError(t, err)

	f.Svc.results = resultsSvc
	f.Svc.heartbeat.results = resultsSvc
	f.Svc.commands.results = resultsSvc
	f.Svc.fileOps.results = resultsSvc

	assert.Equal(t, resultsSvc, f.Svc.results)
}

// ---------------------------------------------------------------------------
// buildHeartbeat — delegates to HeartbeatService
// ---------------------------------------------------------------------------

func TestPubSubCommandService_BuildHeartbeat(t *testing.T) {
	f := newPubsubFixture(t)

	tests := []struct {
		name          string
		heartbeatType models.HeartbeatType
	}{
		{"bootstrap heartbeat", models.HeartbeatTypeBootstrap},
		{"automatic heartbeat", models.HeartbeatTypeAutomatic},
		{"requested heartbeat", models.HeartbeatTypeRequested},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			heartbeat := f.Svc.heartbeat.Build(tt.heartbeatType)

			assert.NotNil(t, heartbeat)
			assert.Equal(t, constants.Event.Operator.Heartbeat, heartbeat.EventType)
			assert.Equal(t, constants.Status.ComponentName.VSA, heartbeat.SourceComponent)
			assert.Equal(t, tt.heartbeatType, heartbeat.HeartbeatType)
			assert.NotEmpty(t, heartbeat.SystemIdentity.Hostname)
			assert.NotEmpty(t, heartbeat.VersionInfo.OperatorVersion)
			assert.NotEmpty(t, heartbeat.Timestamp)
		})
	}
}

// ---------------------------------------------------------------------------
// payloadToExecutionRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_PayloadToExecutionRequest(t *testing.T) {
	t.Run("valid payload", func(t *testing.T) {
		taskID := "task-123"
		invID := "inv-456"

		msg := PubSubCommandMessage{
			ID:              "msg-123",
			EventType:       constants.Event.Operator.Command.Requested,
			CaseID:          "case-789",
			TaskID:          &taskID,
			InvestigationID: invID,
			Payload:         mustMarshalJSON(t, models.CommandRequestPayload{Command: "ls", ExecutionID: "exec-999"}),
			Timestamp:       time.Now().UTC(),
		}

		req, err := payloadToExecutionRequest(msg)

		require.NoError(t, err)
		assert.Equal(t, "exec-999", req.ExecutionID)
		assert.Equal(t, "case-789", req.CaseID)
		assert.Equal(t, "task-123", *req.TaskID)
		assert.Equal(t, "inv-456", req.InvestigationID)
		assert.Equal(t, "ls", req.Command)
		assert.Equal(t, 300, req.TimeoutSeconds)
	})

	t.Run("missing command", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-789",
			Payload:   mustMarshalJSON(t, models.CommandRequestPayload{}),
			Timestamp: time.Now().UTC(),
		}

		_, err := payloadToExecutionRequest(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing command")
	})
}

// ---------------------------------------------------------------------------
// payloadToFileEditRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_PayloadToFileEditRequest(t *testing.T) {
	t.Run("valid write operation", func(t *testing.T) {
		taskID := "task-123"
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-789",
			TaskID:    &taskID,
			Payload: mustMarshalJSON(t, models.FileEditRequestPayload{
				FilePath:        "/tmp/test.txt",
				Operation:       "write",
				Content:         "test content",
				CreateBackup:    true,
				CreateIfMissing: true,
				Justification:   "testing",
			}),
			Timestamp: time.Now().UTC(),
		}

		req, err := payloadToFileEditRequest(msg)

		require.NoError(t, err)
		assert.Equal(t, "msg-123", req.ExecutionID)
		assert.Equal(t, "case-789", req.CaseID)
		assert.Equal(t, "/tmp/test.txt", req.FilePath)
		assert.Equal(t, "write", string(req.Operation))
		assert.NotNil(t, req.Content)
		assert.Equal(t, "test content", *req.Content)
		assert.True(t, req.CreateBackup)
		assert.True(t, req.CreateIfMissing)
	})

	t.Run("valid replace operation", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-789",
			Payload: mustMarshalJSON(t, models.FileEditRequestPayload{
				FilePath:   "/tmp/test.txt",
				Operation:  "replace",
				OldContent: "old text",
				NewContent: "new text",
			}),
			Timestamp: time.Now().UTC(),
		}

		req, err := payloadToFileEditRequest(msg)

		require.NoError(t, err)
		assert.NotNil(t, req.OldContent)
		assert.Equal(t, "old text", *req.OldContent)
		assert.NotNil(t, req.NewContent)
		assert.Equal(t, "new text", *req.NewContent)
	})

	t.Run("missing file_path", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-789",
			Payload:   mustMarshalJSON(t, models.FileEditRequestPayload{Operation: "write"}),
			Timestamp: time.Now().UTC(),
		}

		_, err := payloadToFileEditRequest(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing file_path")
	})

	t.Run("missing operation", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-789",
			Payload:   mustMarshalJSON(t, models.FileEditRequestPayload{FilePath: "/tmp/test.txt"}),
			Timestamp: time.Now().UTC(),
		}

		_, err := payloadToFileEditRequest(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing operation")
	})
}

// ---------------------------------------------------------------------------
// PubSubCommandMessage JSON
// ---------------------------------------------------------------------------

func TestPubSubCommandMessage_UnmarshalJSON(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		jsonData := `{
"id": "msg-123",
"event_type": "g8e.v1.operator.command.requested",
"case_id": "case-456",
"task_id": "task-789",
"payload": {
"command": "ls -la"
},
"timestamp": "2024-01-01T00:00:00Z"
}`

		var msg PubSubCommandMessage
		err := json.Unmarshal([]byte(jsonData), &msg)

		require.NoError(t, err)
		assert.Equal(t, "msg-123", msg.ID)
		assert.Equal(t, constants.Event.Operator.Command.Requested, msg.EventType)
		assert.Equal(t, "case-456", msg.CaseID)
		assert.NotNil(t, msg.TaskID)
		assert.Equal(t, "task-789", *msg.TaskID)
	})

	t.Run("invalid json", func(t *testing.T) {
		var msg PubSubCommandMessage
		err := json.Unmarshal([]byte(`{invalid json`), &msg)
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// handleHeartbeatRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleHeartbeatRequest(t *testing.T) {
	f := newPubsubFixture(t)

	msg := PubSubCommandMessage{
		ID:              "msg-123",
		EventType:       constants.Event.Operator.HeartbeatRequested,
		CaseID:          "case-456",
		InvestigationID: "inv-123",
		Payload:         mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp:       time.Now().UTC(),
	}

	f.Svc.heartbeat.HandleRequest(context.Background(), msg)

	published := f.DB.LastPublished()
	require.NotNil(t, published, "expected heartbeat to be published")
	assert.Contains(t, string(published.Data), constants.Event.Operator.Heartbeat)
}

// ---------------------------------------------------------------------------
// SendsHeartbeatOnStart
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SendsHeartbeatOnStart(t *testing.T) {
	t.Run("publishes bootstrap heartbeat on start", func(t *testing.T) {
		f := newPubsubFixture(t)

		err := f.Svc.Start(context.Background())
		require.NoError(t, err)
		defer f.Svc.Stop()

		// SendAutomatic is called inside the listenForCommands goroutine after
		// Subscribe returns — poll briefly to let the goroutine run.
		var published *MockPublishedMsg
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			published = f.DB.LastPublished()
			if published != nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		require.NotNil(t, published, "expected bootstrap heartbeat to be published on start")
		assert.Contains(t, string(published.Data), constants.Event.Operator.Heartbeat)
	})
}

// ---------------------------------------------------------------------------
// Dispatcher — unknown / invalid messages
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleCommandMessage(t *testing.T) {
	f := newPubsubFixture(t)

	t.Run("handles unknown event type", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: "unknown.event.type",
			CaseID:    "case-456",
			Payload:   json.RawMessage(`{}`),
			Timestamp: time.Now().UTC(),
		}
		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)
		f.Svc.handleCommandPayload(msgJSON)
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		f.Svc.handleCommandPayload([]byte("invalid json {"))
	})
}

// ---------------------------------------------------------------------------
// Heartbeat scheduler — via HeartbeatService
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HeartbeatScheduler(t *testing.T) {
	t.Run("starts and stops heartbeat scheduler", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.HeartbeatInterval = 100 * time.Millisecond
		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		db := NewMockVSODBPubSubClient()
		defer db.Close()

		svc, err := NewPubSubCommandService(CommandServiceConfig{Config: cfg, Logger: logger, Execution: execSvc, FileEdit: fileEditSvc, PubSubClient: db})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		svc.ctx = ctx
		svc.heartbeat.ctx = ctx

		svc.heartbeat.StartScheduler()
		time.Sleep(50 * time.Millisecond)

		svc.heartbeat.mu.Lock()
		assert.NotNil(t, svc.heartbeat.ticker)
		assert.NotNil(t, svc.heartbeat.done)
		svc.heartbeat.mu.Unlock()

		svc.heartbeat.StopScheduler()
		svc.wg.Wait()

		svc.heartbeat.mu.Lock()
		assert.Nil(t, svc.heartbeat.ticker)
		assert.Nil(t, svc.heartbeat.done)
		svc.heartbeat.mu.Unlock()
	})
}

// ---------------------------------------------------------------------------
// Stop
// ---------------------------------------------------------------------------

func TestPubSubCommandService_Stop(t *testing.T) {
	t.Run("stops service gracefully", func(t *testing.T) {
		f := newPubsubFixture(t)

		ctx, cancel := context.WithCancel(context.Background())
		err := f.Svc.Start(ctx)
		require.NoError(t, err)
		_ = cancel

		err = f.Svc.Stop()
		assert.NoError(t, err)
		assert.False(t, f.Svc.running)
	})

	t.Run("stops already stopped service without error", func(t *testing.T) {
		f := newPubsubFixture(t)
		err := f.Svc.Stop()
		assert.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// handleCommandExecutionRequest — missing command guard
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleCommandExecutionRequest(t *testing.T) {
	t.Run("handles missing command without panic", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-456",
			Payload:   json.RawMessage(`{}`),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.commands.HandleExecutionRequest(context.Background(), msg)
		})
	})
}

// ---------------------------------------------------------------------------
// handleFileEditRequest — basic routing guard
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleFileEditRequest(t *testing.T) {
	t.Run("handles file edit request without panic", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-456",
			Payload: mustMarshalJSON(t, models.FileEditRequestPayload{
				FilePath:  "/tmp/test.txt",
				Operation: "write",
				Content:   "test content",
			}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.fileOps.HandleFileEditRequest(context.Background(), msg)
		})
	})
}

// ---------------------------------------------------------------------------
// payloadToFileEditRequest — extended operation types
// ---------------------------------------------------------------------------

func TestPubSubCommandService_PayloadToFileEditRequestExtended(t *testing.T) {
	t.Run("extracts insert operation fields", func(t *testing.T) {
		insertPos := 5
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-789",
			Payload: mustMarshalJSON(t, models.FileEditRequestPayload{
				FilePath:       "/tmp/test.txt",
				Operation:      "insert",
				InsertContent:  "new line",
				InsertPosition: &insertPos,
			}),
			Timestamp: time.Now().UTC(),
		}

		req, err := payloadToFileEditRequest(msg)

		require.NoError(t, err)
		assert.NotNil(t, req.InsertContent)
		assert.Equal(t, "new line", *req.InsertContent)
		assert.NotNil(t, req.InsertPosition)
		assert.Equal(t, 5, *req.InsertPosition)
	})

	t.Run("extracts delete operation fields", func(t *testing.T) {
		startLine, endLine := 10, 20
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-789",
			Payload: mustMarshalJSON(t, models.FileEditRequestPayload{
				FilePath:  "/tmp/test.txt",
				Operation: "delete_lines",
				StartLine: &startLine,
				EndLine:   &endLine,
			}),
			Timestamp: time.Now().UTC(),
		}

		req, err := payloadToFileEditRequest(msg)

		require.NoError(t, err)
		assert.NotNil(t, req.StartLine)
		assert.Equal(t, 10, *req.StartLine)
		assert.NotNil(t, req.EndLine)
		assert.Equal(t, 20, *req.EndLine)
	})

	t.Run("extracts patch operation fields", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    "case-789",
			Payload: mustMarshalJSON(t, models.FileEditRequestPayload{
				FilePath:     "/tmp/test.txt",
				Operation:    "patch",
				PatchContent: "--- a/file\n+++ b/file\n",
			}),
			Timestamp: time.Now().UTC(),
		}

		req, err := payloadToFileEditRequest(msg)

		require.NoError(t, err)
		assert.NotNil(t, req.PatchContent)
		assert.Equal(t, "--- a/file\n+++ b/file\n", *req.PatchContent)
	})
}

// ---------------------------------------------------------------------------
// handleAuditUserMsgRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleAuditUserMsgRequest(t *testing.T) {
	t.Run("records user message in audit vault", func(t *testing.T) {
		f, avs := newPubsubFixtureWithAuditVault(t)

		err := avs.CreateSession(f.Cfg.OperatorSessionId, "Test OperatorSession", "test-user")
		require.NoError(t, err)

		msg := PubSubCommandMessage{
			ID:                "lfaa-user-msg-1",
			EventType:         constants.Event.Operator.Audit.UserMsg,
			CaseID:            "case-123",
			OperatorSessionID: "test-web-session-123",
			Payload:           mustMarshalJSON(t, models.AuditMsgRequestPayload{Content: "Hello, this is a user message"}),
			Timestamp:         time.Now().UTC(),
		}

		f.Svc.audit.HandleUserMsgRequest(context.Background(), msg)

		events, err := avs.GetEvents(f.Cfg.OperatorSessionId, 10, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, storage.EventTypeUserMsg, events[0].Type)
		assert.Equal(t, "Hello, this is a user message", events[0].ContentText)
	})

	t.Run("skips recording when audit vault is nil", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:                "lfaa-user-msg-2",
			EventType:         constants.Event.Operator.Audit.UserMsg,
			OperatorSessionID: "test-web-session-123",
			Payload:           mustMarshalJSON(t, models.AuditMsgRequestPayload{Content: "This should not be recorded"}),
			Timestamp:         time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.audit.HandleUserMsgRequest(context.Background(), msg)
		})
	})

	t.Run("skips recording when content is empty", func(t *testing.T) {
		f, avs := newPubsubFixtureWithAuditVault(t)

		err := avs.CreateSession(f.Cfg.OperatorSessionId, "Test OperatorSession", "test-user")
		require.NoError(t, err)

		msg := PubSubCommandMessage{
			ID:                "lfaa-user-msg-3",
			EventType:         constants.Event.Operator.Audit.UserMsg,
			OperatorSessionID: "test-web-session-123",
			Payload:           mustMarshalJSON(t, models.AuditMsgRequestPayload{Content: ""}),
			Timestamp:         time.Now().UTC(),
		}

		f.Svc.audit.HandleUserMsgRequest(context.Background(), msg)

		events, err := avs.GetEvents(f.Cfg.OperatorSessionId, 10, 0)
		require.NoError(t, err)
		assert.Len(t, events, 0)
	})
}

// ---------------------------------------------------------------------------
// payloadToFsListRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_PayloadToFsListRequest(t *testing.T) {
	t.Run("valid payload with all fields", func(t *testing.T) {
		taskID := "task-123"
		invID := "inv-456"
		msg := PubSubCommandMessage{
			ID:              "msg-123",
			EventType:       constants.Event.Operator.FsList.Requested,
			CaseID:          "case-789",
			TaskID:          &taskID,
			InvestigationID: invID,
			Payload: mustMarshalJSON(t, models.FsListRequestPayload{
				Path:        "/tmp",
				ExecutionID: "exec-999",
				MaxDepth:    2,
				MaxEntries:  50,
			}),
			Timestamp: time.Now().UTC(),
		}

		req, err := payloadToFsListRequest(msg)

		require.NoError(t, err)
		assert.Equal(t, "exec-999", req.ExecutionID)
		assert.Equal(t, "case-789", req.CaseID)
		assert.Equal(t, "task-123", *req.TaskID)
		assert.Equal(t, "inv-456", req.InvestigationID)
		assert.Equal(t, "/tmp", req.Path)
		assert.Equal(t, 2, req.MaxDepth)
		assert.Equal(t, 50, req.MaxEntries)
	})

	t.Run("empty path defaults to current directory", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FsList.Requested,
			CaseID:    "case-789",
			Payload:   mustMarshalJSON(t, models.FsListRequestPayload{}),
			Timestamp: time.Now().UTC(),
		}

		req, err := payloadToFsListRequest(msg)

		require.NoError(t, err)
		assert.Equal(t, ".", req.Path)
		assert.Equal(t, 0, req.MaxDepth)
		assert.Equal(t, 100, req.MaxEntries)
	})

	t.Run("uses message ID when execution_id not provided", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "msg-789",
			EventType: constants.Event.Operator.FsList.Requested,
			CaseID:    "case-789",
			Payload:   mustMarshalJSON(t, models.FsListRequestPayload{Path: "/home"}),
			Timestamp: time.Now().UTC(),
		}

		req, err := payloadToFsListRequest(msg)

		require.NoError(t, err)
		assert.Equal(t, "msg-789", req.ExecutionID)
	})
}

// ---------------------------------------------------------------------------
// handleFsListRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleFsListRequest(t *testing.T) {
	t.Run("handles fs list request without panic", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FsList.Requested,
			CaseID:    "case-456",
			Payload:   mustMarshalJSON(t, models.FsListRequestPayload{Path: "/tmp"}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.fileOps.HandleFsListRequest(context.Background(), msg)
		})
	})

	t.Run("handles empty path defaulting to current directory", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FsList.Requested,
			CaseID:    "case-456",
			Payload:   mustMarshalJSON(t, models.FsListRequestPayload{}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.fileOps.HandleFsListRequest(context.Background(), msg)
		})
	})
}

// ---------------------------------------------------------------------------
// handleFetchLogsRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleFetchLogsRequest(t *testing.T) {
	t.Run("handles missing execution_id", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FetchLogs.Requested,
			CaseID:    "case-456",
			Payload:   mustMarshalJSON(t, models.FetchLogsRequestPayload{}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.history.HandleFetchLogsRequest(context.Background(), msg)
		})
	})

	t.Run("routes to raw vault by default when vault is nil", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FetchLogs.Requested,
			CaseID:    "case-456",
			Payload:   mustMarshalJSON(t, models.FetchLogsRequestPayload{ExecutionID: "exec-123"}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.history.HandleFetchLogsRequest(context.Background(), msg)
		})
	})

	t.Run("routes to scrubbed vault when sentinel_mode is scrubbed", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FetchLogs.Requested,
			CaseID:    "case-456",
			Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
				ExecutionID:  "exec-123",
				SentinelMode: constants.Status.VaultMode.Scrubbed,
			}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.history.HandleFetchLogsRequest(context.Background(), msg)
		})
	})
}

// ---------------------------------------------------------------------------
// handleFetchHistoryRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleFetchHistoryRequest(t *testing.T) {
	t.Run("handles nil history handler gracefully", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FetchHistory.Requested,
			CaseID:    "case-456",
			Payload:   json.RawMessage(`{}`),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.history.HandleFetchHistoryRequest(context.Background(), msg)
		})
	})
}

// ---------------------------------------------------------------------------
// handleFetchFileHistoryRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleFetchFileHistoryRequest(t *testing.T) {
	t.Run("handles nil history handler gracefully", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FetchFileHistory.Requested,
			CaseID:    "case-456",
			Payload:   mustMarshalJSON(t, models.FetchFileHistoryRequestPayload{FilePath: "/tmp/test.txt"}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.history.HandleFetchFileHistoryRequest(context.Background(), msg)
		})
	})
}

// ---------------------------------------------------------------------------
// handleRestoreFileRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleRestoreFileRequest(t *testing.T) {
	t.Run("handles nil history handler gracefully", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.RestoreFile.Requested,
			CaseID:    "case-456",
			Payload:   mustMarshalJSON(t, models.RestoreFileRequestPayload{FilePath: "/tmp/test.txt", CommitHash: "abc123"}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.history.HandleRestoreFileRequest(context.Background(), msg)
		})
	})
}

// ---------------------------------------------------------------------------
// publishLFAAResponse
// ---------------------------------------------------------------------------

func TestPubSubCommandService_PublishLFAAResponse(t *testing.T) {
	t.Run("publishes valid LFAA response", func(t *testing.T) {
		f := newPubsubFixture(t)

		taskID := "task-123"
		invID := "inv-456"
		msg := PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.FetchHistory.Requested,
			CaseID:            "case-789",
			TaskID:            &taskID,
			InvestigationID:   invID,
			OperatorSessionID: "web-123",
			Payload:           json.RawMessage(`{}`),
			Timestamp:         time.Now().UTC(),
		}

		publishLFAAResponseTo(context.Background(), f.DB, f.Cfg, f.Logger, msg, constants.Event.Operator.FetchHistory.Completed, []byte(`{"success": true, "events": []}`))

		published := f.DB.LastPublished()
		require.NotNil(t, published, "expected LFAA response to be published")
		assert.Contains(t, string(published.Data), constants.Event.Operator.FetchHistory.Completed)
	})

	t.Run("handles invalid response JSON gracefully", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FetchHistory.Requested,
			CaseID:    "case-789",
			Payload:   json.RawMessage(`{}`),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			publishLFAAResponseTo(context.Background(), f.DB, f.Cfg, f.Logger, msg, constants.Event.Operator.FetchHistory.Completed, []byte(`{invalid json}`))
		})
	})
}

// ---------------------------------------------------------------------------
// publishLFAAError
// ---------------------------------------------------------------------------

func TestPubSubCommandService_PublishLFAAError(t *testing.T) {
	t.Run("publishes LFAA error response", func(t *testing.T) {
		f := newPubsubFixture(t)

		taskID := "task-123"
		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FetchHistory.Requested,
			CaseID:    "case-789",
			TaskID:    &taskID,
			Payload:   json.RawMessage(`{}`),
			Timestamp: time.Now().UTC(),
		}

		publishLFAAErrorTo(context.Background(), f.DB, f.Cfg, f.Logger, msg, constants.Event.Operator.FetchHistory.Failed, "test error message")

		published := f.DB.LastPublished()
		require.NotNil(t, published, "expected LFAA error to be published")
		assert.Contains(t, string(published.Data), constants.Event.Operator.FetchHistory.Failed)
		assert.Contains(t, string(published.Data), "test error message")
	})
}

// ---------------------------------------------------------------------------
// publishFetchLogsError — via HistoryService
// ---------------------------------------------------------------------------

func TestPubSubCommandService_PublishFetchLogsError(t *testing.T) {
	t.Run("publishes fetch logs error", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:        "msg-123",
			EventType: constants.Event.Operator.FetchLogs.Requested,
			CaseID:    "case-789",
			Payload:   mustMarshalJSON(t, models.FetchLogsRequestPayload{}),
			Timestamp: time.Now().UTC(),
		}

		f.Svc.history.publishFetchLogsError(context.Background(), msg, "exec-123", "execution not found")

		published := f.DB.LastPublished()
		require.NotNil(t, published, "expected fetch logs error to be published")
		assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Failed)
		assert.Contains(t, string(published.Data), "execution not found")
	})
}

// ---------------------------------------------------------------------------
// String helpers (moved to tls_errors.go)
// ---------------------------------------------------------------------------

func TestPubSubCommandService_ObfuscatingLogger(t *testing.T) {
	t.Run("containsAny finds patterns", func(t *testing.T) {
		assert.True(t, containsAny("connection error", []string{"connection", "conn"}))
		assert.True(t, containsAny("EOF received", []string{"EOF"}))
		assert.False(t, containsAny("success", []string{"error", "fail"}))
	})

	t.Run("findSubstring case insensitive", func(t *testing.T) {
		assert.True(t, findSubstring("Connection Error", "connection"))
		assert.True(t, findSubstring("EOF", "eof"))
		assert.False(t, findSubstring("success", "error"))
	})

	t.Run("toLower converts correctly", func(t *testing.T) {
		assert.Equal(t, "hello world", toLower("Hello World"))
		assert.Equal(t, "abc123", toLower("ABC123"))
	})

	t.Run("hasSubstring works correctly", func(t *testing.T) {
		assert.True(t, hasSubstring("hello world", "world"))
		assert.True(t, hasSubstring("test", ""))
		assert.False(t, hasSubstring("short", "longstring"))
		assert.False(t, hasSubstring("abc", "xyz"))
	})
}

func TestPubSubCommandService_HandleShutdownRequest(t *testing.T) {
	f := newPubsubFixture(t)

	t.Run("successful shutdown request", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "shutdown-1",
			EventType: constants.Event.Operator.ShutdownRequested,
			Payload:   mustMarshalJSON(t, models.ShutdownRequestPayload{Reason: "remote control"}),
		}

		f.Svc.handleShutdownRequest(msg)

		select {
		case reason := <-f.Svc.ShutdownChan:
			assert.Equal(t, "remote control", reason)
		case <-time.After(1 * time.Second):
			t.Fatal("shutdown reason not received on channel")
		}
	})

	t.Run("shutdown request without reason", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "shutdown-2",
			EventType: constants.Event.Operator.ShutdownRequested,
			Payload:   json.RawMessage(`{}`),
		}

		f.Svc.handleShutdownRequest(msg)

		select {
		case reason := <-f.Svc.ShutdownChan:
			assert.Equal(t, "No reason provided", reason)
		case <-time.After(1 * time.Second):
			t.Fatal("shutdown reason not received on channel")
		}
	})

	t.Run("shutdown request with invalid payload", func(t *testing.T) {
		msg := PubSubCommandMessage{
			ID:        "shutdown-3",
			EventType: constants.Event.Operator.ShutdownRequested,
			Payload:   json.RawMessage(`invalid`),
		}

		f.Svc.handleShutdownRequest(msg)

		select {
		case reason := <-f.Svc.ShutdownChan:
			assert.Equal(t, "No reason provided", reason)
		case <-time.After(1 * time.Second):
			t.Fatal("shutdown reason not received on channel")
		}
	})
}

// ---------------------------------------------------------------------------
// SendAutomaticHeartbeat
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SendAutomaticHeartbeat(t *testing.T) {
	t.Run("sends automatic heartbeat when results service available", func(t *testing.T) {
		f := newPubsubFixture(t)

		f.Svc.SendAutomaticHeartbeat()

		published := f.DB.LastPublished()
		require.NotNil(t, published, "expected automatic heartbeat to be published")
		assert.Contains(t, string(published.Data), constants.Event.Operator.Heartbeat)
		assert.Contains(t, string(published.Data), string(models.HeartbeatTypeAutomatic))
	})

	t.Run("handles nil results service gracefully", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:         cfg,
			Logger:         logger,
			Execution:      execSvc,
			FileEdit:       fileEditSvc,
			PubSubClient:   NewMockVSODBPubSubClient(),
			ResultsService: nil,
		})
		require.NoError(t, err)

		svc.results = nil
		svc.heartbeat.results = nil
		svc.ctx = context.Background()

		assert.NotPanics(t, func() {
			svc.SendAutomaticHeartbeat()
		})
	})
}

// ---------------------------------------------------------------------------
// handleAuditAIMsgRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleAuditAIMsgRequest(t *testing.T) {
	t.Run("records AI message in audit vault", func(t *testing.T) {
		f, avs := newPubsubFixtureWithAuditVault(t)

		err := avs.CreateSession(f.Cfg.OperatorSessionId, "Test AI OperatorSession", "test-user")
		require.NoError(t, err)

		msg := PubSubCommandMessage{
			ID:                "lfaa-ai-msg-1",
			EventType:         constants.Event.Operator.Audit.AIMsg,
			CaseID:            "case-456",
			OperatorSessionID: "test-web-session-456",
			Payload:           mustMarshalJSON(t, models.AuditMsgRequestPayload{Content: "This is the AI response to your query"}),
			Timestamp:         time.Now().UTC(),
		}

		f.Svc.audit.HandleAIMsgRequest(context.Background(), msg)

		events, err := avs.GetEvents(f.Cfg.OperatorSessionId, 10, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, storage.EventTypeAIMsg, events[0].Type)
		assert.Equal(t, "This is the AI response to your query", events[0].ContentText)
	})

	t.Run("handles missing operator session ID gracefully", func(t *testing.T) {
		f, avs := newPubsubFixtureWithAuditVault(t)

		msg := PubSubCommandMessage{
			ID:                "lfaa-ai-msg-2",
			EventType:         constants.Event.Operator.Audit.AIMsg,
			OperatorSessionID: "",
			Payload:           mustMarshalJSON(t, models.AuditMsgRequestPayload{Content: "AI response without session"}),
			Timestamp:         time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.audit.HandleAIMsgRequest(context.Background(), msg)
		})
		_ = avs
	})
}

// ---------------------------------------------------------------------------
// handleAuditDirectCmdRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleAuditDirectCmdRequest(t *testing.T) {
	t.Run("records direct command in audit vault as CMD_EXEC", func(t *testing.T) {
		f, avs := newPubsubFixtureWithAuditVault(t)

		err := avs.CreateSession(f.Cfg.OperatorSessionId, "Direct CMD OperatorSession", "test-user")
		require.NoError(t, err)

		operatorSessionID := "web-session-direct-1"
		msg := PubSubCommandMessage{
			ID:                "lfaa-direct-cmd-1",
			EventType:         constants.Event.Operator.Audit.DirectCmd,
			CaseID:            "case-direct-1",
			OperatorSessionID: operatorSessionID,
			Payload: mustMarshalJSON(t, models.AuditDirectCmdRequestPayload{
				Command:           "ls -la /var/log",
				ExecutionID:       "exec-direct-1",
				OperatorSessionID: operatorSessionID,
			}),
			Timestamp: time.Now().UTC(),
		}

		f.Svc.audit.HandleDirectCmdRequest(context.Background(), msg)

		events, err := avs.GetEvents(f.Cfg.OperatorSessionId, 10, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, storage.EventTypeCmdExec, events[0].Type)
		assert.Equal(t, "ls -la /var/log", events[0].CommandRaw)
		assert.Equal(t, constants.Status.AISource.TerminalDirect, events[0].ContentText)
	})

	t.Run("skips recording when audit vault is nil", func(t *testing.T) {
		f := newPubsubFixture(t)

		msg := PubSubCommandMessage{
			ID:                "lfaa-direct-cmd-2",
			EventType:         constants.Event.Operator.Audit.DirectCmd,
			OperatorSessionID: "web-session-direct-2",
			Payload: mustMarshalJSON(t, models.AuditDirectCmdRequestPayload{
				Command:     "df -h",
				ExecutionID: "exec-direct-2",
			}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.audit.HandleDirectCmdRequest(context.Background(), msg)
		})
	})

	t.Run("skips recording when command is empty", func(t *testing.T) {
		f, avs := newPubsubFixtureWithAuditVault(t)

		err := avs.CreateSession(f.Cfg.OperatorSessionId, "Empty CMD OperatorSession", "test-user")
		require.NoError(t, err)

		msg := PubSubCommandMessage{
			ID:                "lfaa-direct-cmd-3",
			EventType:         constants.Event.Operator.Audit.DirectCmd,
			OperatorSessionID: "web-session-direct-3",
			Payload:           mustMarshalJSON(t, models.AuditDirectCmdRequestPayload{Command: ""}),
			Timestamp:         time.Now().UTC(),
		}

		f.Svc.audit.HandleDirectCmdRequest(context.Background(), msg)

		events, err := avs.GetEvents(f.Cfg.OperatorSessionId, 10, 0)
		require.NoError(t, err)
		assert.Len(t, events, 0)
	})
}

// ---------------------------------------------------------------------------
// handleAuditDirectCmdResultRequest
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleAuditDirectCmdResultRequest(t *testing.T) {
	t.Run("records direct command result with output", func(t *testing.T) {
		f, avs := newPubsubFixtureWithAuditVault(t)

		err := avs.CreateSession(f.Cfg.OperatorSessionId, "Direct CMD Result OperatorSession", "test-user")
		require.NoError(t, err)

		exitCode := 0
		msg := PubSubCommandMessage{
			ID:                "lfaa-direct-result-1",
			EventType:         constants.Event.Operator.Audit.DirectCmdResult,
			CaseID:            "case-result-1",
			OperatorSessionID: "web-session-result-1",
			Payload: mustMarshalJSON(t, models.AuditDirectCmdResultPayload{
				Command:              "ls -la /var/log",
				ExecutionID:          "exec-result-1",
				ExitCode:             &exitCode,
				Status:               "completed",
				Output:               "total 48\ndrwxr-xr-x 2 root root",
				Stderr:               "",
				ExecutionTimeSeconds: 0.12,
				OperatorSessionID:    "web-session-result-1",
			}),
			Timestamp: time.Now().UTC(),
		}

		f.Svc.audit.HandleDirectCmdResultRequest(context.Background(), msg)

		events, err := avs.GetEvents(f.Cfg.OperatorSessionId, 10, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, storage.EventTypeCmdExec, events[0].Type)
		assert.Equal(t, "ls -la /var/log", events[0].CommandRaw)
		assert.Equal(t, constants.Status.AISource.TerminalDirect, events[0].ContentText)
		assert.NotNil(t, events[0].CommandExitCode)
		assert.Equal(t, 0, *events[0].CommandExitCode)
		assert.Contains(t, events[0].CommandStdout, "total 48")
		assert.Equal(t, int64(120), events[0].ExecutionDurationMs)
	})

	t.Run("records failed direct command result with stderr", func(t *testing.T) {
		f, avs := newPubsubFixtureWithAuditVault(t)

		err := avs.CreateSession(f.Cfg.OperatorSessionId, "Failed CMD Result OperatorSession", "test-user")
		require.NoError(t, err)

		exitCode := 1
		msg := PubSubCommandMessage{
			ID:                "lfaa-direct-result-2",
			EventType:         constants.Event.Operator.Audit.DirectCmdResult,
			OperatorSessionID: "web-session-result-2",
			Payload: mustMarshalJSON(t, models.AuditDirectCmdResultPayload{
				Command:     "cat /nonexistent",
				ExecutionID: "exec-result-2",
				ExitCode:    &exitCode,
				Status:      "failed",
				Output:      "",
				Stderr:      "cat: /nonexistent: No such file or directory",
			}),
			Timestamp: time.Now().UTC(),
		}

		f.Svc.audit.HandleDirectCmdResultRequest(context.Background(), msg)

		events, err := avs.GetEvents(f.Cfg.OperatorSessionId, 10, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, storage.EventTypeCmdExec, events[0].Type)
		assert.Equal(t, "cat /nonexistent", events[0].CommandRaw)
		assert.NotNil(t, events[0].CommandExitCode)
		assert.Equal(t, 1, *events[0].CommandExitCode)
		assert.Contains(t, events[0].CommandStderr, "No such file or directory")
	})

	t.Run("skips recording when audit vault is nil", func(t *testing.T) {
		f := newPubsubFixture(t)

		exitCode := 0
		msg := PubSubCommandMessage{
			ID:                "lfaa-direct-result-3",
			EventType:         constants.Event.Operator.Audit.DirectCmdResult,
			OperatorSessionID: "web-session-result-3",
			Payload: mustMarshalJSON(t, models.AuditDirectCmdResultPayload{
				Command:  "hostname",
				ExitCode: &exitCode,
				Status:   "completed",
				Output:   "myhost",
			}),
			Timestamp: time.Now().UTC(),
		}

		assert.NotPanics(t, func() {
			f.Svc.audit.HandleDirectCmdResultRequest(context.Background(), msg)
		})
	})

	t.Run("skips recording when command is empty", func(t *testing.T) {
		f, avs := newPubsubFixtureWithAuditVault(t)

		err := avs.CreateSession(f.Cfg.OperatorSessionId, "Empty Result OperatorSession", "test-user")
		require.NoError(t, err)

		msg := PubSubCommandMessage{
			ID:                "lfaa-direct-result-4",
			EventType:         constants.Event.Operator.Audit.DirectCmdResult,
			OperatorSessionID: "web-session-result-4",
			Payload:           mustMarshalJSON(t, models.AuditDirectCmdResultPayload{Command: ""}),
			Timestamp:         time.Now().UTC(),
		}

		f.Svc.audit.HandleDirectCmdResultRequest(context.Background(), msg)

		events, err := avs.GetEvents(f.Cfg.OperatorSessionId, 10, 0)
		require.NoError(t, err)
		assert.Len(t, events, 0)
	})
}
