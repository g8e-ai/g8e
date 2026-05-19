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

package models

import (
	"testing"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/stretchr/testify/assert"
)

func TestExecutionStatus(t *testing.T) {
	// Test that protobuf enum values are properly typed
	// This test ensures type safety at the boundary
	tests := []struct {
		name   string
		status operatorv1.ExecutionStatus
	}{
		{"unspecified", operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED},
		{"executing", operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING},
		{"completed", operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED},
		{"failed", operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED},
		{"cancelled", operatorv1.ExecutionStatus_EXECUTION_STATUS_CANCELLED},
		{"timeout", operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the enum value is a valid protobuf enum
			// The zero value (UNSPECIFIED) is valid for all other values
			if tt.name != "unspecified" {
				assert.NotEqual(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED, tt.status)
			}
		})
	}
}

func TestExecutionRequestPayload(t *testing.T) {
	t.Run("creates valid execution request", func(t *testing.T) {
		taskID := "task-123"
		workDir := "/tmp"

		req := &ExecutionRequestPayload{
			ExecutionID:      "req-123",
			CaseID:           "case-456",
			TaskID:           &taskID,
			Command:          "ls",
			Args:             []string{"-la"},
			TimeoutSeconds:   30,
			RequestedBy:      "user@example.com",
			APIKey:           "api-key",
			WorkingDirectory: &workDir,
		}

		assert.Equal(t, "req-123", req.ExecutionID)
		assert.Equal(t, "case-456", req.CaseID)
		assert.Equal(t, "task-123", *req.TaskID)
		assert.Equal(t, "ls", req.Command)
		assert.Equal(t, []string{"-la"}, req.Args)
		assert.Equal(t, 30, req.TimeoutSeconds)
		assert.Equal(t, "/tmp", *req.WorkingDirectory)
	})
}

func TestExecutionResultsPayload(t *testing.T) {
	t.Run("creates valid execution result", func(t *testing.T) {
		taskID := "task-123"
		returnCode := 0
		startTime := time.Now().UTC()
		endTime := startTime.Add(2 * time.Second)

		result := &ExecutionResultsPayload{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			TaskID:          &taskID,
			Command:         "echo",
			Args:            []string{"hello"},
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ReturnCode:      &returnCode,
			Stdout:          "hello\n",
			Stderr:          "",
			StartTime:       &startTime,
			EndTime:         &endTime,
			DurationSeconds: 2.0,
		}

		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, 0, *result.ReturnCode)
		assert.Equal(t, "hello\n", result.Stdout)
		assert.Equal(t, 2.0, result.DurationSeconds)
	})
}

func TestTerminalOutput(t *testing.T) {
	t.Run("creates valid terminal output", func(t *testing.T) {
		output := &TerminalOutput{
			Command:             "ls",
			CommandWithArgs:     "ls -la",
			CombinedOutput:      "file1.txt\nfile2.txt",
			LastLines:           []string{"file1.txt", "file2.txt"},
			TruncatedStdout:     false,
			TruncatedStderr:     false,
			OriginalStdoutLines: 2,
			OriginalStderrLines: 0,
			TotalOriginalLines:  2,
		}

		assert.Equal(t, "ls", output.Command)
		assert.Equal(t, 2, len(output.LastLines))
		assert.False(t, output.TruncatedStdout)
		assert.Equal(t, 2, output.TotalOriginalLines)
	})
}
