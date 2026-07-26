// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatorCpCmdExecution(t *testing.T) {
	t.Run("cp copies binary to directory", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		destDir := filepath.Join(tmpDir, "dest")
		require.NoError(t, os.MkdirAll(destDir, 0o755))

		cmd := operatorCpCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{destDir})
		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Copied")

		entries, _ := os.ReadDir(destDir)
		assert.NotEmpty(t, entries)
	})

	t.Run("cp copies binary to specific file", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		destFile := filepath.Join(tmpDir, "custom-name")

		cmd := operatorCpCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{destFile})
		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Copied")

		info, err := os.Stat(destFile)
		require.NoError(t, err)
		assert.False(t, info.IsDir())
	})
}

func TestOperatorStartCmdFlags(t *testing.T) {
	t.Run("start command has all expected flags", func(t *testing.T) {
		root := &cobra.Command{Use: "g8e"}
		root.PersistentFlags().StringP("endpoint", "e", "", "Gateway endpoint")
		cmd := operatorStartCmd()
		root.AddCommand(cmd)
		_ = cmd.ParseFlags([]string{})

		expectedFlags := []string{
			"endpoint", "key", "cert", "trust-bundle", "working-dir",
			"cloud", "provider", "execution-vault", "no-git", "log", "heartbeat-interval",
			"lattice-endpoint", "lattice-client-id", "lattice-client-secret",
			"lattice-sandboxes-token", "lattice-entity-name", "lattice-posture-floor",
		}
		for _, flagName := range expectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "operator start should have --%s flag", flagName)
		}
	})

	t.Run("start command endpoint has shorthand 'e'", func(t *testing.T) {
		root := &cobra.Command{Use: "g8e"}
		root.PersistentFlags().StringP("endpoint", "e", "", "Gateway endpoint")
		cmd := operatorStartCmd()
		root.AddCommand(cmd)
		_ = cmd.ParseFlags([]string{})

		flag := cmd.Flags().ShorthandLookup("e")
		assert.NotNil(t, flag)
	})

	t.Run("start command heartbeat defaults to 30", func(t *testing.T) {
		cmd := operatorStartCmd()
		flag := cmd.Flags().Lookup("heartbeat-interval")
		require.NotNil(t, flag)
		assert.Equal(t, "30", flag.DefValue)
	})
}

func TestOperatorDeployCmdErrorPaths(t *testing.T) {
	t.Run("deploy fails when no credentials", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		cmd := operatorDeployCmdWithConfig(func(_ string) (*config.Config, error) { return cfg, nil }, fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
	})
}

func TestOperatorStreamCmdExtraFlags(t *testing.T) {
	t.Run("stream command has concurrency and timeout flags", func(t *testing.T) {
		cmd := operatorStreamCmd()
		concurrencyFlag := cmd.Flags().Lookup("concurrency")
		timeoutFlag := cmd.Flags().Lookup("timeout")
		assert.NotNil(t, concurrencyFlag)
		assert.NotNil(t, timeoutFlag)
		assert.Equal(t, "50", concurrencyFlag.DefValue)
		assert.Equal(t, "60", timeoutFlag.DefValue)
	})

	t.Run("stream command has no-git flag", func(t *testing.T) {
		cmd := operatorStreamCmd()
		flag := cmd.Flags().Lookup("no-git")
		assert.NotNil(t, flag)
	})
}
