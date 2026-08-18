// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux || windows

package keystore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileKeyring(t *testing.T) {
	t.Parallel()

	secretsDir := testutil.TempDir(t)
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)
	assert.Equal(t, "file", keyring.Name())
}

func TestFileKeyring_StoreRetrieveDelete(t *testing.T) {
	t.Parallel()

	secretsDir := testutil.TempDir(t)
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)
	assert.Equal(t, "file", keyring.Name())

	testKey := make([]byte, vault.KeySize)
	for i := range testKey {
		testKey[i] = byte(i)
	}

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "store",
			fn: func(t *testing.T) {
				err := keyring.StoreMasterKey(testKey)
				require.NoError(t, err)
			},
		},
		{
			name: "retrieve",
			fn: func(t *testing.T) {
				retrievedKey, err := keyring.RetrieveMasterKey()
				require.NoError(t, err)
				assert.Equal(t, testKey, retrievedKey)
			},
		},
		{
			name: "delete",
			fn: func(t *testing.T) {
				err := keyring.DeleteMasterKey()
				require.NoError(t, err)
			},
		},
		{
			name: "retrieve after delete",
			fn: func(t *testing.T) {
				_, err := keyring.RetrieveMasterKey()
				require.Error(t, err)
				assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestFileKeyring_RetrieveNotFound(t *testing.T) {
	t.Parallel()

	secretsDir := testutil.TempDir(t)
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)

	_, err = keyring.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
}

func TestFileKeyring_DeleteIdempotent(t *testing.T) {
	t.Parallel()

	secretsDir := testutil.TempDir(t)
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "first delete",
			fn: func(t *testing.T) {
				err := keyring.DeleteMasterKey()
				require.NoError(t, err)
			},
		},
		{
			name: "second delete",
			fn: func(t *testing.T) {
				err := keyring.DeleteMasterKey()
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestFileKeyring_WithRealKeystore(t *testing.T) {
	t.Parallel()

	fileSvc, secretsDir := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	err = ks.Initialize()
	require.NoError(t, err)

	plaintext := "test-secret-value"
	err = ks.EncryptSecret("test-secret", plaintext)
	require.NoError(t, err)

	decrypted, err := ks.DecryptSecret("test-secret")
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	secretPath := filepath.Join(secretsDir, "test-secret")
	info, err := os.Stat(secretPath)
	require.NoError(t, err)
	perm := info.Mode().Perm()
	if runtime.GOOS == "windows" {
		// Windows doesn't model Unix permission bits, so only assert the file
		// is not world-writable.
		assert.NotEqual(t, os.FileMode(0777), perm, "secret file should not be world-writable")
	} else {
		assert.Equal(t, os.FileMode(0600), perm)
	}
}
