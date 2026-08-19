// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package fs

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenForAppend_CreatesParentDirectories(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	f, err := svc.OpenForAppend(ctx, "nested/deep/dir/append.log", constants.PermFilePrivate)
	require.NoError(t, err)
	defer f.Close()

	_, err = f.Write([]byte("first"))
	require.NoError(t, err)

	got, err := svc.ReadFile(ctx, "nested/deep/dir/append.log")
	require.NoError(t, err)
	assert.Equal(t, []byte("first"), got)
}

func TestOpenForAppend_CreatesFileIfAbsent(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	f, err := svc.OpenForAppend(ctx, "fresh.log", constants.PermFilePrivate)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	exists, err := svc.FileExists(ctx, "fresh.log")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestOpenForAppend_AppendsToExistingFile(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "append.log", []byte("line1\n"), constants.PermFilePrivate))

	f, err := svc.OpenForAppend(ctx, "append.log", constants.PermFilePrivate)
	require.NoError(t, err)
	_, err = f.Write([]byte("line2\n"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := svc.ReadFile(ctx, "append.log")
	require.NoError(t, err)
	assert.Equal(t, []byte("line1\nline2\n"), got)
}

func TestOpenForAppend_RespectsMode(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	f, err := svc.OpenForAppend(ctx, "mode.log", constants.PermFilePrivate)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	info, err := svc.Stat(ctx, "mode.log")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm())
}

func TestOpenForRead_ReturnsErrNotFoundForMissingFile(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	_, err := svc.OpenForRead(ctx, "missing.log")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}

func TestOpenForRead_ReturnsReadableHandleForExistingFile(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "read.log", []byte("payload"), constants.PermFilePrivate))

	f, err := svc.OpenForRead(ctx, "read.log")
	require.NoError(t, err)
	defer f.Close()

	got, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), got)
}

func TestOpenForAppend_CancelledContextReturnsContextError(t *testing.T) {
	svc := setupTestFS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.OpenForAppend(ctx, "cancelled.log", constants.PermFilePrivate)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestOpenForRead_CancelledContextReturnsContextError(t *testing.T) {
	svc := setupTestFS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.OpenForRead(ctx, "cancelled.log")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
