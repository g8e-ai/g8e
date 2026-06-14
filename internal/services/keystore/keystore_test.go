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
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWithBackend(t *testing.T) {
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid length")
}

func TestKeystore_EncryptSecret(t *testing.T) {
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
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	_, err = ks.DecryptSecret("nonexistent-secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read encrypted secret")
}

func TestKeystore_DecryptSecret_CorruptedFile(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	corruptedData := []byte(`{"version":1,"nonce":"AAAAAA==","ciphertext":"corrupted"}`)
	secretPath := filepath.Join(secretsDir, "test-secret")
	err = os.WriteFile(secretPath, corruptedData, 0600)
	require.NoError(t, err)

	_, err = ks.DecryptSecret("test-secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal encrypted secret")
}

func TestKeystore_DeleteSecret(t *testing.T) {
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
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)

	entries, err := os.ReadDir(secretsDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestKeystore_EnsurePermissions(t *testing.T) {
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

	err = ks.EnforcePermissions()
	require.NoError(t, err)

	info, err := os.Stat(secretsDir)
	require.NoError(t, err)
	// Windows doesn't support Unix permissions exactly, so skip the permission check on Windows
	if runtime.GOOS != "windows" {
		perm := info.Mode().Perm()
		assert.NotEqual(t, os.FileMode(0777), perm, "directory should not be world-writable")
	}

	secretPath := filepath.Join(secretsDir, "test-secret")
	info, err = os.Stat(secretPath)
	require.NoError(t, err)
	// Windows doesn't support Unix permissions exactly, so skip the permission check on Windows
	if runtime.GOOS != "windows" {
		perm := info.Mode().Perm()
		assert.NotEqual(t, os.FileMode(0777), perm, "secret file should not be world-writable")
	}
}

func TestKeystore_BackendName(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)

	assert.Equal(t, "test", ks.BackendName())
}

func TestKeystore_Encrypt(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	plaintext := "my-plaintext-value"
	encoded, err := ks.Encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)
	assert.NotEqual(t, plaintext, encoded)
}

func TestKeystore_Decrypt(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	plaintext := "my-plaintext-value"
	encoded, err := ks.Encrypt(plaintext)
	require.NoError(t, err)

	decrypted, err := ks.Decrypt(encoded)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestKeystore_EncryptDecrypt_RoundTrip(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	testCases := []string{
		"simple text",
		"text with special chars: !@#$%^&*()",
		"text with unicode: café 日本語 🚀",
		"multi\nline\ntext",
		"very long text: " + string(make([]byte, 1000)),
	}

	for _, plaintext := range testCases {
		t.Run(plaintext[:min(20, len(plaintext))], func(t *testing.T) {
			encoded, err := ks.Encrypt(plaintext)
			require.NoError(t, err)

			decrypted, err := ks.Decrypt(encoded)
			require.NoError(t, err)
			assert.Equal(t, plaintext, decrypted)
		})
	}
}

func TestKeystore_Decrypt_InvalidBase64(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	_, err = ks.Decrypt("not-valid-base64!!!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode base64")
}

func TestKeystore_Decrypt_InvalidJSON(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	invalidJSON := base64.StdEncoding.EncodeToString([]byte(`{invalid json`))
	_, err = ks.Decrypt(invalidJSON)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal encrypted value")
}

func TestKeystore_Decrypt_UnsupportedVersion(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	enc := EncryptedSecret{
		Version:    999,
		Nonce:      make([]byte, nonceSize),
		Ciphertext: []byte("fake"),
	}
	data, err := json.Marshal(enc)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(data)

	_, err = ks.Decrypt(encoded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported secret version")
}

func TestKeystore_Decrypt_InvalidCiphertext(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	enc := EncryptedSecret{
		Version:    keyVersion,
		Nonce:      make([]byte, nonceSize),
		Ciphertext: []byte("invalid ciphertext that will fail GCM"),
	}
	data, err := json.Marshal(enc)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(data)

	_, err = ks.Decrypt(encoded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ciphertext")
}

func TestKeystore_Encrypt_EmptyPlaintext(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	encoded, err := ks.Encrypt("")
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decrypted, err := ks.Decrypt(encoded)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestKeystore_Encrypt_LargePlaintext(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	largePlaintext := string(make([]byte, 10000))
	for i := range largePlaintext {
		largePlaintext = largePlaintext[:i] + "A" + largePlaintext[i+1:]
	}
	encoded, err := ks.Encrypt(largePlaintext)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decrypted, err := ks.Decrypt(encoded)
	require.NoError(t, err)
	assert.Equal(t, largePlaintext, decrypted)
}

func TestKeystore_DecryptSecret_InvalidJSON(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	invalidJSON := []byte(`{invalid json`)
	secretPath := filepath.Join(secretsDir, "test-secret")
	err = os.WriteFile(secretPath, invalidJSON, 0600)
	require.NoError(t, err)

	_, err = ks.DecryptSecret("test-secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal encrypted secret")
}

func TestKeystore_DecryptSecret_UnsupportedVersion(t *testing.T) {
	ResetTestStorage()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := NewTestBackend()
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	enc := EncryptedSecret{
		Version:    999,
		Nonce:      make([]byte, nonceSize),
		Ciphertext: []byte("fake"),
	}
	data, err := json.Marshal(enc)
	require.NoError(t, err)
	secretPath := filepath.Join(secretsDir, "test-secret")
	err = os.WriteFile(secretPath, data, 0600)
	require.NoError(t, err)

	_, err = ks.DecryptSecret("test-secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported secret version")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
