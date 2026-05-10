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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
)

// TestExecutionService_CommandExecution tests reliable command execution behavior
func TestExecutionService_CommandExecution(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewExecutionService(cfg, logger)

	t.Run("shell_variable_expansion", func(t *testing.T) {
		// Test that shell variables are properly expanded
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-var-expand-" + time.Now().Format("20060102150405"),
			CaseID:         "case-123",
			Command:        "echo $HOME",
			TimeoutSeconds: 5,
		}

		ctx := context.Background()
		result, err := svc.ExecuteCommand(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		// Should NOT be literal "$HOME", should be expanded
		assert.NotContains(t, result.Stdout, "$HOME")
		assert.Contains(t, result.Stdout, "/") // Should contain a path
	})

	t.Run("tilde_expansion", func(t *testing.T) {
		// Test that tilde is properly expanded
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-tilde-expand-" + time.Now().Format("20060102150405"),
			CaseID:         "case-124",
			Command:        "ls -d ~",
			TimeoutSeconds: 5,
		}

		ctx := context.Background()
		result, err := svc.ExecuteCommand(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		// Should NOT be literal "~", should be expanded to home path
		assert.NotEqual(t, "~\n", result.Stdout)
		assert.Contains(t, result.Stdout, "/") // Should contain a path
	})

	t.Run("pipe_commands", func(t *testing.T) {
		// Test that pipes work correctly
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-pipe-" + time.Now().Format("20060102150405"),
			CaseID:         "case-125",
			Command:        "echo hello world | wc -w",
			TimeoutSeconds: 5,
		}

		ctx := context.Background()
		result, err := svc.ExecuteCommand(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "2") // "hello world" = 2 words
	})

	t.Run("command_timeout", func(t *testing.T) {
		// Test that timeout works correctly
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-timeout-" + time.Now().Format("20060102150405"),
			CaseID:         "case-126",
			Command:        "sleep 10",
			TimeoutSeconds: 2,
		}

		ctx := context.Background()
		start := time.Now().UTC()
		result, err := svc.ExecuteCommand(ctx, req)
		duration := time.Since(start)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusTimeout, result.Status)
		assert.Less(t, duration.Seconds(), 3.0, "Should timeout around 2 seconds")
	})

	t.Run("stdin_closed_prevents_hang", func(t *testing.T) {
		// Commands that try to read stdin should fail fast with EOF, not hang
		// The 'read' command will get EOF immediately since stdin is nil
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-stdin-closed-" + time.Now().Format("20060102150405"),
			CaseID:         "case-127",
			Command:        "read -t 1 input || echo 'read failed as expected'",
			TimeoutSeconds: 5,
		}

		ctx := context.Background()
		start := time.Now().UTC()
		result, err := svc.ExecuteCommand(ctx, req)
		duration := time.Since(start)

		require.NoError(t, err)
		// Should complete quickly, not hang
		assert.Less(t, duration.Seconds(), 3.0)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
	})

	t.Run("glob_expansion", func(t *testing.T) {
		// Test that glob patterns work
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-glob-" + time.Now().Format("20060102150405"),
			CaseID:         "case-128",
			Command:        "ls /etc/*.conf 2>/dev/null | head -1 || echo 'no conf files'",
			TimeoutSeconds: 5,
		}

		ctx := context.Background()
		result, err := svc.ExecuteCommand(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		// Should have expanded the glob, not literal "*.conf"
		assert.NotContains(t, result.Stdout, "*.conf")
	})

	t.Run("command_with_delayed_output", func(t *testing.T) {
		// Commands with initial silence should complete normally
		req := &models.ExecutionRequestPayload{
			ExecutionID:    "test-delayed-" + time.Now().Format("20060102150405"),
			CaseID:         "case-129",
			Command:        "sleep 2; echo 'Done waiting'",
			TimeoutSeconds: 10,
		}

		ctx := context.Background()
		result, err := svc.ExecuteCommand(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Contains(t, result.Stdout, "Done waiting")
	})
}
