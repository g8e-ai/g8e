// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func setupTestFileService(t *testing.T) (fs.RuntimeFileService, string) {
	t.Helper()
	baseDir := testutil.TempDir(t)
	svc, err := fs.NewRuntimeFileService(baseDir, testutil.NewVerboseTestLogger(t))
	require.NoError(t, err)
	require.NoError(t, svc.CreateRuntimeTree(context.Background()))
	return svc, svc.Resolve(constants.SecretsDirname)
}

func TestKeystore_Initialize_RetrieveError(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	retrieveErr := errors.New("retrieve failed")
	keyring := &errorKeyring{retrieveErr: retrieveErr}

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	err = ks.Initialize()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreRetrieveFailed)
}

func TestKeystore_Initialize_StoreError(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := &errorKeyring{storeErr: errors.New("store failed")}

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	err = ks.Initialize()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreStoreFailed)
}

func TestKeystore_Initialize_VerifyFailed_DropsWrites(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	// dropsWritesKeyring accepts StoreMasterKey, but RetrieveMasterKey returns ErrKeyStoreKeyNotFound
	keyring := &dropsWritesKeyring{}

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	err = ks.Initialize()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreVerifyFailed)
}

func TestKeystore_Initialize_VerifyFailed_Corrupted(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	// corruptingKeyring stores one thing but returns altered bytes
	keyring := &corruptingKeyring{}

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	err = ks.Initialize()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreVerifyFailed)
}

func TestKeystore_Encrypt_RetrieveError(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	retrieveErr := errors.New("retrieve failed")
	keyring := &errorKeyring{retrieveErr: retrieveErr}

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	_, err = ks.Encrypt("plaintext")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreRetrieveFailed)
}

func TestKeystore_EncryptSecret_RetrieveError(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	retrieveErr := errors.New("retrieve failed")
	keyring := &errorKeyring{retrieveErr: retrieveErr}

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	err = ks.EncryptSecret("test", "value")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreRetrieveFailed)
}

func TestKeystore_Decrypt_RetrieveError(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	retrieveErr := errors.New("retrieve failed")
	keyring := &errorKeyring{retrieveErr: retrieveErr}

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	_, err = ks.Decrypt("dGVzdA==")
	require.Error(t, err)
}

func TestKeystore_Purge_DeleteMasterKeyError(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := &errorKeyring{deleteErr: errors.New("delete failed")}

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	err = ks.Purge()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreDeleteFailed)
}

func TestKeystore_EnforcePermissions_DeletedDir(t *testing.T) {
	t.Parallel()
	fileSvc, secretsDir := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(secretsDir))

	err = ks.EnforcePermissions()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreChmodDir)
}

func TestKeystore_Purge_WithSecrets(t *testing.T) {
	t.Parallel()
	fileSvc, secretsDir := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	require.NoError(t, ks.EncryptSecret("s1", "v1"))
	require.NoError(t, ks.EncryptSecret("s2", "v2"))

	err = ks.Purge()
	require.NoError(t, err)

	entries, err := os.ReadDir(secretsDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, !e.IsDir(), "unexpected file entry: %s", e.Name())
	}
}

func TestKeystore_EnforcePermissions_ReadDirError(t *testing.T) {
	t.Parallel()
	fileSvc, secretsDir := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(secretsDir))

	err = ks.EnforcePermissions()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreChmodDir)
}

func TestKeystore_Purge_ReadDirError(t *testing.T) {
	t.Parallel()
	fileSvc, secretsDir := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(secretsDir))

	err = ks.Purge()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreReadDir)
}

func TestKeystore_DecryptSecret_CorruptCiphertext(t *testing.T) {
	t.Parallel()
	fileSvc, secretsDir := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	enc := EncryptedSecret{
		Version:    1,
		Nonce:      make([]byte, vault.NonceSize),
		Ciphertext: []byte("invalid ciphertext that will fail GCM"),
	}
	data, err := json.Marshal(enc)
	require.NoError(t, err)

	secretPath := filepath.Join(secretsDir, "test-secret")
	require.NoError(t, os.WriteFile(secretPath, data, 0600))

	_, err = ks.DecryptSecret("test-secret")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidCiphertext)
}

func TestKeystore_EncryptSecret_MarshalError(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := &errorKeyring{storeErr: errors.New("store failed")}

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)

	err = ks.EncryptSecret("test", "value")
	require.Error(t, err)
}

func TestKeystore_DeleteSecret_Error(t *testing.T) {
	t.Parallel()
	fileSvc, secretsDir := setupTestFileService(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	// Create a non-empty directory with the secret name so os.Remove fails with ENOTEMPTY
	secretDir := filepath.Join(secretsDir, "test-secret")
	require.NoError(t, os.Mkdir(secretDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(secretDir, "nested"), []byte("data"), 0o600))

	err = ks.DeleteSecret("test-secret")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreDeleteSecret)
}

// errorKeyring is a test keyring that returns configurable errors.
type errorKeyring struct {
	storeErr    error
	retrieveErr error
	deleteErr   error
}

func (e *errorKeyring) Name() string { return "error" }

func (e *errorKeyring) RetrieveMasterKey() ([]byte, error) {
	if e.retrieveErr != nil {
		return nil, e.retrieveErr
	}
	return nil, constants.ErrKeyStoreKeyNotFound
}

func (e *errorKeyring) StoreMasterKey([]byte) error {
	return e.storeErr
}

func (e *errorKeyring) DeleteMasterKey() error {
	return e.deleteErr
}

// dropsWritesKeyring simulates a keyring that silently drops stores.
type dropsWritesKeyring struct{}

func (d *dropsWritesKeyring) Name() string { return "drops-writes" }

func (d *dropsWritesKeyring) RetrieveMasterKey() ([]byte, error) {
	return nil, constants.ErrKeyStoreKeyNotFound
}

func (d *dropsWritesKeyring) StoreMasterKey([]byte) error {
	return nil
}

func (d *dropsWritesKeyring) DeleteMasterKey() error {
	return nil
}

// corruptingKeyring simulates a keyring that corrupts the stored key.
type corruptingKeyring struct {
	key []byte
}

func (c *corruptingKeyring) Name() string { return "corrupting" }

func (c *corruptingKeyring) RetrieveMasterKey() ([]byte, error) {
	if c.key == nil {
		return nil, constants.ErrKeyStoreKeyNotFound
	}
	corrupted := make([]byte, len(c.key))
	copy(corrupted, c.key)
	corrupted[0] ^= 0xff
	return corrupted, nil
}

func (c *corruptingKeyring) StoreMasterKey(key []byte) error {
	c.key = make([]byte, len(key))
	copy(c.key, key)
	return nil
}

func (c *corruptingKeyring) DeleteMasterKey() error {
	c.key = nil
	return nil
}

// newMemoryKeyring creates a simple in-memory keyring for internal tests.
func newMemoryKeyring() *simpleMemoryKeyring {
	return &simpleMemoryKeyring{}
}

type simpleMemoryKeyring struct {
	key []byte
}

func (m *simpleMemoryKeyring) Name() string { return "memory" }

func (m *simpleMemoryKeyring) RetrieveMasterKey() ([]byte, error) {
	if m.key == nil {
		return nil, constants.ErrKeyStoreKeyNotFound
	}
	cp := make([]byte, len(m.key))
	copy(cp, m.key)
	return cp, nil
}

func (m *simpleMemoryKeyring) StoreMasterKey(key []byte) error {
	cp := make([]byte, len(key))
	copy(cp, key)
	m.key = cp
	return nil
}

func (m *simpleMemoryKeyring) DeleteMasterKey() error {
	m.key = nil
	return nil
}
