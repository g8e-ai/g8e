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
