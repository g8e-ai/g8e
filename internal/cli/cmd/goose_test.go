package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGooseGovernanceConfig(t *testing.T) {
	// Setup a fake home directory
	tmpDir, err := os.MkdirTemp("", "g8e-goose-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	os.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".goose")
	err = os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	// Create an existing config to test the backup mechanism
	existingConfigPath := filepath.Join(configDir, "config.json")
	existingData := []byte(`{"mcpServers": {"existing": {}}}`)
	err = os.WriteFile(existingConfigPath, existingData, 0644)
	require.NoError(t, err)

	// Call writeAgentConfig for goose
	configPath, cleanup, err := writeAgentConfig("goose", "/path/to/g8e")
	require.NoError(t, err)
	if cleanup != nil {
		defer cleanup()
	}
	assert.Equal(t, existingConfigPath, configPath)

	// Verify Backup
	backupPath := configPath + ".bak"
	_, err = os.Stat(backupPath)
	assert.NoError(t, err, "Backup file should exist")
	
	backupData, err := os.ReadFile(backupPath)
	assert.NoError(t, err)
	assert.Equal(t, existingData, backupData)

	// Verify Governance (excludeTools)
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
