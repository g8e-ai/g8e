package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupConfigFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "g8e-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "test.json")
	err = os.WriteFile(configPath, []byte("{}"), 0644)
	require.NoError(t, err)

	err = backupConfigFile(configPath)
	require.NoError(t, err)

	backupPath := configPath + ".bak"
	_, err = os.Stat(backupPath)
	require.NoError(t, err, "Backup file should exist")

	content, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, "{}", string(content))
}
