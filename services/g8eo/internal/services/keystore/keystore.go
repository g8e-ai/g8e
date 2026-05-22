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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

var (
	ErrKeyNotFound       = errors.New("master key not found in OS key store")
	ErrKeyStoreLocked    = errors.New("OS key store is locked/unavailable")
	ErrInvalidCiphertext = errors.New("invalid ciphertext or authentication failed")
	ErrOSNotSupported    = errors.New("OS not supported for OS-native key store")
)

const (
	keyStoreName  = "g8e-platform"
	masterKeyName = "master-encryption-key"
	keyVersion    = 1
	nonceSize     = 12 // GCM standard nonce size
	keySize       = 32 // AES-256
	gcmTagSize    = 16 // GCM tag size
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
	backend    Backend
}

// NewWithBackend creates a new Keystore instance with a custom backend.
// This is primarily used for testing with the in-memory test backend.
func NewWithBackend(secretsDir string, logger *slog.Logger, backend Backend) (*Keystore, error) {
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return nil, err
	}
	return &Keystore{
		logger:     logger,
		secretsDir: secretsDir,
		backend:    backend,
	}, nil
}

// Backend defines the OS-specific key store interface.
type Backend interface {
	// RetrieveMasterKey retrieves the master encryption key from the OS key store.
	RetrieveMasterKey() ([]byte, error)
	// StoreMasterKey stores the master encryption key in the OS key store.
	StoreMasterKey(key []byte) error
	// DeleteMasterKey removes the master encryption key from the OS key store.
	DeleteMasterKey() error
	// Name returns the human-readable name of the backend.
	Name() string
}

// Initialize creates or retrieves the master encryption key from the OS key store.
func (k *Keystore) Initialize() error {
	key, err := k.backend.RetrieveMasterKey()
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			k.logger.Info("[Keystore] Master key not found, generating new key", "backend", k.backend.Name())
			return k.generateAndStoreMasterKey()
		}
		return fmt.Errorf("retrieve master key: %w", err)
	}

	if len(key) != keySize {
		return fmt.Errorf("master key has invalid length %d, expected %d", len(key), keySize)
	}

	k.logger.Info("[Keystore] Master key retrieved from OS key store", "backend", k.backend.Name())
	return nil
}

// generateAndStoreMasterKey generates a new AES-256 key and stores it in the OS key store.
func (k *Keystore) generateAndStoreMasterKey() error {
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return fmt.Errorf("generate master key: %w", err)
	}

	if err := k.backend.StoreMasterKey(key); err != nil {
		return fmt.Errorf("store master key: %w", err)
	}

	k.logger.Info("[Keystore] Master key generated and stored in OS key store", "backend", k.backend.Name())
	return nil
}

// EncryptSecret encrypts a plaintext secret value and writes it to disk.
func (k *Keystore) EncryptSecret(name, plaintext string) error {
	key, err := k.backend.RetrieveMasterKey()
	if err != nil {
		return fmt.Errorf("retrieve master key for encryption: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create GCM mode: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	enc := EncryptedSecret{
		Version:    keyVersion,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}

	data, err := json.Marshal(enc)
	if err != nil {
		return fmt.Errorf("marshal encrypted secret: %w", err)
	}

	path := filepath.Join(k.secretsDir, name)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write encrypted secret: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}

	k.logger.Debug("[Keystore] Secret encrypted and written", "name", name)
	return nil
}

// DecryptSecret reads and decrypts a secret value from disk.
func (k *Keystore) DecryptSecret(name string) (string, error) {
	path := filepath.Join(k.secretsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read encrypted secret: %w", err)
	}

	var enc EncryptedSecret
	if err := json.Unmarshal(data, &enc); err != nil {
		return "", fmt.Errorf("unmarshal encrypted secret: %w", err)
	}

	if enc.Version != keyVersion {
		return "", fmt.Errorf("unsupported secret version %d, expected %d", enc.Version, keyVersion)
	}

	key, err := k.backend.RetrieveMasterKey()
	if err != nil {
		return "", fmt.Errorf("retrieve master key for decryption: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM mode: %w", err)
	}

	plaintext, err := gcm.Open(nil, enc.Nonce, enc.Ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCiphertext, err)
	}

	k.logger.Debug("[Keystore] Secret decrypted", "name", name)
	return string(plaintext), nil
}

// DeleteSecret removes a secret file from disk.
func (k *Keystore) DeleteSecret(name string) error {
	path := filepath.Join(k.secretsDir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete secret: %w", err)
	}
	k.logger.Debug("[Keystore] Secret deleted", "name", name)
	return nil
}

// Purge removes all secrets from disk and deletes the master key from the OS key store.
func (k *Keystore) Purge() error {
	if err := k.backend.DeleteMasterKey(); err != nil {
		return fmt.Errorf("delete master key from OS key store: %w", err)
	}

	entries, err := os.ReadDir(k.secretsDir)
	if err != nil {
		return fmt.Errorf("read secrets directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(k.secretsDir, entry.Name())
		if err := os.Remove(path); err != nil {
			k.logger.Warn("[Keystore] Failed to delete secret file", "path", path, "error", err)
		}
	}

	k.logger.Info("[Keystore] All secrets purged", "backend", k.backend.Name())
	return nil
}

// EnsurePermissions enforces strict filesystem permissions on the secrets directory.
func (k *Keystore) EnsurePermissions() error {
	// Enforce 0700 on directory
	if err := os.Chmod(k.secretsDir, 0700); err != nil {
		return fmt.Errorf("chmod secrets directory: %w", err)
	}

	// Enforce 0600 on all files
	entries, err := os.ReadDir(k.secretsDir)
	if err != nil {
		return fmt.Errorf("read secrets directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(k.secretsDir, entry.Name())
		if err := os.Chmod(path, 0600); err != nil {
			return fmt.Errorf("chmod secret file: %w", err)
		}
	}

	k.logger.Debug("[Keystore] Permissions enforced", "dir", k.secretsDir)
	return nil
}

// BackendName returns the name of the OS key store backend.
func (k *Keystore) BackendName() string {
	return k.backend.Name()
}
