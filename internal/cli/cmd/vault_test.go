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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pathutil"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestVaultCmd(t *testing.T) {
	t.Parallel()

	t.Run("metadata", func(t *testing.T) {
		cmd := vaultCmd()
		assert.Equal(t, "vault", cmd.Use)
		assert.NotEmpty(t, cmd.Short)
		assert.NotEmpty(t, cmd.Long)
	})

	t.Run("subcommands registration", func(t *testing.T) {
		cmd := vaultCmd()
		expected := []string{"init", "unlock", "rekey", "status", "reset", "export", "import"}
		for _, name := range expected {
			found := false
			for _, sub := range cmd.Commands() {
				if sub.Name() == name {
					found = true
					break
				}
			}
			assert.True(t, found, "missing subcommand: %s", name)
		}
	})
}

func TestReadKeyFile(t *testing.T) {
	t.Parallel()

	t.Run("valid key", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		key := make([]byte, vault.KeySize)
		_, err := rand.Read(key)
		require.NoError(t, err)

		keyPath := filepath.Join(tp.BaseDir, "test.key")
		require.NoError(t, os.WriteFile(keyPath, []byte(hex.EncodeToString(key)+"\n"), 0600))

		read, err := readKeyFile(keyPath)
		require.NoError(t, err)
		assert.Equal(t, key, read)
	})

	t.Run("invalid hex", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		keyPath := filepath.Join(tp.BaseDir, "test.key")
		require.NoError(t, os.WriteFile(keyPath, []byte("invalid hex"), 0600))

		_, err := readKeyFile(keyPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode key")
	})

	t.Run("wrong size", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		keyPath := filepath.Join(tp.BaseDir, "test.key")
		require.NoError(t, os.WriteFile(keyPath, []byte(hex.EncodeToString(make([]byte, 16))), 0600))

		_, err := readKeyFile(keyPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid key size")
	})

	t.Run("missing file", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		_, err := readKeyFile(filepath.Join(tp.BaseDir, "missing.key"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read key file")
	})
}

func TestVaultInitCmd(t *testing.T) {

	t.Run("successful init", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		cmd := vaultInitCmd()
		cmd.Flags().Set("vault-dir", tp.VaultDir)
		cmd.Flags().Set("key-path", tp.VaultKeyPath)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		assert.True(t, vault.VaultHeaderExists(tp.VaultDir))
		_, err = os.Stat(tp.VaultKeyPath)
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault initialized")
	})

	t.Run("custom paths", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		customVault := filepath.Join(tp.BaseDir, "custom-vault")
		customKey := filepath.Join(tp.BaseDir, "custom.key")

		cmd := vaultInitCmd()
		cmd.Flags().Set("vault-dir", customVault)
		cmd.Flags().Set("key-path", customKey)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		assert.True(t, vault.VaultHeaderExists(customVault))
		_, err = os.Stat(customKey)
		require.NoError(t, err)
	})

	t.Run("already initialized", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		cmd := vaultInitCmd()
		cmd.Flags().Set("vault-dir", tp.VaultDir)
		cmd.Flags().Set("key-path", tp.VaultKeyPath)
		require.NoError(t, cmd.RunE(cmd, []string{}))
		require.Error(t, cmd.RunE(cmd, []string{}))
	})
}

func TestVaultUnlockCmd(t *testing.T) {

	t.Run("successful unlock", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		initCmd := vaultInitCmd()
		initCmd.Flags().Set("vault-dir", tp.VaultDir)
		initCmd.Flags().Set("key-path", tp.VaultKeyPath)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultUnlockCmd()
		cmd.Flags().Set("vault-dir", tp.VaultDir)
		cmd.Flags().Set("key-path", tp.VaultKeyPath)
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault unlocked successfully")
	})

	t.Run("wrong key", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		initCmd := vaultInitCmd()
		initCmd.Flags().Set("vault-dir", tp.VaultDir)
		initCmd.Flags().Set("key-path", tp.VaultKeyPath)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		require.NoError(t, os.WriteFile(tp.VaultKeyPath, []byte(hex.EncodeToString(make([]byte, vault.KeySize))+"\n"), 0600))

		cmd := vaultUnlockCmd()
		cmd.Flags().Set("vault-dir", tp.VaultDir)
		cmd.Flags().Set("key-path", tp.VaultKeyPath)
		require.Error(t, cmd.RunE(cmd, []string{}))
	})
}

func TestVaultRekeyCmd(t *testing.T) {

	t.Run("successful rekey", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		initCmd := vaultInitCmd()
		initCmd.Flags().Set("vault-dir", tp.VaultDir)
		initCmd.Flags().Set("key-path", tp.VaultKeyPath)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultRekeyCmd()
		cmd.Flags().Set("vault-dir", tp.VaultDir)
		cmd.Flags().Set("key-path", tp.VaultKeyPath)
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault rekeyed successfully")

		newKeyPath := pathutil.SafeJoin(tp.VaultDir, constants.VaultNewKeyFilename)
		_, err = os.Stat(newKeyPath)
		require.NoError(t, err)
	})
}

func TestVaultStatusCmd(t *testing.T) {

	t.Run("not initialized", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		cmd := vaultStatusCmd()
		cmd.Flags().Set("vault-dir", tp.VaultDir)
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))
		assert.Contains(t, out.String(), "Status: not initialized")
	})

	t.Run("initialized", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		initCmd := vaultInitCmd()
		initCmd.Flags().Set("vault-dir", tp.VaultDir)
		initCmd.Flags().Set("key-path", tp.VaultKeyPath)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultStatusCmd()
		cmd.Flags().Set("vault-dir", tp.VaultDir)
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))
		assert.Contains(t, out.String(), "Status: initialized")
	})
}

func TestVaultResetCmd(t *testing.T) {

	t.Run("successful reset with confirm flag", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		initCmd := vaultInitCmd()
		initCmd.Flags().Set("vault-dir", tp.VaultDir)
		initCmd.Flags().Set("key-path", tp.VaultKeyPath)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultResetCmd()
		cmd.Flags().Set("vault-dir", tp.VaultDir)
		cmd.Flags().Set("confirm", "true")
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault reset complete")
		assert.False(t, vault.VaultHeaderExists(tp.VaultDir))
	})

	t.Run("interactive cancellation", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		initCmd := vaultInitCmd()
		initCmd.Flags().Set("vault-dir", tp.VaultDir)
		initCmd.Flags().Set("key-path", tp.VaultKeyPath)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultResetCmd()
		cmd.Flags().Set("vault-dir", tp.VaultDir)
		cmd.SetIn(strings.NewReader("no\n"))
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Reset cancelled")
		assert.True(t, vault.VaultHeaderExists(tp.VaultDir))
	})
}

func TestVaultExportImport(t *testing.T) {

	t.Run("export success", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		initCmd := vaultInitCmd()
		initCmd.Flags().Set("vault-dir", tp.VaultDir)
		initCmd.Flags().Set("key-path", tp.VaultKeyPath)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultExportCmd()
		cmd.Flags().Set("key-path", tp.VaultKeyPath)
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))

		keyHex := strings.TrimSpace(out.String())
		_, err := hex.DecodeString(keyHex)
		require.NoError(t, err)
		assert.Len(t, keyHex, vault.KeySize*2)
	})

	t.Run("import success", func(t *testing.T) {
		tp := testutil.NewTestPathsFromTemp(t)
		require.NoError(t, tp.EnsureDirs())

		require.NoError(t, constants.InitPathsWithBase(tp.BaseDir))

		key := make([]byte, vault.KeySize)
		_, _ = rand.Read(key)
		keyHex := hex.EncodeToString(key)

		cmd := vaultImportCmd()
		cmd.Flags().Set("key-path", tp.VaultKeyPath)
		cmd.Flags().Set("key-hex", keyHex)
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))
		assert.Contains(t, out.String(), "Key imported")

		read, _ := readKeyFile(tp.VaultKeyPath)
		assert.Equal(t, key, read)
	})
}
