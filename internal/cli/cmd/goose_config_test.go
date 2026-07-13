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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestGooseGovernanceConfig(t *testing.T) {
	tempHome := testutil.TempDir(t)
	t.Setenv("HOME", tempHome)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	require.NotEmpty(t, homeDir)

	configDir := filepath.Join(homeDir, ".goose")
	err = os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	existingConfigPath := filepath.Join(configDir, "settings.json")
	existingData := []byte(`{"mcpServers": {"existing": {}}}`)
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

	var config struct {
		MCPServers   map[string]interface{} `json:"mcpServers"`
		ExcludeTools []string               `json:"excludeTools"`
	}
	err = json.Unmarshal(configData, &config)
	require.NoError(t, err)

	assert.Contains(t, config.MCPServers, "g8e")
	assert.Contains(t, config.ExcludeTools, "Bash")
	assert.Contains(t, config.ExcludeTools, "Write")
}
