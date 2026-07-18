package fs

import (
	"context"
	"errors"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnforceFilePermissions_NonexistentFileReturnsErrFileWriteFailed(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	err := svc.EnforceFilePermissions(ctx, "does-not-exist.txt", constants.PermFilePrivate)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrFileWriteFailed))
}

func TestEnforceDirPermissions_NonexistentDirReturnsErrEnforcePermissions(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	err := svc.EnforceDirPermissions(ctx, "no-such-dir", constants.PermDirPrivate)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEnforcePermissions))
}

func TestEnforceDirPermissions_EmptyDirSucceeds(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.MkdirAll(ctx, "empty-dir", constants.PermDirStandard))
	require.NoError(t, svc.EnforceDirPermissions(ctx, "empty-dir", constants.PermDirPrivate))
}

func TestEnforceDirPermissions_CancelledContextReturnsError(t *testing.T) {
	svc := setupTestFS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.EnforceDirPermissions(ctx, "any-dir", constants.PermDirPrivate)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestEnforceFilePermissions_CancelledContextReturnsError(t *testing.T) {
	svc := setupTestFS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.EnforceFilePermissions(ctx, "any-file.txt", constants.PermFilePrivate)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}
