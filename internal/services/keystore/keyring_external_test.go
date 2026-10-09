// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestExternalKeyring_RejectsRelativePath(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)

	_, err := newExternalFileKeyring(fileSvc, "relative/path/key")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreExternalKeyPath)
}

func TestExternalKeyring_RejectsPathInsideRuntimeDir(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)

	insidePath := filepath.Join(fileSvc.Resolve(""), "master.key")
	_, err := newExternalFileKeyring(fileSvc, insidePath)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreExternalKeyPath)
}

func TestExternalKeyring_StoreMasterKey_ReadOnly(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)

	extDir := testutil.TempDir(t)
	extPath := filepath.Join(extDir, "master.key")

	kr, err := newExternalFileKeyring(fileSvc, extPath)
	require.NoError(t, err)
	assert.Equal(t, "external-file", kr.Name())

	err = kr.StoreMasterKey(make([]byte, vault.KeySize))
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreExternalReadOnly)

	// DeleteMasterKey is a no-op
	assert.NoError(t, kr.DeleteMasterKey())
}

func TestExternalKeyring_RetrieveMasterKey_NotFound(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)

	extDir := testutil.TempDir(t)
	extPath := filepath.Join(extDir, "nonexistent.key")

	kr, err := newExternalFileKeyring(fileSvc, extPath)
	require.NoError(t, err)

	_, err = kr.RetrieveMasterKey()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreKeyNotFound)
}

func TestExternalKeyring_RetrieveMasterKey_EmptyFile(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)

	extDir := testutil.TempDir(t)
	extPath := filepath.Join(extDir, "empty.key")
	require.NoError(t, os.WriteFile(extPath, []byte(""), 0600))

	kr, err := newExternalFileKeyring(fileSvc, extPath)
	require.NoError(t, err)

	_, err = kr.RetrieveMasterKey()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreKeyNotFound)
}

func TestExternalKeyring_RetrieveMasterKey_InvalidBase64(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)

	extDir := testutil.TempDir(t)
	extPath := filepath.Join(extDir, "bad_b64.key")
	require.NoError(t, os.WriteFile(extPath, []byte("not!valid@base64"), 0600))

	kr, err := newExternalFileKeyring(fileSvc, extPath)
	require.NoError(t, err)

	_, err = kr.RetrieveMasterKey()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreDecodeFailed)
}

func TestExternalKeyring_WrongKeyLength(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()

	extDir := testutil.TempDir(t)
	extPath := filepath.Join(extDir, "short.key")

	shortKey := make([]byte, 16) // 16 bytes instead of 32
	_, _ = rand.Read(shortKey)
	require.NoError(t, os.WriteFile(extPath, []byte(base64.StdEncoding.EncodeToString(shortKey)), 0600))

	ks, err := NewWithFS(fileSvc, logger, Options{MasterKeyFile: extPath})
	require.NoError(t, err)

	err = ks.Initialize()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreInvalidKeyLength)
}

func TestExternalKeyring_SuccessRoundTrip(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()

	extDir := testutil.TempDir(t)
	extPath := filepath.Join(extDir, "valid.key")

	validKey := make([]byte, vault.KeySize)
	_, err := rand.Read(validKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(extPath, []byte(base64.StdEncoding.EncodeToString(validKey)+"\n"), 0600))

	ks, err := NewWithFS(fileSvc, logger, Options{MasterKeyFile: extPath})
	require.NoError(t, err)
	assert.Equal(t, "external-file", ks.KeyringName())

	require.NoError(t, ks.Initialize())

	// Test encrypt/decrypt secret with external master key
	secretName := "test-ext-secret"
	secretVal := "super-confidential"
	require.NoError(t, ks.EncryptSecret(secretName, secretVal))

	decrypted, err := ks.DecryptSecret(secretName)
	require.NoError(t, err)
	assert.Equal(t, secretVal, decrypted)
}
