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

package keystore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWithBackend(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NotNil(t, ks)
	assert.Equal(t, secretsDir, ks.secretsDir)
	assert.Equal(t, logger, ks.logger)
	assert.Equal(t, backend, ks.backend)
}

func TestNewWithBackend_CreatesSecretsDir(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	baseDir := t.TempDir()
	secretsDir := filepath.Join(baseDir, "secrets")
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	_, err = NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)

	info, err := os.Stat(secretsDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestKeystore_Initialize_GeneratesNewKey(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)

	err = ks.Initialize()
	require.NoError(t, err)

	key, err := backend.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Len(t, key, keySize)
}

func TestKeystore_Initialize_RetrievesExistingKey(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	testKey := make([]byte, keySize)
	for i := range testKey {
		testKey[i] = byte(i)
	}
	err = backend.StoreMasterKey(testKey)
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)

	err = ks.Initialize()
	require.NoError(t, err)

	retrievedKey, err := backend.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrievedKey)
}

func TestKeystore_Initialize_RejectsInvalidKeyLength(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	shortKey := []byte("too-short")
	err = backend.StoreMasterKey(shortKey)
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)

	err = ks.Initialize()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid length")
}

func TestKeystore_EncryptSecret(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	err = ks.EncryptSecret("test-secret", "my-plaintext-value")
	require.NoError(t, err)

	secretPath := filepath.Join(secretsDir, "test-secret")
	info, err := os.Stat(secretPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Size())
}

func TestKeystore_EncryptSecret_Atomically(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	err = ks.EncryptSecret("test-secret", "my-plaintext-value")
	require.NoError(t, err)

	tmpPath := filepath.Join(secretsDir, "test-secret.tmp")
	_, err = os.Stat(tmpPath)
	assert.True(t, os.IsNotExist(err), "temp file should be cleaned up")
}

func TestKeystore_DecryptSecret(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	plaintext := "my-plaintext-value"
	err = ks.EncryptSecret("test-secret", plaintext)
	require.NoError(t, err)

	decrypted, err := ks.DecryptSecret("test-secret")
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestKeystore_DecryptSecret_MissingFile(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	_, err = ks.DecryptSecret("nonexistent-secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read encrypted secret")
}

func TestKeystore_DecryptSecret_CorruptedFile(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	corruptedData := []byte(`{"version":1,"nonce":"AAAA","ciphertext":"corrupted"}`)
	secretPath := filepath.Join(secretsDir, "test-secret")
	err = os.WriteFile(secretPath, corruptedData, 0600)
	require.NoError(t, err)

	_, err = ks.DecryptSecret("test-secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ciphertext")
}

func TestKeystore_DeleteSecret(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	err = ks.EncryptSecret("test-secret", "my-plaintext-value")
	require.NoError(t, err)

	err = ks.DeleteSecret("test-secret")
	require.NoError(t, err)

	secretPath := filepath.Join(secretsDir, "test-secret")
	_, err = os.Stat(secretPath)
	assert.True(t, os.IsNotExist(err))
}

func TestKeystore_DeleteSecret_Nonexistent(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	err = ks.DeleteSecret("nonexistent-secret")
	require.NoError(t, err)
}

func TestKeystore_Purge(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	err = ks.EncryptSecret("secret1", "value1")
	require.NoError(t, err)
	err = ks.EncryptSecret("secret2", "value2")
	require.NoError(t, err)

	err = ks.Purge()
	require.NoError(t, err)

	_, err = backend.RetrieveMasterKey()
	assert.Error(t, err)
	assert.Equal(t, ErrKeyNotFound, err)

	entries, err := os.ReadDir(secretsDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestKeystore_EnsurePermissions(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	err = ks.EncryptSecret("test-secret", "value")
	require.NoError(t, err)

	err = ks.EnsurePermissions()
	require.NoError(t, err)

	info, err := os.Stat(secretsDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())

	secretPath := filepath.Join(secretsDir, "test-secret")
	info, err = os.Stat(secretPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestKeystore_BackendName(t *testing.T) {
	t.Parallel()
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)

	assert.Equal(t, "test", ks.BackendName())
}
