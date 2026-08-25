//go:build unix || linux || darwin

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestFsGrepService_ExecuteFsGrep(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	workDir := testutil.TempDir(t)

	// Setup test files
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "file1.txt"), []byte("hello world\nthis is a test\ng8e is cool"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(workDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "subdir", "file2.txt"), []byte("another file\nwith hello in it\nand more text"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(workDir, ".hidden"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ".hidden", "secret.txt"), []byte("hidden hello"), 0644))

	service := NewFsGrepService(workDir, logger)

	t.Run("simple grep search", func(t *testing.T) {
		t.Parallel()
		req := &models.FsGrepRequest{
			ExecutionID: "test-1",
			CaseID:      "case-1",
			Path:        ".",
			Pattern:     "hello",
			MaxMatches:  100,
		}

		result, err := service.ExecuteFsGrep(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
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
		t.Parallel()
		req := &models.FsGrepRequest{
			ExecutionID: "test-2",
			CaseID:      "case-2",
			Path:        ".",
			Pattern:     "g8e.*cool",
			MaxMatches:  100,
		}

		result, err := service.ExecuteFsGrep(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, 1, result.TotalMatches)
		assert.Contains(t, result.Matches[0].Content, "g8e is cool")
	})

	t.Run("grep with include filter", func(t *testing.T) {
		t.Parallel()
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
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, 1, result.TotalMatches)
		assert.Equal(t, "file1.txt", filepath.Base(result.Matches[0].Path))
	})

	t.Run("respects max_matches limit", func(t *testing.T) {
		t.Parallel()
		req := &models.FsGrepRequest{
			ExecutionID: "test-4",
			CaseID:      "case-4",
			Path:        ".",
			Pattern:     "o", // Matches in multiple files/lines
			MaxMatches:  2,
		}

		result, err := service.ExecuteFsGrep(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		assert.Equal(t, 2, result.TotalMatches)
		assert.True(t, result.Truncated)
	})

	t.Run("handles invalid regex", func(t *testing.T) {
		t.Parallel()
		req := &models.FsGrepRequest{
			ExecutionID: "test-5",
			CaseID:      "case-5",
			Path:        ".",
			Pattern:     "[invalid",
			MaxMatches:  100,
		}

		result, err := service.ExecuteFsGrep(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, result.Status)
		assert.Equal(t, "fs_grep invalid pattern", result.ErrorType)
	})
}
