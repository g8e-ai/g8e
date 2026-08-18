// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import "context"

// TokenStore defines the interface for encrypted UEI token persistence used by
// ScrubbingService. The canonical implementation is gateway.EncryptedKVAdapter,
// which stores tokens in the shared kv_store table of the CanonicalDBService
// (g8e.db).
//
// Contract:
//   - Values are encrypted at rest via the vault; fail-closed if the vault is locked.
//   - Keys are namespaced with constants.SentinelKeyPrefix to avoid collisions
//     with cache/doc invalidation entries.
//   - TTL-bearing: entries expire after ttlSeconds; expired entries are invisible
//     to KVGet and KVScanPrefix.
//   - Entries are written as state_tier='observed' so they do not participate in
//     the bound state root hash.
type TokenStore interface {
	KVSet(ctx context.Context, key, value string, ttlSeconds int) error
	KVGet(ctx context.Context, key string) (string, error)
	KVScanPrefix(ctx context.Context, prefix string) (map[string]string, error)
}
