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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestAgentLaunchArgs_ClaudeIncludesMcpConfigAndDisallowedTools(t *testing.T) {
	args, err := agentLaunchArgs("claude", "/tmp/mcp-config.json")
	require.NoError(t, err)
	assert.Contains(t, args, "--mcp-config")
	assert.Contains(t, args, "/tmp/mcp-config.json")
	assert.Contains(t, args, "--strict-mcp-config")
	assert.Contains(t, args, "--disallowed-tools")
}

func TestAgentLaunchArgs_CodexIncludesMcpConfigAndDisallowedTools(t *testing.T) {
	args, err := agentLaunchArgs("codex", "/tmp/mcp-config.json")
	require.NoError(t, err)
	assert.Contains(t, args, "--mcp-config")
	assert.Contains(t, args, "--strict-mcp-config")
	assert.Contains(t, args, "--disallowed-tools")
}

func TestAgentLaunchArgs_GooseReturnsNoProfileArgs(t *testing.T) {
	args, err := agentLaunchArgs("goose", "/tmp/mcp-config.json")
	require.NoError(t, err)
	assert.Contains(t, args, "session")
	assert.Contains(t, args, "--no-profile")
}

func TestAgentLaunchArgs_GeminiReturnsEmptyArgs(t *testing.T) {
	args, err := agentLaunchArgs("gemini", "/tmp/mcp-config.json")
	require.NoError(t, err)
	assert.Empty(t, args)
}

func TestAgentLaunchArgs_CursorReturnsError(t *testing.T) {
	_, err := agentLaunchArgs("cursor", "/tmp/mcp-config.json")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
}

func TestAgentLaunchArgs_DevinReturnsError(t *testing.T) {
	_, err := agentLaunchArgs("devin", "/tmp/mcp-config.json")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
}

func TestAgentLaunchArgs_AiderReturnsError(t *testing.T) {
	_, err := agentLaunchArgs("aider", "/tmp/mcp-config.json")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
}

func TestAgentLaunchArgs_OllamaReturnsError(t *testing.T) {
	_, err := agentLaunchArgs("ollama", "/tmp/mcp-config.json")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
}

func TestAgentLaunchArgs_UnknownAgentReturnsError(t *testing.T) {
	_, err := agentLaunchArgs("unknown-agent", "/tmp/mcp-config.json")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
}

func TestAgentLaunchArgs_IsCaseInsensitive(t *testing.T) {
	args, err := agentLaunchArgs("CLAUDE", "/tmp/mcp-config.json")
	require.NoError(t, err)
	assert.Contains(t, args, "--mcp-config")
}

func TestRunMCPAgentRun_NoArgsReturnsError(t *testing.T) {
	err := runMCPAgentRun(nil, "", false, newFileSvc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify an agent name")
}

func TestRunMCPAgentRun_UnknownAgentReturnsError(t *testing.T) {
	err := runMCPAgentRun([]string{"unknown-agent"}, "", false, newFileSvc)
	require.Error(t, err)
}

func TestGetSupportedAgents_ReturnsAllExpectedAgents(t *testing.T) {
	agents := getSupportedAgents()
	ids := make(map[string]bool)
	for _, a := range agents {
		ids[a.ID] = true
	}
	assert.True(t, ids["claude"])
	assert.True(t, ids["codex"])
	assert.True(t, ids["goose"])
	assert.True(t, ids["gemini"])
	assert.False(t, ids["cursor"])
	assert.False(t, ids["devin"])
	assert.False(t, ids["aider"])
	assert.False(t, ids["generic"])
}

func TestWriteAgentConfig_GooseWritesConfigFile(t *testing.T) {
	t.Setenv("HOME", testutil.TempDir(t))

	configPath, cleanup, err := WriteAgentConfig("goose", "/fake/g8e")
	require.NoError(t, err)
	if cleanup != nil {
		t.Cleanup(cleanup)
	}
	assert.FileExists(t, configPath)
}

func TestWriteAgentConfig_GeminiWritesSettingsFile(t *testing.T) {
	t.Setenv("HOME", testutil.TempDir(t))

	configPath, cleanup, err := WriteAgentConfig("gemini", "/fake/g8e")
	require.NoError(t, err)
	if cleanup != nil {
		t.Cleanup(cleanup)
	}
	assert.FileExists(t, configPath)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "g8e")
	assert.Contains(t, string(data), "tools")
	assert.Contains(t, string(data), "core")
}

func TestWriteAgentConfig_GeminiMergesExistingSettings(t *testing.T) {
	tmpHome := testutil.TempDir(t)
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	geminiDir := filepath.Join(tmpHome, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0o755))
	existingSettings := `{"mcpServers":{"other":{"command":"other-cmd","args":[]}}}`
	require.NoError(t, os.WriteFile(filepath.Join(geminiDir, "settings.json"), []byte(existingSettings), 0o644))

	configPath, cleanup, err := WriteAgentConfig("gemini", "/fake/g8e")
	require.NoError(t, err)
	if cleanup != nil {
		t.Cleanup(cleanup)
	}

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "g8e")
	assert.Contains(t, string(data), "other")
}

func TestExtractURLFromText_FindsApproveURL(t *testing.T) {
	text := `Transaction pending approval: https://g8e.local/approve/abc123 please review`
	url := extractURLFromText(text)
	assert.Contains(t, url, "https://g8e.local/approve/abc123")
}

func TestExtractURLFromText_FindsGenericURL(t *testing.T) {
	text := `Check this out: https://example.com/page`
	url := extractURLFromText(text)
	assert.Equal(t, "https://example.com/page", url)
}

func TestExtractURLFromText_ReturnsEmptyForNoURL(t *testing.T) {
	text := `No URL here`
	url := extractURLFromText(text)
	assert.Empty(t, url)
}

func TestExtractURLFromText_ReturnsEmptyForEmptyString(t *testing.T) {
	url := extractURLFromText("")
	assert.Empty(t, url)
}
