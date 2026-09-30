// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveRuntime_RenamesInsteadOfDeleting(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()
	require.NoError(t, svc.WriteFile(ctx, "data/keep.txt", []byte("evidence"), constants.PermFilePrivate))
	runtimeDir := svc.Resolve("")

	archived, err := svc.ArchiveRuntime(ctx, time.Date(2026, time.September, 30, 14, 1, 0, 0, time.UTC))
	require.NoError(t, err)

	assert.Equal(t, runtimeDir+"-09301401", archived)
	assert.NoDirExists(t, runtimeDir)
	data, err := os.ReadFile(filepath.Join(archived, "data", "keep.txt"))
	require.NoError(t, err)
	assert.Equal(t, "evidence", string(data))
}

func TestArchiveRuntime_MissingRuntimeIsNoOp(t *testing.T) {
	svc := setupTestFS(t)

	archived, err := svc.ArchiveRuntime(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Empty(t, archived)
}

func TestArchiveRuntime_NeverOverwritesAnExistingArchive(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()
	now := time.Date(2026, time.September, 30, 14, 1, 0, 0, time.UTC)

	require.NoError(t, svc.WriteFile(ctx, "first.txt", []byte("1"), constants.PermFilePrivate))
	first, err := svc.ArchiveRuntime(ctx, now)
	require.NoError(t, err)

	require.NoError(t, svc.WriteFile(ctx, "second.txt", []byte("2"), constants.PermFilePrivate))
	second, err := svc.ArchiveRuntime(ctx, now)
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.FileExists(t, filepath.Join(first, "first.txt"))
	assert.FileExists(t, filepath.Join(second, "second.txt"))
}

func TestArchiveRuntime_HonorsCancelledContext(t *testing.T) {
	svc := setupTestFS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.ArchiveRuntime(ctx, time.Now())
	require.ErrorIs(t, err, context.Canceled)
}
