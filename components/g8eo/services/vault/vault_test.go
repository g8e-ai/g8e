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

package vault

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/g8eo/testutil"
)

const (
	testAPIKey1 = "g8e_test_abc123xyz789_TESTKEY1"
	testAPIKey2 = "g8e_test_def456uvw012_TESTKEY2"
)

func TestDeriveKEK(t *testing.T) {
	t.Run("deterministic derivation", func(t *testing.T) {
		kek1, err := DeriveKEK(testAPIKey1)
		require.NoError(t, err)
		require.Len(t, kek1, KeySize)

		kek2, err := DeriveKEK(testAPIKey1)
		require.NoError(t, err)

		assert.Equal(t, kek1, kek2, "same API key should produce same KEK")
	})

	t.Run("different keys produce different KEKs", func(t *testing.T) {
		kek1, err := DeriveKEK(testAPIKey1)
		require.NoError(t, err)

		kek2, err := DeriveKEK(testAPIKey2)
		require.NoError(t, err)

		assert.NotEqual(t, kek1, kek2, "different API keys should produce different KEKs")
	})

	t.Run("empty key rejected", func(t *testing.T) {
		_, err := DeriveKEK("")
		assert.Error(t, err)
	})
}

func TestGenerateDEK(t *testing.T) {
	t.Run("generates correct size", func(t *testing.T) {
		dek, err := GenerateDEK()
		require.NoError(t, err)
		assert.Len(t, dek, KeySize)
	})

	t.Run("generates unique keys", func(t *testing.T) {
		dek1, err := GenerateDEK()
		require.NoError(t, err)

		dek2, err := GenerateDEK()
		require.NoError(t, err)

		assert.NotEqual(t, dek1, dek2, "generated DEKs should be unique")
	})
}

func TestGenerateNonce(t *testing.T) {
	t.Run("generates correct size", func(t *testing.T) {
		nonce, err := GenerateNonce()
		require.NoError(t, err)
		assert.Len(t, nonce, NonceSize)
	})

	t.Run("generates unique nonces", func(t *testing.T) {
		nonce1, err := GenerateNonce()
		require.NoError(t, err)

		nonce2, err := GenerateNonce()
		require.NoError(t, err)

		assert.NotEqual(t, nonce1, nonce2, "generated nonces should be unique")
	})
}

func TestKeyFingerprint(t *testing.T) {
	t.Run("correct size", func(t *testing.T) {
		key := []byte("test-key-material")
		fp := KeyFingerprint(key)
		assert.Len(t, fp, KeyFingerprintSize)
	})

	t.Run("deterministic", func(t *testing.T) {
		key := []byte("test-key-material")
		fp1 := KeyFingerprint(key)
		fp2 := KeyFingerprint(key)
		assert.Equal(t, fp1, fp2)
	})

	t.Run("different keys produce different fingerprints", func(t *testing.T) {
		fp1 := KeyFingerprint([]byte("key1"))
		fp2 := KeyFingerprint([]byte("key2"))
		assert.NotEqual(t, fp1, fp2)
	})
}

func TestAESKeyWrapUnwrap(t *testing.T) {
	t.Run("wrap and unwrap 32-byte key", func(t *testing.T) {
		kek, err := GenerateDEK()
		require.NoError(t, err)

		plaintext, err := GenerateDEK()
		require.NoError(t, err)

		wrapped, err := AESKeyWrap(kek, plaintext)
		require.NoError(t, err)
		assert.Len(t, wrapped, len(plaintext)+8)

		unwrapped, err := AESKeyUnwrap(kek, wrapped)
		require.NoError(t, err)
		assert.Equal(t, plaintext, unwrapped)
	})

	t.Run("wrap and unwrap 16-byte key", func(t *testing.T) {
		kek, _ := GenerateDEK()
		plaintext := make([]byte, 16)
		copy(plaintext, []byte("sixteen-byte-key"))

		wrapped, err := AESKeyWrap(kek, plaintext)
		require.NoError(t, err)

		unwrapped, err := AESKeyUnwrap(kek, wrapped)
		require.NoError(t, err)
		assert.Equal(t, plaintext, unwrapped)
	})

	t.Run("wrong KEK fails unwrap", func(t *testing.T) {
		kek1, _ := GenerateDEK()
		kek2, _ := GenerateDEK()
		plaintext, _ := GenerateDEK()

		wrapped, err := AESKeyWrap(kek1, plaintext)
		require.NoError(t, err)

		_, err = AESKeyUnwrap(kek2, wrapped)
		assert.ErrorIs(t, err, ErrKeyUnwrapFailed)
	})

	t.Run("tampered ciphertext fails unwrap", func(t *testing.T) {
		kek, _ := GenerateDEK()
		plaintext, _ := GenerateDEK()

		wrapped, err := AESKeyWrap(kek, plaintext)
		require.NoError(t, err)

		wrapped[10] ^= 0xFF

		_, err = AESKeyUnwrap(kek, wrapped)
		assert.ErrorIs(t, err, ErrKeyUnwrapFailed)
	})

	t.Run("invalid key sizes rejected", func(t *testing.T) {
		invalidKEK := make([]byte, 15)
		plaintext, _ := GenerateDEK()

		_, err := AESKeyWrap(invalidKEK, plaintext)
		assert.ErrorIs(t, err, ErrInvalidKeySize)
	})

	t.Run("invalid plaintext size rejected", func(t *testing.T) {
		kek, _ := GenerateDEK()
		invalidPlaintext := make([]byte, 15)

		_, err := AESKeyWrap(kek, invalidPlaintext)
		assert.ErrorIs(t, err, ErrInvalidPlaintextKey)
	})

	t.Run("too short plaintext rejected", func(t *testing.T) {
		kek, _ := GenerateDEK()
		shortPlaintext := make([]byte, 8)

		_, err := AESKeyWrap(kek, shortPlaintext)
		assert.ErrorIs(t, err, ErrInvalidPlaintextKey)
	})
}

func TestAESGCMEncryptDecrypt(t *testing.T) {
	t.Run("encrypt and decrypt", func(t *testing.T) {
		key, _ := GenerateDEK()
		nonce, _ := GenerateNonce()
		plaintext := []byte("Hello, World! This is a test message.")

		ciphertext, err := EncryptAESGCM(key, nonce, plaintext, nil)
		require.NoError(t, err)
		assert.NotEqual(t, plaintext, ciphertext)

		decrypted, err := DecryptAESGCM(key, nonce, ciphertext, nil)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("with additional data", func(t *testing.T) {
		key, _ := GenerateDEK()
		nonce, _ := GenerateNonce()
		plaintext := []byte("Secret message")
		aad := []byte("additional authenticated data")

		ciphertext, err := EncryptAESGCM(key, nonce, plaintext, aad)
		require.NoError(t, err)

		decrypted, err := DecryptAESGCM(key, nonce, ciphertext, aad)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("wrong AAD fails decryption", func(t *testing.T) {
		key, _ := GenerateDEK()
		nonce, _ := GenerateNonce()
		plaintext := []byte("Secret message")
		aad := []byte("correct aad")

		ciphertext, err := EncryptAESGCM(key, nonce, plaintext, aad)
		require.NoError(t, err)

		_, err = DecryptAESGCM(key, nonce, ciphertext, []byte("wrong aad"))
		assert.ErrorIs(t, err, ErrDecryptionFailed)
	})

	t.Run("wrong key fails decryption", func(t *testing.T) {
		key1, _ := GenerateDEK()
		key2, _ := GenerateDEK()
		nonce, _ := GenerateNonce()
		plaintext := []byte("Secret message")

		ciphertext, err := EncryptAESGCM(key1, nonce, plaintext, nil)
		require.NoError(t, err)

		_, err = DecryptAESGCM(key2, nonce, ciphertext, nil)
		assert.ErrorIs(t, err, ErrDecryptionFailed)
	})

	t.Run("tampered ciphertext fails decryption", func(t *testing.T) {
		key, _ := GenerateDEK()
		nonce, _ := GenerateNonce()
		plaintext := []byte("Secret message")

		ciphertext, err := EncryptAESGCM(key, nonce, plaintext, nil)
		require.NoError(t, err)

		ciphertext[5] ^= 0xFF

		_, err = DecryptAESGCM(key, nonce, ciphertext, nil)
		assert.ErrorIs(t, err, ErrDecryptionFailed)
	})

	t.Run("invalid key size rejected", func(t *testing.T) {
		invalidKey := make([]byte, 16)
		nonce, _ := GenerateNonce()
		plaintext := []byte("test")

		_, err := EncryptAESGCM(invalidKey, nonce, plaintext, nil)
		assert.ErrorIs(t, err, ErrInvalidKeySize)
	})

	t.Run("invalid nonce size rejected", func(t *testing.T) {
		key, _ := GenerateDEK()
		invalidNonce := make([]byte, 8)
		plaintext := []byte("test")

		_, err := EncryptAESGCM(key, invalidNonce, plaintext, nil)
		assert.ErrorIs(t, err, ErrInvalidNonceSize)
	})
}

func TestSecureZero(t *testing.T) {
	t.Run("zeros out slice", func(t *testing.T) {
		data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		SecureZero(data)

		for _, b := range data {
			assert.Equal(t, byte(0), b)
		}
	})
}

func TestNewVaultHeader(t *testing.T) {
	t.Run("creates valid header", func(t *testing.T) {
		header, dek, err := NewVaultHeader(testAPIKey1)
		require.NoError(t, err)
		require.NotNil(t, header)
		require.NotNil(t, dek)
		require.Len(t, dek, KeySize)

		assert.Equal(t, VaultHeaderVersion, header.Version)
		assert.Equal(t, KDFAlgorithm, header.KDF.Algorithm)
		assert.Equal(t, KEKAlgorithm, header.KEK.Algorithm)
		assert.Equal(t, DEKAlgorithm, header.DEK.Algorithm)
		assert.NotEmpty(t, header.DEK.Wrapped)
		assert.NotEmpty(t, header.KeyFingerprint)

		SecureZero(dek)
	})

	t.Run("fingerprint matches API key", func(t *testing.T) {
		header, dek, err := NewVaultHeader(testAPIKey1)
		require.NoError(t, err)
		defer SecureZero(dek)

		expectedFP := hex.EncodeToString(APIKeyFingerprint(testAPIKey1))
		assert.Equal(t, expectedFP, header.KeyFingerprint)
	})
}

func TestVaultHeaderUnwrapDEK(t *testing.T) {
	t.Run("unwrap with correct key", func(t *testing.T) {
		header, originalDEK, err := NewVaultHeader(testAPIKey1)
		require.NoError(t, err)
		defer SecureZero(originalDEK)

		unwrappedDEK, err := header.UnwrapDEK(testAPIKey1)
		require.NoError(t, err)
		defer SecureZero(unwrappedDEK)

		assert.Equal(t, originalDEK, unwrappedDEK)
	})

	t.Run("unwrap with wrong key fails", func(t *testing.T) {
		header, dek, err := NewVaultHeader(testAPIKey1)
		require.NoError(t, err)
		defer SecureZero(dek)

		_, err = header.UnwrapDEK(testAPIKey2)
		assert.ErrorIs(t, err, ErrKeyFingerprintMatch)
	})
}

func TestVaultHeaderRekey(t *testing.T) {
	t.Run("rekey to new API key", func(t *testing.T) {
		header, originalDEK, err := NewVaultHeader(testAPIKey1)
		require.NoError(t, err)
		defer SecureZero(originalDEK)

		err = header.Rekey(testAPIKey1, testAPIKey2)
		require.NoError(t, err)

		_, err = header.UnwrapDEK(testAPIKey1)
		assert.ErrorIs(t, err, ErrKeyFingerprintMatch)

		unwrappedDEK, err := header.UnwrapDEK(testAPIKey2)
		require.NoError(t, err)
		defer SecureZero(unwrappedDEK)

		assert.Equal(t, originalDEK, unwrappedDEK)
		assert.NotNil(t, header.LastRekeyedAt)
	})

	t.Run("rekey with wrong old key fails", func(t *testing.T) {
		header, dek, err := NewVaultHeader(testAPIKey1)
		require.NoError(t, err)
		defer SecureZero(dek)

		err = header.Rekey(testAPIKey2, testAPIKey2)
		assert.Error(t, err)
	})
}

func TestVaultHeaderSaveLoad(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("save and load", func(t *testing.T) {
		header, dek, err := NewVaultHeader(testAPIKey1)
		require.NoError(t, err)
		defer SecureZero(dek)

		err = header.Save(tempDir)
		require.NoError(t, err)

		loaded, err := LoadVaultHeader(tempDir)
		require.NoError(t, err)

		assert.Equal(t, header.Version, loaded.Version)
		assert.Equal(t, header.KeyFingerprint, loaded.KeyFingerprint)
		assert.Equal(t, header.DEK.Wrapped, loaded.DEK.Wrapped)
	})

	t.Run("load non-existent returns error", func(t *testing.T) {
		_, err := LoadVaultHeader(filepath.Join(tempDir, "nonexistent"))
		assert.ErrorIs(t, err, ErrHeaderNotFound)
	})
}

func TestVaultHeaderExists(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("returns false when not exists", func(t *testing.T) {
		assert.False(t, VaultHeaderExists(tempDir))
	})

	t.Run("returns true when exists", func(t *testing.T) {
		header, dek, _ := NewVaultHeader(testAPIKey1)
		defer SecureZero(dek)
		header.Save(tempDir)

		assert.True(t, VaultHeaderExists(tempDir))
	})
}

func TestVaultHeaderValidateAPIKey(t *testing.T) {
	header, dek, _ := NewVaultHeader(testAPIKey1)
	defer SecureZero(dek)

	t.Run("correct key validates", func(t *testing.T) {
		assert.True(t, header.ValidateAPIKey(testAPIKey1))
	})

	t.Run("wrong key fails validation", func(t *testing.T) {
		assert.False(t, header.ValidateAPIKey(testAPIKey2))
	})
}

func newTestVault(t *testing.T, dataDir, apiKey string) *Vault {
	t.Helper()
	require.NoError(t, os.MkdirAll(dataDir, 0700))
	header, dek, err := NewVaultHeader(apiKey)
	require.NoError(t, err)
	SecureZero(dek)
	require.NoError(t, header.Save(dataDir))
	v, err := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	require.NoError(t, v.Unlock(apiKey))
	return v
}

func TestVaultUnlock(t *testing.T) {
	t.Run("unlock existing vault", func(t *testing.T) {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")

		vault1 := newTestVault(t, dataDir, testAPIKey1)
		vault1.Close()

		vault2, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
		defer vault2.Close()

		err := vault2.Unlock(testAPIKey1)
		require.NoError(t, err)
		assert.True(t, vault2.IsUnlocked())
	})

	t.Run("unlock with wrong key fails", func(t *testing.T) {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")

		vault1 := newTestVault(t, dataDir, testAPIKey1)
		vault1.Close()

		vault2, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
		defer vault2.Close()

		err := vault2.Unlock(testAPIKey2)
		assert.ErrorIs(t, err, ErrInvalidAPIKey)
	})

	t.Run("unlock non-existent vault fails", func(t *testing.T) {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "nonexistent")

		vault, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
		defer vault.Close()

		err := vault.Unlock(testAPIKey1)
		assert.ErrorIs(t, err, ErrVaultNotInit)
	})
}

func TestVaultRekey(t *testing.T) {
	t.Run("rekey vault", func(t *testing.T) {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")

		vault := newTestVault(t, dataDir, testAPIKey1)

		originalDEK, err := vault.GetDEK()
		require.NoError(t, err)
		defer SecureZero(originalDEK)

		vault.Close()

		vault2, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
		defer vault2.Close()

		err = vault2.Rekey(testAPIKey1, testAPIKey2)
		require.NoError(t, err)

		err = vault2.Unlock(testAPIKey1)
		assert.ErrorIs(t, err, ErrInvalidAPIKey)

		vault3, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
		defer vault3.Close()

		err = vault3.Unlock(testAPIKey2)
		require.NoError(t, err)

		newDEK, err := vault3.GetDEK()
		require.NoError(t, err)
		defer SecureZero(newDEK)

		assert.Equal(t, originalDEK, newDEK)
	})
}

func TestVaultLock(t *testing.T) {
	t.Run("lock clears DEK", func(t *testing.T) {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")

		vault := newTestVault(t, dataDir, testAPIKey1)
		defer vault.Close()

		assert.True(t, vault.IsUnlocked())

		vault.Lock()
		assert.False(t, vault.IsUnlocked())

		_, err := vault.GetDEK()
		assert.ErrorIs(t, err, ErrVaultLocked)
	})
}

func TestVaultEncryptDecrypt(t *testing.T) {
	t.Run("encrypt and decrypt", func(t *testing.T) {
		tempDir := t.TempDir()
		v := newTestVault(t, filepath.Join(tempDir, "data"), testAPIKey1)
		defer v.Close()

		plaintext := []byte("This is a secret message that needs encryption.")

		ciphertext, err := v.Encrypt(plaintext)
		require.NoError(t, err)
		assert.NotEqual(t, plaintext, ciphertext)
		assert.Greater(t, len(ciphertext), len(plaintext))

		decrypted, err := v.Decrypt(ciphertext)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("encrypt empty data", func(t *testing.T) {
		tempDir := t.TempDir()
		v := newTestVault(t, filepath.Join(tempDir, "data"), testAPIKey1)
		defer v.Close()

		plaintext := []byte{}

		ciphertext, err := v.Encrypt(plaintext)
		require.NoError(t, err)

		decrypted, err := v.Decrypt(ciphertext)
		require.NoError(t, err)
		assert.Len(t, decrypted, 0)
	})

	t.Run("large data", func(t *testing.T) {
		tempDir := t.TempDir()
		v := newTestVault(t, filepath.Join(tempDir, "data"), testAPIKey1)
		defer v.Close()

		plaintext := bytes.Repeat([]byte("Large data block. "), 10000)

		ciphertext, err := v.Encrypt(plaintext)
		require.NoError(t, err)

		decrypted, err := v.Decrypt(ciphertext)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("decrypt ciphertext too short", func(t *testing.T) {
		tempDir := t.TempDir()
		v := newTestVault(t, filepath.Join(tempDir, "data"), testAPIKey1)
		defer v.Close()

		_, err := v.Decrypt([]byte("short"))
		assert.Error(t, err)
	})

	t.Run("encrypt fails when locked", func(t *testing.T) {
		tempDir := t.TempDir()
		v := newTestVault(t, filepath.Join(tempDir, "data"), testAPIKey1)
		defer v.Close()

		v.Lock()
		_, err := v.Encrypt([]byte("test"))
		assert.ErrorIs(t, err, ErrVaultLocked)
	})

	t.Run("decrypt fails when locked", func(t *testing.T) {
		tempDir := t.TempDir()
		v := newTestVault(t, filepath.Join(tempDir, "data"), testAPIKey1)
		defer v.Close()

		v.Lock()
		_, err := v.Decrypt(make([]byte, 32))
		assert.ErrorIs(t, err, ErrVaultLocked)
	})
}

func TestVaultVerifyIntegrity(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")

	vault := newTestVault(t, dataDir, testAPIKey1)
	vault.Close()

	t.Run("verify with correct key", func(t *testing.T) {
		vault2, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
		defer vault2.Close()

		err := vault2.VerifyIntegrity(testAPIKey1)
		assert.NoError(t, err)
	})

	t.Run("verify with wrong key fails", func(t *testing.T) {
		vault2, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
		defer vault2.Close()

		err := vault2.VerifyIntegrity(testAPIKey2)
		assert.Error(t, err)
	})
}

func TestVaultReset(t *testing.T) {
	t.Run("reset requires confirmation", func(t *testing.T) {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")

		vault := newTestVault(t, dataDir, testAPIKey1)

		err := vault.Reset(false)
		assert.Error(t, err)
		assert.True(t, vault.IsInitialized())
	})

	t.Run("reset destroys vault", func(t *testing.T) {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")

		vault := newTestVault(t, dataDir, testAPIKey1)

		err := vault.Reset(true)
		require.NoError(t, err)

		assert.False(t, vault.IsUnlocked())
		assert.False(t, vault.IsInitialized())
	})
}

func TestVaultFullLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")

	vault1 := newTestVault(t, dataDir, testAPIKey1)

	secretData := []byte("Highly confidential information")
	encrypted, err := vault1.Encrypt(secretData)
	require.NoError(t, err)

	vault1.Close()

	vault2, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
	err = vault2.Unlock(testAPIKey1)
	require.NoError(t, err)

	decrypted, err := vault2.Decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, secretData, decrypted)

	vault2.Close()

	vault3, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
	err = vault3.Rekey(testAPIKey1, testAPIKey2)
	require.NoError(t, err)
	vault3.Close()

	vault4, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
	err = vault4.Unlock(testAPIKey1)
	assert.ErrorIs(t, err, ErrInvalidAPIKey)
	vault4.Close()

	vault5, _ := NewVault(&VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
	err = vault5.Unlock(testAPIKey2)
	require.NoError(t, err)

	decrypted2, err := vault5.Decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, secretData, decrypted2)

	vault5.Close()
}

func TestNewVault(t *testing.T) {
	t.Run("nil config rejected", func(t *testing.T) {
		_, err := NewVault(nil)
		assert.Error(t, err)
	})

	t.Run("empty data dir rejected", func(t *testing.T) {
		_, err := NewVault(&VaultConfig{DataDir: "", Logger: testutil.NewTestLogger()})
		assert.Error(t, err)
	})

	t.Run("nil logger uses default", func(t *testing.T) {
		tempDir := t.TempDir()
		v, err := NewVault(&VaultConfig{DataDir: tempDir})
		require.NoError(t, err)
		defer v.Close()
		assert.NotNil(t, v)
	})
}

func TestVaultUnlockAlreadyOpen(t *testing.T) {
	t.Run("unlock already unlocked vault fails", func(t *testing.T) {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")

		v := newTestVault(t, dataDir, testAPIKey1)
		defer v.Close()

		err := v.Unlock(testAPIKey1)
		assert.ErrorIs(t, err, ErrVaultAlreadyOpen)
	})
}

func TestVaultGetDataDir(t *testing.T) {
	t.Run("returns configured data dir", func(t *testing.T) {
		tempDir := t.TempDir()
		dataDir := filepath.Join(tempDir, "data")

		v := newTestVault(t, dataDir, testAPIKey1)
		defer v.Close()

		assert.Equal(t, dataDir, v.GetDataDir())
	})
}

func TestAESKeyUnwrapInvalidKEK(t *testing.T) {
	t.Run("invalid KEK size rejected on unwrap", func(t *testing.T) {
		invalidKEK := make([]byte, 15)
		wrapped := make([]byte, 40)

		_, err := AESKeyUnwrap(invalidKEK, wrapped)
		assert.ErrorIs(t, err, ErrInvalidKeySize)
	})
}

func TestDeleteVaultHeader(t *testing.T) {
	t.Run("deletes existing header", func(t *testing.T) {
		tempDir := t.TempDir()

		header, dek, err := NewVaultHeader(testAPIKey1)
		require.NoError(t, err)
		defer SecureZero(dek)

		require.NoError(t, header.Save(tempDir))
		assert.True(t, VaultHeaderExists(tempDir))

		require.NoError(t, DeleteVaultHeader(tempDir))
		assert.False(t, VaultHeaderExists(tempDir))
	})

	t.Run("delete non-existent is no-op", func(t *testing.T) {
		tempDir := t.TempDir()
		err := DeleteVaultHeader(filepath.Join(tempDir, "nonexistent"))
		assert.NoError(t, err)
	})
}

func TestLoadVaultHeaderCorrupted(t *testing.T) {
	t.Run("corrupted JSON returns error", func(t *testing.T) {
		tempDir := t.TempDir()
		headerPath := filepath.Join(tempDir, VaultHeaderFile)

		require.NoError(t, os.WriteFile(headerPath, []byte("not valid json {{{"), 0600))

		_, err := LoadVaultHeader(tempDir)
		assert.ErrorIs(t, err, ErrHeaderCorrupted)
	})
}

func TestVaultConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")

	v := newTestVault(t, dataDir, testAPIKey1)
	defer v.Close()

	var wg sync.WaitGroup
	errors := make([]error, 10)
	matches := make([]bool, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			plaintext := []byte("Concurrent test data")
			encrypted, err := v.Encrypt(plaintext)
			if err != nil {
				errors[idx] = err
				return
			}
			decrypted, err := v.Decrypt(encrypted)
			if err != nil {
				errors[idx] = err
				return
			}
			matches[idx] = bytes.Equal(plaintext, decrypted)
		}(i)
	}

	t.Cleanup(wg.Wait)
	wg.Wait()

	for i := 0; i < 10; i++ {
		require.NoError(t, errors[i])
		assert.True(t, matches[i])
	}
}
