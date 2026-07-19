package fs

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRuntimeTree_CancelledContextReturnsError(t *testing.T) {
	svc := setupTestFS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.CreateRuntimeTree(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestCreateRuntimeTree_DataDirIsStandard(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.CreateRuntimeTree(ctx))

	info, err := os.Stat(paths.Infra.DataDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermDirStandard), info.Mode().Perm())
}

func TestCreateRuntimeTree_LogDirIsStandard(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.CreateRuntimeTree(ctx))

	info, err := os.Stat(paths.Infra.LogDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermDirStandard), info.Mode().Perm())
}

func TestCreateRuntimeTree_PidDirIsStandard(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.CreateRuntimeTree(ctx))

	info, err := os.Stat(paths.Infra.PidDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermDirStandard), info.Mode().Perm())
}
