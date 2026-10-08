// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func setupKVStore(t *testing.T) *KVStoreService {
	t.Helper()
	dir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(filepath.Join(dir, "test.db"))
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Initialize schema
	_, err = db.Exec(gatewaySchema)
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

func TestKVStore_Keys(t *testing.T) {
	s := setupKVStore(t)

	require.NoError(t, s.KVSet("user:1", "alice", 0))
	require.NoError(t, s.KVSet("user:2", "bob", 0))
	require.NoError(t, s.KVSet("other:1", "data", 0))

	keys, err := s.KVKeys("user:*")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"user:1", "user:2"}, keys)
}

func TestKVStore_Expiration(t *testing.T) {
	// Not parallel due to time sensitivity
	s := setupKVStore(t)

	// Set with negative TTL so it is immediately expired
	require.NoError(t, s.KVSet("short", "val", -1))

	_, found := s.KVGet("short")
	assert.False(t, found, "Key should be expired and not found via KVGet")

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
