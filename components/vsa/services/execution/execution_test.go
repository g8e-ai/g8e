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
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionService_ExecuteCommand(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("simple command execution", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-1",
			CaseID:         "test-case-1",
			Command:        "echo",
			Args:           []string{"hello", "world"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, 0, *result.ReturnCode)
		assert.Contains(t, result.Stdout, "hello world")
		assert.Empty(t, result.Stderr)
		assert.NotNil(t, result.StartTime)
		assert.NotNil(t, result.EndTime)
		assert.Greater(t, result.DurationSeconds, 0.0)
	})

	t.Run("command with non-zero exit code", func(t *testing.T) {
		// Use a single command string - shell execution handles it
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-2",
			CaseID:         "test-case-2",
			Command:        "exit 42",
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, 42, *result.ReturnCode)
	})

	t.Run("command not found", func(t *testing.T) {
		// Shell returns exit code 127 for command not found
		// Status is "failed" so AI can handle the error appropriately
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-3",
			CaseID:         "test-case-3",
			Command:        "nonexistent_command_12345",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Equal(t, 127, *result.ReturnCode)
		assert.Equal(t, "command_not_found", *result.ErrorType)
		assert.Contains(t, result.Stderr, "not found")
	})

	t.Run("command timeout", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-4",
			CaseID:         "test-case-4",
			Command:        "sleep",
			Args:           []string{"10"},
			TimeoutSeconds: 1,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constants.ExecutionStatusTimeout, result.Status)
		assert.Equal(t, 124, *result.ReturnCode)
		assert.NotNil(t, result.ErrorMessage)
		assert.Contains(t, *result.ErrorMessage, "timed out")
	})

	t.Run("command with working directory", func(t *testing.T) {
		workDir := t.TempDir()
		req := &models.ExecutionRequestPayload{
			ExecutionID:      "test-req-5",
			CaseID:           "test-case-5",
			Command:          "pwd",
			Args:             []string{},
			TimeoutSeconds:   5,
			WorkingDirectory: &workDir,
			RequestedBy:      "test-user",
			APIKey:           "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, workDir)
	})

	t.Run("command with environment variables", func(t *testing.T) {
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
			APIKey:      "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "test_value")
	})

	t.Run("shell command with pipes", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-7",
			CaseID:         "test-case-7",
			Command:        "echo hello | grep hello",
			Args:           []string{},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "hello")
	})

	t.Run("context cancellation during wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-8",
			CaseID:         "test-case-8",
			Command:        "sleep",
			Args:           []string{"2"},
			TimeoutSeconds: 10,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		// Start execution in goroutine
		done := make(chan bool)
		var result *models.ExecutionResultsPayload
		var err error
		go func() {
			result, err = svc.ExecuteCommand(ctx, req)
			done <- true
		}()

		// Cancel after a brief delay
		time.Sleep(100 * time.Millisecond)
		cancel()

		// Wait for completion
		<-done

		// Command should complete but may be cancelled
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("terminal output creation", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-9",
			CaseID:         "test-case-9",
			Command:        "echo",
			Args:           []string{"line1\nline2\nline3"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.TerminalOutput)
		assert.Equal(t, "echo", result.TerminalOutput.Command)
		assert.Contains(t, result.TerminalOutput.CommandWithArgs, "echo")
		assert.NotEmpty(t, result.TerminalOutput.CombinedOutput)
	})

	t.Run("system info collection", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-10",
			CaseID:         "test-case-10",
			Command:        "echo",
			Args:           []string{"test"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.SystemInfo)
		assert.NotEmpty(t, result.SystemInfo.Hostname)
		assert.NotEmpty(t, result.SystemInfo.OS)
		assert.NotEmpty(t, result.SystemInfo.Architecture)
	})

	t.Run("environment info collection", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-req-11",
			CaseID:         "test-case-11",
			Command:        "echo",
			Args:           []string{"test"},
			TimeoutSeconds: 5,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		result, err := svc.ExecuteCommand(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, result.EnvironmentInfo)
	})
}

func TestExecutionService_BuildCommandString(t *testing.T) {
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
			result := svc.BuildCommandString(tt.command, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecutionService_CreateTerminalOutput(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("basic output", func(t *testing.T) {
		output := svc.createTerminalOutput("echo", []string{"test"}, "test output\n", "")

		assert.Equal(t, "echo", output.Command)
		assert.Equal(t, "echo test", output.CommandWithArgs)
		assert.Contains(t, output.CombinedOutput, "test output")
		assert.False(t, output.TruncatedStdout)
		assert.False(t, output.TruncatedStderr)
	})

	t.Run("output with stderr", func(t *testing.T) {
		output := svc.createTerminalOutput("test", []string{}, "stdout\n", "stderr\n")

		assert.Contains(t, output.CombinedOutput, "stdout")
		assert.Contains(t, output.CombinedOutput, "stderr")
	})

	t.Run("truncated output", func(t *testing.T) {
		// Create output with more than 50 lines
		stdout := ""
		for i := 0; i < 100; i++ {
			stdout += "line\n"
		}

		output := svc.createTerminalOutput("test", []string{}, stdout, "")

		assert.True(t, output.TruncatedStdout)
		assert.Equal(t, 100, output.OriginalStdoutLines)
		assert.Equal(t, 50, len(output.LastLines))
	})
}

func TestExecutionService_Stop(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("stop with no active executions", func(t *testing.T) {
		assert.NotPanics(t, func() {
			svc.Stop()
		})
	})

	t.Run("stop with active executions", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "stop-test-1",
			Command:        "sleep",
			Args:           []string{"30"},
			TimeoutSeconds: 60,
		}

		// Start execution in background
		execDone := make(chan struct{})
		go func() {
			defer close(execDone)
			svc.ExecuteCommand(context.Background(), req)
		}()

		// Wait for execution to start and be tracked
		require.Eventually(t, func() bool {
			active := svc.GetActiveExecutions()
			return len(active) == 1
		}, 2*time.Second, 10*time.Millisecond)

		// Stop the service
		svc.Stop()

		// Verify map is cleared
		assert.Empty(t, svc.GetActiveExecutions())

		// Verify execution completed (cancelled)
		select {
		case <-execDone:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Execution did not complete after stop")
		}
	})
}

func TestExecutionService_GetActiveExecutions(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	active := svc.GetActiveExecutions()
	assert.NotNil(t, active)
	assert.Empty(t, active)
}

func TestExecutionService_CancelExecution(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("cancel non-existent execution", func(t *testing.T) {
		err := svc.CancelExecution("non-existent-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "execution not found")
	})

	t.Run("cancel running execution", func(t *testing.T) {
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "cancel-test-1",
			CaseID:         "test-case",
			Command:        "sleep",
			Args:           []string{"30"},
			TimeoutSeconds: 60,
			RequestedBy:    "test-user",
			APIKey:         "test-key",
		}

		// Start execution in background
		done := make(chan bool)
		go func() {
			svc.ExecuteCommand(context.Background(), req)
			done <- true
		}()

		// Wait for execution to start
		time.Sleep(200 * time.Millisecond)

		// Cancel the execution
		err := svc.CancelExecution("cancel-test-1")
		assert.NoError(t, err)

		// Wait for execution to complete
		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Execution did not complete after cancel")
		}

		// Verify execution was removed from active list
		active := svc.GetActiveExecutions()
		_, exists := active["cancel-test-1"]
		assert.False(t, exists)
	})

}

func TestExecutionService_CancelExecution_DoesNotSetCancelledStatus(t *testing.T) {
	// Regression: CancelExecution previously wrote ExecutionStatusCancelled after
	// unlocking the mutex, creating a window where it raced with
	// executeCommandInternal's authoritative status write. The dead write was
	// removed — verify it no longer appears in the result.
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	req := &models.ExecutionRequestPayload{
		ExecutionID:    "cancel-status-1",
		CaseID:         "test-case",
		Command:        "sleep 30",
		TimeoutSeconds: 60,
		RequestedBy:    "test-user",
		APIKey:         "test-key",
	}

	var result *models.ExecutionResultsPayload
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, _ = svc.ExecuteCommand(context.Background(), req)
	}()

	time.Sleep(150 * time.Millisecond)
	require.NoError(t, svc.CancelExecution("cancel-status-1"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("execution did not complete after cancel")
	}

	require.NotNil(t, result)
	// The only invariant guaranteed after cancel: status is never Cancelled,
	// because CancelExecution no longer writes that value. The kill path in
	// executeCommandInternal is the sole status writer.
	assert.NotEqual(t, constants.ExecutionStatusCancelled, result.Status,
		"CancelExecution must not inject ExecutionStatusCancelled into the result")
}

func TestExecutionService_CancelExecution_NoConcurrentDeadlock(t *testing.T) {
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
				Args:           []string{"30"},
				TimeoutSeconds: 60,
				RequestedBy:    "test-user",
				APIKey:         "test-key",
			}
			execDone := make(chan struct{})
			go func() {
				defer close(execDone)
				svc.ExecuteCommand(context.Background(), req) //nolint:errcheck
			}()
			time.Sleep(100 * time.Millisecond)
			svc.CancelExecution(id) //nolint:errcheck
			select {
			case <-execDone:
			case <-time.After(5 * time.Second):
				t.Errorf("execution %s did not complete — possible deadlock", id)
			}
		}(reqID)
	}

	for i := 0; i < workers; i++ {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatal("workers did not finish — deadlock suspected")
		}
	}
}

func TestExecutionService_CollectSystemInfo(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	info := svc.collectSystemInfo()

	assert.NotNil(t, info)
	assert.NotEmpty(t, info.Hostname)
	assert.NotEmpty(t, info.OS)
	assert.NotEmpty(t, info.Architecture)
	assert.Greater(t, info.NumCPU, 0)
}

func TestExecutionService_CollectEnvironmentInfo(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	info := svc.collectEnvironmentInfo()

	assert.NotNil(t, info)
	assert.Equal(t, "vsa", info.ServiceName)
	assert.Equal(t, "test-project", info.ProjectID)
}

func TestExecutionService_FinalizeResult(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	startTime := time.Now().Add(-2 * time.Second)
	result := &models.ExecutionResultsPayload{
		ExecutionID: "test-req",
		CaseID:      "test-case",
		Command:     "echo",
		Status:      constants.ExecutionStatusCompleted,
		StartTime:   &startTime,
	}

	svc.finalizeResult(result)

	assert.NotNil(t, result.EndTime)
	assert.Greater(t, result.DurationSeconds, 1.0)
}

func TestExecutionService_GetActiveExecutionsEmpty(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	active := svc.GetActiveExecutions()
	assert.NotNil(t, active)
	assert.Empty(t, active)
}
