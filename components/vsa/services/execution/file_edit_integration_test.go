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
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for FileEditService
func TestFileEditService_Integration(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("write and track file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test-file.txt")
		content := "This is test content\nLine 2\nLine 3"

		req := &models.FileEditRequest{
			ExecutionID:     "file-write-1",
			CaseID:          "test-case-1",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &content,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.FileExists(t, testFile)

		// Verify content
		data, _ := os.ReadFile(testFile)
		assert.Equal(t, content, string(data))

	})

	t.Run("read file content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "read-test.txt")
		content := "Content to read\nSecond line\nThird line"
		os.WriteFile(testFile, []byte(content), 0644)

		includeStats := true
		req := &models.FileEditRequest{
			ExecutionID: "file-read-1",
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
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotNil(t, result.Content)
		assert.Equal(t, content, *result.Content)
		assert.NotNil(t, result.FileStats)
		assert.Greater(t, result.FileStats.Size, int64(0))

	})

	t.Run("write with backup", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "backup-test.txt")

		// Create initial file
		initialContent := "Original content"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		newContent := "Updated content after backup"
		req := &models.FileEditRequest{
			ExecutionID:  "file-backup-1",
			CaseID:       "test-case-backup",
			Operation:    models.FileEditOperationWrite,
			FilePath:     testFile,
			Content:      &newContent,
			CreateBackup: true,
			RequestedBy:  "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotNil(t, result.BackupPath)
		assert.FileExists(t, *result.BackupPath)

		// Verify backup contains original
		backupData, _ := os.ReadFile(*result.BackupPath)
		assert.Equal(t, initialContent, string(backupData))

		// Verify file has new content
		fileData, _ := os.ReadFile(testFile)
		assert.Equal(t, newContent, string(fileData))

	})

	t.Run("replace content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "replace-test.txt")

		initialContent := "Hello world\nThis is a test\nGoodbye world"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		oldContent := "world"
		newContent := "universe"
		req := &models.FileEditRequest{
			ExecutionID: "file-replace-1",
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

		// Verify replacement
		fileData, _ := os.ReadFile(testFile)
		assert.Contains(t, string(fileData), "Hello universe")
		assert.Contains(t, string(fileData), "Goodbye universe")
		assert.NotContains(t, string(fileData), "world")

	})

	t.Run("insert lines", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "insert-test.txt")

		initialContent := "Line 1\nLine 3\nLine 4"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		insertContent := "Line 2"
		insertPos := 2
		req := &models.FileEditRequest{
			ExecutionID:    "file-insert-1",
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

		// Verify insertion
		fileData, _ := os.ReadFile(testFile)
		lines := strings.Split(string(fileData), "\n")
		assert.Contains(t, lines, "Line 1")
		assert.Contains(t, lines, "Line 2")
		assert.Contains(t, lines, "Line 3")

	})

	t.Run("delete lines", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "delete-test.txt")

		initialContent := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		startLine := 2
		endLine := 4
		req := &models.FileEditRequest{
			ExecutionID: "file-delete-1",
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

		// Verify deletion
		fileData, _ := os.ReadFile(testFile)
		assert.Contains(t, string(fileData), "Line 1")
		assert.NotContains(t, string(fileData), "Line 2")
		assert.NotContains(t, string(fileData), "Line 3")
		assert.NotContains(t, string(fileData), "Line 4")
		assert.Contains(t, string(fileData), "Line 5")

	})

	t.Run("concurrent file edits", func(t *testing.T) {
		tmpDir := t.TempDir()
		var wg sync.WaitGroup
		numEdits := 5

		for i := 0; i < numEdits; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				testFile := filepath.Join(tmpDir, fmt.Sprintf("concurrent-%d.txt", idx))
				content := fmt.Sprintf("Content for file %d", idx)

				req := &models.FileEditRequest{
					ExecutionID:     fmt.Sprintf("concurrent-write-%d", idx),
					CaseID:          "test-case-concurrent",
					Operation:       models.FileEditOperationWrite,
					FilePath:        testFile,
					Content:         &content,
					CreateIfMissing: true,
					RequestedBy:     "test-user",
				}

				_, _ = svc.ExecuteFileEdit(context.Background(), req)
			}(i)
		}

		wg.Wait()

		// Verify all files created
		for i := 0; i < numEdits; i++ {
			testFile := filepath.Join(tmpDir, fmt.Sprintf("concurrent-%d.txt", i))
			assert.FileExists(t, testFile)
		}
	})

	t.Run("read specific line range with caching", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "range-test.txt")

		var lines []string
		for i := 1; i <= 50; i++ {
			lines = append(lines, fmt.Sprintf("Line %d", i))
		}
		content := strings.Join(lines, "\n")
		os.WriteFile(testFile, []byte(content), 0644)

		startLine := 10
		endLine := 20
		req := &models.FileEditRequest{
			ExecutionID: "file-range-read-1",
			CaseID:      "test-case-range",
			Operation:   models.FileEditOperationRead,
			FilePath:    testFile,
			ReadOptions: &models.FileReadOptions{
				StartLine: &startLine,
				EndLine:   &endLine,
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotNil(t, result.Content)
		assert.Contains(t, *result.Content, "Line 10")
		assert.Contains(t, *result.Content, "Line 20")
		assert.NotContains(t, *result.Content, "Line 9")
		assert.NotContains(t, *result.Content, "Line 21")

	})

	t.Run("large file operations", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "large-file.txt")

		// Create large content (1000 lines)
		var lines []string
		for i := 1; i <= 1000; i++ {
			lines = append(lines, fmt.Sprintf("Line %d: %s", i, strings.Repeat("x", 50)))
		}
		content := strings.Join(lines, "\n")

		req := &models.FileEditRequest{
			ExecutionID:     "file-large-write-1",
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
		assert.FileExists(t, testFile)
		assert.Greater(t, *result.BytesWritten, int64(50000))
	})

	t.Run("file edit pipeline", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "pipeline.txt")
		pipelineID := "pipeline-1"

		// Step 1: Create file
		content1 := "Initial content"
		req1 := &models.FileEditRequest{
			ExecutionID:     fmt.Sprintf("%s-step1", pipelineID),
			CaseID:          "test-case-pipeline",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &content1,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result1, err := svc.ExecuteFileEdit(context.Background(), req1)
		require.NoError(t, err)

		// Step 2: Replace content
		oldContent := "Initial"
		newContent := "Modified"
		req2 := &models.FileEditRequest{
			ExecutionID: fmt.Sprintf("%s-step2", pipelineID),
			CaseID:      "test-case-pipeline",
			Operation:   models.FileEditOperationReplace,
			FilePath:    testFile,
			OldContent:  &oldContent,
			NewContent:  &newContent,
			RequestedBy: "test-user",
		}

		result2, err := svc.ExecuteFileEdit(context.Background(), req2)
		require.NoError(t, err)

		// Step 3: Insert content
		insertContent := "Added line"
		insertPos := 2
		req3 := &models.FileEditRequest{
			ExecutionID:    fmt.Sprintf("%s-step3", pipelineID),
			CaseID:         "test-case-pipeline",
			Operation:      models.FileEditOperationInsert,
			FilePath:       testFile,
			InsertContent:  &insertContent,
			InsertPosition: &insertPos,
			RequestedBy:    "test-user",
		}

		result3, err := svc.ExecuteFileEdit(context.Background(), req3)
		require.NoError(t, err)

		// Verify final state
		fileData, _ := os.ReadFile(testFile)
		assert.Contains(t, string(fileData), "Modified")
		assert.Contains(t, string(fileData), "Added line")

		_ = result1
		_ = result2
		_ = result3
	})

	t.Run("error handling", func(t *testing.T) {
		// Test 1: Non-existent file without create flag
		tmpDir := t.TempDir()
		nonExistent := filepath.Join(tmpDir, "does-not-exist.txt")
		content := "test"

		req := &models.FileEditRequest{
			ExecutionID:     "file-error-1",
			CaseID:          "test-case-error",
			Operation:       models.FileEditOperationWrite,
			FilePath:        nonExistent,
			Content:         &content,
			CreateIfMissing: false,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.NotNil(t, result.ErrorMessage)

	})

	t.Run("file with special characters", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "special-chars.txt")

		content := "Special: ñ, é, ü, 日本語, 🚀, <xml>, {\"json\": true}, 'quotes'"

		req := &models.FileEditRequest{
			ExecutionID:     "file-special-1",
			CaseID:          "test-case-special",
			Operation:       models.FileEditOperationWrite,
			FilePath:        testFile,
			Content:         &content,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		// Verify special characters preserved
		fileData, _ := os.ReadFile(testFile)
		assert.Equal(t, content, string(fileData))

	})
}

func TestFileEditService_AdvancedScenarios(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewFileEditService(cfg, logger)

	t.Run("multiple backups", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "multi-backup.txt")

		backupPaths := make([]string, 0)

		// Create initial file and make multiple edits with backups
		for i := 0; i < 3; i++ {
			content := fmt.Sprintf("Version %d content", i)

			// Create file if first iteration
			if i == 0 {
				os.WriteFile(testFile, []byte(content), 0644)
				continue
			}

			req := &models.FileEditRequest{
				ExecutionID:  fmt.Sprintf("backup-multi-%d", i),
				CaseID:       "test-case-multi-backup",
				Operation:    models.FileEditOperationWrite,
				FilePath:     testFile,
				Content:      &content,
				CreateBackup: true,
				RequestedBy:  "test-user",
			}

			result, err := svc.ExecuteFileEdit(context.Background(), req)
			require.NoError(t, err)
			assert.NotNil(t, result.BackupPath)
			backupPaths = append(backupPaths, *result.BackupPath)
		}

		// Verify all backups exist
		for _, backupPath := range backupPaths {
			assert.FileExists(t, backupPath)
		}
	})

	t.Run("read with max lines limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "maxlines-test.txt")

		// Create file with 100 lines
		var lines []string
		for i := 1; i <= 100; i++ {
			lines = append(lines, fmt.Sprintf("Line %d", i))
		}
		os.WriteFile(testFile, []byte(strings.Join(lines, "\n")), 0644)

		maxLines := 10
		req := &models.FileEditRequest{
			ExecutionID: "file-maxlines-1",
			CaseID:      "test-case-maxlines",
			Operation:   models.FileEditOperationRead,
			FilePath:    testFile,
			ReadOptions: &models.FileReadOptions{
				MaxLines: &maxLines,
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		readLines := strings.Split(*result.Content, "\n")
		assert.LessOrEqual(t, len(readLines), maxLines)
	})

	t.Run("complex replace with multiple occurrences", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "multi-replace.txt")

		content := "foo bar foo baz foo qux"
		os.WriteFile(testFile, []byte(content), 0644)

		oldContent := "foo"
		newContent := "FOO"
		req := &models.FileEditRequest{
			ExecutionID: "file-multi-replace-1",
			CaseID:      "test-case-multi-replace",
			Operation:   models.FileEditOperationReplace,
			FilePath:    testFile,
			OldContent:  &oldContent,
			NewContent:  &newContent,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		// Verify all occurrences replaced
		fileData, _ := os.ReadFile(testFile)
		assert.Equal(t, "FOO bar FOO baz FOO qux", string(fileData))
		assert.NotContains(t, string(fileData), "foo")

	})

	t.Run("file permissions and stats collection", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "perms-test.txt")

		content := "test content"
		os.WriteFile(testFile, []byte(content), 0644)

		includeStats := true
		req := &models.FileEditRequest{
			ExecutionID: "file-perms-1",
			CaseID:      "test-case-perms",
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
		assert.NotEmpty(t, result.FileStats.Mode)
		assert.Greater(t, result.FileStats.Lines, 0)
		assert.NotNil(t, result.FileStats.ModTime)

	})
}
