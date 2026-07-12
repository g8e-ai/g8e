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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTunnelCmdStructure(t *testing.T) {
	t.Run("tunnel command has correct use", func(t *testing.T) {
		cmd := tunnelCmd()
		assert.Equal(t, "tunnel", cmd.Use)
		assert.NotEmpty(t, cmd.Short)
		assert.NotEmpty(t, cmd.Long)
	})

	t.Run("tunnel command has expected subcommands", func(t *testing.T) {
		cmd := tunnelCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"create", "run", "status"}
		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "tunnel command should have %s subcommand", subcmd)
		}
	})
}

func TestTunnelCreateCmdFlags(t *testing.T) {
	t.Run("create command has all expected flags", func(t *testing.T) {
		cmd := tunnelCreateCmd()
		require.NotNil(t, cmd)

		expectedFlags := []string{"name", "hostname", "config-dir", "https-port", "ca-bundle", "origin-server-name", "skip-dns"}
		for _, flagName := range expectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "tunnel create should have --%s flag", flagName)
		}
	})

	t.Run("create command name defaults to g8e", func(t *testing.T) {
		cmd := tunnelCreateCmd()
		flag := cmd.Flags().Lookup("name")
		require.NotNil(t, flag)
		assert.Equal(t, "g8e", flag.DefValue)
	})
}

func TestTunnelRunCmdFlags(t *testing.T) {
	t.Run("run command has all expected flags", func(t *testing.T) {
		cmd := tunnelRunCmd()
		require.NotNil(t, cmd)

		expectedFlags := []string{"name", "config-dir"}
		for _, flagName := range expectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "tunnel run should have --%s flag", flagName)
		}
	})
}

func TestTunnelStatusCmdFlags(t *testing.T) {
	t.Run("status command has all expected flags", func(t *testing.T) {
		cmd := tunnelStatusCmd()
		require.NotNil(t, cmd)

		expectedFlags := []string{"hostname", "name"}
		for _, flagName := range expectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "tunnel status should have --%s flag", flagName)
		}
	})
}

func TestGenerateTunnelConfig(t *testing.T) {
	t.Run("generates config with noTLSVerify when no CA bundle", func(t *testing.T) {
		config := generateTunnelConfig(
			"abc123",
			"/home/user/.cloudflared/abc123.json",
			"console.g8e.ai",
			8443,
			"",
			"",
		)

		assert.Contains(t, config, "tunnel: abc123")
		assert.Contains(t, config, "credentials-file: /home/user/.cloudflared/abc123.json")
		assert.Contains(t, config, "hostname: console.g8e.ai")
		assert.Contains(t, config, "service: https://localhost:8443")
		assert.Contains(t, config, "noTLSVerify: true")
		assert.Contains(t, config, "http2Origin: true")
		assert.Contains(t, config, "http_status:404")
	})

	t.Run("generates config with CA bundle and originServerName", func(t *testing.T) {
		config := generateTunnelConfig(
			"abc123",
			"/home/user/.cloudflared/abc123.json",
			"console.g8e.ai",
			8443,
			"./.g8e/pki/g8eg-ca-bundle.pem",
			"g8e.local",
		)

		assert.Contains(t, config, "originCaPool: ./.g8e/pki/g8eg-ca-bundle.pem")
		assert.Contains(t, config, "originServerName: g8e.local")
		assert.NotContains(t, config, "noTLSVerify")
	})

	t.Run("uses custom HTTPS port", func(t *testing.T) {
		config := generateTunnelConfig(
			"abc123",
			"/home/user/.cloudflared/abc123.json",
			"console.g8e.ai",
			9443,
			"",
			"",
		)

		assert.Contains(t, config, "service: https://localhost:9443")
	})
}

func TestParseTunnelID(t *testing.T) {
	t.Run("extracts UUID from create output", func(t *testing.T) {
		output := []byte("Created tunnel g8e with id 12345678-1234-1234-1234-123456789abc")
		id := parseTunnelID(output)
		assert.Equal(t, "12345678-1234-1234-1234-123456789abc", id)
	})

	t.Run("returns empty for no UUID", func(t *testing.T) {
		output := []byte("No tunnel created")
		id := parseTunnelID(output)
		assert.Empty(t, id)
	})
}

func TestCloudflaredAuthenticated(t *testing.T) {
	t.Run("returns true when cert.pem exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("fake cert"), 0o600))

		assert.True(t, cloudflaredAuthenticated(tmpDir))
	})

	t.Run("returns false when cert.pem does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		assert.False(t, cloudflaredAuthenticated(tmpDir))
	})
}
