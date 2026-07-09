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

//go:build integration

package gateway

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func setupKVStore(t *testing.T) *KVStoreService {
	t.Helper()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(filepath.Join(dir, "test.db"))
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Initialize schema
	_, err = db.Exec(GatewaySchema())
	require.NoError(t, err)

	return NewKVStoreService(db, logger)
}

func TestKVStore_SetGet(t *testing.T) {
	s := setupKVStore(t)

	err := s.KVSet("foo", "bar", 0)
	require.NoError(t, err)

	val, found := s.KVGet("foo")
	assert.True(t, found)
	assert.Equal(t, "bar", val)

	val, found = s.KVGet("nonexistent")
	assert.False(t, found)
	assert.Empty(t, val)
}

func TestKVStore_Overwrite(t *testing.T) {
	s := setupKVStore(t)

	err := s.KVSet("foo", "bar", 0)
	require.NoError(t, err)

	err = s.KVSet("foo", "baz", 0)
	require.NoError(t, err)

	val, found := s.KVGet("foo")
	assert.True(t, found)
	assert.Equal(t, "baz", val)
}

func TestKVStore_Delete(t *testing.T) {
	s := setupKVStore(t)

	err := s.KVSet("foo", "bar", 0)
	require.NoError(t, err)

	err = s.KVDelete("foo")
	require.NoError(t, err)

	_, found := s.KVGet("foo")
	assert.False(t, found)
}

func TestKVStore_DeletePattern(t *testing.T) {
	s := setupKVStore(t)

	require.NoError(t, s.KVSet("user:1", "alice", 0))
	require.NoError(t, s.KVSet("user:2", "bob", 0))
	require.NoError(t, s.KVSet("other:1", "data", 0))

	n, err := s.KVDeletePattern("user:*")
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	assert.False(t, s.KVExists("user:1"))
	assert.False(t, s.KVExists("user:2"))
	assert.True(t, s.KVExists("other:1"))
}

func TestKVStore_Keys(t *testing.T) {
	s := setupKVStore(t)

	require.NoError(t, s.KVSet("user:1", "alice", 0))
	require.NoError(t, s.KVSet("user:2", "bob", 0))
	require.NoError(t, s.KVSet("other:1", "data", 0))

	keys, err := s.KVKeys("user:*")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"user:1", "user:2"}, keys)
}

func TestKVStore_Scan(t *testing.T) {
	s := setupKVStore(t)

	for i := 0; i < 10; i++ {
		require.NoError(t, s.KVSet(fmt.Sprintf("key:%02d", i), "val", 0))
	}

	// Page 1
	next, keys, err := s.KVScan("key:*", 0, 4)
	require.NoError(t, err)
	assert.Equal(t, 4, next)
	assert.Len(t, keys, 4)
	assert.Equal(t, "key:00", keys[0])
	assert.Equal(t, "key:03", keys[3])

	// Page 2
	next, keys, err = s.KVScan("key:*", 4, 4)
	require.NoError(t, err)
	assert.Equal(t, 8, next)
	assert.Len(t, keys, 4)
	assert.Equal(t, "key:04", keys[0])
	assert.Equal(t, "key:07", keys[3])

	// Page 3 (remainder)
	next, keys, err = s.KVScan("key:*", 8, 4)
	require.NoError(t, err)
	assert.Equal(t, 0, next)
	assert.Len(t, keys, 2)
	assert.Equal(t, "key:08", keys[0])
	assert.Equal(t, "key:09", keys[1])
}

func TestKVStore_Exists(t *testing.T) {
	s := setupKVStore(t)

	assert.False(t, s.KVExists("foo"))
	require.NoError(t, s.KVSet("foo", "bar", 0))
	assert.True(t, s.KVExists("foo"))
}

func TestKVStore_TTL(t *testing.T) {
	s := setupKVStore(t)

	// No TTL
	require.NoError(t, s.KVSet("no-ttl", "val", 0))
	assert.Equal(t, -1, s.KVTTL("no-ttl"))

	// With TTL
	require.NoError(t, s.KVSet("with-ttl", "val", 10))
	ttl := s.KVTTL("with-ttl")
	assert.True(t, ttl > 0 && ttl <= 10)

	// Not found
	assert.Equal(t, -2, s.KVTTL("missing"))
}

func TestKVStore_Expire(t *testing.T) {
	s := setupKVStore(t)

	require.NoError(t, s.KVSet("foo", "bar", 0))
	assert.Equal(t, -1, s.KVTTL("foo"))

	ok := s.KVExpire("foo", 60)
	assert.True(t, ok)
	ttl := s.KVTTL("foo")
	assert.True(t, ttl > 0 && ttl <= 60)

	ok = s.KVExpire("missing", 60)
	assert.False(t, ok)
}

func TestKVStore_Expiration(t *testing.T) {
	// Not parallel due to time sensitivity
	s := setupKVStore(t)

	// Set with short TTL
	require.NoError(t, s.KVSet("short", "val", 1))
	assert.True(t, s.KVExists("short"))

	// Wait for expiration
	time.Sleep(1200 * time.Millisecond)

	_, found := s.KVGet("short")
	assert.False(t, found, "Key should be expired and not found via KVGet")
	assert.False(t, s.KVExists("short"), "Key should be expired and not found via KVExists")

	// Ensure it still exists in DB but is just filtered out (lazy delete)
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM kv_store WHERE key = 'short'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Key should still exist in DB before maintenance")

	// Run maintenance
	err = s.RunMaintenance()
	require.NoError(t, err)

	err = s.db.QueryRow("SELECT COUNT(*) FROM kv_store WHERE key = 'short'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Key should be removed from DB after maintenance")
}
