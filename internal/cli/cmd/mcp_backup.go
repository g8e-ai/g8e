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
