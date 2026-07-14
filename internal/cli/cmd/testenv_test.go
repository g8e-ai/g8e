package cmd

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// newCmdTestEnv returns a pre-aligned (fileSvc, cfg) pair for hermetic cmd tests.
// The fileSvc is rooted at tmpDir, and cfg.RuntimeDir == fileSvc.Resolve("")
// because setupTestConfig uses config.LoadWithPaths(tmpDir, ...) which sets
// RuntimeDir = pathutil.SafeJoin(tmpDir, constants.RuntimeDirname), and
// fileSvc.Resolve("") == pathutil.SafeJoin(tmpDir, constants.RuntimeDirname).
func newCmdTestEnv(t *testing.T) (fs.RuntimeFileService, *config.Config) {
	t.Helper()
	tmpDir := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	cfg := setupTestConfig(t, tmpDir)
	return fileSvc, cfg
}

// fileSvcFactoryFor returns a fileSvcFactory that always returns the given fileSvc.
// Used in tests to inject a hermetic fileSvc into *WithConfig functions.
func fileSvcFactoryFor(fileSvc fs.RuntimeFileService) func() (fs.RuntimeFileService, error) {
	return func() (fs.RuntimeFileService, error) { return fileSvc, nil }
}
