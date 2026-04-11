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
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for ExecutionService
func TestExecutionService_Integration(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	svc := NewExecutionService(cfg, logger)

	t.Run("execute and publish result", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "exec-1",
			CaseID:         "test-case-1",
			Command:        "echo",
			Args:           []string{"hello", "from", "operator"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "hello from operator")

	})

	t.Run("concurrent command execution", func(t *testing.T) {
		var wg sync.WaitGroup
		numCommands := 5
		results := make(chan *models.ExecutionResultsPayload, numCommands)

		for i := 0; i < numCommands; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				req := &models.ExecutionRequestPayload{
					ExecutionID:    fmt.Sprintf("concurrent-exec-%d", idx),
					CaseID:         "test-case-concurrent",
					Command:        "echo",
					Args:           []string{fmt.Sprintf("message-%d", idx)},
					TimeoutSeconds: 5,
					RequestedBy:    "test-user",
					APIKey:         "test-key",
				}

				result, err := svc.ExecuteCommand(context.Background(), req)
				if err == nil && result != nil {
					results <- result
				}
			}(i)
		}

		wg.Wait()
		close(results)

		// Verify all results
		count := 0
		for result := range results {
			assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
			count++
		}
		assert.Equal(t, numCommands, count)
	})

	t.Run("complex command with file I/O", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test-output.txt")

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "complex-exec-1",
			CaseID:         "test-case-complex",
			Command:        "sh",
			Args:           []string{"-c", fmt.Sprintf("echo 'Line 1' > %s && echo 'Line 2' >> %s && cat %s", testFile, testFile, testFile)},
			TimeoutSeconds: 10,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "Line 1")
		assert.Contains(t, result.Stdout, "Line 2")

		// Verify file was created
		assert.FileExists(t, testFile)
	})

	t.Run("command with working directory and environment", func(t *testing.T) {
		tmpDir := t.TempDir()

		req := &models.ExecutionRequestPayload{
			ExecutionID:      "env-exec-1",
			CaseID:           "test-case-env",
			Command:          "sh",
			Args:             []string{"-c", "pwd && echo $CUSTOM_VAR"},
			TimeoutSeconds:   5,
			WorkingDirectory: &tmpDir,
			Environment: map[string]string{
				"CUSTOM_VAR": "custom_value_123",
			},
			RequestedBy: "test-user",
			APIKey:      "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, tmpDir)
		assert.Contains(t, result.Stdout, "custom_value_123")

	})

	t.Run("pipeline commands with multiple steps", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Step 1: Create a file
		step1Req := &models.ExecutionRequestPayload{
			ExecutionID:    "pipeline-exec-1",
			CaseID:         "test-case-pipeline",
			Command:        "sh",
			Args:           []string{"-c", fmt.Sprintf("echo 'Initial content' > %s/data.txt", tmpDir)},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result1, err := svc.ExecuteCommand(context.Background(), step1Req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result1.Status)

		// Step 2: Append to file
		step2Req := &models.ExecutionRequestPayload{
			ExecutionID:    "pipeline-exec-2",
			CaseID:         "test-case-pipeline",
			Command:        "sh",
			Args:           []string{"-c", fmt.Sprintf("echo 'Appended content' >> %s/data.txt", tmpDir)},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result2, err := svc.ExecuteCommand(context.Background(), step2Req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result2.Status)

		// Step 3: Read and verify
		step3Req := &models.ExecutionRequestPayload{
			ExecutionID:    "pipeline-exec-3",
			CaseID:         "test-case-pipeline",
			Command:        "cat",
			Args:           []string{fmt.Sprintf("%s/data.txt", tmpDir)},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result3, err := svc.ExecuteCommand(context.Background(), step3Req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result3.Status)
		assert.Contains(t, result3.Stdout, "Initial content")
		assert.Contains(t, result3.Stdout, "Appended content")

	})

	t.Run("command timeout", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "timeout-exec-1",
			CaseID:         "test-case-timeout",
			Command:        "sleep",
			Args:           []string{"30"},
			TimeoutSeconds: 1,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusTimeout, result.Status)
		assert.NotNil(t, result.ErrorMessage)

	})

	t.Run("command with large output", func(t *testing.T) {
		// Generate large output (100 lines)
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "large-output-exec-1",
			CaseID:         "test-case-large",
			Command:        "sh",
			Args:           []string{"-c", "for i in $(seq 1 100); do echo \"Line $i: $(date)\"; done"},
			TimeoutSeconds: 10,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotEmpty(t, result.Stdout)
		assert.NotNil(t, result.TerminalOutput)

		// Terminal output should be truncated to last 50 lines
		if result.TerminalOutput.TruncatedStdout {
			assert.Equal(t, 50, len(result.TerminalOutput.LastLines))
			assert.Equal(t, 100, result.TerminalOutput.OriginalStdoutLines)
		}

	})

	t.Run("command cancellation with cleanup", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "cancel-exec-1",
			CaseID:         "test-case-cancel",
			Command:        "sleep",
			Args:           []string{"60"},
			TimeoutSeconds: 120,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		// Start execution in background
		done := make(chan *models.ExecutionResultsPayload)
		go func() {
			result, _ := svc.ExecuteCommand(context.Background(), req)
			done <- result
		}()

		// Wait for execution to start
		time.Sleep(200 * time.Millisecond)

		// Verify it's in active executions
		active := svc.GetActiveExecutions()
		_, exists := active[req.ExecutionID]
		assert.True(t, exists)

		// Cancel execution
		err := svc.CancelExecution(req.ExecutionID)
		require.NoError(t, err)

		// Wait for completion
		result := <-done
		assert.NotNil(t, result)

		// Verify no longer in active executions
		active = svc.GetActiveExecutions()
		_, exists = active[req.ExecutionID]
		assert.False(t, exists)
	})

	t.Run("command with stderr output", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "stderr-exec-1",
			CaseID:         "test-case-stderr",
			Command:        "sh",
			Args:           []string{"-c", "echo 'stdout message' && echo 'stderr message' >&2"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "stdout message")
		assert.Contains(t, result.Stderr, "stderr message")

	})

	t.Run("system info collection accuracy", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "sysinfo-exec-1",
			CaseID:         "test-case-sysinfo",
			Command:        "echo",
			Args:           []string{"test"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.SystemInfo)

		// Verify system info fields
		assert.NotEmpty(t, result.SystemInfo.Hostname)
		assert.NotEmpty(t, result.SystemInfo.OS)
		assert.NotEmpty(t, result.SystemInfo.Architecture)
		assert.Greater(t, result.SystemInfo.NumCPU, 0)

		// Verify environment info
		assert.NotNil(t, result.EnvironmentInfo)
	})

	t.Run("multiple commands in quick succession", func(t *testing.T) {
		const numCommands = 10
		requestIDs := make([]string, numCommands)

		for i := 0; i < numCommands; i++ {
			requestIDs[i] = fmt.Sprintf("queue-exec-%d", i)
		}

		// Process commands
		results := make([]*models.ExecutionResultsPayload, 0, numCommands)
		for i := 0; i < numCommands; i++ {
			req := &models.ExecutionRequestPayload{
				ExecutionID:    requestIDs[i],
				CaseID:         "test-case-queue",
				Command:        "echo",
				Args:           []string{fmt.Sprintf("message-%d", i)},
				TimeoutSeconds: 5,
				RequestedBy:    "test-user",
				APIKey:         "test-key",
			}

			result, err := svc.ExecuteCommand(context.Background(), req)
			require.NoError(t, err)
			results = append(results, result)
		}

		// Verify all results
		assert.Len(t, results, numCommands)
		for _, result := range results {
			assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		}
	})
}

func TestExecutionService_AdvancedScenarios(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("script execution with multiple commands", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptPath := filepath.Join(tmpDir, "test-script.sh")

		scriptContent := `#!/bin/bash
echo "Starting script"
sleep 0.1
echo "Middle of script"
sleep 0.1
echo "Ending script"
exit 0
`
		err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
		require.NoError(t, err)

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "script-exec-1",
			CaseID:         "test-case-script",
			Command:        scriptPath,
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "Starting script")
		assert.Contains(t, result.Stdout, "Middle of script")
		assert.Contains(t, result.Stdout, "Ending script")

	})

	t.Run("command with specific return codes", func(t *testing.T) {
		testCases := []struct {
			name           string
			exitCode       int
			expectedStatus constants.ExecutionStatus
		}{
			{"exit 0", 0, constants.ExecutionStatusCompleted},
			{"exit 1", 1, constants.ExecutionStatusCompleted},
			{"exit 42", 42, constants.ExecutionStatusCompleted},
			{"exit 127", 127, constants.ExecutionStatusCompleted},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := &models.ExecutionRequestPayload{
					ExecutionID:    fmt.Sprintf("exitcode-exec-%d", tc.exitCode),
					CaseID:         "test-case-exitcode",
					Command:        "sh",
					Args:           []string{"-c", fmt.Sprintf("exit %d", tc.exitCode)},
					TimeoutSeconds: 5,
					RequestedBy:    "test-user",
					APIKey:         "test-key",
				}

				result, err := svc.ExecuteCommand(context.Background(), req)

				require.NoError(t, err)
				assert.Equal(t, tc.expectedStatus, result.Status)
				assert.Equal(t, tc.exitCode, *result.ReturnCode)

			})
		}
	})

	t.Run("resource-intensive command tracking", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a command that does some work
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "resource-exec-1",
			CaseID:         "test-case-resource",
			Command:        "sh",
			Args:           []string{"-c", fmt.Sprintf("for i in $(seq 1 50); do echo $i >> %s/output.txt; done && wc -l %s/output.txt", tmpDir, tmpDir)},
			TimeoutSeconds: 10,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "50")

	})
}
