// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestVaultKey_InitVault_Success(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	// Initially, vault header does not exist
	exists, err := vault.VaultHeaderExists(fileSvc)
	require.NoError(t, err)
	assert.False(t, exists)

	// InitVault succeeds
	err = ks.InitVault()
	require.NoError(t, err)

	// Vault header now exists
	exists, err = vault.VaultHeaderExists(fileSvc)
	require.NoError(t, err)
	assert.True(t, exists)

	// Secret vault_key exists in fileSvc
	vaultKeyRel := filepath.Join(constants.SecretsDirname, constants.SecretsFileVaultKey)
	keyData, err := fileSvc.ReadFile(context.Background(), vaultKeyRel)
	require.NoError(t, err)
	assert.NotEmpty(t, keyData)

	// LoadVaultKey decrypts the key
	keyBytes, err := ks.LoadVaultKey()
	require.NoError(t, err)
	defer vault.SecureZero(keyBytes)
	assert.Len(t, keyBytes, vault.KeySize)

	// The raw key should never appear in the ciphertext on disk
	assert.False(t, bytes.Contains(keyData, keyBytes), "vault key material leaked unencrypted on disk")

	// Vault can be unlocked with this key
	v, err := vault.NewVault(&vault.VaultConfig{FileSvc: fileSvc, Logger: logger})
	require.NoError(t, err)
	require.NoError(t, v.Unlock(keyBytes))
	assert.True(t, v.IsUnlocked())
	t.Cleanup(func() { v.Close() })
}

func TestVaultKey_InitVault_AlreadyInitialized(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.InitVault())

	// Calling InitVault again fails with ErrVaultAlreadyInitialized
	err = ks.InitVault()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrVaultAlreadyInitialized)
}

func TestVaultKey_LoadVaultKey_NotFound(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	_, err = ks.LoadVaultKey()
	require.Error(t, err)
}

func TestVaultKey_RekeyVault_Success(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.InitVault())

	oldKey, err := ks.LoadVaultKey()
	require.NoError(t, err)
	defer vault.SecureZero(oldKey)

	// Rekey vault
	err = ks.RekeyVault()
	require.NoError(t, err)

	// Staged file should no longer exist (it was renamed to current)
	stagedRel := filepath.Join(constants.SecretsDirname, constants.SecretsFileVaultKeyStaged)
	stagedExists, err := fileSvc.FileExists(context.Background(), stagedRel)
	require.NoError(t, err)
	assert.False(t, stagedExists, "staged key file should be renamed away after successful rekey")

	newKey, err := ks.LoadVaultKey()
	require.NoError(t, err)
	defer vault.SecureZero(newKey)

	assert.NotEqual(t, oldKey, newKey, "new key must differ from old key")

	// Old key can no longer unlock the vault
	vOld, err := vault.NewVault(&vault.VaultConfig{FileSvc: fileSvc, Logger: logger})
	require.NoError(t, err)
	defer vOld.Close()
	err = vOld.Unlock(oldKey)
	require.Error(t, err, "old key should fail to unlock rekeyed vault")

	// New key unlocks successfully
	vNew, err := vault.NewVault(&vault.VaultConfig{FileSvc: fileSvc, Logger: logger})
	require.NoError(t, err)
	defer vNew.Close()
	require.NoError(t, vNew.Unlock(newKey))
	assert.True(t, vNew.IsUnlocked())
}

type renameFailingFileService struct {
	fs.RuntimeFileService
	failPattern string
}

func (f *renameFailingFileService) Rename(ctx context.Context, oldPath, newPath string) error {
	if strings.Contains(oldPath, f.failPattern) {
		return errors.New("simulated rename failure")
	}
	return f.RuntimeFileService.Rename(ctx, oldPath, newPath)
}

func TestVaultKey_RekeyVault_StagedFileFailurePath(t *testing.T) {
	t.Parallel()
	baseFileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	wrappedFileSvc := &renameFailingFileService{
		RuntimeFileService: baseFileSvc,
		failPattern:        constants.SecretsFileVaultKeyStaged,
	}

	ks, err := NewWithKeyringAndFS(logger, keyring, wrappedFileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.InitVault())

	// RekeyVault should fail at the rename step
	err = ks.RekeyVault()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrVaultKeyWriteFailed)

	// Error message should name the staged file
	assert.Contains(t, err.Error(), constants.SecretsFileVaultKeyStaged)

	// The staged file must remain present on disk so it can be recovered
	stagedRel := filepath.Join(constants.SecretsDirname, constants.SecretsFileVaultKeyStaged)
	stagedExists, err := baseFileSvc.FileExists(context.Background(), stagedRel)
	require.NoError(t, err)
	assert.True(t, stagedExists, "staged key file should remain on disk when rename fails")
}
