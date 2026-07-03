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
)

func TestBackupConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "test.json")
	err := os.WriteFile(configPath, []byte("{}"), 0644)
	require.NoError(t, err)

	err = BackupConfigFile(configPath)
	require.NoError(t, err)

	backupPath := configPath + ".bak"
	_, err = os.Stat(backupPath)
	require.NoError(t, err, "Backup file should exist")

	content, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, "{}", string(content))
}

func TestBackupConfigFile_NoExistingFile(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "nonexistent.json")
	err := BackupConfigFile(configPath)
	require.NoError(t, err)

	backupPath := configPath + ".bak"
	_, err = os.Stat(backupPath)
	assert.True(t, os.IsNotExist(err), "backup should not be created when source does not exist")
}
