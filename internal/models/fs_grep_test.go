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

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
)

func TestFsGrepRequest(t *testing.T) {
	t.Run("creates valid grep request", func(t *testing.T) {
		taskID := "task-123"

		req := &FsGrepRequest{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			TaskID:          &taskID,
			InvestigationID: "inv-789",
			Path:            "/tmp",
			Pattern:         "test",
			Includes:        []string{"*.go", "*.py"},
			MaxMatches:      100,
		}

		assert.Equal(t, "req-123", req.ExecutionID)
		assert.Equal(t, "test", req.Pattern)
		assert.Len(t, req.Includes, 2)
		assert.Equal(t, 100, req.MaxMatches)
	})
}

func TestFsGrepResult(t *testing.T) {
	t.Run("creates successful grep result", func(t *testing.T) {
		taskID := "task-123"
		startTime := time.Now().UTC()
		endTime := startTime.Add(1 * time.Second)

		result := &FsGrepResult{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			TaskID:          &taskID,
			InvestigationID: "inv-789",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Path:            "/tmp",
			Pattern:         "test",
			Matches: []FsGrepMatch{
				{
					Path:       "/tmp/file.go",
					LineNumber: 10,
					Content:    "test line",
					Before:     []string{"line 9"},
					After:      []string{"line 11"},
				},
			},
			TotalMatches:    1,
			Truncated:       false,
			StartTime:       &startTime,
			EndTime:         &endTime,
			DurationSeconds: 1.0,
		}

		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, 1, result.TotalMatches)
		assert.Len(t, result.Matches, 1)
		assert.False(t, result.Truncated)
	})

	t.Run("creates failed grep result", func(t *testing.T) {
		errorMsg := "permission denied"
		errorType := "permission_error"

		result := &FsGrepResult{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			InvestigationID: "inv-789",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			Path:            "/root",
			Pattern:         "test",
			Matches:         []FsGrepMatch{},
			TotalMatches:    0,
			Truncated:       false,
			DurationSeconds: 0.1,
			ErrorMessage:    &errorMsg,
			ErrorType:       &errorType,
		}

		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, result.Status)
		assert.Equal(t, "permission denied", *result.ErrorMessage)
	})

	t.Run("creates truncated grep result", func(t *testing.T) {
		result := &FsGrepResult{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			InvestigationID: "inv-789",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Path:            "/tmp",
			Pattern:         "test",
			Matches:         []FsGrepMatch{},
			TotalMatches:    1000,
			Truncated:       true,
			DurationSeconds: 2.0,
		}

		assert.True(t, result.Truncated)
		assert.Equal(t, 1000, result.TotalMatches)
	})
}
