package cmd

import (
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
	return setupTestConfig(t, tmpDir)
}

// fileSvcFactoryFor returns a fileSvcFactory that always returns the given fileSvc.
// Used in tests to inject a hermetic fileSvc into *WithConfig functions.
func fileSvcFactoryFor(fileSvc fs.RuntimeFileService) func() (fs.RuntimeFileService, error) {
	return func() (fs.RuntimeFileService, error) { return fileSvc, nil }
}

// mustRel converts an absolute .g8e/ path to a relative path, failing the test on error.
func mustRel(t *testing.T, fileSvc fs.RuntimeFileService, absPath string) string {
	t.Helper()
	rel, err := fileSvc.RelFromAbs(absPath)
	require.NoError(t, err)
	return rel
}

// failingFileSvcFactory returns a fileSvcFactory that always returns the given error.
// Used in tests to verify that *WithConfig functions handle fileSvcFactory errors correctly.
func failingFileSvcFactory(err error) func() (fs.RuntimeFileService, error) {
	return func() (fs.RuntimeFileService, error) { return nil, err }
}

// panickingClientFactory returns an apiClientFactory that panics if called.
// Used in factory-error tests to assert that clientFactory is never reached
// when fileSvcFactory fails.
func panickingClientFactory() apiClientFactory {
	return func(fs.RuntimeFileService, *config.Config) (apiClient, error) {
		panic("clientFactory should not be called when fileSvcFactory fails")
	}
}
