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

package models

import (
	"path/filepath"
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
)

func TestCommandRequestPayload(t *testing.T) {
	t.Run("creates valid command request", func(t *testing.T) {
		req := &CommandRequestPayload{
			Command:        "ls",
			ExecutionID:    "req-123",
			Justification:  "list files",
			VaultMode:      "strict",
			TimeoutSeconds: 30,
		}

		assert.Equal(t, "ls", req.Command)
		assert.Equal(t, "list files", req.Justification)
		assert.Equal(t, 30, req.TimeoutSeconds)
	})
}

func TestCommandCancelRequestPayload(t *testing.T) {
	t.Run("creates valid cancel request", func(t *testing.T) {
		req := &CommandCancelRequestPayload{
			ExecutionID: "req-123",
		}

		assert.Equal(t, "req-123", req.ExecutionID)
	})
}

func TestFileEditRequestPayload(t *testing.T) {
	t.Run("creates write request", func(t *testing.T) {
		tmpDir := t.TempDir()
		insertPos := 10
		startLine := 5
		endLine := 10

		req := &FileEditRequestPayload{
			FilePath:        filepath.Join(tmpDir, "test.txt"),
			Operation:       "write",
			ExecutionID:     "req-123",
			VaultMode:       "strict",
			Justification:   "testing",
			Content:         "test content",
			OldContent:      "old",
			NewContent:      "new",
			InsertContent:   "inserted",
			InsertPosition:  &insertPos,
			StartLine:       &startLine,
			EndLine:         &endLine,
			PatchContent:    "patch",
			CreateBackup:    true,
			CreateIfMissing: true,
		}

		assert.Equal(t, filepath.Join(tmpDir, "test.txt"), req.FilePath)
		assert.Equal(t, "write", req.Operation)
		assert.True(t, req.CreateBackup)
		assert.Equal(t, 10, *req.InsertPosition)
	})
}

func TestFsListRequestPayload(t *testing.T) {
	t.Run("creates valid list request", func(t *testing.T) {
		tmpDir := t.TempDir()
		req := &FsListRequestPayload{
			Path:        tmpDir,
			ExecutionID: "req-123",
			MaxDepth:    3,
			MaxEntries:  100,
		}

		assert.Equal(t, tmpDir, req.Path)
		assert.Equal(t, 3, req.MaxDepth)
		assert.Equal(t, 100, req.MaxEntries)
	})
}

func TestFsGrepRequestPayload(t *testing.T) {
	t.Run("creates valid grep request", func(t *testing.T) {
		tmpDir := t.TempDir()
		req := &FsGrepRequestPayload{
			Path:        tmpDir,
			ExecutionID: "req-123",
			Pattern:     "test",
			Includes:    []string{"*.go"},
			MaxMatches:  50,
		}

		assert.Equal(t, "test", req.Pattern)
		assert.Equal(t, 50, req.MaxMatches)
		assert.Len(t, req.Includes, 1)
	})
}

func TestFetchLogsRequestPayload(t *testing.T) {
	t.Run("creates valid logs request", func(t *testing.T) {
		req := &FetchLogsRequestPayload{
			ExecutionID: "req-123",
			VaultMode:   "strict",
		}

		assert.Equal(t, "req-123", req.ExecutionID)
	})
}

func TestFetchFileDiffRequestPayload(t *testing.T) {
	t.Run("creates valid diff request", func(t *testing.T) {
		tmpDir := t.TempDir()
		req := &FetchFileDiffRequestPayload{
			DiffID:            "diff-123",
			OperatorSessionID: "session-123",
			FilePath:          filepath.Join(tmpDir, "test.txt"),
			Limit:             10,
		}

		assert.Equal(t, "diff-123", req.DiffID)
		assert.Equal(t, 10, req.Limit)
	})
}

func TestFetchHistoryRequestPayload(t *testing.T) {
	t.Run("creates valid history request", func(t *testing.T) {
		req := &FetchHistoryRequestPayload{}
		assert.NotNil(t, req)
	})
}

func TestFetchFileHistoryRequestPayload(t *testing.T) {
	t.Run("creates valid file history request", func(t *testing.T) {
		tmpDir := t.TempDir()
		req := &FetchFileHistoryRequestPayload{
			FilePath: filepath.Join(tmpDir, "test.txt"),
		}

		assert.Equal(t, filepath.Join(tmpDir, "test.txt"), req.FilePath)
	})
}

func TestRestoreFileRequestPayload(t *testing.T) {
	t.Run("creates valid restore request", func(t *testing.T) {
		tmpDir := t.TempDir()
		req := &RestoreFileRequestPayload{
			FilePath:   filepath.Join(tmpDir, "test.txt"),
			CommitHash: "abc123",
		}

		assert.Equal(t, filepath.Join(tmpDir, "test.txt"), req.FilePath)
		assert.Equal(t, "abc123", req.CommitHash)
	})
}

func TestShutdownRequestPayload(t *testing.T) {
	t.Run("creates valid shutdown request", func(t *testing.T) {
		req := &ShutdownRequestPayload{
			Reason: "maintenance",
		}

		assert.Equal(t, "maintenance", req.Reason)
	})
}

func TestAuditMsgRequestPayload(t *testing.T) {
	t.Run("creates valid audit message request", func(t *testing.T) {
		req := &AuditMsgRequestPayload{
			Content:           "test message",
			OperatorSessionID: "session-123",
		}

		assert.Equal(t, "test message", req.Content)
		assert.Equal(t, "session-123", req.OperatorSessionID)
	})
}

func TestAuditDirectCmdRequestPayload(t *testing.T) {
	t.Run("creates valid direct command request", func(t *testing.T) {
		req := &AuditDirectCmdRequestPayload{
			Command:           "ls",
			ExecutionID:       "req-123",
			OperatorSessionID: "session-123",
		}

		assert.Equal(t, "ls", req.Command)
		assert.Equal(t, "session-123", req.OperatorSessionID)
	})
}

func TestAuditDirectCmdResultPayload(t *testing.T) {
	t.Run("creates successful command result", func(t *testing.T) {
		exitCode := 0
		result := &AuditDirectCmdResultPayload{
			Command:              "ls",
			ExecutionID:          "req-123",
			ExitCode:             &exitCode,
			Status:               operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Output:               "file1.txt\nfile2.txt",
			Stderr:               "",
			ExecutionTimeSeconds: 0.5,
			OperatorSessionID:    "session-123",
		}

		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, 0, *result.ExitCode)
		assert.InEpsilon(t, 0.5, result.ExecutionTimeSeconds, 0.0)
	})
}

func TestFsReadRequestPayload(t *testing.T) {
	t.Run("creates valid read request", func(t *testing.T) {
		tmpDir := t.TempDir()
		req := &FsReadRequestPayload{
			Path:        filepath.Join(tmpDir, "test.txt"),
			ExecutionID: "req-123",
			MaxSize:     1024,
		}

		assert.Equal(t, filepath.Join(tmpDir, "test.txt"), req.Path)
		assert.Equal(t, 1024, req.MaxSize)
	})
}

func TestPortCheckRequestPayload(t *testing.T) {
	t.Run("creates valid port check request", func(t *testing.T) {
		req := &PortCheckRequestPayload{
			ExecutionID: "req-123",
			Host:        "localhost",
			Port:        8080,
			Protocol:    "tcp",
		}

		assert.Equal(t, 8080, req.Port)
		assert.Equal(t, "tcp", req.Protocol)
	})
}

func TestHeartbeatRequestPayload(t *testing.T) {
	t.Run("creates valid heartbeat request", func(t *testing.T) {
		req := &HeartbeatRequestPayload{}
		assert.NotNil(t, req)
	})
}
