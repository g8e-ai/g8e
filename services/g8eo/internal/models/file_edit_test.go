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
	"testing"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/stretchr/testify/assert"
)

func TestFileEditOperations(t *testing.T) {
	tests := []struct {
		name      string
		operation constants.FileOperation
		expected  string
	}{
		{"read", constants.FileOperationRead, "read"},
		{"write", constants.FileOperationWrite, "write"},
		{"replace", constants.FileOperationReplace, "replace"},
		{"delete", constants.FileOperationDelete, "delete"},
		{"insert", constants.FileOperationInsert, "insert"},
		{"patch", constants.FileOperationPatch, "patch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.operation))
		})
	}
}

func TestFileEditRequest(t *testing.T) {
	t.Run("creates valid write request", func(t *testing.T) {
		content := "test content"
		taskID := "task-123"

		req := &FileEditRequest{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			TaskID:          &taskID,
			Operation:       constants.FileOperationWrite,
			FilePath:        "/tmp/test.txt",
			Content:         &content,
			RequestedBy:     "user@example.com",
			Justification:   "testing",
			CreateBackup:    true,
			CreateIfMissing: true,
		}

		assert.Equal(t, "req-123", req.ExecutionID)
		assert.Equal(t, constants.FileOperationWrite, req.Operation)
		assert.Equal(t, "/tmp/test.txt", req.FilePath)
		assert.Equal(t, "test content", *req.Content)
		assert.True(t, req.CreateBackup)
		assert.True(t, req.CreateIfMissing)
	})

	t.Run("creates valid replace request", func(t *testing.T) {
		oldContent := "old"
		newContent := "new"

		req := &FileEditRequest{
			ExecutionID: "req-123",
			CaseID:      "case-456",
			Operation:   constants.FileOperationReplace,
			FilePath:    "/tmp/test.txt",
			OldContent:  &oldContent,
			NewContent:  &newContent,
		}

		assert.Equal(t, constants.FileOperationReplace, req.Operation)
		assert.Equal(t, "old", *req.OldContent)
		assert.Equal(t, "new", *req.NewContent)
	})

	t.Run("creates valid insert request", func(t *testing.T) {
		insertContent := "inserted text"
		insertPos := 10

		req := &FileEditRequest{
			ExecutionID:    "req-123",
			CaseID:         "case-456",
			Operation:      constants.FileOperationInsert,
			FilePath:       "/tmp/test.txt",
			InsertContent:  &insertContent,
			InsertPosition: &insertPos,
		}

		assert.Equal(t, constants.FileOperationInsert, req.Operation)
		assert.Equal(t, "inserted text", *req.InsertContent)
		assert.Equal(t, 10, *req.InsertPosition)
	})

	t.Run("creates valid delete request with line range", func(t *testing.T) {
		startLine := 5
		endLine := 10

		req := &FileEditRequest{
			ExecutionID: "req-123",
			CaseID:      "case-456",
			Operation:   constants.FileOperationDelete,
			FilePath:    "/tmp/test.txt",
			StartLine:   &startLine,
			EndLine:     &endLine,
		}

		assert.Equal(t, constants.FileOperationDelete, req.Operation)
		assert.Equal(t, 5, *req.StartLine)
		assert.Equal(t, 10, *req.EndLine)
	})
}

func TestFileEditResult(t *testing.T) {
	t.Run("creates successful result", func(t *testing.T) {
		taskID := "task-123"
		backupPath := "/tmp/test.txt.bak"
		bytesWritten := int64(100)
		linesChanged := 10

		result := &FileEditResult{
			ExecutionID:  "req-123",
			CaseID:       "case-456",
			TaskID:       &taskID,
			Operation:    constants.FileOperationWrite,
			FilePath:     "/tmp/test.txt",
			Status:       operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			BackupPath:   &backupPath,
			BytesWritten: &bytesWritten,
			LinesChanged: &linesChanged,
		}

		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, constants.FileOperationWrite, result.Operation)
		assert.Equal(t, "/tmp/test.txt.bak", *result.BackupPath)
		assert.Equal(t, int64(100), *result.BytesWritten)
		assert.Equal(t, 10, *result.LinesChanged)
	})

	t.Run("creates failed result", func(t *testing.T) {
		errorMsg := "file not found"
		errorType := "not_found"

		result := &FileEditResult{
			ExecutionID:  "req-123",
			CaseID:       "case-456",
			Operation:    constants.FileOperationWrite,
			FilePath:     "/tmp/nonexistent.txt",
			Status:       operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			ErrorMessage: &errorMsg,
			ErrorType:    &errorType,
		}

		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, result.Status)
		assert.Equal(t, "file not found", *result.ErrorMessage)
		assert.Equal(t, "not_found", *result.ErrorType)
	})

}
