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

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/stretchr/testify/assert"
)

func TestExecutionStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   constants.ExecutionStatus
		expected string
	}{
		{"pending", constants.ExecutionStatusPending, "pending"},
		{"executing", constants.ExecutionStatusExecuting, "executing"},
		{"completed", constants.ExecutionStatusCompleted, "completed"},
		{"failed", constants.ExecutionStatusFailed, "failed"},
		{"timeout", constants.ExecutionStatusTimeout, "timeout"},
		{"cancelled", constants.ExecutionStatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
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
			Status:          constants.ExecutionStatusCompleted,
			ReturnCode:      &returnCode,
			Stdout:          "hello\n",
			Stderr:          "",
			StartTime:       &startTime,
			EndTime:         &endTime,
			DurationSeconds: 2.0,
		}

		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
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
