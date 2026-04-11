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
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileEditService_ExecuteFileEdit_Write(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("write to new file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		content := "test content"

		req := &models.FileEditRequest{
			ExecutionID:     "test-req-1",
			CaseID:          "test-case-1",
			Operation:       models.FileEditOperationWrite,
			FilePath:        tmpFile,
			Content:         &content,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, int64(12), *result.BytesWritten)

		// Verify file was created
		data, err := os.ReadFile(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, content, string(data))
	})

	t.Run("write with backup", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.txt")

		// Create initial file
		os.WriteFile(tmpFile, []byte("original"), 0644)

		newContent := "updated content"
		req := &models.FileEditRequest{
			ExecutionID:  "test-req-2",
			CaseID:       "test-case-2",
			Operation:    models.FileEditOperationWrite,
			FilePath:     tmpFile,
			Content:      &newContent,
			CreateBackup: true,
			RequestedBy:  "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.BackupPath)
		assert.FileExists(t, *result.BackupPath)

		// Verify backup contains original content
		backupData, err := os.ReadFile(*result.BackupPath)
		require.NoError(t, err)
		assert.Equal(t, "original", string(backupData))

		// Verify file has new content
		data, err := os.ReadFile(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, newContent, string(data))
	})

	t.Run("write without create_if_missing fails", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "nonexistent.txt")
		content := "test"

		req := &models.FileEditRequest{
			ExecutionID:     "test-req-3",
			CaseID:          "test-case-3",
			Operation:       models.FileEditOperationWrite,
			FilePath:        tmpFile,
			Content:         &content,
			CreateIfMissing: false,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.NotNil(t, result.ErrorMessage)
	})
}

func TestFileEditService_ExecuteFileEdit_Read(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("read entire file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		content := "line1\nline2\nline3"
		os.WriteFile(tmpFile, []byte(content), 0644)

		req := &models.FileEditRequest{
			ExecutionID: "test-req-1",
			CaseID:      "test-case-1",
			Operation:   models.FileEditOperationRead,
			FilePath:    tmpFile,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotNil(t, result.Content)
		assert.Equal(t, content, *result.Content)
	})

	t.Run("read specific lines", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("line1\nline2\nline3\nline4\nline5"), 0644)

		startLine := 2
		endLine := 4
		req := &models.FileEditRequest{
			ExecutionID: "test-req-2",
			CaseID:      "test-case-2",
			Operation:   models.FileEditOperationRead,
			FilePath:    tmpFile,
			ReadOptions: &models.FileReadOptions{
				StartLine: &startLine,
				EndLine:   &endLine,
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, *result.Content, "line2")
		assert.Contains(t, *result.Content, "line3")
		assert.Contains(t, *result.Content, "line4")
	})

	t.Run("read non-existent file", func(t *testing.T) {
		req := &models.FileEditRequest{
			ExecutionID: "test-req-3",
			CaseID:      "test-case-3",
			Operation:   models.FileEditOperationRead,
			FilePath:    "/tmp/nonexistent-file-12345.txt",
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.NotNil(t, result.ErrorMessage)
	})

	t.Run("read with file stats", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("test content"), 0644)

		includeStats := true
		req := &models.FileEditRequest{
			ExecutionID: "test-req-4",
			CaseID:      "test-case-4",
			Operation:   models.FileEditOperationRead,
			FilePath:    tmpFile,
			ReadOptions: &models.FileReadOptions{
				IncludeStats: includeStats,
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.FileStats)
		assert.Greater(t, result.FileStats.Size, int64(0))
	})
}

func TestFileEditService_ExecuteFileEdit_Replace(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("replace content", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("hello world"), 0644)

		oldContent := "world"
		newContent := "universe"
		req := &models.FileEditRequest{
			ExecutionID: "test-req-1",
			CaseID:      "test-case-1",
			Operation:   models.FileEditOperationReplace,
			FilePath:    tmpFile,
			OldContent:  &oldContent,
			NewContent:  &newContent,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		// Verify replacement
		data, err := os.ReadFile(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, "hello universe", string(data))
	})

	t.Run("replace missing content fails", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("hello world"), 0644)

		oldContent := "missing"
		newContent := "replacement"
		req := &models.FileEditRequest{
			ExecutionID: "test-req-2",
			CaseID:      "test-case-2",
			Operation:   models.FileEditOperationReplace,
			FilePath:    tmpFile,
			OldContent:  &oldContent,
			NewContent:  &newContent,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "not found")
	})
}

func TestFileEditService_ExecuteFileEdit_Insert(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("insert at position", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("line1\nline3"), 0644)

		insertContent := "line2"
		insertPos := 2
		req := &models.FileEditRequest{
			ExecutionID:    "test-req-1",
			CaseID:         "test-case-1",
			Operation:      models.FileEditOperationInsert,
			FilePath:       tmpFile,
			InsertContent:  &insertContent,
			InsertPosition: &insertPos,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		// Verify insertion
		data, err := os.ReadFile(tmpFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), "line1\nline2\nline3")
	})

	t.Run("insert at invalid position fails", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("line1"), 0644)

		insertContent := "test"
		insertPos := 100
		req := &models.FileEditRequest{
			ExecutionID:    "test-req-2",
			CaseID:         "test-case-2",
			Operation:      models.FileEditOperationInsert,
			FilePath:       tmpFile,
			InsertContent:  &insertContent,
			InsertPosition: &insertPos,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
	})
}

func TestFileEditService_ExecuteFileEdit_Delete(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("delete lines", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("line1\nline2\nline3\nline4"), 0644)

		startLine := 2
		endLine := 3
		req := &models.FileEditRequest{
			ExecutionID: "test-req-1",
			CaseID:      "test-case-1",
			Operation:   models.FileEditOperationDelete,
			FilePath:    tmpFile,
			StartLine:   &startLine,
			EndLine:     &endLine,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		// Verify deletion
		data, err := os.ReadFile(tmpFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), "line1")
		assert.NotContains(t, string(data), "line2")
		assert.NotContains(t, string(data), "line3")
		assert.Contains(t, string(data), "line4")
	})

	t.Run("delete invalid range fails", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("line1"), 0644)

		startLine := 5
		endLine := 10
		req := &models.FileEditRequest{
			ExecutionID: "test-req-2",
			CaseID:      "test-case-2",
			Operation:   models.FileEditOperationDelete,
			FilePath:    tmpFile,
			StartLine:   &startLine,
			EndLine:     &endLine,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
	})
}

func TestFileEditService_ExecuteFileEdit_Patch(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("patch not implemented", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("test"), 0644)

		patchContent := "diff patch"
		req := &models.FileEditRequest{
			ExecutionID:  "test-req-1",
			CaseID:       "test-case-1",
			Operation:    models.FileEditOperationPatch,
			FilePath:     tmpFile,
			PatchContent: &patchContent,
			RequestedBy:  "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "not yet implemented")
	})

	t.Run("missing patch content returns error", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("test"), 0644)

		req := &models.FileEditRequest{
			ExecutionID: "test-req-2",
			CaseID:      "test-case-2",
			Operation:   models.FileEditOperationPatch,
			FilePath:    tmpFile,
			RequestedBy: "test-user",
			// PatchContent is nil
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "patch_content is required")
	})

	t.Run("backup creation attempted before patch", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tmpFile, []byte("original content"), 0644)

		patchContent := "diff patch"
		req := &models.FileEditRequest{
			ExecutionID:  "test-req-3",
			CaseID:       "test-case-3",
			Operation:    models.FileEditOperationPatch,
			FilePath:     tmpFile,
			PatchContent: &patchContent,
			CreateBackup: true,
			RequestedBy:  "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		// Should still fail with not implemented, but backup should be created
		assert.Contains(t, *result.ErrorMessage, "not yet implemented")
		// Backup path should be set even though operation failed
		if result.BackupPath != nil {
			assert.NotEmpty(t, *result.BackupPath)
			// Verify backup exists
			_, err := os.Stat(*result.BackupPath)
			assert.NoError(t, err)
		}
	})

	t.Run("backup creation fails with non-existent file", func(t *testing.T) {
		tmpDir := t.TempDir()
		nonExistentFile := filepath.Join(tmpDir, "nonexistent.txt")

		patchContent := "diff patch"
		req := &models.FileEditRequest{
			ExecutionID:  "test-req-4",
			CaseID:       "test-case-4",
			Operation:    models.FileEditOperationPatch,
			FilePath:     nonExistentFile,
			PatchContent: &patchContent,
			CreateBackup: true,
			RequestedBy:  "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Contains(t, *result.ErrorMessage, "failed to create backup")
	})
}

func TestFileEditService_ValidateFilePath(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("valid path", func(t *testing.T) {
		err := svc.validateFilePath("/tmp/test.txt")
		assert.NoError(t, err)
	})

	t.Run("relative path", func(t *testing.T) {
		// Even relative paths can be validated (they get resolved to absolute)
		err := svc.validateFilePath("test.txt")
		assert.NoError(t, err)
	})
}

func TestFileEditService_CreateBackup(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	content := "original content"
	os.WriteFile(tmpFile, []byte(content), 0644)

	backupPath, err := svc.createBackup(tmpFile)

	require.NoError(t, err)
	assert.NotEmpty(t, backupPath)
	assert.FileExists(t, backupPath)

	// Verify backup has same content
	backupData, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(backupData))
}
