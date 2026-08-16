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
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
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

// TestVaultTestFileSvc_RootsAtTempDirNotCWD guards against regressions where
// the vault test file service roots at CWD instead of an isolated temp dir.
// newCmdTestEnv returns a temp-rooted fileSvc; if a future change reverts to
// a CWD-rooted helper, this test fails.
func TestVaultTestFileSvc_RootsAtTempDirNotCWD(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	fileSvc, _ := newCmdTestEnv(t)
	root := fileSvc.Resolve("")

	cwdRuntimeRoot := filepath.Join(cwd, constants.RuntimeDirname)
	assert.NotEqual(t, cwdRuntimeRoot, root, "vault test fileSvc roots at CWD (%s); it must root at an isolated temp dir", cwdRuntimeRoot)
}

func TestReadKeyFile(t *testing.T) {
	t.Parallel()

	t.Run("valid key", func(t *testing.T) {
		t.Parallel()
		baseDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(baseDir, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

		key := make([]byte, vault.KeySize)
		_, err = rand.Read(key)
		require.NoError(t, err)

		relPath := filepath.Join(constants.VaultDirname, "test.key")
		require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, []byte(hex.EncodeToString(key)+"\n"), constants.PermFilePrivate))

		read, err := readKeyFile(fileSvc, relPath)
		require.NoError(t, err)
		assert.Equal(t, key, read)
	})

	t.Run("invalid hex", func(t *testing.T) {
		t.Parallel()
		baseDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(baseDir, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

		relPath := filepath.Join(constants.VaultDirname, "test.key")
		require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, []byte("invalid hex"), constants.PermFilePrivate))

		_, err = readKeyFile(fileSvc, relPath)
		require.Error(t, err)
	})

	t.Run("wrong size", func(t *testing.T) {
		t.Parallel()
		baseDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(baseDir, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

		relPath := filepath.Join(constants.VaultDirname, "test.key")
		require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, []byte(hex.EncodeToString(make([]byte, 16))), constants.PermFilePrivate))

		_, err = readKeyFile(fileSvc, relPath)
		require.Error(t, err)
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		baseDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(baseDir, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

		_, err = readKeyFile(fileSvc, filepath.Join(constants.VaultDirname, "missing.key"))
		require.Error(t, err)
	})
}

func TestVaultInitCmd(t *testing.T) {

	t.Run("successful init", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		var out bytes.Buffer
		cmd.SetOut(&out)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		assert.True(t, vault.VaultHeaderExists(fileSvc.Resolve(constants.VaultDirname)))
		exists, err := fileSvc.FileExists(context.Background(), filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Contains(t, out.String(), "Vault initialized")
	})

	t.Run("custom paths", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", "custom-vault")
		cmd.Flags().Set("key-path", "custom.key")

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		assert.True(t, vault.VaultHeaderExists(fileSvc.Resolve("custom-vault")))
		exists, err := fileSvc.FileExists(context.Background(), "custom.key")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("already initialized", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, cmd.RunE(cmd, []string{}))
		require.Error(t, cmd.RunE(cmd, []string{}))
	})
}

func TestVaultUnlockCmd(t *testing.T) {

	t.Run("successful unlock", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultUnlockCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault unlocked successfully")
	})

	t.Run("wrong key", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		require.NoError(t, fileSvc.WriteFile(context.Background(),
			filepath.Join(constants.VaultDirname, constants.VaultKeyFilename),
			[]byte(hex.EncodeToString(make([]byte, vault.KeySize))+"\n"),
			constants.PermFilePrivate))

		cmd := vaultUnlockCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.Error(t, cmd.RunE(cmd, []string{}))
	})
}

func TestVaultRekeyCmd(t *testing.T) {

	t.Run("successful rekey", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultRekeyCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault rekeyed successfully")

		newKeyRelPath := filepath.Join(constants.VaultDirname, constants.VaultNewKeyFilename)
		exists, err := fileSvc.FileExists(context.Background(), newKeyRelPath)
		require.NoError(t, err)
		assert.True(t, exists)
	})
}

func TestVaultStatusCmd(t *testing.T) {

	t.Run("not initialized", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultStatusCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))
		assert.Contains(t, out.String(), "Status: not initialized")
	})

	t.Run("initialized", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultStatusCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))
		assert.Contains(t, out.String(), "Status: initialized")
	})
}

func TestVaultResetCmd(t *testing.T) {

	t.Run("successful reset with confirm flag", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultResetCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("confirm", "true")
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault reset complete")
		assert.False(t, vault.VaultHeaderExists(fileSvc.Resolve(constants.VaultDirname)))
	})

	t.Run("interactive cancellation", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultResetCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.SetIn(strings.NewReader("no\n"))
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Reset cancelled")
		assert.True(t, vault.VaultHeaderExists(fileSvc.Resolve(constants.VaultDirname)))
	})
}

func TestVaultUnlockCmd_ErrorPaths(t *testing.T) {

	t.Run("unlock not initialized", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultUnlockCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultNotInitialized)
	})

	t.Run("unlock missing key file", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultUnlockCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, "nonexistent.key"))
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
	})

	t.Run("unlock corrupt key file", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		require.NoError(t, fileSvc.WriteFile(context.Background(),
			filepath.Join(constants.VaultDirname, constants.VaultKeyFilename),
			[]byte("corrupt-key-data"),
			constants.PermFilePrivate))

		cmd := vaultUnlockCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
	})
}

func TestVaultRekeyCmd_ErrorPaths(t *testing.T) {

	t.Run("rekey not initialized", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultRekeyCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultNotInitialized)
	})

	t.Run("rekey wrong old key", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		wrongKey := make([]byte, vault.KeySize)
		_, _ = rand.Read(wrongKey)
		require.NoError(t, fileSvc.WriteFile(context.Background(),
			filepath.Join(constants.VaultDirname, constants.VaultKeyFilename),
			[]byte(hex.EncodeToString(wrongKey)+"\n"),
			constants.PermFilePrivate))

		cmd := vaultRekeyCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		newKeyPath := filepath.Join(constants.VaultDirname, "new.key")
		cmd.Flags().Set("new-key-path", newKeyPath)
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
	})
}

func TestVaultResetCmd_ErrorPaths(t *testing.T) {

	t.Run("reset not initialized", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultResetCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.Flags().Set("confirm", "true")
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultNotInitialized)
	})

	t.Run("interactive confirm with 'destroy' text", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultResetCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.SetIn(strings.NewReader("destroy\n"))
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault reset complete")
		assert.False(t, vault.VaultHeaderExists(fileSvc.Resolve(constants.VaultDirname)))
	})

	t.Run("interactive confirm with wrong text", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultResetCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("vault-dir", constants.VaultDirname)
		cmd.SetIn(strings.NewReader("yes\n"))
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Reset cancelled")
		assert.True(t, vault.VaultHeaderExists(fileSvc.Resolve(constants.VaultDirname)))
	})
}

func TestVaultExportCmd_ErrorPaths(t *testing.T) {

	t.Run("export missing key file", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultExportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, "nonexistent.key"))
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
	})

	t.Run("export corrupt key file", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		require.NoError(t, fileSvc.WriteFile(context.Background(),
			filepath.Join(constants.VaultDirname, constants.VaultKeyFilename),
			[]byte("not-hex"),
			constants.PermFilePrivate))

		cmd := vaultExportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
	})
}

func TestVaultImportCmd_ErrorPaths(t *testing.T) {

	t.Run("import invalid hex", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultImportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		cmd.Flags().Set("key-hex", "invalid-hex-string")
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultKeyDecodeFailed)
	})

	t.Run("import wrong key size", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		shortKey := hex.EncodeToString(make([]byte, 16))
		cmd := vaultImportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		cmd.Flags().Set("key-hex", shortKey)
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultKeyInvalidSize)
	})

	t.Run("import from stdin", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		key := make([]byte, vault.KeySize)
		_, _ = rand.Read(key)
		keyHex := hex.EncodeToString(key)

		cmd := vaultImportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		cmd.SetIn(strings.NewReader(keyHex + "\n"))
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Key imported")

		read, err := readKeyFile(fileSvc, filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, err)
		assert.Equal(t, key, read)
	})

	t.Run("import from stdin invalid hex", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		cmd := vaultImportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		cmd.SetIn(strings.NewReader("not-valid-hex\n"))
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultKeyDecodeFailed)
	})
}

func TestVaultExportImport(t *testing.T) {

	t.Run("export success", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultExportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))

		keyHex := strings.TrimSpace(out.String())
		_, err := hex.DecodeString(keyHex)
		require.NoError(t, err)
		assert.Len(t, keyHex, vault.KeySize*2)
	})

	t.Run("import success", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		key := make([]byte, vault.KeySize)
		_, _ = rand.Read(key)
		keyHex := hex.EncodeToString(key)

		cmd := vaultImportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		cmd.Flags().Set("key-hex", keyHex)
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))
		assert.Contains(t, out.String(), "Key imported")

		read, err := readKeyFile(fileSvc, filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, err)
		assert.Equal(t, key, read)
	})

	t.Run("export-import round-trip", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)

		initCmd := vaultInitCmdWithConfig(fileSvcFactoryFor(fileSvc))
		initCmd.Flags().Set("vault-dir", constants.VaultDirname)
		initCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		exportCmd := vaultExportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		exportCmd.Flags().Set("key-path", filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		var exportBuf bytes.Buffer
		exportCmd.SetOut(&exportBuf)
		require.NoError(t, exportCmd.RunE(exportCmd, []string{}))
		exportedHex := strings.TrimSpace(exportBuf.String())

		importKeyRelPath := filepath.Join(constants.VaultDirname, "imported.key")
		importCmd := vaultImportCmdWithConfig(fileSvcFactoryFor(fileSvc))
		importCmd.Flags().Set("key-path", importKeyRelPath)
		importCmd.Flags().Set("key-hex", exportedHex)
		var importBuf bytes.Buffer
		importCmd.SetOut(&importBuf)
		require.NoError(t, importCmd.RunE(importCmd, []string{}))
		assert.Contains(t, importBuf.String(), "Key imported")

		original, err := readKeyFile(fileSvc, filepath.Join(constants.VaultDirname, constants.VaultKeyFilename))
		require.NoError(t, err)
		imported, err := readKeyFile(fileSvc, importKeyRelPath)
		require.NoError(t, err)
		assert.Equal(t, original, imported)
	})
}
