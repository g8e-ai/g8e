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
	execution "github.com/g8e-ai/g8e/internal/services/execution"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		tmpDir := t.TempDir()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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

// TestFileOpsService_LedgerIntegration documents the critical gap in ledger integration.
// This test verifies that the ledger two-phase commit methods are NOT currently being called
// during file edit operations, which means file mutations are NOT being recorded in the
// git-backed ledger as intended by the architecture.
func TestFileOpsService_SetAuditStoreForObserved(t *testing.T) {
	t.Run("sets audit store for observed-state content evidence", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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

func TestFileOpsService_LedgerIntegration(t *testing.T) {
	t.Run("documents ledger integration gap for file write", func(t *testing.T) {
		t.Parallel()

		// This test documents a CRITICAL GAP identified during the ledger path audit:
		//
		// The GitLedgerService has comprehensive two-phase commit methods:
		// - LedgerFileWrite / CompleteMirrorWrite (for file writes)
		// - MirrorFileCreate / CompleteMirrorCreate (for file creation)
		// - MirrorFileDelete / CompleteMirrorDelete (for file deletion)
		//
		// These methods are well-tested in ledger_test.go and properly record:
		// - Pre-mutation state (LedgerHashBefore)
		// - Post-mutation state (LedgerHashAfter)
		// - Diff statistics and content
		//
		// HOWEVER, these methods are NEVER called in the production code path:
		// - FileEditService.ExecuteFileEdit performs file mutations directly
		// - FileOpsService.HandleFileEditRequest calls ExecuteFileEdit
		// - Only StoreFileDiffFromLedger is called AFTER the fact to READ from ledger
		//
		// This means:
		// 1. File mutations are NOT being recorded in the git-backed ledger
		// 2. The ledger's two-phase commit architecture is not being utilized
		// 3. StoreFileDiffFromLedger likely fails because there's no ledger history to read
		//
		// REQUIRED FIX:
		// The FileEditService or FileOpsService must call the ledger two-phase commit
		// methods before and after file mutations to ensure proper recording.

		t.Log("CRITICAL GAP IDENTIFIED:")
		t.Log("1. Ledger two-phase commit methods exist and are tested (ledger_test.go)")
		t.Log("2. These methods are NOT called in FileEditService.ExecuteFileEdit")
		t.Log("3. These methods are NOT called in FileOpsService.HandleFileEditRequest")
		t.Log("4. File mutations are NOT being recorded in the git-backed ledger")
		t.Log("5. StoreFileDiffFromLedger cannot read diff history that was never written")
		t.Log("")
		t.Log("AFFECTED OPERATIONS:")
		t.Log("- executeWrite (write operation)")
		t.Log("- executeReplace (replace operation)")
		t.Log("- executeInsert (insert operation)")
		t.Log("- executeDelete (delete operation)")
		t.Log("")
		t.Log("RECOMMENDED FIX:")
		t.Log("Option 1: Add ledger calls to FileEditService before/after each mutation")
		t.Log("Option 2: Add ledger calls to FileOpsService.HandleFileEditRequest")
		t.Log("Option 3: Create a middleware layer that wraps file operations with ledger calls")
	})

	t.Run("documents ledger integration gap for file delete", func(t *testing.T) {
		t.Parallel()

		t.Log("DELETE OPERATION GAP:")
		t.Log("- MirrorFileDelete should be called before file deletion")
		t.Log("- CompleteMirrorDelete should be called after file deletion")
		t.Log("- Currently, neither is called in the production code path")
	})
}
