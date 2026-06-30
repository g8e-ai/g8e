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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/vault"
)

const (
	keyStoreName  = "g8e-platform"
	masterKeyName = "master-encryption-key"
	keyVersion    = 1
)

// EncryptedSecret represents an encrypted secret value on disk.
type EncryptedSecret struct {
	Version    int    `json:"version"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// Keystore provides OS-native key storage and encryption for platform secrets.
type Keystore struct {
	logger     *slog.Logger
	secretsDir string
	keyring    Keyring
}

// NewWithKeyring creates a new Keystore instance with a custom keyring.
// This is primarily used for testing with the in-memory memory keyring.
func NewWithKeyring(secretsDir string, logger *slog.Logger, keyring Keyring) (*Keystore, error) {
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	return &Keystore{
		logger:     logger,
		secretsDir: secretsDir,
		keyring:    keyring,
	}, nil
}

// Initialize creates or retrieves the master encryption key from the OS key store.
func (k *Keystore) Initialize() error {
	key, err := k.keyring.RetrieveMasterKey()
	if err != nil {
		if errors.Is(err, constants.ErrKeyStoreKeyNotFound) {
			k.logger.Info("[Keystore] Master key not found, generating new key", "keyring", k.keyring.Name())
			return k.generateAndStoreMasterKey()
		}
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreRetrieveFailed, err)
	}
	defer vault.SecureZero(key)

	if len(key) != vault.KeySize {
		return fmt.Errorf("%w: got %d, expected %d", constants.ErrKeyStoreInvalidKeyLength, len(key), vault.KeySize)
	}

	k.logger.Info("[Keystore] Master key retrieved from OS key store", "keyring", k.keyring.Name())
	return nil
}

// generateAndStoreMasterKey generates a new AES-256 key and stores it in the OS key store.
func (k *Keystore) generateAndStoreMasterKey() error {
	key := make([]byte, vault.KeySize)
	defer vault.SecureZero(key)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreGenerateFailed, err)
	}

	if err := k.keyring.StoreMasterKey(key); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreStoreFailed, err)
	}

	k.logger.Info("[Keystore] Master key generated and stored in OS key store", "keyring", k.keyring.Name())
	return nil
}

// encrypt performs AES-256-GCM encryption on plaintext and returns the EncryptedSecret structure.
func (k *Keystore) encrypt(plaintext string) (*EncryptedSecret, error) {
	key, err := k.keyring.RetrieveMasterKey()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrKeyStoreRetrieveFailed, err)
	}
	defer vault.SecureZero(key)

	nonce, err := vault.GenerateNonce()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrKeyStoreNonceGenerate, err)
	}

	ciphertext, err := vault.EncryptAESGCM(key, nonce, []byte(plaintext), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrKeyStoreCipherCreate, err)
	}

	return &EncryptedSecret{
		Version:    keyVersion,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

// decrypt performs AES-256-GCM decryption on an EncryptedSecret and returns plaintext.
func (k *Keystore) decrypt(enc *EncryptedSecret) (string, error) {
	if enc.Version != keyVersion {
		return "", fmt.Errorf("%w: got %d, expected %d", constants.ErrKeyStoreUnsupportedVersion, enc.Version, keyVersion)
	}

	key, err := k.keyring.RetrieveMasterKey()
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrKeyStoreRetrieveFailed, err)
	}
	defer vault.SecureZero(key)

	plaintext, err := vault.DecryptAESGCM(key, enc.Nonce, enc.Ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInvalidCiphertext, err)
	}

	return string(plaintext), nil
}

// EncryptSecret encrypts a plaintext secret value and writes it to disk.
func (k *Keystore) EncryptSecret(name, plaintext string) error {
	enc, err := k.encrypt(plaintext)
	if err != nil {
		return err
	}

	data, err := json.Marshal(enc)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreMarshalFailed, err)
	}

	path := filepath.Join(k.secretsDir, name)
	tmpPath := path + constants.TmpFileSuffix
	if err := os.WriteFile(tmpPath, data, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreWriteFailed, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreRenameFailed, err)
	}

	k.logger.Debug("[Keystore] Secret encrypted and written", "name", name)
	return nil
}

// DecryptSecret reads and decrypts a secret value from disk.
func (k *Keystore) DecryptSecret(name string) (string, error) {
	path := filepath.Join(k.secretsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrKeyStoreReadFailed, err)
	}

	var enc EncryptedSecret
	if err := json.Unmarshal(data, &enc); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrKeyStoreUnmarshalFailed, err)
	}

	plaintext, err := k.decrypt(&enc)
	if err != nil {
		return "", err
	}

	k.logger.Debug("[Keystore] Secret decrypted", "name", name)
	return plaintext, nil
}

// Encrypt encrypts a plaintext value and returns a base64-encoded ciphertext string.
// This is for in-memory encryption (e.g., SQLite values), not file-based secrets.
func (k *Keystore) Encrypt(plaintext string) (string, error) {
	enc, err := k.encrypt(plaintext)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(enc)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrKeyStoreMarshalFailed, err)
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// Decrypt decrypts a base64-encoded ciphertext string and returns the plaintext.
// This is for in-memory decryption (e.g., SQLite values), not file-based secrets.
func (k *Keystore) Decrypt(encodedCiphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrKeyStoreDecodeBase64, err)
	}

	var enc EncryptedSecret
	if err := json.Unmarshal(data, &enc); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrKeyStoreUnmarshalFailed, err)
	}

	return k.decrypt(&enc)
}

// DeleteSecret removes a secret file from disk.
func (k *Keystore) DeleteSecret(name string) error {
	path := filepath.Join(k.secretsDir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreDeleteSecret, err)
	}
	k.logger.Debug("[Keystore] Secret deleted", "name", name)
	return nil
}

// Purge removes all secrets from disk and deletes the master key from the OS key store.
func (k *Keystore) Purge() error {
	if err := k.keyring.DeleteMasterKey(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreDeleteFailed, err)
	}

	entries, err := os.ReadDir(k.secretsDir)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreReadDir, err)
	}

	var purgeErrors []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(k.secretsDir, entry.Name())
		if err := os.Remove(path); err != nil {
			purgeErrors = append(purgeErrors, fmt.Errorf("%w: %s: %w", constants.ErrKeyStoreDeleteFile, path, err))
		}
	}

	if len(purgeErrors) > 0 {
		return fmt.Errorf("%w: %d errors, first: %w", constants.ErrKeyStorePurgeFailed, len(purgeErrors), purgeErrors[0])
	}

	k.logger.Info("[Keystore] All secrets purged", "keyring", k.keyring.Name())
	return nil
}

// EnforcePermissions enforces strict filesystem permissions on the secrets directory.
func (k *Keystore) EnforcePermissions() error {
	if err := os.Chmod(k.secretsDir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreChmodDir, err)
	}

	entries, err := os.ReadDir(k.secretsDir)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreReadDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(k.secretsDir, entry.Name())
		if err := os.Chmod(path, constants.PermFilePrivate); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrKeyStoreChmodFile, err)
		}
	}

	k.logger.Debug("[Keystore] Permissions enforced", "dir", k.secretsDir)
	return nil
}

// KeyringName returns the name of the OS-native keyring.
func (k *Keystore) KeyringName() string {
	return k.keyring.Name()
}
