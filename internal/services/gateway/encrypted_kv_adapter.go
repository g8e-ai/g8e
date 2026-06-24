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

package gateway

import (
	"context"
	"fmt"
	"strings"

	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
)

// sentinelKeyPrefix namespaces sentinel UEI tokens in kv_store to avoid collisions
// with cache/doc invalidation entries written by the document store triggers.
const sentinelKeyPrefix = "g8e:sentinel:"

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
		return fmt.Errorf("vault is locked, cannot encrypt value for key %s", key)
	}
	encrypted, err := a.vault.Encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("failed to encrypt value for key %s: %w", key, err)
	}
	return a.kv.KVSetObserved(sentinelKeyPrefix+key, string(encrypted), ttlSeconds)
}

func (a *EncryptedKVAdapter) KVGet(_ context.Context, key string) (string, error) {
	value, found := a.kv.KVGet(sentinelKeyPrefix + key)
	if !found {
		return "", fmt.Errorf("key not found: %s", key)
	}
	if !a.vault.IsUnlocked() {
		return "", fmt.Errorf("vault is locked, cannot decrypt value for key %s", key)
	}
	decrypted, err := a.vault.Decrypt([]byte(value))
	if err != nil {
		return "", fmt.Errorf("failed to decrypt value for key %s: %w", key, err)
	}
	return string(decrypted), nil
}

func (a *EncryptedKVAdapter) KVScanPrefix(_ context.Context, prefix string) (map[string]string, error) {
	fullKeys, err := a.kv.KVKeys(sentinelKeyPrefix + prefix + "*")
	if err != nil {
		return nil, fmt.Errorf("failed to scan keys with prefix %q: %w", prefix, err)
	}

	result := make(map[string]string, len(fullKeys))
	for _, fullKey := range fullKeys {
		value, found := a.kv.KVGet(fullKey)
		if !found {
			continue
		}
		if !a.vault.IsUnlocked() {
			continue
		}
		decrypted, err := a.vault.Decrypt([]byte(value))
		if err != nil {
			continue
		}
		result[strings.TrimPrefix(fullKey, sentinelKeyPrefix)] = string(decrypted)
	}
	return result, nil
}
