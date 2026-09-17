// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package fs

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestIsBindMountedRuntimePath(t *testing.T) {
	assert.True(t, isBindMountedRuntimePath("public-feed/outbox.jsonl"))
	assert.True(t, isBindMountedRuntimePath(filepath.Join("public-mirror", "state.json")))
	assert.False(t, isBindMountedRuntimePath("data/eval/runs/run-1/run.json"))
}

func TestWriteFile_AlignsBindMountOwnershipToDirectoryOwner(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to simulate container bind-mount ownership drift")
	}

	baseDir := t.TempDir()
	publicFeedDir := filepath.Join(baseDir, constants.PublicFeedDirname)
	require.NoError(t, os.MkdirAll(publicFeedDir, constants.PermDirPrivate))
	require.NoError(t, os.Chown(publicFeedDir, 1000, 1000))

	svc, err := NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(t, err)

	ctx := t.Context()
	require.NoError(t, svc.WriteFile(ctx, constants.PublicFeedOutboxPath, []byte("line\n"), constants.PermFilePrivate))

	info, err := os.Stat(filepath.Join(baseDir, constants.PublicFeedOutboxPath))
	require.NoError(t, err)
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	assert.Equal(t, uint32(1000), stat.Uid)
	assert.Equal(t, uint32(1000), stat.Gid)
}
