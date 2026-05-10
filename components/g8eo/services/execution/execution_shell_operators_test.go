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

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for shell Operator handling in command execution
func TestExecutionService_ShellOperators(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("pipe operator", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "shell-pipe-1",
			CaseID:         "test-case-shell",
			Command:        "echo test | grep test | wc -l",
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

	t.Run("output redirection", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "output.txt")

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "shell-redirect-1",
			CaseID:         "test-case-shell",
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

	t.Run("input redirection", func(t *testing.T) {
		tmpDir := t.TempDir()
		inputFile := filepath.Join(tmpDir, "input.txt")
		os.WriteFile(inputFile, []byte("input content"), 0644)

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "shell-input-1",
			CaseID:         "test-case-shell",
			Command:        fmt.Sprintf("cat < %s", inputFile),
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "input content")
	})

	t.Run("append redirection", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "append.txt")

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "shell-append-1",
			CaseID:         "test-case-shell",
			Command:        fmt.Sprintf("echo 'line1' > %s && echo 'line2' >> %s", outputFile, outputFile),
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)

		data, _ := os.ReadFile(outputFile)
		assert.Contains(t, string(data), "line1")
		assert.Contains(t, string(data), "line2")
	})

	t.Run("logical AND operator", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "shell-and-1",
			CaseID:         "test-case-shell",
			Command:        "true && echo success",
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

	t.Run("logical OR operator", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "shell-or-1",
			CaseID:         "test-case-shell",
			Command:        "false || echo fallback",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "fallback")
	})

	t.Run("semicolon command separator", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "shell-semicolon-1",
			CaseID:         "test-case-shell",
			Command:        "echo first; echo second",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "first")
		assert.Contains(t, result.Stdout, "second")
	})

	t.Run("background operator", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "bg.txt")

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "shell-background-1",
			CaseID:         "test-case-shell",
			Command:        fmt.Sprintf("echo 'done' > %s &", testFile),
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
	})

	t.Run("simple command with spaces no operators", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "shell-simple-spaces-1",
			CaseID:         "test-case-shell",
			Command:        "echo hello world",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "hello world")
	})
}

func TestExecutionService_SystemMetrics(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("Linux extended metrics", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "metrics-linux-1",
			CaseID:         "test-case-metrics",
			Command:        "echo",
			Args:           []string{"test"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.SystemInfo)

		t.Logf("System info: %+v", result.SystemInfo)

		if result.SystemInfo.LoadAverage != nil {
			t.Logf("Load average: %v", result.SystemInfo.LoadAverage)
			assert.Len(t, result.SystemInfo.LoadAverage, 3)
		}
		if result.SystemInfo.Memory != nil {
			t.Logf("Memory info: %+v", result.SystemInfo.Memory)
			assert.Greater(t, result.SystemInfo.Memory.MemTotal, int64(0))
		}
	})

	t.Run("active executions tracking", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "metrics-active-1",
			CaseID:         "test-case-metrics",
			Command:        "sleep",
			Args:           []string{"1"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		done := make(chan bool)
		go func() {
			svc.ExecuteCommand(context.Background(), req)
			done <- true
		}()

		// Give it time to start
		time.Sleep(100 * time.Millisecond)

		// Verify tracking
		active := svc.GetActiveExecutions()
		_, exists := active[req.ExecutionID]
		if exists {
			t.Logf("Found active execution as expected")
		}

		<-done
	})

	t.Run("signal terminated process", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "metrics-signal-1",
			CaseID:         "test-case-metrics",
			Command:        "sh",
			Args:           []string{"-c", "kill -9 $$"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestExecutionService_ConcurrencyStress(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("max concurrent executions", func(t *testing.T) {
		maxConcurrent := cfg.MaxConcurrentTasks
		var wg sync.WaitGroup
		results := make(chan *models.ExecutionResultsPayload, maxConcurrent)

		for i := 0; i < maxConcurrent; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := &models.ExecutionRequestPayload{
					ExecutionID:    fmt.Sprintf("concurrent-max-%d", idx),
					CaseID:         "test-concurrent",
					Command:        "echo",
					Args:           []string{fmt.Sprintf("task-%d", idx)},
					TimeoutSeconds: 5,
					RequestedBy:    "test-user",
					APIKey:         "test-key",
				}

				result, err := svc.ExecuteCommand(context.Background(), req)
				require.NoError(t, err)
				results <- result
			}(i)
		}

		wg.Wait()
		close(results)

		count := 0
		for result := range results {
			assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
			count++
		}
		assert.Equal(t, maxConcurrent, count)
	})

	t.Run("context cancelled while waiting for semaphore", func(t *testing.T) {
		maxConcurrent := cfg.MaxConcurrentTasks
		var wg sync.WaitGroup

		// Fill all semaphore slots
		for i := 0; i < maxConcurrent; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := &models.ExecutionRequestPayload{
					ExecutionID:    fmt.Sprintf("semaphore-fill-%d", idx),
					CaseID:         "test-semaphore",
					Command:        "sleep",
					Args:           []string{"2"},
					TimeoutSeconds: 5,
					RequestedBy:    "test-user",
					APIKey:         "test-key",
				}
				svc.ExecuteCommand(context.Background(), req)
			}(i)
		}

		time.Sleep(200 * time.Millisecond)

		// Try with cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "semaphore-cancelled",
			CaseID:         "test-semaphore",
			Command:        "echo",
			Args:           []string{"blocked"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "cancelled")

		wg.Wait()
	})

	t.Run("concurrent active executions tracking", func(t *testing.T) {
		var wg sync.WaitGroup
		numTasks := 8

		for i := 0; i < numTasks; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := &models.ExecutionRequestPayload{
					ExecutionID:    fmt.Sprintf("tracking-%d", idx),
					CaseID:         "test-tracking",
					Command:        "sleep",
					Args:           []string{"0.2"},
					TimeoutSeconds: 5,
					RequestedBy:    "test-user",
					APIKey:         "test-key",
				}
				svc.ExecuteCommand(context.Background(), req)
			}(i)
		}

		time.Sleep(100 * time.Millisecond)
		active := svc.GetActiveExecutions()
		assert.NotEmpty(t, active)

		wg.Wait()
		time.Sleep(100 * time.Millisecond)
		active = svc.GetActiveExecutions()
		assert.Empty(t, active)
	})
}

func TestExecutionService_ErrorPaths(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("empty command string", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "error-empty-1",
			CaseID:         "test-error",
			Command:        "",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
	})

	t.Run("whitespace only command", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "error-whitespace-1",
			CaseID:         "test-error",
			Command:        "   ",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
	})

	t.Run("nonexistent working directory", func(t *testing.T) {
		badDir := "/nonexistent/directory/path"
		req := &models.ExecutionRequestPayload{
			ExecutionID:      "error-baddir-1",
			CaseID:           "test-error",
			Command:          "echo",
			Args:             []string{"test"},
			TimeoutSeconds:   5,
			WorkingDirectory: &badDir,
			RequestedBy:      "test-user",
			APIKey:           "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
	})

	t.Run("permission denied exit code 126", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptPath := filepath.Join(tmpDir, "no-exec.sh")

		err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho test"), 0644)
		require.NoError(t, err)

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "error-perm-1",
			CaseID:         "test-error",
			Command:        scriptPath,
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

	t.Run("command not found exit code 127", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "error-notfound-1",
			CaseID:         "test-error",
			Command:        "/nonexistent/command/path",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Equal(t, 127, *result.ReturnCode)
		assert.NotNil(t, result.ErrorMessage)
	})

	t.Run("timeout exit code 124", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "error-timeout-1",
			CaseID:         "test-error",
			Command:        "sleep",
			Args:           []string{"10"},
			TimeoutSeconds: 1,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusTimeout, result.Status)
		assert.Equal(t, 124, *result.ReturnCode)
		assert.NotNil(t, result.ErrorMessage)
		assert.Contains(t, *result.ErrorMessage, "timed out")
	})

	t.Run("large output truncation", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "error-largeout-1",
			CaseID:         "test-error",
			Command:        "sh",
			Args:           []string{"-c", "for i in $(seq 1 1000); do echo \"Line $i with additional text\"; done"},
			TimeoutSeconds: 30,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotNil(t, result.TerminalOutput)

		if result.TerminalOutput.TruncatedStdout {
			assert.Equal(t, 50, len(result.TerminalOutput.LastLines))
			assert.Equal(t, 1000, result.TerminalOutput.OriginalStdoutLines)
		}
	})

	t.Run("mixed stderr and stdout", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "error-mixed-1",
			CaseID:         "test-error",
			Command:        "sh",
			Args:           []string{"-c", "echo out1; echo err1 >&2; echo out2; echo err2 >&2"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "out1")
		assert.Contains(t, result.Stdout, "out2")
		assert.Contains(t, result.Stderr, "err1")
		assert.Contains(t, result.Stderr, "err2")
	})
}

func TestExecutionService_ShellComplexity(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("multiple piped commands", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "complex-multipipe-1",
			CaseID:         "test-complex",
			Command:        "echo 'hello world test' | grep hello | grep world | grep test | wc -l",
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

	t.Run("subshell execution", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "complex-subshell-1",
			CaseID:         "test-complex",
			Command:        "echo $(echo nested)",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "nested")
	})

	t.Run("command with backticks", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "complex-backticks-1",
			CaseID:         "test-complex",
			Command:        "echo `echo backtick`",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "backtick")
	})

	t.Run("stderr to file redirection", func(t *testing.T) {
		tmpDir := t.TempDir()
		errFile := filepath.Join(tmpDir, "error.log")

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "complex-stderr-redirect-1",
			CaseID:         "test-complex",
			Command:        fmt.Sprintf("sh -c 'echo stdout; echo stderr >&2' 2> %s", errFile),
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "stdout")
		assert.FileExists(t, errFile)
	})

	t.Run("both stdout and stderr redirection", func(t *testing.T) {
		tmpDir := t.TempDir()
		outFile := filepath.Join(tmpDir, "output.log")
		errFile := filepath.Join(tmpDir, "error.log")

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "complex-both-redirect-1",
			CaseID:         "test-complex",
			Command:        fmt.Sprintf("sh -c 'echo stdout; echo stderr >&2' > %s 2> %s", outFile, errFile),
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.FileExists(t, outFile)
		assert.FileExists(t, errFile)
	})

	t.Run("environment variable expansion in shell", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "complex-envexpand-1",
			CaseID:         "test-complex",
			Command:        "sh",
			Args:           []string{"-c", "echo prefix_${CUSTOM_VAR}_suffix"},
			TimeoutSeconds: 5,
			Environment: map[string]string{
				"CUSTOM_VAR": "EXPANDED",
			},
			RequestedBy: "test-user",
			APIKey:      "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "prefix_EXPANDED_suffix")
	})

	t.Run("script with multiple commands", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptPath := filepath.Join(tmpDir, "test-script.sh")

		scriptContent := `#!/bin/bash
echo "Start"
sleep 0.1
echo "Middle"
sleep 0.1
echo "End"
exit 0
`
		err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
		require.NoError(t, err)

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "complex-script-1",
			CaseID:         "test-complex",
			Command:        scriptPath,
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "Start")
		assert.Contains(t, result.Stdout, "Middle")
		assert.Contains(t, result.Stdout, "End")
	})
}
