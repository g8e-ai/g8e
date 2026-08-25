// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	storage "github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
)

// EncryptedKVAdapter bridges KVStoreService (in the canonical gateway DB) to the
// storage.TokenStore interface expected by ScrubbingService. Values are encrypted
// at rest via the vault. Entries are written as state_tier='observed' so they
// do not participate in the bound state root hash.
type EncryptedKVAdapter struct {
	kv    *KVStoreService
	vault *vault.Vault
}

// Ensure EncryptedKVAdapter implements storage.TokenStore.
var _ storage.TokenStore = (*EncryptedKVAdapter)(nil)

// NewEncryptedKVAdapter returns an EncryptedKVAdapter that satisfies storage.TokenStore.
func NewEncryptedKVAdapter(kv *KVStoreService, v *vault.Vault) *EncryptedKVAdapter {
	return &EncryptedKVAdapter{kv: kv, vault: v}
}

func (a *EncryptedKVAdapter) KVSet(_ context.Context, key, value string, ttlSeconds int) error {
	if !a.vault.IsUnlocked() {
		return fmt.Errorf("encrypted_kv_adapter: cannot encrypt key %s: %w", key, constants.ErrVaultLocked)
	}
	encrypted, err := a.vault.Encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("failed to encrypt value for key %s: %w", key, err)
	}
	return a.kv.KVSetObserved(constants.SentinelKeyPrefix+key, string(encrypted), ttlSeconds)
}

func (a *EncryptedKVAdapter) KVGet(_ context.Context, key string) (string, error) {
	value, found := a.kv.KVGet(constants.SentinelKeyPrefix + key)
	if !found {
		return "", fmt.Errorf("encrypted_kv_adapter: key %s: %w", key, constants.ErrKeyNotFound)
	}
	if !a.vault.IsUnlocked() {
		return "", fmt.Errorf("encrypted_kv_adapter: cannot decrypt key %s: %w", key, constants.ErrVaultLocked)
	}
	decrypted, err := a.vault.Decrypt([]byte(value))
	if err != nil {
		return "", fmt.Errorf("failed to decrypt value for key %s: %w", key, err)
	}
	return string(decrypted), nil
}

func (a *EncryptedKVAdapter) KVScanPrefix(_ context.Context, prefix string) (map[string]string, error) {
	fullKeys, err := a.kv.KVKeys(constants.SentinelKeyPrefix + prefix + "*")
	if err != nil {
		return nil, fmt.Errorf("failed to scan keys with prefix %q: %w", prefix, err)
	}

	if !a.vault.IsUnlocked() {
		return nil, fmt.Errorf("encrypted_kv_adapter: cannot decrypt prefix %s: %w", prefix, constants.ErrVaultLocked)
	}

	result := make(map[string]string, len(fullKeys))
	for _, fullKey := range fullKeys {
		value, found := a.kv.KVGet(fullKey)
		if !found {
			continue
		}
		decrypted, err := a.vault.Decrypt([]byte(value))
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt value for key %s: %w", fullKey, err)
		}
		result[strings.TrimPrefix(fullKey, constants.SentinelKeyPrefix)] = string(decrypted)
	}
	return result, nil
}
