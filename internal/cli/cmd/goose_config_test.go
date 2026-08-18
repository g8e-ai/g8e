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

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGooseGovernanceConfig(t *testing.T) {
	tempHome := testutil.TempDir(t)
	t.Setenv("HOME", tempHome)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	require.NotEmpty(t, homeDir)

	configDir := filepath.Join(homeDir, ".config", "goose")
	err = os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	existingConfigPath := filepath.Join(configDir, "config.yaml")
	existingData := []byte("provider:\n  name: openai\n  model: gpt-4o\nextensions:\n  existing:\n    enabled: true\n    config:\n      type: stdio\n      name: existing\n      cmd: existing-cmd\n")
	err = os.WriteFile(existingConfigPath, existingData, 0644)
	require.NoError(t, err)

	binaryPath, err := os.Executable()
	require.NoError(t, err)

	configPath, cleanup, err := WriteAgentConfig("goose", binaryPath)
	require.NoError(t, err)
	if cleanup != nil {
		defer cleanup()
	}
	assert.Equal(t, existingConfigPath, configPath)

	backupPath := configPath + ".bak"
	_, err = os.Stat(backupPath)
	require.NoError(t, err, "Backup file should exist")

	backupData, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, existingData, backupData)

	configData, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var cfg gooseConfig
	err = yaml.Unmarshal(configData, &cfg)
	require.NoError(t, err)

	assert.Contains(t, cfg.Extensions, "g8e")
	assert.True(t, cfg.Extensions["g8e"].Enabled)
	assert.Equal(t, "stdio", cfg.Extensions["g8e"].Config.Type)
	assert.Equal(t, "g8e", cfg.Extensions["g8e"].Config.Name)
	assert.Equal(t, binaryPath, cfg.Extensions["g8e"].Config.Cmd)
	assert.Equal(t, []string{"mcp", "stdio"}, cfg.Extensions["g8e"].Config.Args)

	// Existing extension must be preserved (not wiped out)
	assert.Contains(t, cfg.Extensions, "existing", "existing extension should be preserved")
	assert.True(t, cfg.Extensions["existing"].Enabled)

	// Provider and other non-extension fields must be preserved
	var raw map[string]any
	err = yaml.Unmarshal(configData, &raw)
	require.NoError(t, err)
	provider, ok := raw["provider"].(map[string]any)
	require.True(t, ok, "provider field should be preserved")
	assert.Equal(t, "openai", provider["name"])
	assert.Equal(t, "gpt-4o", provider["model"])
}

func TestGooseLaunchArgs_NoProfile(t *testing.T) {
	args, err := agentLaunchArgs("goose", "/tmp/mcp-config.json", "/fake/g8e")
	require.NoError(t, err)
	assert.Contains(t, args, "session")
	assert.Contains(t, args, "--no-profile")
	assert.Contains(t, args, "--with-extension")
}
