// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmdtest

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// NewCmdTestEnv returns a pre-aligned (fileSvc, cfg) pair for hermetic command tests.
func NewCmdTestEnv(t *testing.T) (fs.RuntimeFileService, *config.Config) {
	t.Helper()
	return SetupTestConfig(t, testutil.TempDir(t))
}

// SetupTestConfig builds a runtime file service and config rooted at tmpDir.
func SetupTestConfig(t *testing.T, tmpDir string) (fs.RuntimeFileService, *config.Config) {
	t.Helper()

	fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)
	require.Equal(t, cfg.RuntimeDir, fileSvc.Resolve(""))
	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), []byte("dummy-trust-bundle"), constants.PermFilePublic))
	return fileSvc, cfg
}

// SilentCobraCommand returns a command whose stdout and stderr are discarded.
func SilentCobraCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

// FileSvcFactoryFor returns a file service factory that always returns fileSvc.
func FileSvcFactoryFor(fileSvc fs.RuntimeFileService) func(string, *slog.Logger) (fs.RuntimeFileService, error) {
	return func(string, *slog.Logger) (fs.RuntimeFileService, error) { return fileSvc, nil }
}

// MustRel converts an absolute runtime path to a relative path.
func MustRel(t *testing.T, fileSvc fs.RuntimeFileService, absPath string) string {
	t.Helper()
	rel, err := fileSvc.RelFromAbs(absPath)
	require.NoError(t, err)
	return rel
}

// FailingFileSvcFactory returns a file service factory that always returns err.
func FailingFileSvcFactory(err error) func(string, *slog.Logger) (fs.RuntimeFileService, error) {
	return func(string, *slog.Logger) (fs.RuntimeFileService, error) { return nil, err }
}

// ConfigLoaderFor returns a config loader that always returns cfg.
func ConfigLoaderFor(cfg *config.Config) func(string) (*config.Config, error) {
	return func(string) (*config.Config, error) { return cfg, nil }
}

// EnableGlobalJSON attaches cmd under a synthetic root with the global --json flag enabled.
func EnableGlobalJSON(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	root := cmd.Root()
	if root == cmd {
		wrapper := &cobra.Command{Use: "g8e"}
		wrapper.PersistentFlags().Bool("json", false, "Emit machine-readable JSON output")
		wrapper.AddCommand(cmd)
		root = wrapper
	}
	require.NoError(t, root.PersistentFlags().Set("json", "true"))
}

// GlobalJSONRoot wraps cmd in a synthetic g8e root with --json enabled.
func GlobalJSONRoot(t *testing.T, cmd *cobra.Command) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "g8e"}
	root.PersistentFlags().Bool("json", false, "Emit machine-readable JSON output")
	root.AddCommand(cmd)
	require.NoError(t, root.PersistentFlags().Set("json", "true"))
	return root
}

// UseCLISourceRoot points source-tree discovery at dir for the rest of t.
func UseCLISourceRoot(t *testing.T, dir string) {
	t.Helper()
	previous := shared.CLISourceRoot
	shared.CLISourceRoot = func() (string, error) { return dir, nil }
	t.Cleanup(func() { shared.CLISourceRoot = previous })
}
