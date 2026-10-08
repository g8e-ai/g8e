// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func mustLookup(t *testing.T, id constants.AgentBinary) Integration {
	t.Helper()
	integration, err := Lookup(string(id))
	require.NoError(t, err)
	return integration
}

func TestStdioServerArgs_SelectsCredentialsByAppName(t *testing.T) {
	assert.Equal(t, []string{"mcp", "stdio", "--app", "claude"}, StdioServerArgs("claude"))
}

func TestWriteConfig_Goose_MergesExtensionAndPreservesExistingConfig(t *testing.T) {
	home := testutil.TempDir(t)
	configDir := filepath.Join(home, ".config", "goose")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	existingPath := filepath.Join(configDir, "config.yaml")
	existing := []byte("provider:\n  name: openai\n  model: gpt-4o\nextensions:\n  existing:\n    enabled: true\n    config:\n      type: stdio\n      name: existing\n      cmd: existing-cmd\n")
	require.NoError(t, os.WriteFile(existingPath, existing, 0o644))

	configPath, cleanup, err := mustLookup(t, constants.AgentBinaryGoose).WriteConfig(home, "/fake/g8e", "goose")
	require.NoError(t, err)
	assert.Nil(t, cleanup, "goose config is the user's real config, not a throwaway")
	assert.Equal(t, existingPath, configPath)

	backup, err := os.ReadFile(configPath + ".bak")
	require.NoError(t, err)
	assert.Equal(t, existing, backup)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var cfg gooseConfig
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	g8e := cfg.Extensions[constants.MCPServerNameG8E]
	assert.True(t, g8e.Enabled)
	assert.Equal(t, "/fake/g8e", g8e.Config.Cmd)
	assert.Equal(t, []string{"mcp", "stdio", "--app", "goose"}, g8e.Config.Args)
	assert.Contains(t, cfg.Extensions, "existing", "other extensions must survive the merge")

	var raw map[string]any
	require.NoError(t, yaml.Unmarshal(data, &raw))
	provider, ok := raw["provider"].(map[string]any)
	require.True(t, ok, "provider settings must survive the merge")
	assert.Equal(t, "openai", provider["name"])
}

func TestWriteConfig_Gemini_DisablesBuiltinToolsAndMergesExistingServers(t *testing.T) {
	home := testutil.TempDir(t)
	geminiDir := filepath.Join(home, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(geminiDir, "settings.json"),
		[]byte(`{"mcpServers":{"other":{"command":"other-cmd","args":[]}}}`), 0o644))

	configPath, _, err := mustLookup(t, constants.AgentBinaryGemini).WriteConfig(home, "/fake/g8e", "gemini")
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var settings geminiSettings
	require.NoError(t, json.Unmarshal(data, &settings))
	assert.Contains(t, settings.MCPServers, "other")
	assert.Equal(t, []string{"mcp", "stdio", "--app", "gemini"}, settings.MCPServers[constants.MCPServerNameG8E].Args)
	require.NotNil(t, settings.Tools)
	assert.NotNil(t, settings.Tools.Core)
	assert.Empty(t, settings.Tools.Core)
}

func TestWriteConfig_Gemini_RejectsCorruptExistingSettings(t *testing.T) {
	home := testutil.TempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".gemini"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gemini", "settings.json"), []byte(`{not json`), 0o644))

	_, _, err := mustLookup(t, constants.AgentBinaryGemini).WriteConfig(home, "/fake/g8e", "gemini")
	require.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestWriteConfig_Devin_WritesOnlyTheG8EServer(t *testing.T) {
	home := testutil.TempDir(t)

	configPath, _, err := mustLookup(t, constants.AgentBinaryDevin).WriteConfig(home, "/fake/g8e", "devin")
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var cfg mcpConfig
	require.NoError(t, json.Unmarshal(data, &cfg))
	require.Len(t, cfg.MCPServers, 1)
	assert.Equal(t, []string{"mcp", "stdio", "--app", "devin"}, cfg.MCPServers[constants.MCPServerNameG8E].Args,
		"the agent's stdio bridge must select its credentials by app name, not env or paths")
}

func TestWriteConfig_TempJSON_ListsNativeToolsAndCleansUp(t *testing.T) {
	for _, id := range []constants.AgentBinary{constants.AgentBinaryClaude, constants.AgentBinaryCodex} {
		t.Run(string(id), func(t *testing.T) {
			configPath, cleanup, err := mustLookup(t, id).WriteConfig(testutil.TempDir(t), "/fake/g8e", string(id))
			require.NoError(t, err)
			require.NotNil(t, cleanup)

			data, err := os.ReadFile(configPath)
			require.NoError(t, err)
			var cfg mcpConfig
			require.NoError(t, json.Unmarshal(data, &cfg))
			assert.Equal(t, nativeToolsToDisable, cfg.ExcludeTools)
			assert.Contains(t, cfg.MCPServers, constants.MCPServerNameG8E)

			cleanup()
			_, err = os.Stat(configPath)
			assert.True(t, errors.Is(err, os.ErrNotExist))
		})
	}
}

func TestWriteConfig_UnknownStrategyFailsClosed(t *testing.T) {
	_, _, err := Integration{ID: "bogus", ConfigStrategy: "bogus"}.WriteConfig(testutil.TempDir(t), "/fake/g8e", "bogus")
	require.ErrorIs(t, err, constants.ErrAgentNotSupported)
}

func TestLaunchArgs(t *testing.T) {
	tests := []struct {
		id   constants.AgentBinary
		want []string
	}{
		{constants.AgentBinaryClaude, []string{"--mcp-config", "/tmp/cfg.json", "--strict-mcp-config", "--disallowed-tools", strings.Join(nativeToolsToDisable, ",")}},
		{constants.AgentBinaryCodex, []string{"--mcp-config", "/tmp/cfg.json", "--strict-mcp-config", "--disallowed-tools", strings.Join(nativeToolsToDisable, ",")}},
		{constants.AgentBinaryGoose, []string{"session", "--no-profile", "--with-extension", "/fake/g8e mcp stdio --app goose"}},
		{constants.AgentBinaryGemini, []string{}},
		{constants.AgentBinaryDevin, []string{}},
	}
	for _, tt := range tests {
		t.Run(string(tt.id), func(t *testing.T) {
			got, err := mustLookup(t, tt.id).LaunchArgs("/tmp/cfg.json", "/fake/g8e", string(tt.id))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNativeToolsToDisable_CoversEveryExecutionAndEgressTool(t *testing.T) {
	required := []string{
		"Bash", "PowerShell", "REPL", "Monitor",
		"Read", "Write", "Edit", "MultiEdit", "NotebookEdit", "Glob", "Grep",
		"EnterWorktree", "ExitWorktree",
		"WebSearch", "WebFetch", "PushNotification", "RemoteTrigger",
		"Artifact", "ArtifactComments", "ArtifactData", "DesignSync",
		"Agent", "Skill", "ListAgents", "SendMessage",
		"CronCreate", "CronDelete", "CronList", "ScheduleWakeup",
	}
	assert.ElementsMatch(t, required, nativeToolsToDisable,
		"every built-in that can execute, touch files, reach the network, spawn agents, or schedule work must be disallowed")
}

func TestLaunchArgs_UnknownStrategyFailsClosed(t *testing.T) {
	_, err := Integration{ID: "bogus", LaunchStrategy: "bogus"}.LaunchArgs("/tmp/cfg.json", "/fake/g8e", "bogus")
	require.ErrorIs(t, err, constants.ErrAgentNotSupported)
}

func TestResolveHomeDir_PrefersUserHomeDir(t *testing.T) {
	home := testutil.TempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := ResolveHomeDir()
	require.NoError(t, err)
	assert.Equal(t, home, got)
}
