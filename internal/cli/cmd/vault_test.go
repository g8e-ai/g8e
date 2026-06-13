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
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultCmd(t *testing.T) {
	t.Run("vault command has correct use and description", func(t *testing.T) {
		cmd := vaultCmd()
		assert.Equal(t, "vault", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage the encryption vault")
		assert.Contains(t, cmd.Long, "Initialize, unlock, re-key")
	})

	t.Run("vault command has all subcommands", func(t *testing.T) {
		cmd := vaultCmd()
		subcommands := cmd.Commands()
		subcommandNames := make(map[string]bool)
		for _, sub := range subcommands {
			subcommandNames[sub.Use] = true
		}

		assert.True(t, subcommandNames["init"], "missing init subcommand")
		assert.True(t, subcommandNames["unlock"], "missing unlock subcommand")
		assert.True(t, subcommandNames["rekey"], "missing rekey subcommand")
		assert.True(t, subcommandNames["status"], "missing status subcommand")
		assert.True(t, subcommandNames["reset"], "missing reset subcommand")
		assert.True(t, subcommandNames["export"], "missing export subcommand")
		assert.True(t, subcommandNames["import"], "missing import subcommand")
	})
}

func TestReadKeyFile(t *testing.T) {
	t.Run("reads valid hex key file", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "key")

		testKey := make([]byte, vault.KeySize)
		_, err := rand.Read(testKey)
		require.NoError(t, err)

		keyHex := hex.EncodeToString(testKey) + "\n"
		require.NoError(t, os.WriteFile(keyPath, []byte(keyHex), 0600))

		loadedKey, err := readKeyFile(keyPath)
		require.NoError(t, err)
		assert.Equal(t, testKey, loadedKey)
	})

	t.Run("trims whitespace from key", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "key")

		testKey := make([]byte, vault.KeySize)
		_, err := rand.Read(testKey)
		require.NoError(t, err)

		keyHex := "  " + hex.EncodeToString(testKey) + "  \n"
		require.NoError(t, os.WriteFile(keyPath, []byte(keyHex), 0600))

		loadedKey, err := readKeyFile(keyPath)
		require.NoError(t, err)
		assert.Equal(t, testKey, loadedKey)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := readKeyFile("/nonexistent/key/file")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read key file")
	})

	t.Run("returns error for invalid hex", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "key")

		require.NoError(t, os.WriteFile(keyPath, []byte("not-valid-hex\n"), 0600))

		_, err := readKeyFile(keyPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode key")
	})

	t.Run("returns error for wrong key size", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "key")

		shortKey := make([]byte, 16)
		_, err := rand.Read(shortKey)
		require.NoError(t, err)

		keyHex := hex.EncodeToString(shortKey) + "\n"
		require.NoError(t, os.WriteFile(keyPath, []byte(keyHex), 0600))

		_, err = readKeyFile(keyPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid key size")
	})
}

func TestVaultInitCmd(t *testing.T) {
	t.Run("init command has correct use and description", func(t *testing.T) {
		cmd := vaultInitCmd()
		assert.Equal(t, "init", cmd.Use)
		assert.Contains(t, cmd.Short, "Initialize a new encryption vault")
		assert.Contains(t, cmd.Long, "Generate a new encryption vault")
	})

	t.Run("init has vault-dir and key-path flags", func(t *testing.T) {
		cmd := vaultInitCmd()
		vaultDirFlag := cmd.Flags().Lookup("vault-dir")
		keyPathFlag := cmd.Flags().Lookup("key-path")

		assert.NotNil(t, vaultDirFlag)
		assert.NotNil(t, keyPathFlag)
	})

	t.Run("init creates vault with default paths", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		cmd := vaultInitCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Vault initialized")
		assert.Contains(t, output, "WARNING: Back up this key")

		vaultDir := filepath.Join(tmpDir, constants.Paths.Infra.VaultDir)
		assert.True(t, vault.VaultHeaderExists(vaultDir))

		keyPath := filepath.Join(vaultDir, "key")
		_, err = os.Stat(keyPath)
		require.NoError(t, err)
	})

	t.Run("init creates vault with custom vault-dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		customVaultDir := filepath.Join(tmpDir, "custom-vault")

		cmd := vaultInitCmd()
		cmd.Flags().Set("vault-dir", customVaultDir)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		assert.True(t, vault.VaultHeaderExists(customVaultDir))
	})

	t.Run("init creates vault with custom key-path", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		customKeyPath := filepath.Join(tmpDir, "custom-key")

		cmd := vaultInitCmd()
		cmd.Flags().Set("key-path", customKeyPath)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		_, err = os.Stat(customKeyPath)
		require.NoError(t, err)
	})

	t.Run("init fails when vault already exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		cmd := vaultInitCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vault already initialized")
	})

	t.Run("init resolves relative vault-dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		cmd := vaultInitCmd()
		cmd.Flags().Set("vault-dir", "relative-vault")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		expectedVaultDir := filepath.Join(tmpDir, "relative-vault")
		assert.True(t, vault.VaultHeaderExists(expectedVaultDir))
	})
}

func TestVaultUnlockCmd(t *testing.T) {
	t.Run("unlock command has correct use and description", func(t *testing.T) {
		cmd := vaultUnlockCmd()
		assert.Equal(t, "unlock", cmd.Use)
		assert.Contains(t, cmd.Short, "Unlock the encryption vault")
		assert.Contains(t, cmd.Long, "Unlock an existing vault")
	})

	t.Run("unlock has vault-dir and key-path flags", func(t *testing.T) {
		cmd := vaultUnlockCmd()
		vaultDirFlag := cmd.Flags().Lookup("vault-dir")
		keyPathFlag := cmd.Flags().Lookup("key-path")

		assert.NotNil(t, vaultDirFlag)
		assert.NotNil(t, keyPathFlag)
	})

	t.Run("unlock succeeds with valid key", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		initCmd := vaultInitCmd()
		var initBuf bytes.Buffer
		initCmd.SetOut(&initBuf)
		initCmd.SetErr(&initBuf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := initCmd.RunE(initCmd, []string{})
		require.NoError(t, err)

		unlockCmd := vaultUnlockCmd()
		var unlockBuf bytes.Buffer
		unlockCmd.SetOut(&unlockBuf)
		unlockCmd.SetErr(&unlockBuf)

		err = unlockCmd.RunE(unlockCmd, []string{})
		require.NoError(t, err)

		output := unlockBuf.String()
		assert.Contains(t, output, "Vault unlocked successfully")
	})

	t.Run("unlock fails when vault not initialized", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		cmd := vaultUnlockCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vault not initialized")
	})

	t.Run("unlock fails with invalid key", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		vaultDir := filepath.Join(tmpDir, constants.Paths.Infra.VaultDir)
		keyPath := filepath.Join(vaultDir, "key")

		initCmd := vaultInitCmd()
		initCmd.Flags().Set("vault-dir", vaultDir)
		initCmd.Flags().Set("key-path", keyPath)
		var initBuf bytes.Buffer
		initCmd.SetOut(&initBuf)
		initCmd.SetErr(&initBuf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := initCmd.RunE(initCmd, []string{})
		require.NoError(t, err)

		wrongKey := make([]byte, vault.KeySize)
		_, err = rand.Read(wrongKey)
		require.NoError(t, err)

		err = os.WriteFile(keyPath, []byte(hex.EncodeToString(wrongKey)+"\n"), 0600)
		require.NoError(t, err)

		unlockCmd := vaultUnlockCmd()
		unlockCmd.Flags().Set("vault-dir", vaultDir)
		unlockCmd.Flags().Set("key-path", keyPath)
		var unlockBuf bytes.Buffer
		unlockCmd.SetOut(&unlockBuf)
		unlockCmd.SetErr(&unlockBuf)

		err = unlockCmd.RunE(unlockCmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unlock vault")
	})

	t.Run("unlock fails with missing key file", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		vaultDir := filepath.Join(tmpDir, constants.Paths.Infra.VaultDir)
		keyPath := filepath.Join(vaultDir, "key")

		initCmd := vaultInitCmd()
		initCmd.Flags().Set("vault-dir", vaultDir)
		initCmd.Flags().Set("key-path", keyPath)
		var initBuf bytes.Buffer
		initCmd.SetOut(&initBuf)
		initCmd.SetErr(&initBuf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := initCmd.RunE(initCmd, []string{})
		require.NoError(t, err)

		err = os.Remove(keyPath)
		require.NoError(t, err)

		unlockCmd := vaultUnlockCmd()
		unlockCmd.Flags().Set("vault-dir", vaultDir)
		unlockCmd.Flags().Set("key-path", keyPath)
		var unlockBuf bytes.Buffer
		unlockCmd.SetOut(&unlockBuf)
		unlockCmd.SetErr(&unlockBuf)

		err = unlockCmd.RunE(unlockCmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read key file")
	})
}

func TestVaultRekeyCmd(t *testing.T) {
	t.Run("rekey command has correct use and description", func(t *testing.T) {
		cmd := vaultRekeyCmd()
		assert.Equal(t, "rekey", cmd.Use)
		assert.Contains(t, cmd.Short, "Re-key the vault")
		assert.Contains(t, cmd.Long, "Re-encrypt the vault's DEK")
	})

	t.Run("rekey has vault-dir, key-path, and new-key-path flags", func(t *testing.T) {
		cmd := vaultRekeyCmd()
		vaultDirFlag := cmd.Flags().Lookup("vault-dir")
		keyPathFlag := cmd.Flags().Lookup("key-path")
		newKeyPathFlag := cmd.Flags().Lookup("new-key-path")

		assert.NotNil(t, vaultDirFlag)
		assert.NotNil(t, keyPathFlag)
		assert.NotNil(t, newKeyPathFlag)
	})

	t.Run("rekey succeeds with valid keys", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		vaultDir := filepath.Join(tmpDir, constants.Paths.Infra.VaultDir)
		keyPath := filepath.Join(vaultDir, "key")

		initCmd := vaultInitCmd()
		var initBuf bytes.Buffer
		initCmd.SetOut(&initBuf)
		initCmd.SetErr(&initBuf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := initCmd.RunE(initCmd, []string{})
		require.NoError(t, err)

		rekeyCmd := vaultRekeyCmd()
		var rekeyBuf bytes.Buffer
		rekeyCmd.SetOut(&rekeyBuf)
		rekeyCmd.SetErr(&rekeyBuf)

		err = rekeyCmd.RunE(rekeyCmd, []string{})
		require.NoError(t, err)

		output := rekeyBuf.String()
		assert.Contains(t, output, "Vault rekeyed successfully")
		assert.Contains(t, output, "WARNING: The old key is no longer valid")

		newKeyPath := keyPath + ".new"
		_, err = os.Stat(newKeyPath)
		require.NoError(t, err)
	})

	t.Run("rekey fails when vault not initialized", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		cmd := vaultRekeyCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vault not initialized")
	})

	t.Run("rekey with custom new-key-path", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		customNewKeyPath := filepath.Join(tmpDir, "custom-new-key")

		initCmd := vaultInitCmd()
		var initBuf bytes.Buffer
		initCmd.SetOut(&initBuf)
		initCmd.SetErr(&initBuf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := initCmd.RunE(initCmd, []string{})
		require.NoError(t, err)

		rekeyCmd := vaultRekeyCmd()
		rekeyCmd.Flags().Set("new-key-path", customNewKeyPath)
		var rekeyBuf bytes.Buffer
		rekeyCmd.SetOut(&rekeyBuf)
		rekeyCmd.SetErr(&rekeyBuf)

		err = rekeyCmd.RunE(rekeyCmd, []string{})
		require.NoError(t, err)

		_, err = os.Stat(customNewKeyPath)
		require.NoError(t, err)
	})
}

func TestVaultStatusCmd(t *testing.T) {
	t.Run("status command has correct use and description", func(t *testing.T) {
		cmd := vaultStatusCmd()
		assert.Equal(t, "status", cmd.Use)
		assert.Contains(t, cmd.Short, "Show vault status")
		assert.Contains(t, cmd.Long, "Display whether the vault is initialized")
	})

	t.Run("status has vault-dir flag", func(t *testing.T) {
		cmd := vaultStatusCmd()
		vaultDirFlag := cmd.Flags().Lookup("vault-dir")

		assert.NotNil(t, vaultDirFlag)
	})

	t.Run("status shows not initialized for non-existent vault", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		cmd := vaultStatusCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Status: not initialized")
		assert.Contains(t, output, "Lock state: locked")
	})

	t.Run("status shows initialized for existing vault", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		initCmd := vaultInitCmd()
		var initBuf bytes.Buffer
		initCmd.SetOut(&initBuf)
		initCmd.SetErr(&initBuf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := initCmd.RunE(initCmd, []string{})
		require.NoError(t, err)

		statusCmd := vaultStatusCmd()
		var statusBuf bytes.Buffer
		statusCmd.SetOut(&statusBuf)
		statusCmd.SetErr(&statusBuf)

		err = statusCmd.RunE(statusCmd, []string{})
		require.NoError(t, err)

		output := statusBuf.String()
		assert.Contains(t, output, "Status: initialized")
		assert.Contains(t, output, "Lock state: locked")
	})
}

func TestVaultResetCmd(t *testing.T) {
	t.Run("reset command has correct use and description", func(t *testing.T) {
		cmd := vaultResetCmd()
		assert.Equal(t, "reset", cmd.Use)
		assert.Contains(t, cmd.Short, "Destroy the vault")
		assert.Contains(t, cmd.Long, "Reset the vault completely")
	})

	t.Run("reset has vault-dir and confirm flags", func(t *testing.T) {
		cmd := vaultResetCmd()
		vaultDirFlag := cmd.Flags().Lookup("vault-dir")
		confirmFlag := cmd.Flags().Lookup("confirm")

		assert.NotNil(t, vaultDirFlag)
		assert.NotNil(t, confirmFlag)
	})

	t.Run("reset requires confirmation without --confirm flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		initCmd := vaultInitCmd()
		var initBuf bytes.Buffer
		initCmd.SetOut(&initBuf)
		initCmd.SetErr(&initBuf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := initCmd.RunE(initCmd, []string{})
		require.NoError(t, err)

		resetCmd := vaultResetCmd()
		var resetBuf bytes.Buffer
		resetCmd.SetOut(&resetBuf)
		resetCmd.SetErr(&resetBuf)
		resetCmd.SetIn(strings.NewReader("cancel\n"))

		err = resetCmd.RunE(resetCmd, []string{})
		require.NoError(t, err)

		output := resetBuf.String()
		assert.Contains(t, output, "Reset cancelled")

		vaultDir := filepath.Join(tmpDir, constants.Paths.Infra.VaultDir)
		assert.True(t, vault.VaultHeaderExists(vaultDir))
	})

	t.Run("reset succeeds with --confirm flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		initCmd := vaultInitCmd()
		var initBuf bytes.Buffer
		initCmd.SetOut(&initBuf)
		initCmd.SetErr(&initBuf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := initCmd.RunE(initCmd, []string{})
		require.NoError(t, err)

		resetCmd := vaultResetCmd()
		resetCmd.Flags().Set("confirm", "true")
		var resetBuf bytes.Buffer
		resetCmd.SetOut(&resetBuf)
		resetCmd.SetErr(&resetBuf)

		err = resetCmd.RunE(resetCmd, []string{})
		require.NoError(t, err)

		output := resetBuf.String()
		assert.Contains(t, output, "Vault reset complete")

		vaultDir := filepath.Join(tmpDir, constants.Paths.Infra.VaultDir)
		assert.False(t, vault.VaultHeaderExists(vaultDir))
	})

	t.Run("reset fails when vault not initialized", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		cmd := vaultResetCmd()
		cmd.Flags().Set("confirm", "true")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vault not initialized")
	})
}

func TestVaultExportCmd(t *testing.T) {
	t.Run("export command has correct use and description", func(t *testing.T) {
		cmd := vaultExportCmd()
		assert.Equal(t, "export", cmd.Use)
		assert.Contains(t, cmd.Short, "Export the vault key")
		assert.Contains(t, cmd.Long, "Export the vault private key")
	})

	t.Run("export has key-path flag", func(t *testing.T) {
		cmd := vaultExportCmd()
		keyPathFlag := cmd.Flags().Lookup("key-path")

		assert.NotNil(t, keyPathFlag)
	})

	t.Run("export outputs key in hex", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		initCmd := vaultInitCmd()
		var initBuf bytes.Buffer
		initCmd.SetOut(&initBuf)
		initCmd.SetErr(&initBuf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := initCmd.RunE(initCmd, []string{})
		require.NoError(t, err)

		exportCmd := vaultExportCmd()
		var exportBuf bytes.Buffer
		exportCmd.SetOut(&exportBuf)
		exportCmd.SetErr(&exportBuf)

		err = exportCmd.RunE(exportCmd, []string{})
		require.NoError(t, err)

		output := strings.TrimSpace(exportBuf.String())
		_, err = hex.DecodeString(output)
		require.NoError(t, err)
		assert.Len(t, output, vault.KeySize*2)
	})

	t.Run("export fails with missing key file", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		cmd := vaultExportCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read key file")
	})
}

func TestVaultImportCmd(t *testing.T) {
	t.Run("import command has correct use and description", func(t *testing.T) {
		cmd := vaultImportCmd()
		assert.Equal(t, "import", cmd.Use)
		assert.Contains(t, cmd.Short, "Import a vault key")
		assert.Contains(t, cmd.Long, "Import a vault private key")
	})

	t.Run("import has key-path and key-hex flags", func(t *testing.T) {
		cmd := vaultImportCmd()
		keyPathFlag := cmd.Flags().Lookup("key-path")
		keyHexFlag := cmd.Flags().Lookup("key-hex")

		assert.NotNil(t, keyPathFlag)
		assert.NotNil(t, keyHexFlag)
	})

	t.Run("import with --key-hex flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		testKey := make([]byte, vault.KeySize)
		_, err := rand.Read(testKey)
		require.NoError(t, err)

		cmd := vaultImportCmd()
		cmd.Flags().Set("key-hex", hex.EncodeToString(testKey))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Key imported")

		vaultDir := filepath.Join(tmpDir, constants.Paths.Infra.VaultDir)
		keyPath := filepath.Join(vaultDir, "key")
		_, err = os.Stat(keyPath)
		require.NoError(t, err)
	})

	t.Run("import fails with invalid hex", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		cmd := vaultImportCmd()
		cmd.Flags().Set("key-hex", "not-valid-hex")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode key")
	})

	t.Run("import fails with wrong key size", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		shortKey := make([]byte, 16)
		_, err := rand.Read(shortKey)
		require.NoError(t, err)

		cmd := vaultImportCmd()
		cmd.Flags().Set("key-hex", hex.EncodeToString(shortKey))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err = cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid key size")
	})

	t.Run("import with custom key-path", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestPaths(t, tmpDir)

		customKeyPath := filepath.Join(tmpDir, "custom-imported-key")
		testKey := make([]byte, vault.KeySize)
		_, err := rand.Read(testKey)
		require.NoError(t, err)

		cmd := vaultImportCmd()
		cmd.Flags().Set("key-hex", hex.EncodeToString(testKey))
		cmd.Flags().Set("key-path", customKeyPath)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		_, err = os.Stat(customKeyPath)
		require.NoError(t, err)
	})
}

func TestVaultCommandFlags(t *testing.T) {
	t.Run("all vault commands have consistent flag naming", func(t *testing.T) {
		commands := []*cobra.Command{
			vaultInitCmd(),
			vaultUnlockCmd(),
			vaultRekeyCmd(),
			vaultStatusCmd(),
			vaultResetCmd(),
		}

		for _, cmd := range commands {
			vaultDirFlag := cmd.Flags().Lookup("vault-dir")
			if vaultDirFlag != nil {
				assert.Equal(t, "vault-dir", vaultDirFlag.Name)
			}
		}
	})
}

func setupTestPaths(t *testing.T, tmpDir string) {
	t.Helper()
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	protocolDir := filepath.Join(tmpDir, "protocol")
	constantsDir := filepath.Join(protocolDir, "constants")

	require.NoError(t, os.MkdirAll(runtimeDir, 0755))
	require.NoError(t, os.MkdirAll(constantsDir, 0755))

	pathsJSON := minimalPathsJSON(t)
	pathsPath := filepath.Join(constantsDir, "paths.json")
	require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", originalHome) })
}
