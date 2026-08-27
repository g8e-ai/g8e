// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	execution "github.com/g8e-ai/g8e/v2/internal/services/execution"
	pubsubtest "github.com/g8e-ai/g8e/v2/internal/services/pubsub/pubsubtest"
	storage "github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// mockAuditEventRecorder is a test-only implementation of AuditEventRecorder
type mockAuditEventRecorder struct {
	recordedEvents []*storage.Event
}

func (m *mockAuditEventRecorder) RecordEvent(event *storage.Event) (int64, error) {
	m.recordedEvents = append(m.recordedEvents, event)
	return int64(len(m.recordedEvents)), nil
}

func TestPayloadToFileEditRequest(t *testing.T) {
	t.Run("converts valid payload", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FileEditRequested{
			FilePath:        filepath.Join(tmpDir, "test.txt"),
			Operation:       "write",
			ExecutionId:     "exec-1",
			Justification:   "test",
			CreateBackup:    true,
			CreateIfMissing: true,
			Content:         "test content",
			OldContent:      "old content",
			NewContent:      "new content",
			InsertContent:   "insert content",
			InsertPosition:  5,
			StartLine:       10,
			EndLine:         20,
			PatchContent:    "patch content",
		}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:              "msg-1",
			EventType:       constants.Event.Operator.FileEdit.Requested,
			CaseID:          "case-1",
			TaskID:          func() *string { s := "task-1"; return &s }(),
			InvestigationID: "investigation-1",
			Payload:         payload,
		}

		editReq, err := payloadToFileEditRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, "exec-1", editReq.ExecutionID)
		assert.Equal(t, "case-1", editReq.CaseID)
		assert.Equal(t, "task-1", editReq.TaskID)
		assert.Equal(t, "investigation-1", editReq.InvestigationID)
		assert.Equal(t, constants.FileOperation("write"), editReq.Operation)
		assert.Equal(t, filepath.Join(tmpDir, "test.txt"), editReq.FilePath)
		assert.Equal(t, "test", editReq.Justification)
		assert.True(t, editReq.CreateBackup)
		assert.True(t, editReq.CreateIfMissing)
		assert.Equal(t, "test content", editReq.Content)
		assert.Equal(t, "old content", editReq.OldContent)
		assert.Equal(t, "new content", editReq.NewContent)
		assert.Equal(t, "insert content", editReq.InsertContent)
		assert.Equal(t, 5, editReq.InsertPosition)
		assert.Equal(t, 10, editReq.StartLine)
		assert.Equal(t, 20, editReq.EndLine)
		assert.Equal(t, "patch content", editReq.PatchContent)
	})

	t.Run("rejects invalid protobuf", func(t *testing.T) {
		t.Parallel()
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FileEdit.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		_, err := payloadToFileEditRequest(msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode")
	})

	t.Run("rejects missing file_path", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FileEditRequested{Operation: "write"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FileEdit.Requested,
			Payload:   payload,
		}

		_, err := payloadToFileEditRequest(msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing file_path")
	})

	t.Run("rejects missing operation", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FileEditRequested{FilePath: filepath.Join(tmpDir, "test.txt")}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FileEdit.Requested,
			Payload:   payload,
		}

		_, err := payloadToFileEditRequest(msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing operation")
	})

	t.Run("uses message ID when payload has no execution_id", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FileEditRequested{FilePath: filepath.Join(tmpDir, "test.txt"), Operation: "write"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FileEdit.Requested,
			Payload:   payload,
		}

		editReq, err := payloadToFileEditRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, "msg-1", editReq.ExecutionID)
	})

	t.Run("uses default justification when not specified", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FileEditRequested{FilePath: filepath.Join(tmpDir, "test.txt"), Operation: "write"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FileEdit.Requested,
			Payload:   payload,
		}

		editReq, err := payloadToFileEditRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, "pub/sub command request", editReq.Justification)
	})
}

func TestPayloadToFsListRequest(t *testing.T) {
	t.Run("converts valid payload", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FsListRequested{
			Path:        tmpDir,
			ExecutionId: "exec-1",
			MaxDepth:    5,
			MaxEntries:  200,
		}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:              "msg-1",
			EventType:       constants.Event.Operator.FsList.Requested,
			CaseID:          "case-1",
			TaskID:          func() *string { s := "task-1"; return &s }(),
			InvestigationID: "investigation-1",
			Payload:         payload,
		}

		listReq, err := payloadToFsListRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, "exec-1", listReq.ExecutionID)
		assert.Equal(t, "case-1", listReq.CaseID)
		assert.Equal(t, "task-1", listReq.TaskID)
		assert.Equal(t, "investigation-1", listReq.InvestigationID)
		assert.Equal(t, tmpDir, listReq.Path)
		assert.Equal(t, 5, listReq.MaxDepth)
		assert.Equal(t, 200, listReq.MaxEntries)
	})

	t.Run("rejects invalid protobuf", func(t *testing.T) {
		t.Parallel()
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsList.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		_, err := payloadToFsListRequest(msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode")
	})

	t.Run("uses default path when not specified", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FsListRequested{}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsList.Requested,
			Payload:   payload,
		}

		listReq, err := payloadToFsListRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, ".", listReq.Path)
	})

	t.Run("uses message ID when payload has no execution_id", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FsListRequested{Path: tmpDir}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsList.Requested,
			Payload:   payload,
		}

		listReq, err := payloadToFsListRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, "msg-1", listReq.ExecutionID)
	})

	t.Run("uses default max_entries when not specified", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FsListRequested{Path: tmpDir}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsList.Requested,
			Payload:   payload,
		}

		listReq, err := payloadToFsListRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, 100, listReq.MaxEntries)
	})
}

func TestPayloadToFsGrepRequest(t *testing.T) {
	t.Run("converts valid payload", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FsGrepRequested{
			Path:        tmpDir,
			Pattern:     "test",
			ExecutionId: "exec-1",
			Includes:    []string{"*.go", "*.py"},
			MaxMatches:  200,
		}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:              "msg-1",
			EventType:       constants.Event.Operator.FsGrep.Requested,
			CaseID:          "case-1",
			TaskID:          func() *string { s := "task-1"; return &s }(),
			InvestigationID: "investigation-1",
			Payload:         payload,
		}

		grepReq, err := payloadToFsGrepRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, "exec-1", grepReq.ExecutionID)
		assert.Equal(t, "case-1", grepReq.CaseID)
		assert.Equal(t, "task-1", grepReq.TaskID)
		assert.Equal(t, "investigation-1", grepReq.InvestigationID)
		assert.Equal(t, tmpDir, grepReq.Path)
		assert.Equal(t, "test", grepReq.Pattern)
		assert.Equal(t, []string{"*.go", "*.py"}, grepReq.Includes)
		assert.Equal(t, 200, grepReq.MaxMatches)
	})

	t.Run("rejects invalid protobuf", func(t *testing.T) {
		t.Parallel()
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsGrep.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		_, err := payloadToFsGrepRequest(msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode")
	})

	t.Run("uses default path when not specified", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FsGrepRequested{Pattern: "test"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsGrep.Requested,
			Payload:   payload,
		}

		grepReq, err := payloadToFsGrepRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, ".", grepReq.Path)
	})

	t.Run("uses message ID when payload has no execution_id", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FsGrepRequested{Path: tmpDir, Pattern: "test"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsGrep.Requested,
			Payload:   payload,
		}

		grepReq, err := payloadToFsGrepRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, "msg-1", grepReq.ExecutionID)
	})

	t.Run("uses default max_matches when not specified", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		req := &operatorv1.FsGrepRequested{Path: tmpDir, Pattern: "test"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsGrep.Requested,
			Payload:   payload,
		}

		grepReq, err := payloadToFsGrepRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, 100, grepReq.MaxMatches)
	})
}

func TestFileOpsService_HandleFileEditRequest(t *testing.T) {
	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FileEdit.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleFileEditRequest(context.Background(), msg)
		// Should log error and return without panic
	})

	t.Run("rejects missing file_path", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		req := &operatorv1.FileEditRequested{Operation: "write"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FileEdit.Requested,
			Payload:   payload,
		}

		svc.HandleFileEditRequest(context.Background(), msg)
		// Should log error and return without panic
	})

	t.Run("rejects missing operation", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		req := &operatorv1.FileEditRequested{FilePath: filepath.Join(tmpDir, "test.txt")}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FileEdit.Requested,
			Payload:   payload,
		}

		svc.HandleFileEditRequest(context.Background(), msg)
		// Should log error and return without panic
	})
}

func TestFileOpsService_HandleFsGrepRequest(t *testing.T) {
	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsGrep.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleFsGrepRequest(context.Background(), msg)
		// Should log error and return without panic
	})
}

func TestFileOpsService_HandleFsReadRequest(t *testing.T) {
	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsRead.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleFsReadRequest(context.Background(), msg)
		// Should log error and return without panic
	})

	t.Run("rejects missing path", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		req := &operatorv1.FsReadRequested{Path: ""}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsRead.Requested,
			Payload:   payload,
		}

		svc.HandleFsReadRequest(context.Background(), msg)
		// Should log error and return without panic
	})
}

func TestFileOpsService_SetAuditStoreForObserved(t *testing.T) {
	t.Run("sets audit store for observed-state content evidence", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		// Create a mock audit store
		mockAuditStore := &mockAuditEventRecorder{}
		svc.SetAuditStoreForObserved(mockAuditStore)

		assert.Equal(t, mockAuditStore, svc.auditStoreForObserved)
	})

	t.Run("sets nil audit store", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		svc.SetAuditStoreForObserved(nil)
		assert.Nil(t, svc.auditStoreForObserved)
	})
}

func TestFileOpsService_HandleFsListRequest(t *testing.T) {
	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsList.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleFsListRequest(context.Background(), msg)
		// Should log error and return without panic
	})

	t.Run("uses default path when not specified", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

		req := &operatorv1.FsListRequested{}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.FsList.Requested,
			Payload:   payload,
		}

		svc.HandleFsListRequest(context.Background(), msg)
		// Should not panic
	})
}

func TestFileExistsOnDisk(t *testing.T) {
	t.Run("returns true for existing file", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		testFile := filepath.Join(tmpDir, "exists.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("test"), constants.PermFilePrivate))

		assert.True(t, fileExistsOnDisk(testFile))
	})

	t.Run("returns false for non-existent file", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)

		assert.False(t, fileExistsOnDisk(filepath.Join(tmpDir, "missing.txt")))
	})

	t.Run("returns false for directory", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)

		assert.False(t, fileExistsOnDisk(tmpDir))
	})
}

func TestFileOpsService_BeginLedgerTwoPhaseCommit(t *testing.T) {
	t.Run("returns nil when ledger is not git-ready", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		cfg := testutil.NewTestConfig(t)
		cfg.WorkDir = tmpDir
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)
		svc.ledger = &storage.GitLedgerService{}

		testFile := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("content"), constants.PermFilePrivate))

		result := svc.beginLedgerTwoPhaseCommit(testFile, "write")
		assert.Nil(t, result)
	})

	t.Run("returns nil for path traversal attempt", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		cfg := testutil.NewTestConfig(t)
		cfg.WorkDir = tmpDir
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)
		svc.ledger = &storage.GitLedgerService{}

		result := svc.beginLedgerTwoPhaseCommit(filepath.Join(tmpDir, "..", "escape.txt"), "write")
		assert.Nil(t, result)
	})

	t.Run("returns nil when file does not exist and operation is not create/write", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		cfg := testutil.NewTestConfig(t)
		cfg.WorkDir = tmpDir
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)
		svc.ledger = &storage.GitLedgerService{}

		nonExistentFile := filepath.Join(tmpDir, "missing.txt")
		result := svc.beginLedgerTwoPhaseCommit(nonExistentFile, "replace")
		assert.Nil(t, result)
	})
}

func TestFileOpsService_CompleteLedgerTwoPhaseCommit(t *testing.T) {
	t.Run("returns nil when ledger is not git-ready", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)
		svc.ledger = &storage.GitLedgerService{}

		ledgerResult := &storage.LedgerResult{
			Operation: storage.FileMutationWrite,
		}
		err := svc.completeLedgerTwoPhaseCommit(ledgerResult)
		assert.NoError(t, err)
	})

	t.Run("returns nil for nil ledger result", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		svc := NewFileOpsService(cfg, logger, fileEditSvc, client)
		svc.ledger = &storage.GitLedgerService{}

		err := svc.completeLedgerTwoPhaseCommit(nil)
		assert.NoError(t, err)
	})
}
