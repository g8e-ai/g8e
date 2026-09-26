// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestEnsureDockerHostRuntimeLayout_CreatesRuntimeTree(t *testing.T) {
	baseDir := t.TempDir()
	svc, err := NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, EnsureDockerHostRuntimeLayout(ctx, svc))

	info, statErr := os.Stat(filepath.Join(baseDir, constants.RuntimeDirname, constants.PkiDirname))
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestEnsureDockerHostRuntimeLayout_NilContext(t *testing.T) {
	baseDir := t.TempDir()
	svc, err := NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(t, err)

	var nilCtx context.Context
	require.NoError(t, EnsureDockerHostRuntimeLayout(nilCtx, svc))

	info, statErr := os.Stat(filepath.Join(baseDir, constants.RuntimeDirname, constants.PkiDirname))
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestVerifyRuntimeDirWritable_AllowsMissingRuntimeDir(t *testing.T) {
	baseDir := t.TempDir()
	svc, err := NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(t, err)

	assert.NoError(t, verifyRuntimeDirWritable(svc))
}

func TestVerifyRuntimeDirWritable_RejectsUnwritableRuntimeDir(t *testing.T) {
	baseDir := t.TempDir()
	runtimeDir := filepath.Join(baseDir, constants.RuntimeDirname)
	require.NoError(t, os.MkdirAll(runtimeDir, 0o500))

	svc, err := NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(t, err)

	err = verifyRuntimeDirWritable(svc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrRuntimeDirNotWritable)
}

func TestCreateRuntimeTree_CreatesPublicFeedDirs(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.CreateRuntimeTree(ctx))

	for _, dir := range []string{constants.PublicFeedDirname, constants.PublicMirrorDirname} {
		info, err := os.Stat(filepath.Join(svc.Resolve(""), dir))
		require.NoError(t, err, dir)
		assert.True(t, info.IsDir())
	}
}
