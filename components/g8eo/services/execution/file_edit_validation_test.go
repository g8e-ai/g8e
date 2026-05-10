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

package execution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileEditService_ValidationErrors tests input validation edge cases
func TestFileEditService_ValidationErrors(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("read non-existent file returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		nonExistent := filepath.Join(tmpDir, "does-not-exist.txt")

		req := &models.FileEditRequest{
			ExecutionID: "read-nonexistent-1",
			CaseID:      "test-case-validation",
			Operation:   models.FileEditOperationRead,
			FilePath:    nonExistent,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "does not exist")
	})

	t.Run("write without content returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")

		req := &models.FileEditRequest{
			ExecutionID:     "write-no-content-1",
			CaseID:          "test-case-validation",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         nil,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "content is required")
	})

	t.Run("write to non-existent file without create flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "does-not-exist.txt")
		content := "test"

		req := &models.FileEditRequest{
			ExecutionID:     "write-no-create-1",
			CaseID:          "test-case-validation",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &content,
			CreateIfMissing: false,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "does not exist")
	})

	t.Run("replace without old_content returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("content"), 0644)

		newContent := "new"
		req := &models.FileEditRequest{
			ExecutionID: "replace-no-old-1",
			CaseID:      "test-case-validation",
			Operation:   models.FileEditOperationReplace,
			FilePath:    testFile,
			OldContent:  nil,
			NewContent:  &newContent,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "old_content and new_content are required")
	})

	t.Run("replace old_content not found in file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("existing content"), 0644)

		oldContent := "missing_text"
		newContent := "new"
		req := &models.FileEditRequest{
			ExecutionID: "replace-not-found-1",
			CaseID:      "test-case-validation",
			Operation:   models.FileEditOperationReplace,
			FilePath:    testFile,
			OldContent:  &oldContent,
			NewContent:  &newContent,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "old_content not found")
	})

	t.Run("insert without position returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("content"), 0644)

		insertContent := "new line"
		req := &models.FileEditRequest{
			ExecutionID:    "insert-no-pos-1",
			CaseID:         "test-case-validation",
			Operation:      models.FileEditOperationInsert,
			FilePath:       testFile,
			InsertContent:  &insertContent,
			InsertPosition: nil,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "insert_content and insert_position are required")
	})

	t.Run("insert position out of range", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("Line 1\nLine 2"), 0644)

		insertContent := "new line"
		insertPos := 100
		req := &models.FileEditRequest{
			ExecutionID:    "insert-out-range-1",
			CaseID:         "test-case-validation",
			Operation:      models.FileEditOperationInsert,
			FilePath:       testFile,
			InsertContent:  &insertContent,
			InsertPosition: &insertPos,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "out of range")
	})

	t.Run("delete without line range returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("content"), 0644)

		req := &models.FileEditRequest{
			ExecutionID: "delete-no-range-1",
			CaseID:      "test-case-validation",
			Operation:   models.FileEditOperationDelete,
			FilePath:    testFile,
			StartLine:   nil,
			EndLine:     nil,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "start_line and end_line are required")
	})

	t.Run("delete invalid line range", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("Line 1\nLine 2"), 0644)

		startLine := 5
		endLine := 10
		req := &models.FileEditRequest{
			ExecutionID: "delete-invalid-1",
			CaseID:      "test-case-validation",
			Operation:   models.FileEditOperationDelete,
			FilePath:    testFile,
			StartLine:   &startLine,
			EndLine:     &endLine,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "invalid line range")
	})

	t.Run("delete with start_line greater than end_line", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("Line 1\nLine 2\nLine 3"), 0644)

		startLine := 3
		endLine := 1
		req := &models.FileEditRequest{
			ExecutionID: "delete-backwards-1",
			CaseID:      "test-case-validation",
			Operation:   models.FileEditOperationDelete,
			FilePath:    testFile,
			StartLine:   &startLine,
			EndLine:     &endLine,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "invalid line range")
	})
}

// TestFileEditService_EdgeCaseOperations tests boundary and edge case scenarios
func TestFileEditService_EdgeCaseOperations(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("read empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "empty.txt")
		os.WriteFile(testFile, []byte(""), 0644)

		req := &models.FileEditRequest{
			ExecutionID: "read-empty-1",
			CaseID:      "test-case-edge",
			Operation:   models.FileEditOperationRead,
			FilePath:    testFile,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, "", *result.Content)
	})

	t.Run("write empty content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "empty-write.txt")
		content := ""

		req := &models.FileEditRequest{
			ExecutionID:     "write-empty-1",
			CaseID:          "test-case-edge",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &content,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		data, _ := os.ReadFile(testFile)
		assert.Equal(t, "", string(data))
	})

	t.Run("insert at beginning (position 1)", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "insert-begin.txt")
		os.WriteFile(testFile, []byte("Line 1\nLine 2"), 0644)

		insertContent := "New First Line"
		insertPos := 1
		req := &models.FileEditRequest{
			ExecutionID:    "insert-begin-1",
			CaseID:         "test-case-edge",
			Operation:      models.FileEditOperationInsert,
			FilePath:       testFile,
			InsertContent:  &insertContent,
			InsertPosition: &insertPos,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		data, _ := os.ReadFile(testFile)
		lines := strings.Split(string(data), "\n")
		assert.Equal(t, "New First Line", lines[0])
	})

	t.Run("insert at end", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "insert-end.txt")
		os.WriteFile(testFile, []byte("Line 1\nLine 2"), 0644)

		insertContent := "New Last Line"
		insertPos := 3
		req := &models.FileEditRequest{
			ExecutionID:    "insert-end-1",
			CaseID:         "test-case-edge",
			Operation:      models.FileEditOperationInsert,
			FilePath:       testFile,
			InsertContent:  &insertContent,
			InsertPosition: &insertPos,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		data, _ := os.ReadFile(testFile)
		assert.Contains(t, string(data), "New Last Line")
	})

	t.Run("delete entire file content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "delete-all.txt")
		os.WriteFile(testFile, []byte("Line 1\nLine 2\nLine 3"), 0644)

		startLine := 1
		endLine := 3
		req := &models.FileEditRequest{
			ExecutionID: "delete-all-1",
			CaseID:      "test-case-edge",
			Operation:   models.FileEditOperationDelete,
			FilePath:    testFile,
			StartLine:   &startLine,
			EndLine:     &endLine,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
	})

	t.Run("read beyond file end returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "short.txt")
		os.WriteFile(testFile, []byte("Line 1\nLine 2"), 0644)

		startLine := 100
		req := &models.FileEditRequest{
			ExecutionID: "read-beyond-1",
			CaseID:      "test-case-edge",
			Operation:   models.FileEditOperationRead,
			FilePath:    testFile,
			ReadOptions: &models.FileReadOptions{
				StartLine: &startLine,
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, "", *result.Content)
	})

	t.Run("replace with multiline content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "multiline.txt")
		os.WriteFile(testFile, []byte("Line 1\nOld Block\nOld Content\nLine 4"), 0644)

		oldContent := "Old Block\nOld Content"
		newContent := "New Block\nNew Content\nExtra Line"
		req := &models.FileEditRequest{
			ExecutionID: "replace-multi-1",
			CaseID:      "test-case-edge",
			Operation:   models.FileEditOperationReplace,
			FilePath:    testFile,
			OldContent:  &oldContent,
			NewContent:  &newContent,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		data, _ := os.ReadFile(testFile)
		assert.Contains(t, string(data), "New Block")
		assert.Contains(t, string(data), "Extra Line")
		assert.NotContains(t, string(data), "Old Block")
	})

	t.Run("file with special characters", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "special.txt")

		content := "Special: ñ, é, ü, 日本語, 🚀, <xml>, {\"json\": true}"
		req := &models.FileEditRequest{
			ExecutionID:     "special-chars-1",
			CaseID:          "test-case-edge",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &content,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		data, _ := os.ReadFile(testFile)
		assert.Equal(t, content, string(data))
	})

	t.Run("write to deeply nested directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedPath := filepath.Join(tmpDir, "a", "b", "c", "d", "e", "test.txt")
		content := "nested content"

		req := &models.FileEditRequest{
			ExecutionID:     "nested-write-1",
			CaseID:          "test-case-edge",
			Operation:       models.FileEditOperationWrite,
			FilePath:        nestedPath,
			Content:         &content,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.FileExists(t, nestedPath)
	})
}

// TestFileEditService_PermissionsAndStats tests file stats collection and permission handling
func TestFileEditService_PermissionsAndStats(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("read with stats collection", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "stats.txt")
		content := "Line 1\nLine 2\nLine 3"
		os.WriteFile(testFile, []byte(content), 0644)

		includeStats := true
		req := &models.FileEditRequest{
			ExecutionID: "stats-read-1",
			CaseID:      "test-case-stats",
			Operation:   models.FileEditOperationRead,
			FilePath:    testFile,
			ReadOptions: &models.FileReadOptions{
				IncludeStats: includeStats,
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.FileStats)
		assert.Greater(t, result.FileStats.Size, int64(0))
		assert.NotEmpty(t, result.FileStats.Mode)
		assert.Equal(t, 3, result.FileStats.Lines)
		assert.NotNil(t, result.FileStats.ModTime)
	})

	t.Run("handle read-only file write failure", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create a regular file, then target a path that treats it as a directory.
		// Writing to a path whose parent component is a file (not a dir) is
		// structurally impossible — os.MkdirAll will fail even as root.
		barrier := filepath.Join(tmpDir, "barrier")
		require.NoError(t, os.WriteFile(barrier, []byte("not-a-dir"), 0644))
		testFile := filepath.Join(barrier, "target.txt")

		newContent := "updated"
		req := &models.FileEditRequest{
			ExecutionID:     "readonly-write-1",
			CaseID:          "test-case-perms",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &newContent,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
	})

	t.Run("large file operation", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "large.txt")

		// Create 10MB file
		var lines []string
		for i := 0; i < 50000; i++ {
			lines = append(lines, fmt.Sprintf("Line %d with content to increase size", i))
		}
		content := strings.Join(lines, "\n")

		req := &models.FileEditRequest{
			ExecutionID:     "large-file-1",
			CaseID:          "test-case-large",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &content,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Greater(t, *result.BytesWritten, int64(500000))
	})
}
