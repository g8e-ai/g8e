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

package interfaces

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTokenStore is a simple in-memory implementation of TokenStore for testing
type mockTokenStore struct {
	mu     sync.RWMutex
	data   map[string]kvEntry
	closed bool
}

type kvEntry struct {
	value     string
	expiresAt *time.Time
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{
		data: make(map[string]kvEntry),
	}
}

func (m *mockTokenStore) IsEnabled() bool {
	return !m.closed
}

func (m *mockTokenStore) KVSet(key, value string, ttlSeconds int) error {
	if m.closed {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var expiresAt *time.Time
	if ttlSeconds > 0 {
		exp := time.Now().Add(time.Duration(ttlSeconds) * time.Second)
		expiresAt = &exp
	}

	m.data[key] = kvEntry{
		value:     value,
		expiresAt: expiresAt,
	}
	return nil
}

func (m *mockTokenStore) KVGet(key string) (string, bool) {
	if m.closed {
		return "", false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.data[key]
	if !exists {
		return "", false
	}

	if entry.expiresAt != nil && time.Now().After(*entry.expiresAt) {
		return "", false
	}

	return entry.value, true
}

func (m *mockTokenStore) KVScanPrefix(prefix string) (map[string]string, error) {
	if m.closed {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string)
	now := time.Now()

	for key, entry := range m.data {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			if entry.expiresAt == nil || now.Before(*entry.expiresAt) {
				result[key] = entry.value
			}
		}
	}
	return result, nil
}

func (m *mockTokenStore) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

// TestTokenStore_Contract verifies the TokenStore interface contract
func TestTokenStore_Contract(t *testing.T) {
	t.Parallel()

	ts := newMockTokenStore()
	defer ts.close()

	t.Run("IsEnabled", func(t *testing.T) {
		assert.True(t, ts.IsEnabled(), "TokenStore should be enabled when LocalStoreService is initialized")
	})

	t.Run("KVSet and KVGet", func(t *testing.T) {
		key := "test-key-1"
		value := "test-value-1"
		ttl := 60

		err := ts.KVSet(key, value, ttl)
		require.NoError(t, err, "KVSet should succeed")

		retrieved, found := ts.KVGet(key)
		assert.True(t, found, "Key should be found after KVSet")
		assert.Equal(t, value, retrieved, "Retrieved value should match stored value")
	})

	t.Run("KVGet missing key", func(t *testing.T) {
		_, found := ts.KVGet("non-existent-key")
		assert.False(t, found, "Missing key should return false")
	})

	t.Run("KVSet with zero TTL", func(t *testing.T) {
		key := "test-key-no-ttl"
		value := "test-value-no-ttl"

		err := ts.KVSet(key, value, 0)
		require.NoError(t, err, "KVSet with zero TTL should succeed")

		retrieved, found := ts.KVGet(key)
		assert.True(t, found, "Key with zero TTL should be found")
		assert.Equal(t, value, retrieved, "Retrieved value should match")
	})

	t.Run("KVSet with negative TTL", func(t *testing.T) {
		key := "test-key-negative-ttl"
		value := "test-value-negative-ttl"

		err := ts.KVSet(key, value, -1)
		require.NoError(t, err, "KVSet with negative TTL should succeed")

		retrieved, found := ts.KVGet(key)
		assert.True(t, found, "Key with negative TTL should be found")
		assert.Equal(t, value, retrieved, "Retrieved value should match")
	})

	t.Run("KVScanPrefix", func(t *testing.T) {
		prefix := "scan-test-"
		keys := []string{"scan-test-1", "scan-test-2", "scan-test-3"}
		values := []string{"value-1", "value-2", "value-3"}

		for i, key := range keys {
			err := ts.KVSet(key, values[i], 60)
			require.NoError(t, err)
		}

		results, err := ts.KVScanPrefix(prefix)
		require.NoError(t, err, "KVScanPrefix should succeed")
		assert.Len(t, results, 3, "Should find all keys with prefix")

		for i, key := range keys {
			assert.Equal(t, values[i], results[key], "Value for key %s should match", key)
		}
	})

	t.Run("KVScanPrefix empty result", func(t *testing.T) {
		results, err := ts.KVScanPrefix("non-existent-prefix-")
		require.NoError(t, err, "KVScanPrefix should succeed even with no matches")
		assert.Empty(t, results, "Should return empty map for non-existent prefix")
	})

	t.Run("KVSet update existing key", func(t *testing.T) {
		key := "update-test-key"
		value1 := "initial-value"
		value2 := "updated-value"

		err := ts.KVSet(key, value1, 60)
		require.NoError(t, err)

		err = ts.KVSet(key, value2, 60)
		require.NoError(t, err, "Updating existing key should succeed")

		retrieved, found := ts.KVGet(key)
		assert.True(t, found)
		assert.Equal(t, value2, retrieved, "Retrieved value should be the updated value")
	})

	t.Run("KVScanPrefix with mixed keys", func(t *testing.T) {
		prefix := "mixed-test-"
		matchingKeys := []string{"mixed-test-a", "mixed-test-b"}
		nonMatchingKeys := []string{"other-test-a", "mixed-other-b"}

		for _, key := range matchingKeys {
			err := ts.KVSet(key, "match", 60)
			require.NoError(t, err)
		}

		for _, key := range nonMatchingKeys {
			err := ts.KVSet(key, "no-match", 60)
			require.NoError(t, err)
		}

		results, err := ts.KVScanPrefix(prefix)
		require.NoError(t, err)
		assert.Len(t, results, 2, "Should only find keys with matching prefix")

		for _, key := range matchingKeys {
			assert.Contains(t, results, key, "Should contain matching key")
		}

		for _, key := range nonMatchingKeys {
			assert.NotContains(t, results, key, "Should not contain non-matching key")
		}
	})
}

// TestTokenStore_TTLExpiry verifies that KVGet respects TTL expiration
func TestTokenStore_TTLExpiry(t *testing.T) {
	t.Parallel()

	ts := newMockTokenStore()
	defer ts.close()

	key := "expiry-test-key"
	value := "expiry-test-value"

	err := ts.KVSet(key, value, 1)
	require.NoError(t, err)

	retrieved, found := ts.KVGet(key)
	assert.True(t, found, "Key should be found immediately after set")
	assert.Equal(t, value, retrieved)

	time.Sleep(2 * time.Second)

	retrieved, found = ts.KVGet(key)
	assert.False(t, found, "Key should not be found after TTL expires")
	assert.Empty(t, retrieved, "Retrieved value should be empty when not found")
}

// TestTokenStore_ClosedStore verifies behavior when TokenStore is closed
func TestTokenStore_ClosedStore(t *testing.T) {
	t.Parallel()

	ts := newMockTokenStore()
	ts.close()

	t.Run("IsEnabled on closed", func(t *testing.T) {
		assert.False(t, ts.IsEnabled(), "Closed TokenStore should return false for IsEnabled")
	})

	t.Run("KVSet on closed", func(t *testing.T) {
		err := ts.KVSet("key", "value", 60)
		assert.NoError(t, err, "KVSet on closed should not error (graceful degradation)")
	})

	t.Run("KVGet on closed", func(t *testing.T) {
		value, found := ts.KVGet("key")
		assert.False(t, found, "KVGet on closed should return false")
		assert.Empty(t, value, "KVGet on closed should return empty value")
	})

	t.Run("KVScanPrefix on closed", func(t *testing.T) {
		results, err := ts.KVScanPrefix("prefix")
		assert.NoError(t, err, "KVScanPrefix on closed should not error")
		assert.Nil(t, results, "KVScanPrefix on closed should return nil results")
	})
}
