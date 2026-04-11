//go:build unix || linux || darwin

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
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
)

func TestFsListService_ExecuteFsList(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "sentinel.txt"), []byte("x"), 0644))
	service := NewFsListService(workDir, logger)

	t.Run("lists current directory", func(t *testing.T) {
		req := &models.FsListRequest{
			ExecutionID: "test-1",
			CaseID:      "case-1",
			Path:        ".",
			MaxDepth:    0,
			MaxEntries:  100,
		}

		result, err := service.ExecuteFsList(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.NotEmpty(t, result.Path)
		assert.NotEmpty(t, result.Entries)
		assert.Greater(t, result.TotalCount, 0)
	})

	t.Run("lists temp directory with file metadata", func(t *testing.T) {
		// Create temp directory with test files
		tmpDir := t.TempDir()

		// Create test files
		testFile := filepath.Join(tmpDir, "test.txt")
		err := os.WriteFile(testFile, []byte("hello world"), 0644)
		require.NoError(t, err)

		testDir := filepath.Join(tmpDir, "subdir")
		err = os.Mkdir(testDir, 0755)
		require.NoError(t, err)

		req := &models.FsListRequest{
			ExecutionID: "test-2",
			CaseID:      "case-2",
			Path:        tmpDir,
			MaxDepth:    0,
			MaxEntries:  100,
		}

		result, err := service.ExecuteFsList(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, tmpDir, result.Path)
		assert.Equal(t, 2, result.TotalCount) // test.txt and subdir

		// Verify readdirplus-style metadata
		var foundFile, foundDir bool
		for _, entry := range result.Entries {
			if entry.Name == "test.txt" {
				foundFile = true
				assert.False(t, entry.IsDir)
				assert.Equal(t, int64(11), entry.Size) // "hello world" = 11 bytes
				assert.NotEmpty(t, entry.Mode)
				assert.NotZero(t, entry.ModTime)
				assert.NotZero(t, entry.Inode)
			}
			if entry.Name == "subdir" {
				foundDir = true
				assert.True(t, entry.IsDir)
			}
		}
		assert.True(t, foundFile, "should find test.txt")
		assert.True(t, foundDir, "should find subdir")
	})

	t.Run("respects max_entries limit", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create 10 files
		for i := 0; i < 10; i++ {
			f := filepath.Join(tmpDir, filepath.Base(tmpDir)+string(rune('a'+i))+".txt")
			err := os.WriteFile(f, []byte("x"), 0644)
			require.NoError(t, err)
		}

		req := &models.FsListRequest{
			ExecutionID: "test-3",
			CaseID:      "case-3",
			Path:        tmpDir,
			MaxDepth:    0,
			MaxEntries:  5,
		}

		result, err := service.ExecuteFsList(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, 5, result.TotalCount)
		assert.True(t, result.Truncated)
	})

	t.Run("recursive listing with max_depth", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create nested structure: tmpDir/a/b/c.txt
		subA := filepath.Join(tmpDir, "a")
		subB := filepath.Join(subA, "b")
		err := os.MkdirAll(subB, 0755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(subB, "c.txt"), []byte("nested"), 0644)
		require.NoError(t, err)

		// Depth 0 - only see 'a'
		req := &models.FsListRequest{
			ExecutionID: "test-4a",
			CaseID:      "case-4",
			Path:        tmpDir,
			MaxDepth:    0,
			MaxEntries:  100,
		}
		result, err := service.ExecuteFsList(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, 1, result.TotalCount) // just 'a'

		// Depth 1 - see 'a' and 'a/b'
		req.ExecutionID = "test-4b"
		req.MaxDepth = 1
		result, err = service.ExecuteFsList(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, 2, result.TotalCount) // 'a' and 'a/b'

		// Depth 2 - see 'a', 'a/b', and 'a/b/c.txt'
		req.ExecutionID = "test-4c"
		req.MaxDepth = 2
		result, err = service.ExecuteFsList(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, 3, result.TotalCount)
	})

	t.Run("handles non-existent path", func(t *testing.T) {
		req := &models.FsListRequest{
			ExecutionID: "test-5",
			CaseID:      "case-5",
			Path:        "/nonexistent/path/that/does/not/exist",
			MaxDepth:    0,
			MaxEntries:  100,
		}

		result, err := service.ExecuteFsList(context.Background(), req)
		require.NoError(t, err) // Returns result with error, not Go error
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.NotNil(t, result.ErrorType)
		assert.Equal(t, "path_not_found", *result.ErrorType)
	})

	t.Run("handles file path (not directory)", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "fslist-file-*")
		require.NoError(t, err)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		req := &models.FsListRequest{
			ExecutionID: "test-6",
			CaseID:      "case-6",
			Path:        tmpFile.Name(),
			MaxDepth:    0,
			MaxEntries:  100,
		}

		result, err := service.ExecuteFsList(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.NotNil(t, result.ErrorType)
		assert.Equal(t, "not_a_directory", *result.ErrorType)
	})
}
