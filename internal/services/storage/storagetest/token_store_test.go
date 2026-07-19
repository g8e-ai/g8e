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

package storagetest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestTokenStore_SetGetRoundTrip(t *testing.T) {
	t.Parallel()
	store := NewTestTokenStore()
	ctx := context.Background()

	cases := []struct {
		key   string
		value string
		ttl   int
	}{
		{"alpha", "value1", 0},
		{"beta", "value2", 0},
		{"gamma:sub", "value3", 0},
	}

	for _, tc := range cases {
		err := store.KVSet(ctx, tc.key, tc.value, tc.ttl)
		require.NoError(t, err)

		got, err := store.KVGet(ctx, tc.key)
		require.NoError(t, err)
		assert.Equal(t, tc.value, got)
	}
}

func TestTestTokenStore_KeyNotFound(t *testing.T) {
	t.Parallel()
	store := NewTestTokenStore()
	ctx := context.Background()

	_, err := store.KVGet(ctx, "nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrKeyNotFound),
		"expected constants.ErrKeyNotFound, got %v", err)
}

func TestTestTokenStore_TTLExpiry(t *testing.T) {
	t.Parallel()
	store := NewTestTokenStore()
	ctx := context.Background()

	err := store.KVSet(ctx, "ephemeral", "data", 1)
	require.NoError(t, err)

	got, err := store.KVGet(ctx, "ephemeral")
	require.NoError(t, err)
	assert.Equal(t, "data", got)

	time.Sleep(2 * time.Second)

	_, err = store.KVGet(ctx, "ephemeral")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrKeyNotFound),
		"expected constants.ErrKeyNotFound after TTL expiry, got %v", err)
}

func TestTestTokenStore_ScanPrefix(t *testing.T) {
	t.Parallel()
	store := NewTestTokenStore()
	ctx := context.Background()

	keys := map[string]string{
		"prefix:a": "val1",
		"prefix:b": "val2",
		"prefix:c": "val3",
		"other:d":  "val4",
	}
	for k, v := range keys {
		require.NoError(t, store.KVSet(ctx, k, v, 0))
	}

	result, err := store.KVScanPrefix(ctx, "prefix:")
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "val1", result["prefix:a"])
	assert.Equal(t, "val2", result["prefix:b"])
	assert.Equal(t, "val3", result["prefix:c"])
	assert.NotContains(t, result, "other:d")
}

func TestTestTokenStore_ScanPrefix_ExcludesExpired(t *testing.T) {
	t.Parallel()
	store := NewTestTokenStore()
	ctx := context.Background()

	require.NoError(t, store.KVSet(ctx, "group:alive", "v1", 0))
	require.NoError(t, store.KVSet(ctx, "group:dead", "v2", 1))

	time.Sleep(2 * time.Second)

	result, err := store.KVScanPrefix(ctx, "group:")
	require.NoError(t, err)
	assert.Len(t, result, 1, "expired key should be excluded from scan")
	assert.Equal(t, "v1", result["group:alive"])
}

func TestTestTokenStore_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	store := NewTestTokenStore()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "concurrent:" + string(rune('A'+n%26))
			require.NoError(t, store.KVSet(ctx, key, "val", 0))
			_, _ = store.KVGet(ctx, key)
		}(i)
	}
	wg.Wait()

	result, err := store.KVScanPrefix(ctx, "concurrent:")
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}
