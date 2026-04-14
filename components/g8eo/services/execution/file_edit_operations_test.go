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

// Comprehensive tests for all file edit operations
func TestFileEditService_ReadOperations(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("read with end line only", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "endonly.txt")

		content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
		os.WriteFile(testFile, []byte(content), 0644)

		endLine := 3
		req := &models.FileEditRequest{
			ExecutionID: "read-endline-1",
			CaseID:      "test-case-read",
			Operation:   models.FileEditOperationRead,
			FilePath:    testFile,
			ReadOptions: &models.FileReadOptions{
				EndLine: &endLine,
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, *result.Content, "Line 1")
		assert.Contains(t, *result.Content, "Line 3")
		assert.NotContains(t, *result.Content, "Line 4")
	})

	t.Run("read with max lines from middle", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "maxlines.txt")

		var lines []string
		for i := 1; i <= 50; i++ {
			lines = append(lines, fmt.Sprintf("Line %d", i))
		}
		os.WriteFile(testFile, []byte(strings.Join(lines, "\n")), 0644)

		startLine := 20
		maxLines := 5
		req := &models.FileEditRequest{
			ExecutionID: "read-maxlines-1",
			CaseID:      "test-case-read",
			Operation:   models.FileEditOperationRead,
			FilePath:    testFile,
			ReadOptions: &models.FileReadOptions{
				StartLine: &startLine,
				MaxLines:  &maxLines,
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		readLines := strings.Split(*result.Content, "\n")
		assert.LessOrEqual(t, len(readLines), maxLines)
		assert.Contains(t, *result.Content, "Line 20")
	})

	t.Run("read with complete file stats", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "stats.txt")

		content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
		os.WriteFile(testFile, []byte(content), 0644)

		includeStats := true
		req := &models.FileEditRequest{
			ExecutionID: "read-stats-1",
			CaseID:      "test-case-read",
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
		assert.Greater(t, result.FileStats.Lines, 0)
		assert.NotNil(t, result.FileStats.ModTime)
	})

	t.Run("read symlink with stats", func(t *testing.T) {

		tmpDir := t.TempDir()
		targetFile := filepath.Join(tmpDir, "target.txt")
		symlinkFile := filepath.Join(tmpDir, "link.txt")

		os.WriteFile(targetFile, []byte("target content"), 0644)
		os.Symlink(targetFile, symlinkFile)

		includeStats := true
		req := &models.FileEditRequest{
			ExecutionID: "read-symlink-1",
			CaseID:      "test-case-read",
			Operation:   models.FileEditOperationRead,
			FilePath:    symlinkFile,
			ReadOptions: &models.FileReadOptions{
				IncludeStats: includeStats,
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.FileStats)

		if result.FileStats.IsSymlink {
			assert.NotNil(t, result.FileStats.SymlinkTarget)
		}
	})
}

func TestFileEditService_WriteOperations(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("write to existing file without create flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "exists.txt")

		os.WriteFile(testFile, []byte("original"), 0644)

		newContent := "updated"
		req := &models.FileEditRequest{
			ExecutionID:     "write-exists-1",
			CaseID:          "test-case-write",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &newContent,
			CreateIfMissing: false,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		data, _ := os.ReadFile(testFile)
		assert.Equal(t, newContent, string(data))
	})

	t.Run("write new file without backup", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "new.txt")

		content := "new content"
		req := &models.FileEditRequest{
			ExecutionID:     "write-new-1",
			CaseID:          "test-case-write",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &content,
			CreateIfMissing: true,
			CreateBackup:    false,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Nil(t, result.BackupPath)
	})

	t.Run("write with nested directory creation", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedPath := filepath.Join(tmpDir, "a", "b", "c", "test.txt")
		content := "nested"

		req := &models.FileEditRequest{
			ExecutionID:     "write-nested-1",
			CaseID:          "test-case-write",
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

func TestFileEditService_ReplaceOperations(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("replace with backup", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "replace.txt")

		original := "original text"
		os.WriteFile(testFile, []byte(original), 0644)

		oldContent := "original"
		newContent := "replaced"
		req := &models.FileEditRequest{
			ExecutionID:  "replace-backup-1",
			CaseID:       "test-case-replace",
			Operation:    models.FileEditOperationReplace,
			FilePath:     testFile,
			OldContent:   &oldContent,
			NewContent:   &newContent,
			CreateBackup: true,
			RequestedBy:  "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotNil(t, result.BackupPath)

		backupData, _ := os.ReadFile(*result.BackupPath)
		assert.Equal(t, original, string(backupData))
	})

	t.Run("replace with empty new content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "replace-empty.txt")

		initialContent := "keep this remove_this keep that"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		oldContent := "remove_this "
		newContent := ""
		req := &models.FileEditRequest{
			ExecutionID: "replace-empty-1",
			CaseID:      "test-case-replace",
			Operation:   models.FileEditOperationReplace,
			FilePath:    testFile,
			OldContent:  &oldContent,
			NewContent:  &newContent,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		fileData, _ := os.ReadFile(testFile)
		assert.Equal(t, "keep this keep that", string(fileData))
	})
}

func TestFileEditService_InsertOperations(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("insert with backup", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "insert.txt")

		original := "Line 1\nLine 3"
		os.WriteFile(testFile, []byte(original), 0644)

		insertContent := "Line 2"
		insertPos := 2
		req := &models.FileEditRequest{
			ExecutionID:    "insert-backup-1",
			CaseID:         "test-case-insert",
			Operation:      models.FileEditOperationInsert,
			FilePath:       testFile,
			InsertContent:  &insertContent,
			InsertPosition: &insertPos,
			CreateBackup:   true,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotNil(t, result.BackupPath)
	})

	t.Run("insert multiline content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "insert-multi.txt")

		initialContent := "Line 1\nLine 4"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		insertContent := "Line 2\nLine 3"
		insertPos := 2
		req := &models.FileEditRequest{
			ExecutionID:    "insert-multi-1",
			CaseID:         "test-case-insert",
			Operation:      models.FileEditOperationInsert,
			FilePath:       testFile,
			InsertContent:  &insertContent,
			InsertPosition: &insertPos,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		fileData, _ := os.ReadFile(testFile)
		assert.Contains(t, string(fileData), "Line 2")
		assert.Contains(t, string(fileData), "Line 3")
	})
}

func TestFileEditService_DeleteOperations(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("delete with backup", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "delete.txt")

		original := "Line 1\nLine 2\nLine 3\nLine 4"
		os.WriteFile(testFile, []byte(original), 0644)

		startLine := 2
		endLine := 3
		req := &models.FileEditRequest{
			ExecutionID:  "delete-backup-1",
			CaseID:       "test-case-delete",
			Operation:    models.FileEditOperationDelete,
			FilePath:     testFile,
			StartLine:    &startLine,
			EndLine:      &endLine,
			CreateBackup: true,
			RequestedBy:  "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotNil(t, result.BackupPath)
	})

	t.Run("delete first line", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "delete-first.txt")

		initialContent := "Line 1\nLine 2\nLine 3"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		startLine := 1
		endLine := 1
		req := &models.FileEditRequest{
			ExecutionID: "delete-first-1",
			CaseID:      "test-case-delete",
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
}

func TestFileEditService_ErrorHandling(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("patch operation not implemented", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "patch.txt")

		os.WriteFile(testFile, []byte("content"), 0644)

		patchContent := "some patch"
		req := &models.FileEditRequest{
			ExecutionID:  "patch-1",
			CaseID:       "test-case-error",
			Operation:    models.FileEditOperationPatch,
			FilePath:     testFile,
			PatchContent: &patchContent,
			RequestedBy:  "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "not yet implemented")
	})

	t.Run("unsupported operation", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "unsupported.txt")

		os.WriteFile(testFile, []byte("content"), 0644)

		req := &models.FileEditRequest{
			ExecutionID: "unsupported-1",
			CaseID:      "test-case-error",
			Operation:   "invalid_operation",
			FilePath:    testFile,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
	})
}
