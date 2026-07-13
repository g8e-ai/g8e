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

//go:build linux || windows

package keystore

import (
	"encoding/json"
	"errors"
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

func TestFileKeyring_StoreMasterKey_InvalidKeyLength(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)

	err = keyring.StoreMasterKey([]byte("too-short"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid master key length")
}

func TestFileKeyring_RetrieveMasterKey_InvalidBase64(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)

	keyPath := filepath.Join(secretsDir, constants.MasterKeyFilename)
	require.NoError(t, os.WriteFile(keyPath, []byte("!!!not-base64!!!"), 0600))

	_, err = keyring.RetrieveMasterKey()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode base64")
}

func TestFileKeyring_RetrieveMasterKey_EmptyFile(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)

	keyPath := filepath.Join(secretsDir, constants.MasterKeyFilename)
	require.NoError(t, os.WriteFile(keyPath, []byte(""), 0600))

	_, err = keyring.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
}

func TestFileKeyring_RetrieveMasterKey_PermissionDenied(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0000 does not prevent reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	secretsDir := testutil.TempDir(t)
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)

	keyPath := filepath.Join(secretsDir, constants.MasterKeyFilename)
	require.NoError(t, os.WriteFile(keyPath, []byte("data"), 0000))
	t.Cleanup(func() { _ = os.Chmod(keyPath, 0600) })

	_, err = keyring.RetrieveMasterKey()
	require.Error(t, err)
}

func TestFileKeyring_StoreAndRetrieve_RoundTrip(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	keyring, err := newFileKeyring(secretsDir)
	require.NoError(t, err)

	testKey := make([]byte, vault.KeySize)
	for i := range testKey {
		testKey[i] = byte(i)
	}

	require.NoError(t, keyring.StoreMasterKey(testKey))

	retrieved, err := keyring.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrieved)
}

func TestKeystore_NewWithKeyring_MkdirFail(t *testing.T) {
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	filePath := filepath.Join(testutil.TempDir(t), "notadir")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0644))

	_, err := NewWithKeyring(filePath, logger, keyring)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDirCreateFailed)
}

func TestKeystore_Initialize_RetrieveError(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	retrieveErr := errors.New("retrieve failed")
	keyring := &errorKeyring{retrieveErr: retrieveErr}

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	err = ks.Initialize()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreRetrieveFailed)
}

func TestKeystore_Encrypt_RetrieveError(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	retrieveErr := errors.New("retrieve failed")
	keyring := &errorKeyring{retrieveErr: retrieveErr}

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	_, err = ks.Encrypt("plaintext")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreRetrieveFailed)
}

func TestKeystore_EncryptSecret_RetrieveError(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	retrieveErr := errors.New("retrieve failed")
	keyring := &errorKeyring{retrieveErr: retrieveErr}

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	err = ks.EncryptSecret("test", "value")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreRetrieveFailed)
}

func TestKeystore_Decrypt_RetrieveError(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	retrieveErr := errors.New("retrieve failed")
	keyring := &errorKeyring{retrieveErr: retrieveErr}

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	_, err = ks.Decrypt("dGVzdA==")
	require.Error(t, err)
}

func TestKeystore_Purge_DeleteMasterKeyError(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	keyring := &errorKeyring{deleteErr: errors.New("delete failed")}

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	err = ks.Purge()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreDeleteFailed)
}

func TestKeystore_EnforcePermissions_DeletedDir(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(secretsDir))

	err = ks.EnforcePermissions()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreChmodDir)
}

func TestKeystore_Purge_WithSecrets(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
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

func TestKeystore_Initialize_StoreError(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	keyring := &errorKeyring{storeErr: errors.New("store failed")}

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	err = ks.Initialize()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreStoreFailed)
}

func TestKeystore_EnforcePermissions_ReadDirError(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(secretsDir))

	err = ks.EnforcePermissions()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreChmodDir)
}

func TestKeystore_Purge_ReadDirError(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(secretsDir))

	err = ks.Purge()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreReadDir)
}

func TestKeystore_DecryptSecret_CorruptCiphertext(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
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
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	keyring := &errorKeyring{storeErr: errors.New("store failed")}

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
	require.NoError(t, err)

	err = ks.EncryptSecret("test", "value")
	require.Error(t, err)
}

func TestKeystore_DeleteSecret_Error(t *testing.T) {
	t.Parallel()
	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	keyring := newMemoryKeyring()

	ks, err := NewWithKeyring(secretsDir, logger, keyring)
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
