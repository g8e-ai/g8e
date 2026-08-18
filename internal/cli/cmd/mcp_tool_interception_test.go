// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// ─── verifyToolInterception dispatch ──────────────────────────────────────────

func TestVerifyToolInterception_UnsupportedAgent(t *testing.T) {
	t.Run("returns ErrAgentNotSupported for unknown agent", func(t *testing.T) {
		err := verifyToolInterception("cursor", "/tmp/config.json", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
	})
}

// ─── verifyClaudeCodexInterception ────────────────────────────────────────────

func TestVerifyClaudeCodexInterception_ValidConfig(t *testing.T) {
	t.Run("passes with valid config and flags", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := writeTestMCPConfig(t, tmpDir)

		launchArgs := []string{
			"--mcp-config", configPath,
			"--strict-mcp-config",
			"--disallowed-tools", "Bash,Read,Write",
		}

		err := verifyClaudeCodexInterception(configPath, launchArgs)
		require.NoError(t, err)
	})
}

func TestVerifyClaudeCodexInterception_MissingConfigFile(t *testing.T) {
	t.Run("fails when config file does not exist", func(t *testing.T) {
		missingPath := filepath.Join(testutil.TempDir(t), "nonexistent.json")
		err := verifyClaudeCodexInterception(missingPath, []string{"--strict-mcp-config", "--disallowed-tools", "Bash"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read mcp config")
	})
}

func TestVerifyClaudeCodexInterception_MissingG8EServer(t *testing.T) {
	t.Run("fails when config has no g8e MCP server entry", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "mcp-config.json")
		require.NoError(t, os.WriteFile(configPath, []byte(`{"mcpServers":{}}`), 0644))

		err := verifyClaudeCodexInterception(configPath, []string{"--strict-mcp-config", "--disallowed-tools", "Bash"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing g8e server entry")
	})
}

func TestVerifyClaudeCodexInterception_MissingStrictMcpConfig(t *testing.T) {
	t.Run("fails when --strict-mcp-config flag is missing", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := writeTestMCPConfig(t, tmpDir)

		launchArgs := []string{
			"--mcp-config", configPath,
			"--disallowed-tools", "Bash",
		}

		err := verifyClaudeCodexInterception(configPath, launchArgs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing --strict-mcp-config")
	})
}

func TestVerifyClaudeCodexInterception_MissingDisallowedTools(t *testing.T) {
	t.Run("fails when --disallowed-tools flag is missing", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := writeTestMCPConfig(t, tmpDir)

		launchArgs := []string{
			"--mcp-config", configPath,
			"--strict-mcp-config",
		}

		err := verifyClaudeCodexInterception(configPath, launchArgs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing --disallowed-tools")
	})
}

func TestVerifyClaudeCodexInterception_InvalidJSON(t *testing.T) {
	t.Run("fails when config file contains invalid JSON", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "mcp-config.json")
		require.NoError(t, os.WriteFile(configPath, []byte(`{invalid json`), 0644))

		err := verifyClaudeCodexInterception(configPath, []string{"--strict-mcp-config", "--disallowed-tools", "Bash"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse mcp config")
	})
}

// ─── verifyGooseInterception ──────────────────────────────────────────────────

func TestVerifyGooseInterception_ValidConfig(t *testing.T) {
	t.Run("passes with --no-profile and --with-extension flags and valid config", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := writeTestGooseConfig(t, tmpDir)

		launchArgs := []string{"session", "--no-profile", "--with-extension", "/fake/g8e mcp stdio"}

		err := verifyGooseInterception(configPath, launchArgs)
		require.NoError(t, err)
	})
}

func TestVerifyGooseInterception_MissingNoProfile(t *testing.T) {
	t.Run("fails when --no-profile flag is missing", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := writeTestGooseConfig(t, tmpDir)

		launchArgs := []string{"session", "--with-extension", "/fake/g8e mcp stdio"}

		err := verifyGooseInterception(configPath, launchArgs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing --no-profile")
	})
}

func TestVerifyGooseInterception_MissingWithExtension(t *testing.T) {
	t.Run("fails when --with-extension flag is missing", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := writeTestGooseConfig(t, tmpDir)

		launchArgs := []string{"session", "--no-profile"}

		err := verifyGooseInterception(configPath, launchArgs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing --with-extension")
	})
}

func TestVerifyGooseInterception_MissingG8EExtension(t *testing.T) {
	t.Run("fails when config has no g8e extension entry", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("extensions: {}\n"), 0644))

		err := verifyGooseInterception(configPath, []string{"session", "--no-profile", "--with-extension", "/fake/g8e mcp stdio"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing g8e extension entry")
	})
}

func TestVerifyGooseInterception_MissingConfigFile(t *testing.T) {
	t.Run("fails when config file does not exist", func(t *testing.T) {
		missingPath := filepath.Join(testutil.TempDir(t), "nonexistent.yaml")
		err := verifyGooseInterception(missingPath, []string{"session", "--no-profile", "--with-extension", "/fake/g8e mcp stdio"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read goose config")
	})
}

// writeTestGooseConfig writes a valid goose config.yaml with g8e as the sole extension.
func writeTestGooseConfig(t *testing.T, dir string) string {
	t.Helper()
	configPath := filepath.Join(dir, "config.yaml")
	cfg := gooseConfig{
		Extensions: map[string]gooseExtension{
			"g8e": {
				Enabled: true,
				Config: gooseExtConfig{
					Type: "stdio",
					Name: "g8e",
					Cmd:  "g8e",
					Args: []string{"mcp", "stdio"},
				},
			},
		},
	}
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0644))
	return configPath
}

// ─── verifyDevinInterception ──────────────────────────────────────────────────

func TestVerifyDevinInterception_ValidConfig(t *testing.T) {
	t.Run("passes with g8e MCP server entry", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := writeTestDevinConfig(t, tmpDir)

		err := verifyDevinInterception(configPath)
		require.NoError(t, err)
	})
}

func TestVerifyDevinInterception_MissingConfigFile(t *testing.T) {
	t.Run("fails when config file does not exist", func(t *testing.T) {
		missingPath := filepath.Join(testutil.TempDir(t), "nonexistent.json")
		err := verifyDevinInterception(missingPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read devin config")
	})
}

func TestVerifyDevinInterception_MissingG8EServer(t *testing.T) {
	t.Run("fails when config has no g8e MCP server entry", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "config.json")
		require.NoError(t, os.WriteFile(configPath, []byte(`{"mcpServers":{}}`), 0644))

		err := verifyDevinInterception(configPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing g8e MCP server entry")
	})
}

func TestVerifyDevinInterception_InvalidJSON(t *testing.T) {
	t.Run("fails when config file contains invalid JSON", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "config.json")
		require.NoError(t, os.WriteFile(configPath, []byte(`{invalid json`), 0644))

		err := verifyDevinInterception(configPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse devin config")
	})
}

// writeTestDevinConfig writes a valid Devin config.json with g8e as the sole MCP server.
func writeTestDevinConfig(t *testing.T, dir string) string {
	t.Helper()
	configPath := filepath.Join(dir, "config.json")
	cfg := agentMCPConfig{
		MCPServers: map[string]agentMCPServer{
			"g8e": {
				Command: "g8e",
				Args:    []string{"mcp", "stdio"},
			},
		},
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0644))
	return configPath
}

// ─── verifyGeminiInterception ─────────────────────────────────────────────────

func TestVerifyGeminiInterception_ValidConfig(t *testing.T) {
	t.Run("passes with tools.core empty array and g8e MCP server", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := writeTestGeminiSettings(t, tmpDir, true, true)

		err := verifyGeminiInterception(configPath)
		require.NoError(t, err)
	})
}

func TestVerifyGeminiInterception_MissingToolsConfig(t *testing.T) {
	t.Run("fails when tools configuration is missing", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "settings.json")
		settings := `{"mcpServers":{"g8e":{"command":"g8e","args":["mcp","stdio"]}}}`
		require.NoError(t, os.WriteFile(configPath, []byte(settings), 0644))

		err := verifyGeminiInterception(configPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing tools.core")
	})
}

func TestVerifyGeminiInterception_CoreHasEntries(t *testing.T) {
	t.Run("fails when tools.core has entries instead of empty array", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "settings.json")
		settings := `{"mcpServers":{"g8e":{"command":"g8e","args":["mcp","stdio"]}},"tools":{"core":["read_file","write_file"]}}`
		require.NoError(t, os.WriteFile(configPath, []byte(settings), 0644))

		err := verifyGeminiInterception(configPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tools.core has 2 entries")
	})
}

func TestVerifyGeminiInterception_CoreIsNull(t *testing.T) {
	t.Run("fails when tools.core is null", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "settings.json")
		settings := `{"mcpServers":{"g8e":{"command":"g8e","args":["mcp","stdio"]}},"tools":{"exclude":["read_file"]}}`
		require.NoError(t, os.WriteFile(configPath, []byte(settings), 0644))

		err := verifyGeminiInterception(configPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tools.core is null")
	})
}

func TestVerifyGeminiInterception_MissingG8EServer(t *testing.T) {
	t.Run("fails when g8e MCP server is missing", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "settings.json")
		settings := `{"mcpServers":{},"tools":{"core":[]}}`
		require.NoError(t, os.WriteFile(configPath, []byte(settings), 0644))

		err := verifyGeminiInterception(configPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing g8e MCP server entry")
	})
}

func TestVerifyGeminiInterception_MissingConfigFile(t *testing.T) {
	t.Run("fails when settings file does not exist", func(t *testing.T) {
		missingPath := filepath.Join(testutil.TempDir(t), "nonexistent.json")
		err := verifyGeminiInterception(missingPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read gemini config")
	})
}

func TestVerifyGeminiInterception_InvalidJSON(t *testing.T) {
	t.Run("fails when settings file contains invalid JSON", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		configPath := filepath.Join(tmpDir, "settings.json")
		require.NoError(t, os.WriteFile(configPath, []byte(`{invalid json`), 0644))

		err := verifyGeminiInterception(configPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse gemini config")
	})
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// writeTestMCPConfig writes a valid MCP config with g8e as the sole server.
func writeTestMCPConfig(t *testing.T, dir string) string {
	t.Helper()
	configPath := filepath.Join(dir, "mcp-config.json")
	cfg := agentMCPConfig{
		MCPServers: map[string]agentMCPServer{
			"g8e": {
				Command: "g8e",
				Args:    []string{"mcp", "stdio"},
			},
		},
		ExcludeTools: nativeToolsToDisable,
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0644))
	return configPath
}

// writeTestGeminiSettings writes a valid Gemini settings.json with tools.core
// set to an empty array and g8e as the sole MCP server.
func writeTestGeminiSettings(t *testing.T, dir string, withTools, withG8E bool) string {
	t.Helper()
	configPath := filepath.Join(dir, "settings.json")

	settings := geminiSettings{}
	if withG8E {
		settings.MCPServers = map[string]agentMCPServer{
			"g8e": {
				Command: "g8e",
				Args:    []string{"mcp", "stdio"},
			},
		}
	}
	if withTools {
		settings.Tools = &geminiToolsConfig{
			Core: []string{},
		}
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0644))
	return configPath
}
