// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"context"
	"crypto/rand"
	"fmt"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
)

// The vault key wraps the vault DEK. It exists only as key material encrypted
// under the master key in the secrets directory; it is never written in the
// clear.

// InitVault generates a vault key, stores it under the master key, and writes
// a new vault header wrapped by it. The key is stored before the header so a
// failed store never leaves a header that no stored key can open.
func (k *Keystore) InitVault() error {
	exists, err := vault.VaultHeaderExists(k.fileSvc)
	if err != nil {
		return fmt.Errorf("keystore: init vault: %w", err)
	}
	if exists {
		return fmt.Errorf("keystore: init vault: %w", constants.ErrVaultAlreadyInitialized)
	}
	if err := k.fileSvc.MkdirAll(context.Background(), constants.VaultDirname, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	key := make([]byte, vault.KeySize)
	defer vault.SecureZero(key)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrVaultKeyGenerateFailed, err)
	}

	header, dek, err := vault.NewVaultHeader(key)
	vault.SecureZero(dek)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrVaultHeaderCreateFailed, err)
	}

	if err := k.StoreKeyMaterial(constants.SecretsFileVaultKey, key); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrVaultKeyWriteFailed, err)
	}
	if err := header.Save(k.fileSvc); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrVaultHeaderSaveFailed, err)
	}
	return nil
}

// LoadVaultKey decrypts the vault key. The caller must SecureZero it.
func (k *Keystore) LoadVaultKey() ([]byte, error) {
	return k.LoadKeyMaterial(constants.SecretsFileVaultKey, vault.KeySize)
}

// RekeyVault rewraps the vault DEK under a freshly generated vault key. The
// new key is staged under the master key before the header changes, so a
// crash after the header is saved leaves the new key recoverable from the
// staged secret rather than lost.
func (k *Keystore) RekeyVault() error {
	oldKey, err := k.LoadVaultKey()
	if err != nil {
		return fmt.Errorf("keystore: rekey vault: load current key: %w", err)
	}
	defer vault.SecureZero(oldKey)

	newKey := make([]byte, vault.KeySize)
	defer vault.SecureZero(newKey)
	if _, err := rand.Read(newKey); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrVaultKeyGenerateFailed, err)
	}

	if err := k.StoreKeyMaterial(constants.SecretsFileVaultKeyStaged, newKey); err != nil {
		return fmt.Errorf("keystore: rekey vault: stage new key: %w", err)
	}

	v, err := vault.NewVault(&vault.VaultConfig{FileSvc: k.fileSvc, Logger: k.logger})
	if err != nil {
		_ = k.DeleteSecret(constants.SecretsFileVaultKeyStaged)
		return fmt.Errorf("%w: %w", constants.ErrVaultCreateFailed, err)
	}
	if err := v.Rekey(oldKey, newKey); err != nil {
		_ = k.DeleteSecret(constants.SecretsFileVaultKeyStaged)
		return fmt.Errorf("%w: %w", constants.ErrVaultRekeyFailed, err)
	}

	staged := filepath.Join(constants.SecretsDirname, constants.SecretsFileVaultKeyStaged)
	current := filepath.Join(constants.SecretsDirname, constants.SecretsFileVaultKey)
	if err := k.fileSvc.Rename(context.Background(), staged, current); err != nil {
		return fmt.Errorf("%w: header now uses the key staged at %s: %w", constants.ErrVaultKeyWriteFailed, k.fileSvc.Resolve(staged), err)
	}
	return nil
}
