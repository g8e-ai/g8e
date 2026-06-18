package cmd

import (
	"fmt"
	"os"

	"github.com/g8e-ai/g8e/internal/pathutil"
)

// BackupConfigFile creates a backup of the existing config file if it exists.
func BackupConfigFile(configPath string) error {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}

	backupPath := configPath + ".bak"
	// If a backup already exists, we could either overwrite it or fail.
	// Overwriting it is generally safer to ensure we always have the most recent pre-governance state.
	displayPath := pathutil.ToSlash(backupPath)
	fmt.Fprintf(os.Stderr, "[g8e] Backing up existing config to %s\n", displayPath)

	input, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config for backup: %w", err)
	}

	err = os.WriteFile(backupPath, input, 0644)
	if err != nil {
		return fmt.Errorf("write backup config: %w", err)
	}

	return nil
}
