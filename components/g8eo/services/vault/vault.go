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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Vault manages the encrypted LFAA data store.
// It handles initialization, unlocking, re-keying, and provides
// the DEK for database encryption operations.
type Vault struct {
	dataDir string
	logger  *slog.Logger

	header   *VaultHeader
	dek      []byte
	unlocked bool

	mu sync.RWMutex
}

// VaultConfig holds configuration for vault initialization
type VaultConfig struct {
	// DataDir is the directory where vault data is stored
	DataDir string

	// Logger for vault operations
	Logger *slog.Logger
}

// Vault-related errors
var (
	ErrVaultLocked      = errors.New("vault is locked")
	ErrVaultNotInit     = errors.New("vault is not initialized")
	ErrVaultAlreadyInit = errors.New("vault is already initialized")
	ErrVaultAlreadyOpen = errors.New("vault is already unlocked")
	ErrInvalidAPIKey    = errors.New("invalid API key for this vault")
)

// NewVault creates a new Vault instance.
// The vault is not initialized or unlocked until Initialize() or Unlock() is called.
func NewVault(config *VaultConfig) (*Vault, error) {
	if config == nil {
		return nil, errors.New("vault config is required")
	}
	if config.DataDir == "" {
		return nil, errors.New("vault data directory is required")
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	v := &Vault{
		dataDir: config.DataDir,
		logger:  logger,
	}

	return v, nil
}

// Unlock opens an existing vault using the provided API key.
// The DEK is unwrapped and held in memory for encryption operations.
func (v *Vault) Unlock(apiKey string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.unlocked {
		return ErrVaultAlreadyOpen
	}

	header, err := LoadVaultHeader(v.dataDir)
	if err != nil {
		if errors.Is(err, ErrHeaderNotFound) {
			return ErrVaultNotInit
		}
		return fmt.Errorf("failed to load vault header: %w", err)
	}

	dek, err := header.UnwrapDEK(apiKey)
	if err != nil {
		if errors.Is(err, ErrKeyFingerprintMatch) {
			return ErrInvalidAPIKey
		}
		return fmt.Errorf("failed to unwrap DEK: %w", err)
	}

	v.header = header
	v.dek = dek
	v.unlocked = true

	v.logger.Info("Vault unlocked",
		"data_dir", v.dataDir,
		"key_fingerprint", header.KeyFingerprint[:8]+"...")

	return nil
}

// Rekey re-encrypts the DEK with a new API key.
// Both old and new API keys are required.
// The vault data itself is not re-encrypted (only the DEK wrapper changes).
func (v *Vault) Rekey(oldAPIKey, newAPIKey string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	header := v.header
	if header == nil {
		var err error
		header, err = LoadVaultHeader(v.dataDir)
		if err != nil {
			if errors.Is(err, ErrHeaderNotFound) {
				return ErrVaultNotInit
			}
			return fmt.Errorf("failed to load vault header: %w", err)
		}
	}

	if err := header.Rekey(oldAPIKey, newAPIKey); err != nil {
		return fmt.Errorf("failed to rekey vault: %w", err)
	}

	if err := header.Save(v.dataDir); err != nil {
		return fmt.Errorf("failed to save rekeyed vault header: %w", err)
	}

	if v.unlocked && v.dek != nil {
		v.header = header
	}

	v.logger.Info("Vault rekeyed",
		"data_dir", v.dataDir,
		"new_key_fingerprint", header.KeyFingerprint[:8]+"...")

	return nil
}

// Lock securely clears the DEK from memory and locks the vault.
func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.dek != nil {
		SecureZero(v.dek)
		v.dek = nil
	}
	v.unlocked = false

	v.logger.Info("Vault locked")
}

// Close locks the vault and releases resources.
func (v *Vault) Close() error {
	v.Lock()
	return nil
}

// IsUnlocked returns whether the vault is currently unlocked.
func (v *Vault) IsUnlocked() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.unlocked
}

// IsInitialized returns whether the vault has been initialized (header exists).
func (v *Vault) IsInitialized() bool {
	return VaultHeaderExists(v.dataDir)
}

// GetDEK returns the Data Encryption Key for database encryption.
// Returns an error if the vault is locked.
// SECURITY: The caller must not store the DEK or allow it to escape to disk.
func (v *Vault) GetDEK() ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if !v.unlocked || v.dek == nil {
		return nil, ErrVaultLocked
	}

	dekCopy := make([]byte, len(v.dek))
	copy(dekCopy, v.dek)
	return dekCopy, nil
}

// Encrypt encrypts plaintext using the vault's DEK with AES-256-GCM.
// A random nonce is generated and prepended to the ciphertext.
// Returns error if vault is locked.
func (v *Vault) Encrypt(plaintext []byte) ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if !v.unlocked || v.dek == nil {
		return nil, ErrVaultLocked
	}

	nonce, err := GenerateNonce()
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext, err := EncryptAESGCM(v.dek, nonce, plaintext, nil)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	result := make([]byte, NonceSize+len(ciphertext))
	copy(result[:NonceSize], nonce)
	copy(result[NonceSize:], ciphertext)

	return result, nil
}

// Decrypt decrypts ciphertext using the vault's DEK with AES-256-GCM.
// Expects the nonce to be prepended to the ciphertext (as produced by Encrypt).
// Returns error if vault is locked or decryption fails.
func (v *Vault) Decrypt(ciphertext []byte) ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if !v.unlocked || v.dek == nil {
		return nil, ErrVaultLocked
	}

	if len(ciphertext) < NonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce := ciphertext[:NonceSize]
	encryptedData := ciphertext[NonceSize:]

	plaintext, err := DecryptAESGCM(v.dek, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// GetDataDir returns the vault's data directory.
func (v *Vault) GetDataDir() string {
	return v.dataDir
}

// VerifyIntegrity checks the vault's integrity by attempting to unwrap the DEK.
// Returns nil if the vault is healthy, error otherwise.
func (v *Vault) VerifyIntegrity(apiKey string) error {
	header, err := LoadVaultHeader(v.dataDir)
	if err != nil {
		return fmt.Errorf("header load failed: %w", err)
	}

	dek, err := header.UnwrapDEK(apiKey)
	if err != nil {
		return fmt.Errorf("DEK unwrap failed: %w", err)
	}
	SecureZero(dek)

	return nil
}

// Reset destroys the vault completely. All encrypted data becomes unrecoverable.
// This is a destructive operation that requires explicit confirmation.
func (v *Vault) Reset(confirmDestroy bool) error {
	if !confirmDestroy {
		return errors.New("vault reset requires explicit confirmation")
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.dek != nil {
		SecureZero(v.dek)
		v.dek = nil
	}
	v.unlocked = false
	v.header = nil

	if err := DeleteVaultHeader(v.dataDir); err != nil {
		return fmt.Errorf("failed to delete vault header: %w", err)
	}

	dbPath := filepath.Join(v.dataDir, "g8e.db")
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		v.logger.Warn("Failed to delete database file", "error", err)
	}

	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")

	v.logger.Info("Vault reset complete - all data destroyed")

	return nil
}
