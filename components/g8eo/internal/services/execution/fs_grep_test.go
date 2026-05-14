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

	"github.com/g8e-ai/g8e/components/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/components/g8eo/internal/models"
)

func TestFsGrepService_ExecuteFsGrep(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	workDir := t.TempDir()
	
	// Setup test files
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "file1.txt"), []byte("hello world\nthis is a test\ng8e is cool"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(workDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "subdir", "file2.txt"), []byte("another file\nwith hello in it\nand more text"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(workDir, ".hidden"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ".hidden", "secret.txt"), []byte("hidden hello"), 0644))

	service := NewFsGrepService(workDir, logger)

	t.Run("simple grep search", func(t *testing.T) {
		req := &models.FsGrepRequest{
			ExecutionID: "test-1",
			CaseID:      "case-1",
			Path:        ".",
			Pattern:     "hello",
			MaxMatches:  100,
		}

		result, err := service.ExecuteFsGrep(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, 2, result.TotalMatches)
		
		foundFiles := make(map[string]bool)
		for _, m := range result.Matches {
			foundFiles[filepath.Base(m.Path)] = true
			assert.Contains(t, m.Content, "hello")
		}
		assert.True(t, foundFiles["file1.txt"])
		assert.True(t, foundFiles["file2.txt"])
		assert.False(t, foundFiles["secret.txt"], "should skip hidden directories")
	})

	t.Run("grep with regex pattern", func(t *testing.T) {
		req := &models.FsGrepRequest{
			ExecutionID: "test-2",
			CaseID:      "case-2",
			Path:        ".",
			Pattern:     "g8e.*cool",
			MaxMatches:  100,
		}

		result, err := service.ExecuteFsGrep(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, 1, result.TotalMatches)
		assert.Contains(t, result.Matches[0].Content, "g8e is cool")
	})

	t.Run("grep with include filter", func(t *testing.T) {
		req := &models.FsGrepRequest{
			ExecutionID: "test-3",
			CaseID:      "case-3",
			Path:        ".",
			Pattern:     "hello",
			Includes:    []string{"file1.txt"},
			MaxMatches:  100,
		}

		result, err := service.ExecuteFsGrep(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, 1, result.TotalMatches)
		assert.Equal(t, "file1.txt", filepath.Base(result.Matches[0].Path))
	})

	t.Run("respects max_matches limit", func(t *testing.T) {
		req := &models.FsGrepRequest{
			ExecutionID: "test-4",
			CaseID:      "case-4",
			Path:        ".",
			Pattern:     "o", // Matches in multiple files/lines
			MaxMatches:  2,
		}

		result, err := service.ExecuteFsGrep(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusCompleted, result.Status)
		assert.Equal(t, 2, result.TotalMatches)
		assert.True(t, result.Truncated)
	})

	t.Run("handles invalid regex", func(t *testing.T) {
		req := &models.FsGrepRequest{
			ExecutionID: "test-5",
			CaseID:      "case-5",
			Path:        ".",
			Pattern:     "[invalid",
			MaxMatches:  100,
		}

		result, err := service.ExecuteFsGrep(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, constants.ExecutionStatusFailed, result.Status)
		assert.Equal(t, "invalid_pattern", *result.ErrorType)
	})
}
