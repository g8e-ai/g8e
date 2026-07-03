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

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatorCpCmdExecution(t *testing.T) {
	t.Run("cp copies binary to directory", func(t *testing.T) {
		tmpDir := t.TempDir()
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
		tmpDir := t.TempDir()
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

func TestOperatorRunCmdFlags(t *testing.T) {
	t.Run("run command has all expected flags", func(t *testing.T) {
		root := &cobra.Command{Use: "g8e"}
		root.PersistentFlags().StringP("endpoint", "e", "", "Gateway endpoint")
		cmd := operatorRunCmd()
		root.AddCommand(cmd)
		_ = cmd.ParseFlags([]string{})

		expectedFlags := []string{
			"endpoint", "key", "cert", "trust-bundle", "working-dir",
			"cloud", "provider", "execution-vault", "no-git", "log", "heartbeat-interval",
		}
		for _, flagName := range expectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "operator run should have --%s flag", flagName)
		}
	})

	t.Run("run command endpoint has shorthand 'e'", func(t *testing.T) {
		root := &cobra.Command{Use: "g8e"}
		root.PersistentFlags().StringP("endpoint", "e", "", "Gateway endpoint")
		cmd := operatorRunCmd()
		root.AddCommand(cmd)
		_ = cmd.ParseFlags([]string{})

		flag := cmd.Flags().ShorthandLookup("e")
		assert.NotNil(t, flag)
	})

	t.Run("run command heartbeat defaults to 30", func(t *testing.T) {
		cmd := operatorRunCmd()
		flag := cmd.Flags().Lookup("heartbeat-interval")
		require.NotNil(t, flag)
		assert.Equal(t, "30", flag.DefValue)
	})
}

func TestOperatorDeployCmdErrorPaths(t *testing.T) {
	t.Run("deploy fails when no credentials", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestConfig(t, tmpDir)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		cmd := operatorDeployCmd()
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
