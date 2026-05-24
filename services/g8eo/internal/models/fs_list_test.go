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

func TestFsListRequest(t *testing.T) {
	t.Run("creates valid list request", func(t *testing.T) {
		taskID := "task-123"

		req := &FsListRequest{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			TaskID:          &taskID,
			InvestigationID: "inv-789",
			Path:            "/tmp",
			MaxDepth:        3,
			MaxEntries:      100,
			RequestedBy:     "user@example.com",
		}

		assert.Equal(t, "req-123", req.ExecutionID)
		assert.Equal(t, "/tmp", req.Path)
		assert.Equal(t, 3, req.MaxDepth)
		assert.Equal(t, 100, req.MaxEntries)
	})
}

func TestFsListEntry(t *testing.T) {
	t.Run("creates valid file entry", func(t *testing.T) {
		owner := "user"
		group := "group"
		target := "/path/to/target"

		entry := &FsListEntry{
			Name:    "file.txt",
			Path:    "/tmp/file.txt",
			IsDir:   false,
			Size:    1024,
			Mode:    "0644",
			ModTime: 1234567890,
			IsSymlink:     true,
			SymlinkTarget: &target,
			Owner:         &owner,
			Group:         &group,
			Inode:         12345,
			Nlink:         1,
		}

		assert.Equal(t, "file.txt", entry.Name)
		assert.False(t, entry.IsDir)
		assert.Equal(t, int64(1024), entry.Size)
		assert.True(t, entry.IsSymlink)
		assert.Equal(t, "/path/to/target", *entry.SymlinkTarget)
	})

	t.Run("creates valid directory entry", func(t *testing.T) {
		entry := &FsListEntry{
			Name:    "dir",
			Path:    "/tmp/dir",
			IsDir:   true,
			Size:    4096,
			Mode:    "0755",
			ModTime: 1234567890,
		}

		assert.Equal(t, "dir", entry.Name)
		assert.True(t, entry.IsDir)
	})
}

func TestFsListResult(t *testing.T) {
	t.Run("creates successful list result", func(t *testing.T) {
		taskID := "task-123"
		startTime := time.Now().UTC()
		endTime := startTime.Add(1 * time.Second)

		result := &FsListResult{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			TaskID:          &taskID,
			InvestigationID: "inv-789",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Path:            "/tmp",
			Entries: []FsListEntry{
				{
					Name:  "file1.txt",
					Path:  "/tmp/file1.txt",
					IsDir: false,
					Size:  1024,
				},
				{
					Name:  "file2.txt",
					Path:  "/tmp/file2.txt",
					IsDir: false,
					Size:  2048,
				},
			},
			TotalCount:      2,
			Truncated:       false,
			StartTime:       &startTime,
			EndTime:         &endTime,
			DurationSeconds: 1.0,
		}

		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, 2, result.TotalCount)
		assert.Len(t, result.Entries, 2)
		assert.False(t, result.Truncated)
	})

	t.Run("creates failed list result", func(t *testing.T) {
		errorMsg := "directory not found"
		errorType := "not_found"

		result := &FsListResult{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			InvestigationID: "inv-789",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			Path:            "/nonexistent",
			Entries:         []FsListEntry{},
			TotalCount:      0,
			Truncated:       false,
			DurationSeconds: 0.1,
			ErrorMessage:    &errorMsg,
			ErrorType:       &errorType,
		}

		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, result.Status)
		assert.Equal(t, "directory not found", *result.ErrorMessage)
	})

	t.Run("creates truncated list result", func(t *testing.T) {
		result := &FsListResult{
			ExecutionID:     "req-123",
			CaseID:          "case-456",
			InvestigationID: "inv-789",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Path:            "/tmp",
			Entries:         []FsListEntry{},
			TotalCount:      1000,
			Truncated:       true,
			DurationSeconds: 2.0,
		}

		assert.True(t, result.Truncated)
		assert.Equal(t, 1000, result.TotalCount)
	})
}
