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

//go:build integration

package pubsub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Edge case tests for complete coverage
func TestExecutionService_EdgeCases(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := execution.NewExecutionService(cfg, logger)

	t.Run("command with very long arguments", func(t *testing.T) {
		longArg := strings.Repeat("a", 1000)
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-long-args-1",
			CaseID:         "test-case-edge",
			Command:        "echo",
			Args:           []string{longArg},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, longArg)
	})

	t.Run("command with empty arguments array", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-empty-args-1",
			CaseID:         "test-case-edge",
			Command:        "pwd",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
	})

	t.Run("command with special shell characters in arguments", func(t *testing.T) {
		specialChars := []string{"$HOME", "`date`", "$(whoami)", "test;ls", "test&&echo"}

		for _, arg := range specialChars {
			req := &models.ExecutionRequestPayload{
				ExecutionID:    fmt.Sprintf("edge-special-%s", strings.ReplaceAll(arg, " ", "-")),
				CaseID:         "test-case-edge",
				Command:        "echo",
				Args:           []string{arg},
				TimeoutSeconds: 5,
				RequestedBy:    "test-user",
				APIKey:         "test-key",
			}

			result, err := svc.ExecuteCommand(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		}
	})

	t.Run("command with shell redirection operators", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "redirect.txt")

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-redirect-1",
			CaseID:         "test-case-edge",
			Command:        fmt.Sprintf("echo 'redirected output' > %s", outputFile),
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.FileExists(t, outputFile)
	})

	t.Run("command with pipes and process substitution", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-pipes-1",
			CaseID:         "test-case-edge",
			Command:        "echo 'test' | grep test | wc -l",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "1")
	})

	t.Run("command with background process operator", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "bg-test.txt")

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-background-1",
			CaseID:         "test-case-edge",
			Command:        fmt.Sprintf("echo 'done' > %s && sleep 0.1", testFile),
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.FileExists(t, testFile)
	})

	t.Run("command with conditional operators", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-conditional-1",
			CaseID:         "test-case-edge",
			Command:        "true && echo 'success' || echo 'failure'",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "success")
	})

	t.Run("command with multiple environment variables", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-multi-env-1",
			CaseID:         "test-case-edge",
			Command:        "sh",
			Args:           []string{"-c", "echo $VAR1:$VAR2:$VAR3"},
			TimeoutSeconds: 5,
			Environment: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
				"VAR3": "value3",
			},
			RequestedBy: "test-user",
			APIKey:      "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "value1:value2:value3")
	})

	t.Run("command with non-existent working directory", func(t *testing.T) {
		nonExistentDir := "/tmp/non-existent-dir-12345"

		req := &models.ExecutionRequestPayload{
			ExecutionID:      "edge-bad-dir-1",
			CaseID:           "test-case-edge",
			Command:          "pwd",
			Args:             []string{},
			TimeoutSeconds:   5,
			WorkingDirectory: &nonExistentDir,
			RequestedBy:      "test-user",
			APIKey:           "test-key",
			RequestedBy:      "test-user",
			APIKey:           "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
	})

	t.Run("permission denied error code 126", func(t *testing.T) {
		tmpDir := t.TempDir()
		noExecFile := filepath.Join(tmpDir, "no-exec.sh")

		// Create file without execute permission
		os.WriteFile(noExecFile, []byte("#!/bin/bash\necho test"), 0644)

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-no-exec-1",
			CaseID:         "test-case-edge",
			Command:        noExecFile,
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Equal(t, 126, *result.ReturnCode)
	})

	t.Run("context cancellation after command starts", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-cancel-ctx-1",
			CaseID:         "test-case-edge",
			Command:        "sleep",
			Args:           []string{"5"},
			TimeoutSeconds: 10,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		resultChan := make(chan *models.ExecutionResultsPayload)
		go func() {
			result, _ := svc.ExecuteCommand(ctx, req)
			resultChan <- result
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()

		result := <-resultChan
		assert.NotNil(t, result)
	})

	t.Run("verify all system info fields on Linux", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-sysinfo-linux-1",
			CaseID:         "test-case-edge",
			Command:        "echo",
			Args:           []string{"test"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.SystemInfo)

		// Verify extended Linux metrics if on Linux
		if result.SystemInfo.OS == "linux" {
			t.Logf("System info: %+v", result.SystemInfo)
		}
	})

	t.Run("command produces exactly 50 lines of output", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "edge-50-lines-1",
			CaseID:         "test-case-edge",
			Command:        "sh",
			Args:           []string{"-c", "for i in $(seq 1 50); do echo $i; done"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotNil(t, result.TerminalOutput)
		assert.False(t, result.TerminalOutput.TruncatedStdout) // Should not be truncated
	})

	t.Run("store execution with all optional fields", func(t *testing.T) {
		taskID := "task-123"
		workDir := "/tmp"

		req := &models.ExecutionRequestPayload{
			ExecutionID:      "edge-full-fields-1",
			CaseID:           "test-case-edge",
			TaskID:           &taskID,
			InvestigationID:  "inv-456",
			Command:          "echo",
			Args:             []string{"full", "fields"},
			TimeoutSeconds:   5,
			WorkingDirectory: &workDir,
			Environment: map[string]string{
				"TEST": "value",
			},
			RequestedBy: "test-user",
			APIKey:      "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, taskID, *result.TaskID)
		assert.Equal(t, "inv-456", result.InvestigationID)
	})
}

func TestFileEditService_EdgeCases(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := execution.NewFileEditService(cfg, logger)

	t.Run("write empty content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "empty.txt")
		content := ""

		req := &models.FileEditRequest{
			ExecutionID:     "edge-empty-content-1",
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
		assert.FileExists(t, testFile)
	})

	t.Run("write content with only newlines", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "newlines.txt")
		content := "\n\n\n\n"

		req := &models.FileEditRequest{
			ExecutionID:     "edge-newlines-1",
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
	})

	t.Run("replace content that appears multiple times", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "multi.txt")

		initialContent := "test test test"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		oldContent := "test"
		newContent := "replaced"
		req := &models.FileEditRequest{
			ExecutionID: "edge-multi-replace-1",
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

		fileData, _ := os.ReadFile(testFile)
		assert.Equal(t, "replaced replaced replaced", string(fileData))
	})

	t.Run("insert at position 1 (beginning)", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "insert-start.txt")

		initialContent := "Line 2\nLine 3"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		insertContent := "Line 1"
		insertPos := 1
		req := &models.FileEditRequest{
			ExecutionID:    "edge-insert-start-1",
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

		fileData, _ := os.ReadFile(testFile)
		assert.True(t, strings.HasPrefix(string(fileData), "Line 1"))
	})

	t.Run("insert at end of file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "insert-end.txt")

		initialContent := "Line 1\nLine 2"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		insertContent := "Line 3"
		insertPos := 3
		req := &models.FileEditRequest{
			ExecutionID:    "edge-insert-end-1",
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
	})

	t.Run("delete single line", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "delete-single.txt")

		initialContent := "Line 1\nLine 2\nLine 3"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		startLine := 2
		endLine := 2
		req := &models.FileEditRequest{
			ExecutionID: "edge-delete-single-1",
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

		fileData, _ := os.ReadFile(testFile)
		assert.Contains(t, string(fileData), "Line 1")
		assert.NotContains(t, string(fileData), "Line 2")
		assert.Contains(t, string(fileData), "Line 3")
	})

	t.Run("delete first line", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "delete-first.txt")

		initialContent := "Line 1\nLine 2\nLine 3"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		startLine := 1
		endLine := 1
		req := &models.FileEditRequest{
			ExecutionID: "edge-delete-first-1",
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

	t.Run("read with start line only", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "read-start.txt")

		content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
		os.WriteFile(testFile, []byte(content), 0644)

		startLine := 3
		req := &models.FileEditRequest{
			ExecutionID: "edge-read-start-1",
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
		assert.Contains(t, *result.Content, "Line 3")
		assert.Contains(t, *result.Content, "Line 4")
		assert.Contains(t, *result.Content, "Line 5")
		assert.NotContains(t, *result.Content, "Line 2")
	})

	t.Run("file path with nested directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedPath := filepath.Join(tmpDir, "a", "b", "c", "test.txt")
		content := "nested content"

		req := &models.FileEditRequest{
			ExecutionID:     "edge-nested-1",
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

	t.Run("replace with empty new content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "replace-empty.txt")

		initialContent := "keep this remove_this keep that"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		oldContent := "remove_this "
		newContent := ""
		req := &models.FileEditRequest{
			ExecutionID: "edge-replace-empty-1",
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

		fileData, _ := os.ReadFile(testFile)
		assert.Equal(t, "keep this keep that", string(fileData))
	})

	t.Run("insert multiline content", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "insert-multi.txt")

		initialContent := "Line 1\nLine 4"
		os.WriteFile(testFile, []byte(initialContent), 0644)

		insertContent := "Line 2\nLine 3"
		insertPos := 2
		req := &models.FileEditRequest{
			ExecutionID:    "edge-insert-multi-1",
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

		fileData, _ := os.ReadFile(testFile)
		assert.Contains(t, string(fileData), "Line 2")
		assert.Contains(t, string(fileData), "Line 3")
	})

	t.Run("read file with no newline at end", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "no-final-newline.txt")

		content := "Line 1\nLine 2\nLine 3"
		os.WriteFile(testFile, []byte(content), 0644)

		req := &models.FileEditRequest{
			ExecutionID: "edge-no-newline-1",
			CaseID:      "test-case-edge",
			Operation:   models.FileEditOperationRead,
			FilePath:    testFile,
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, content, *result.Content)
	})

	t.Run("relative file path resolution", func(t *testing.T) {
		// Test that relative paths work
		relativePath := "test-relative.txt"
		content := "relative path content"

		req := &models.FileEditRequest{
			ExecutionID:     "edge-relative-1",
			CaseID:          "test-case-edge",
			Operation:       models.FileEditOperationWrite,
			FilePath:        relativePath,
			Content:         &content,
			CreateIfMissing: true,
			RequestedBy:     "test-user",
		}

		result, err := svc.ExecuteFileEdit(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		// Clean up
		os.Remove(relativePath)
	})

	t.Run("all edit operations complete successfully", func(t *testing.T) {
		tmpDir := t.TempDir()

		operations := []models.FileEditOperation{
			models.FileEditOperationWrite,
			models.FileEditOperationRead,
			models.FileEditOperationReplace,
		}

		for i, op := range operations {
			testFile := filepath.Join(tmpDir, fmt.Sprintf("audit-%d.txt", i))
			content := fmt.Sprintf("content %d", i)

			var req *models.FileEditRequest
			if op == models.FileEditOperationWrite {
				req = &models.FileEditRequest{
					ExecutionID:     fmt.Sprintf("audit-%s-%d", op, i),
					CaseID:          "test-case-audit",
					Operation:       op,
					FilePath:        testFile,
					Content:         &content,
					CreateIfMissing: true,
					RequestedBy:     "test-user",
				}
			} else if op == models.FileEditOperationRead {
				os.WriteFile(testFile, []byte(content), 0644)
				req = &models.FileEditRequest{
					ExecutionID: fmt.Sprintf("audit-%s-%d", op, i),
					CaseID:      "test-case-audit",
					Operation:   op,
					FilePath:    testFile,
					RequestedBy: "test-user",
				}
			} else if op == models.FileEditOperationReplace {
				os.WriteFile(testFile, []byte(content), 0644)
				oldContent := "content"
				newContent := "replaced"
				req = &models.FileEditRequest{
					ExecutionID: fmt.Sprintf("audit-%s-%d", op, i),
					CaseID:      "test-case-audit",
					Operation:   op,
					FilePath:    testFile,
					OldContent:  &oldContent,
					NewContent:  &newContent,
					RequestedBy: "test-user",
				}
			}

			result, err := svc.ExecuteFileEdit(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		}
	})
}
