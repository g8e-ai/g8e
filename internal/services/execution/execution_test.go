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
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionService_ExecuteCommand(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("simple command execution", func(t *testing.T) {
		t.Parallel()
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-1",
			CaseID:         "test-case-1",
			Command:        "echo",
			Args:           []string{"hello", "world"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, 0, *result.ReturnCode)
		assert.Contains(t, result.Stdout, "hello world")
		assert.Empty(t, result.Stderr)
		assert.NotNil(t, result.StartTime)
		assert.NotNil(t, result.EndTime)
		assert.Greater(t, result.DurationSeconds, 0.0)
	})

	t.Run("command with non-zero exit code", func(t *testing.T) {
		t.Parallel()
		// Use shell execution to get proper exit code handling
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-2",
			CaseID:         "test-case-2",
			Command:        "sh",
			Args:           []string{"-c", "exit 42"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, 42, *result.ReturnCode)
	})

	t.Run("command not found", func(t *testing.T) {
		t.Parallel()
		// Use shell execution to get proper shell exit code 127
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-3",
			CaseID:         "test-case-3",
			Command:        "sh",
			Args:           []string{"-c", "nonexistent_command_12345"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, result.Status)
		assert.Equal(t, 127, *result.ReturnCode)
		assert.Equal(t, "command_not_found", *result.ErrorType)
		assert.Contains(t, result.Stderr, "not found")
	})

	t.Run("command timeout", func(t *testing.T) {
		t.Parallel()
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-4",
			CaseID:         "test-case-4",
			Command:        "sleep",
			Args:           []string{"10"},
			TimeoutSeconds: 1,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT, result.Status)
		assert.Equal(t, 124, *result.ReturnCode)
		assert.NotNil(t, result.ErrorMessage)
		assert.Contains(t, *result.ErrorMessage, "timed out")
	})

	t.Run("command with working directory", func(t *testing.T) {
		t.Parallel()
		workDir := t.TempDir()
		req := &models.ExecutionRequestPayload{
			ExecutionID:      "test-req-5",
			CaseID:           "test-case-5",
			Command:          "pwd",
			Args:             []string{},
			TimeoutSeconds:   5,
			WorkingDirectory: &workDir,
			RequestedBy:      "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		// On Windows, bash might return a Posix-style path (e.g. /c/Users/...)
		// while t.TempDir() returns a Windows path (e.g. C:\Users\...)
		// Normalize both to forward slashes for comparison
		actualStdout := filepath.ToSlash(strings.TrimSpace(result.Stdout))
		// On Windows with Git Bash, paths might be mapped (e.g. C:/Users/... to /c/Users/... or /tmp/...)
		// Check if the base name of the temp directory is present in the output
		assert.Contains(t, actualStdout, filepath.Base(workDir))
	})

	t.Run("command with environment variables", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("custom env var inheritance not reliable with POSIX shell on Windows")
		}
		// Use single command string - shell handles variable expansion
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-6",
			CaseID:         "test-case-6",
			Command:        "echo $TEST_VAR",
			TimeoutSeconds: 5,
			Environment: map[string]string{
				"TEST_VAR": "test_value",
			},
			RequestedBy: "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Contains(t, result.Stdout, "test_value")
	})

	t.Run("shell command with pipes", func(t *testing.T) {
		t.Parallel()
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-7",
			CaseID:         "test-case-7",
			Command:        "echo hello | grep hello",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Contains(t, result.Stdout, "hello")
	})

	t.Run("context cancellation during wait", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-8",
			CaseID:         "test-case-8",
			Command:        "sleep",
			Args:           []string{"2"},
			TimeoutSeconds: 10,
			RequestedBy:    "test-user",
		}

		// Start execution in goroutine
		done := make(chan bool)
		var result *models.ExecutionResultsPayload
		var err error
		go func() {
			result, err = svc.ExecuteCommand(ctx, req)
			done <- true
		}()

		// Wait for execution to be tracked before cancelling
		require.Eventually(t, func() bool {
			active := svc.GetActiveExecutions()
			_, exists := active[req.ExecutionID]
			return exists
		}, 1*time.Second, 10*time.Millisecond)
		cancel()

		// Wait for completion
		<-done

		// Command should complete but may be cancelled
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("terminal output creation", func(t *testing.T) {
		t.Parallel()
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-9",
			CaseID:         "test-case-9",
			Command:        "echo",
			Args:           []string{"line1\nline2\nline3"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.TerminalOutput)
		assert.Equal(t, "echo", result.TerminalOutput.Command)
		assert.Contains(t, result.TerminalOutput.CommandWithArgs, "echo")
		assert.NotEmpty(t, result.TerminalOutput.CombinedOutput)
	})

	t.Run("system info collection", func(t *testing.T) {
		t.Parallel()
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-10",
			CaseID:         "test-case-10",
			Command:        "echo",
			Args:           []string{"test"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.SystemInfo)
		assert.NotEmpty(t, result.SystemInfo.Hostname)
		assert.NotEmpty(t, result.SystemInfo.OS)
		assert.NotEmpty(t, result.SystemInfo.Architecture)
	})

	t.Run("environment info collection", func(t *testing.T) {
		t.Parallel()
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-11",
			CaseID:         "test-case-11",
			Command:        "echo",
			Args:           []string{"test"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.EnvironmentInfo)
	})
}

func TestExecutionService_BuildCommandString(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	tests := []struct {
		name     string
		command  string
		args     []string
		expected string
	}{
		{
			name:     "command without args",
			command:  "ls",
			args:     []string{},
			expected: "ls",
		},
		{
			name:     "command with single arg",
			command:  "ls",
			args:     []string{"-la"},
			expected: "ls -la",
		},
		{
			name:     "command with multiple args",
			command:  "echo",
			args:     []string{"hello", "world"},
			expected: "echo hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := svc.BuildCommandString(tt.command, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecutionService_CreateTerminalOutput(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("basic output", func(t *testing.T) {
		t.Parallel()
		output := svc.createTerminalOutput("echo", []string{"test"}, "test output\n", "")

		assert.Equal(t, "echo", output.Command)
		assert.Equal(t, "echo test", output.CommandWithArgs)
		assert.Contains(t, output.CombinedOutput, "test output")
		assert.False(t, output.TruncatedStdout)
		assert.False(t, output.TruncatedStderr)
	})

	t.Run("output with stderr", func(t *testing.T) {
		t.Parallel()
		output := svc.createTerminalOutput("test", []string{}, "stdout\n", "stderr\n")

		assert.Contains(t, output.CombinedOutput, "stdout")
		assert.Contains(t, output.CombinedOutput, "stderr")
	})

	t.Run("truncated output", func(t *testing.T) {
		t.Parallel()
		// Create output with more than 50 lines
		stdout := ""
		for i := 0; i < 100; i++ {
			stdout += "line\n"
		}

		output := svc.createTerminalOutput("test", []string{}, stdout, "")

		assert.True(t, output.TruncatedStdout)
		assert.Equal(t, 100, output.OriginalStdoutLines)
		assert.Len(t, output.LastLines, 50)
	})
}

func TestExecutionService_Stop(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("stop with no active executions", func(t *testing.T) {
		t.Parallel()
		assert.NotPanics(t, func() {
			svc.Stop()
		})
	})

	t.Run("stop during initialization race", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.MaxConcurrentTasks = 1 // Ensure we only need to occupy one slot
		logger := testutil.NewTestLogger()
		svc := NewExecutionService(cfg, logger)

		// Occupy the semaphore to block new tasks
		svc.semaphore <- struct{}{}

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "race-test-1",
			Command:        "sleep",
			Args:           []string{"0.1"},
			TimeoutSeconds: 5,
		}

		// Start execution in background - it will block on the semaphore
		execDone := make(chan error, 1)
		go func() {
			_, err := svc.ExecuteCommand(context.Background(), req)
			execDone <- err
		}()

		// Give it a moment to reach the semaphore wait
		time.Sleep(50 * time.Millisecond)

		// Stop the service - it should set isStopping=true and then wait on wg
		stopDone := make(chan struct{})
		go func() {
			svc.Stop()
			close(stopDone)
		}()

		// Give Stop() a moment to set isStopping=true and block on wg.Wait()
		time.Sleep(50 * time.Millisecond)

		// Release the semaphore - the blocked task should now proceed and see isStopping=true
		<-svc.semaphore

		// Verify execution returns an error about stopping
		select {
		case err := <-execDone:
			assert.Error(t, err)
			if err != nil {
				assert.ErrorIs(t, err, constants.ErrExecutionServiceStopping)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Execution did not return after semaphore release")
		}

		// Verify Stop() completes
		select {
		case <-stopDone:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Stop() did not complete")
		}
	})
}

func TestExecutionService_GetActiveExecutions(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	active := svc.GetActiveExecutions()
	assert.NotNil(t, active)
	assert.Empty(t, active)
}

func TestExecutionService_CancelExecution(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "cancel non-existent execution",
			test: func(t *testing.T) {
				t.Parallel()
				err := svc.CancelExecution("non-existent-id")
				require.Error(t, err)
				assert.ErrorIs(t, err, constants.ErrExecutionNotFound)
			},
		},
		{
			name: "cancel running execution",
			test: func(t *testing.T) {
				t.Parallel()
				req := &models.ExecutionRequestPayload{
					ExecutionID:    "cancel-test-1",
					CaseID:         "test-case",
					Command:        "sleep",
					Args:           []string{"30"},
					TimeoutSeconds: 60,
					RequestedBy:    "test-user",
				}

				done := make(chan bool)
				go func() {
					svc.ExecuteCommand(context.Background(), req)
					done <- true
				}()

				require.Eventually(t, func() bool {
					active := svc.GetActiveExecutions()
					return len(active) > 0
				}, 1*time.Second, 10*time.Millisecond)

				err := svc.CancelExecution("cancel-test-1")
				require.NoError(t, err)

				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal(constants.ErrExecutionNotFound)
				}

				active := svc.GetActiveExecutions()
				_, exists := active["cancel-test-1"]
				assert.False(t, exists)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestExecutionService_CancelExecution_DoesNotSetCancelledStatus(t *testing.T) {
	t.Parallel()
	// Regression: CancelExecution previously wrote ExecutionStatusCancelled after
	// unlocking the mutex, creating a window where it raced with
	// executeCommandInternal's authoritative status write. The dead write was
	// removed - verify it no longer appears in the result.
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	req := &models.ExecutionRequestPayload{
		ExecutionID:    "cancel-status-1",
		CaseID:         "test-case",
		Command:        "sleep 30",
		TimeoutSeconds: 60,
		RequestedBy:    "test-user",
	}

	var result *models.ExecutionResultsPayload
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, _ = svc.ExecuteCommand(context.Background(), req)
	}()

	require.Eventually(t, func() bool {
		active := svc.GetActiveExecutions()
		_, exists := active["cancel-status-1"]
		return exists
	}, 1*time.Second, 10*time.Millisecond)
	require.NoError(t, svc.CancelExecution("cancel-status-1"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal(constants.ErrExecutionNotFound)
	}

	require.NotNil(t, result)
	// The only invariant guaranteed after cancel: status is never Cancelled,
	// because CancelExecution no longer writes that value. The kill path in
	// executeCommandInternal is the sole status writer.
	assert.NotEqual(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_CANCELLED, result.Status,
		"CancelExecution must not inject ExecutionStatusCancelled into the result")
}

func TestExecutionService_CancelExecution_NoConcurrentDeadlock(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	// Run multiple concurrent cancel+execute pairs to surface any deadlock from
	// the previously double-locked CancelExecution.
	const workers = 5
	done := make(chan struct{}, workers)
	for i := 0; i < workers; i++ {
		reqID := fmt.Sprintf("deadlock-test-%d", i)
		go func(id string) {
			defer func() { done <- struct{}{} }()
			req := &models.ExecutionRequestPayload{
				ExecutionID:    id,
				CaseID:         "test-case",
				Command:        "sleep",
				Args:           []string{"0.1"},
				TimeoutSeconds: 60,
				RequestedBy:    "test-user",
			}
			execDone := make(chan error, 1)
			go func() {
				defer close(execDone)
				_, err := svc.ExecuteCommand(context.Background(), req)
				execDone <- err
			}()
			assert.Eventually(t, func() bool {
				active := svc.GetActiveExecutions()
				_, exists := active[id]
				return exists
			}, 1*time.Second, 10*time.Millisecond)
			err := svc.CancelExecution(id)
			require.NoError(t, err, "CancelExecution should not fail")
			select {
			case <-execDone:
			case <-time.After(10 * time.Second):
				t.Errorf("execution %s: %s", id, constants.ErrProcessStopFailed)
			}
		}(reqID)
	}

	for i := 0; i < workers; i++ {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatal(constants.ErrProcessStopFailed)
		}
	}
}

func TestExecutionService_CollectSystemInfo(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	info := svc.collectSystemInfo()

	assert.NotNil(t, info)
	assert.NotEmpty(t, info.Hostname)
	assert.NotEmpty(t, info.OS)
	assert.NotEmpty(t, info.Architecture)
	assert.Positive(t, info.NumCPU)
}

func TestExecutionService_CollectEnvironmentInfo(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	info := svc.collectEnvironmentInfo()

	assert.NotNil(t, info)
	assert.Equal(t, constants.ComponentNameG8EO, info.ComponentName)
	assert.Equal(t, "test-project", info.ProjectID)
}

func TestExecutionService_FinalizeResult(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	startTime := time.Now().Add(-2 * time.Second)
	result := &models.ExecutionResultsPayload{
		ExecutionID: "test-req",
		CaseID:      "test-case",
		Command:     "echo",
		Status:      operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		StartTime:   &startTime,
	}

	svc.finalizeResult(result)

	assert.NotNil(t, result.EndTime)
	assert.Greater(t, result.DurationSeconds, 1.0)
}
