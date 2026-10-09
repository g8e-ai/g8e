// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package vaultcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newTestVaultEnv(t *testing.T) (fs.RuntimeFileService, keystoreFactory, *keystore.Keystore) {
	t.Helper()
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	keyring := keystoretest.NewMemoryKeyring()
	factory := func(fs fs.RuntimeFileService, opts keystore.Options, create bool) (*keystore.Keystore, error) {
		ks, err := keystore.NewWithKeyringAndFS(testutil.NewTestLogger(), keyring, fs)
		if err != nil {
			return nil, err
		}
		if create {
			if err := ks.Initialize(); err != nil {
				return nil, err
			}
		}
		return ks, nil
	}
	refKS, err := keystore.NewWithKeyringAndFS(testutil.NewTestLogger(), keyring, fileSvc)
	require.NoError(t, err)
	return fileSvc, factory, refKS
}

func TestVaultCmd(t *testing.T) {
	t.Parallel()

	t.Run("metadata", func(t *testing.T) {
		cmd := Cmd()
		assert.Equal(t, "vault", cmd.Use)
		assert.NotEmpty(t, cmd.Short)
		assert.NotEmpty(t, cmd.Long)
	})

	t.Run("subcommands registration", func(t *testing.T) {
		cmd := Cmd()
		expected := []string{"init", "unlock", "rekey", "status", "reset"}
		subcommands := cmd.Commands()
		assert.Len(t, subcommands, len(expected))
		names := make(map[string]bool, len(subcommands))
		for _, sub := range subcommands {
			names[sub.Name()] = true
		}
		for _, name := range expected {
			assert.True(t, names[name], "missing subcommand: %s", name)
		}
		assert.False(t, names["export"], "vault should not retain 'export' subcommand")
		assert.False(t, names["import"], "vault should not retain 'import' subcommand")
	})
}

// TestVaultTestFileSvc_RootsAtTempDirNotCWD guards against regressions where
// the vault test file service roots at CWD instead of an isolated temp dir.
func TestVaultTestFileSvc_RootsAtTempDirNotCWD(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	root := fileSvc.Resolve("")

	cwdRuntimeRoot := filepath.Join(cwd, constants.RuntimeDirname)
	assert.NotEqual(t, cwdRuntimeRoot, root, "vault test fileSvc roots at CWD (%s); it must root at an isolated temp dir", cwdRuntimeRoot)
}

func TestVaultInitCmd(t *testing.T) {
	t.Run("successful init", func(t *testing.T) {
		fileSvc, ksFactory, _ := newTestVaultEnv(t)

		cmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		var out bytes.Buffer
		cmd.SetOut(&out)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		headerExists, err := vault.VaultHeaderExists(fileSvc)
		require.NoError(t, err)
		assert.True(t, headerExists)

		sealedKeyExists, err := fileSvc.FileExists(context.Background(), filepath.Join(constants.SecretsDirname, constants.SecretsFileVaultKey))
		require.NoError(t, err)
		assert.True(t, sealedKeyExists)
		assert.Contains(t, out.String(), "Vault initialized")
	})

	t.Run("already initialized", func(t *testing.T) {
		fileSvc, ksFactory, _ := newTestVaultEnv(t)

		cmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		require.NoError(t, cmd.RunE(cmd, []string{}))

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultAlreadyInitialized)
	})
}

func TestVaultUnlockCmd(t *testing.T) {
	t.Run("successful unlock", func(t *testing.T) {
		fileSvc, ksFactory, _ := newTestVaultEnv(t)

		initCmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultUnlockCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault unlocked successfully")
	})

	t.Run("unlock not initialized", func(t *testing.T) {
		fileSvc, ksFactory, _ := newTestVaultEnv(t)

		cmd := vaultUnlockCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultNotInitialized)
	})

	t.Run("wrong key", func(t *testing.T) {
		fileSvc, ksFactory, refKS := newTestVaultEnv(t)

		initCmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		// Replace sealed vault key with an invalid key that does not match header
		wrongKey := make([]byte, vault.KeySize)
		wrongKey[0] = 0xff
		require.NoError(t, refKS.StoreKeyMaterial(constants.SecretsFileVaultKey, wrongKey))

		cmd := vaultUnlockCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultUnlockFailed)
	})
}

func TestVaultRekeyCmd(t *testing.T) {
	t.Run("successful rekey", func(t *testing.T) {
		fileSvc, ksFactory, refKS := newTestVaultEnv(t)

		initCmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		oldKey, err := refKS.LoadVaultKey()
		require.NoError(t, err)

		cmd := vaultRekeyCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		var out bytes.Buffer
		cmd.SetOut(&out)
		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault rekeyed successfully")

		newKey, err := refKS.LoadVaultKey()
		require.NoError(t, err)
		assert.NotEqual(t, oldKey, newKey)

		// Confirm vault unlocks with newly stored key
		unlockCmd := vaultUnlockCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		require.NoError(t, unlockCmd.RunE(unlockCmd, []string{}))

		// Tampering back to old key should fail unlock
		require.NoError(t, refKS.StoreKeyMaterial(constants.SecretsFileVaultKey, oldKey))
		require.Error(t, unlockCmd.RunE(unlockCmd, []string{}))
	})

	t.Run("rekey not initialized", func(t *testing.T) {
		fileSvc, ksFactory, _ := newTestVaultEnv(t)

		cmd := vaultRekeyCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultNotInitialized)
	})
}

func TestVaultStatusCmd(t *testing.T) {
	t.Run("not initialized", func(t *testing.T) {
		fileSvc, _ := cmdtest.NewCmdTestEnv(t)

		cmd := vaultStatusCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))
		assert.Contains(t, out.String(), "Status: not initialized")
		assert.Contains(t, out.String(), "Lock state: locked")
	})

	t.Run("initialized", func(t *testing.T) {
		fileSvc, ksFactory, _ := newTestVaultEnv(t)

		initCmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultStatusCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.RunE(cmd, []string{}))
		assert.Contains(t, out.String(), "Status: initialized")
		assert.Contains(t, out.String(), "Lock state: locked")
	})
}

func TestVaultResetCmd(t *testing.T) {
	t.Run("successful reset with confirm flag", func(t *testing.T) {
		fileSvc, ksFactory, _ := newTestVaultEnv(t)

		initCmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultResetCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("confirm", "true")
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault reset complete")

		headerExists, headerErr := vault.VaultHeaderExists(fileSvc)
		require.NoError(t, headerErr)
		assert.False(t, headerExists)

		sealedKeyExists, err := fileSvc.FileExists(context.Background(), filepath.Join(constants.SecretsDirname, constants.SecretsFileVaultKey))
		require.NoError(t, err)
		assert.False(t, sealedKeyExists)
	})

	t.Run("interactive confirmation with destroy", func(t *testing.T) {
		fileSvc, ksFactory, _ := newTestVaultEnv(t)

		initCmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultResetCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))
		cmd.SetIn(strings.NewReader("destroy\n"))
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Vault reset complete")

		headerExists, headerErr := vault.VaultHeaderExists(fileSvc)
		require.NoError(t, headerErr)
		assert.False(t, headerExists)
	})

	t.Run("interactive cancellation", func(t *testing.T) {
		fileSvc, ksFactory, _ := newTestVaultEnv(t)

		initCmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), ksFactory)
		require.NoError(t, initCmd.RunE(initCmd, []string{}))

		cmd := vaultResetCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))
		cmd.SetIn(strings.NewReader("no\n"))
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Reset cancelled")

		headerExists, headerErr := vault.VaultHeaderExists(fileSvc)
		require.NoError(t, headerErr)
		assert.True(t, headerExists)
	})

	t.Run("reset not initialized", func(t *testing.T) {
		fileSvc, _ := cmdtest.NewCmdTestEnv(t)

		cmd := vaultResetCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("confirm", "true")
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrVaultNotInitialized)
	})
}
