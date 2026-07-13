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
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayCmdStructure(t *testing.T) {
	t.Run("gateway command has correct use and aliases", func(t *testing.T) {
		cmd := gatewayCmd()
		assert.Equal(t, "gw", cmd.Use)
		assert.Contains(t, cmd.Aliases, "gateway")
		assert.Contains(t, cmd.Short, "Gateway")
	})

	t.Run("gateway command has all expected subcommands including data and security", func(t *testing.T) {
		cmd := gatewayCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"start", "stop", "status", "restart", "logs", "settings", "reset", "clean", "data", "security", "tunnel"}
		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "gateway command should have %s subcommand", subcmd)
		}
	})
}

func TestGatewayStartCmdFlags(t *testing.T) {
	t.Run("start command has all expected flags", func(t *testing.T) {
		cmd := gatewayStartCmd()
		require.NotNil(t, cmd)

		expectedFlags := []string{
			"posture", "http-port", "https-port", "data-dir", "pki-dir", "secrets-dir",
			"vault-dir", "vault-key", "vault-require-unlock",
			"passkey-rp-id", "passkey-rp-name",
			"rate-limit-rps", "rate-limit-burst",
			"log", "cert-mode", "tribunal-id", "tribunal-url",
			"mcp-downstream-url", "a2a-downstream-url", "follow",
		}
		for _, flagName := range expectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "gateway start should have --%s flag", flagName)
		}
	})

	t.Run("start command has follow shorthand", func(t *testing.T) {
		cmd := gatewayStartCmd()
		flag := cmd.Flags().ShorthandLookup("f")
		assert.NotNil(t, flag)
	})

	t.Run("start command posture defaults to doctrine", func(t *testing.T) {
		cmd := gatewayStartCmd()
		flag := cmd.Flags().Lookup("posture")
		require.NotNil(t, flag)
		assert.Equal(t, "doctrine", flag.DefValue)
	})
}

func TestGatewayLogsCmdNoLogFile(t *testing.T) {
	t.Run("logs command reports no log file when none exists", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		runtimeDir := tmpDir + "/.g8e"
		require.NoError(t, os.MkdirAll(runtimeDir, 0o700))

		cmd := gatewayLogsCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No log file found")
	})
}
