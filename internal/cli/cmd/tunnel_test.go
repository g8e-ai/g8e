// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
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

		expectedSubcommands := []string{"create", "route-dns", "run", "status"}
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

		expectedFlags := []string{"name", "hostname", "config-dir", "https-port", "service", "ca-bundle", "origin-server-name", "skip-dns"}
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

func TestBuildTunnelRunArgs_ConfigPrecedesTunnelSubcommand(t *testing.T) {
	tests := []struct {
		name      string
		tunnel    string
		configDir string
		expected  []string
	}{
		{
			name:      "explicit config directory",
			tunnel:    "opendevops-feed",
			configDir: "/home/user/.cloudflared",
			expected:  []string{"--config", "/home/user/.cloudflared/config.yml", "tunnel", "run", "opendevops-feed"},
		},
		{
			name:     "default config discovery",
			tunnel:   "g8e",
			expected: []string{"tunnel", "run", "g8e"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, buildTunnelRunArgs(tt.tunnel, tt.configDir))
		})
	}
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
			"https://localhost:8443",
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
		caBundle := filepath.Join(constants.RuntimeDirname, constants.PkiDirname, constants.PkiFileGatewayBundle)
		config := generateTunnelConfig(
			"abc123",
			"/home/user/.cloudflared/abc123.json",
			"console.g8e.ai",
			"https://localhost:8443",
			caBundle,
			"g8e.local",
		)

		assert.Contains(t, config, "originCaPool: "+caBundle)
		assert.Contains(t, config, "originServerName: g8e.local")
		assert.NotContains(t, config, "noTLSVerify")
	})

	t.Run("uses custom HTTPS port", func(t *testing.T) {
		config := generateTunnelConfig(
			"abc123",
			"/home/user/.cloudflared/abc123.json",
			"console.g8e.ai",
			"https://localhost:9443",
			"",
			"",
		)

		assert.Contains(t, config, "service: https://localhost:9443")
	})

	t.Run("uses plain HTTP mirror service without TLS origin settings", func(t *testing.T) {
		config := generateTunnelConfig(
			"abc123",
			"/home/user/.cloudflared/abc123.json",
			"feed.opendevops.ai",
			"http://localhost:8081",
			"",
			"",
		)

		assert.Contains(t, config, "service: http://localhost:8081")
		assert.NotContains(t, config, "originRequest:")
		assert.NotContains(t, config, "noTLSVerify")
		assert.NotContains(t, config, "http2Origin")
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
		tmpDir := testutil.TempDir(t)
		certPath := filepath.Join(tmpDir, "cert.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("fake cert"), 0o600))

		assert.True(t, cloudflaredAuthenticated(tmpDir))
	})

	t.Run("returns false when cert.pem does not exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		assert.False(t, cloudflaredAuthenticated(tmpDir))
	})
}
