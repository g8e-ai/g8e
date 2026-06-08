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

import (
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestTokenStore creates a TokenStoreService with real infrastructure
// (SQLite database, encryption vault) for testing.
func setupTestTokenStore(t *testing.T) (*TokenStoreService, *vault.Vault, string) {
	t.Helper()
	tempDir := t.TempDir()

	// Create vault for encryption
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	testVault := createTestVault(t, vaultDir, privKey)

	logger := testutil.NewTestLogger()

	config := &TokenStoreConfig{
		DBPath:               filepath.Join(tempDir, "token_store.db"),
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}

	ts, err := NewTokenStoreService(config, logger, testVault)
	require.NoError(t, err)
	require.NotNil(t, ts)

	t.Cleanup(func() {
		ts.Wait()
		ts.Close()
	})

	return ts, testVault, tempDir
}

// TestDefaultTokenStoreConfig verifies that the default configuration
// has sensible values for all fields.
func TestDefaultTokenStoreConfig(t *testing.T) {
	t.Parallel()
	config := DefaultTokenStoreConfig()

	assert.NotNil(t, config)
	assert.Equal(t, ".g8e/token_store.db", config.DBPath)
	assert.Equal(t, int64(512), config.MaxDBSizeMB)
	assert.Equal(t, 30, config.RetentionDays)
	assert.Equal(t, 60, config.PruneIntervalMinutes)
	assert.True(t, config.Enabled)
}

// TestNewTokenStoreService_NilConfig verifies that the constructor
// uses default config when nil is provided.
func TestNewTokenStoreService_NilConfig(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	logger := testutil.NewTestLogger()

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	testVault := createTestVault(t, vaultDir, privKey)
	defer testVault.Close()

	// Override DB path to temp dir
	defaultConfig := DefaultTokenStoreConfig()
	defaultConfig.DBPath = filepath.Join(tempDir, "token_store.db")

	ts, err := NewTokenStoreService(nil, logger, testVault)
	require.NoError(t, err)
	require.NotNil(t, ts)
	defer ts.Close()

	assert.True(t, ts.IsEnabled())
}

// TestNewTokenStoreService_Disabled verifies that the constructor
// returns nil when the service is disabled in config.
func TestNewTokenStoreService_Disabled(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	logger := testutil.NewTestLogger()

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	testVault := createTestVault(t, vaultDir, privKey)
	defer testVault.Close()

	config := &TokenStoreConfig{
		DBPath:  filepath.Join(tempDir, "token_store.db"),
		Enabled: false,
	}

	ts, err := NewTokenStoreService(config, logger, testVault)
	require.NoError(t, err)
	assert.Nil(t, ts, "TokenStoreService should be nil when disabled")
}

// TestNewTokenStoreService_NilVault verifies that the constructor
// fails with an error when vault is nil and service is enabled.
func TestNewTokenStoreService_NilVault(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	config := DefaultTokenStoreConfig()
	config.Enabled = true

	ts, err := NewTokenStoreService(config, logger, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encryption vault is required")
	assert.Nil(t, ts)
}

// TestNewTokenStoreService_DatabaseInitFailure verifies that the constructor
// fails when database initialization fails.
func TestNewTokenStoreService_DatabaseInitFailure(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := t.TempDir()
	testVault := createTestVault(t, vaultDir, privKey)
	defer testVault.Close()

	// Create a file (not a directory) and try to use a path inside it
	// This will fail because you can't create directories inside a file
	tempFile, err := os.CreateTemp("", "test-file-*")
	require.NoError(t, err)
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	config := &TokenStoreConfig{
		DBPath:  filepath.Join(tempFile.Name(), "db", "token_store.db"),
		Enabled: true,
	}

	ts, err := NewTokenStoreService(config, logger, testVault)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize database")
	assert.Nil(t, ts)
}

// TestTokenStoreService_KVSetAndGet verifies basic key-value storage
// and retrieval with encryption/decryption.
func TestTokenStoreService_KVSetAndGet(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	key := "test-key-1"
	value := "test-value-123"

	err := ts.KVSet(context.Background(), key, value, 0)
	require.NoError(t, err)

	retrieved, err := ts.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)
}

// TestTokenStoreService_KVSetWithTTL verifies that keys with TTL
// are stored correctly and expire after the TTL.
func TestTokenStoreService_KVSetWithTTL(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	key := "test-ttl-key"
	value := "test-ttl-value"

	// Set with 1 second TTL
	err := ts.KVSet(context.Background(), key, value, 1)
	require.NoError(t, err)

	// Should be retrievable immediately
	retrieved, err := ts.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Should not be retrievable after TTL
	_, err = ts.KVGet(context.Background(), key)
	assert.Error(t, err)
}

// TestTokenStoreService_KVSetUpdate verifies that setting an existing key
// updates the value (upsert behavior).
func TestTokenStoreService_KVSetUpdate(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	key := "test-update-key"
	value1 := "initial-value"
	value2 := "updated-value"

	err := ts.KVSet(context.Background(), key, value1, 0)
	require.NoError(t, err)

	retrieved, err := ts.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, value1, retrieved)

	// Update the key
	err = ts.KVSet(context.Background(), key, value2, 0)
	require.NoError(t, err)

	retrieved, err = ts.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, value2, retrieved)
}

// TestTokenStoreService_KVSetLockedVault verifies that KVSet fails
// when the vault is locked (fail-closed behavior).
func TestTokenStoreService_KVSetLockedVault(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	// Lock the vault
	testVault.Lock()

	key := "test-locked-key"
	value := "test-value"

	err := ts.KVSet(context.Background(), key, value, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vault is locked")
}

// TestTokenStoreService_KVGetLockedVault verifies that KVGet fails
// when the vault is locked (fail-closed behavior).
func TestTokenStoreService_KVGetLockedVault(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	key := "test-locked-get-key"
	value := "test-value"

	// Set while unlocked
	err := ts.KVSet(context.Background(), key, value, 0)
	require.NoError(t, err)

	// Lock the vault
	testVault.Lock()

	// Get should fail when vault is locked
	_, err = ts.KVGet(context.Background(), key)
	assert.Error(t, err)
}

// TestTokenStoreService_KVGetNonExistent verifies that KVGet returns
// false for non-existent keys.
func TestTokenStoreService_KVGetNonExistent(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	_, err := ts.KVGet(context.Background(), "non-existent-key")
	assert.Error(t, err)
}

// TestTokenStoreService_KVDelete verifies that KVDelete removes
// keys from the store.
func TestTokenStoreService_KVDelete(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	key := "test-delete-key"
	value := "test-value"

	err := ts.KVSet(context.Background(), key, value, 0)
	require.NoError(t, err)

	// Verify it exists
	retrieved, err := ts.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)

	// Delete the key
	err = ts.KVDelete(key)
	require.NoError(t, err)

	// Verify it's gone
	_, err = ts.KVGet(context.Background(), key)
	assert.Error(t, err)
}

// TestTokenStoreService_KVDeleteNonExistent verifies that KVDelete
// succeeds even for non-existent keys (idempotent).
func TestTokenStoreService_KVDeleteNonExistent(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	err := ts.KVDelete("non-existent-key")
	assert.NoError(t, err)
}

// TestTokenStoreService_KVScanPrefix verifies that KVScanPrefix
// returns all keys matching a prefix.
func TestTokenStoreService_KVScanPrefix(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	// Set multiple keys with different prefixes
	pairs := map[string]string{
		"prefix:key1": "value1",
		"prefix:key2": "value2",
		"prefix:key3": "value3",
		"other:key1":  "othervalue",
	}

	for key, value := range pairs {
		err := ts.KVSet(context.Background(), key, value, 0)
		require.NoError(t, err)
	}

	// Scan for prefix
	result, err := ts.KVScanPrefix(context.Background(), "prefix:")
	require.NoError(t, err)

	assert.Len(t, result, 3)
	assert.Equal(t, "value1", result["prefix:key1"])
	assert.Equal(t, "value2", result["prefix:key2"])
	assert.Equal(t, "value3", result["prefix:key3"])
	assert.NotContains(t, result, "other:key1")
}

// TestTokenStoreService_KVScanPrefixWithTTL verifies that KVScanPrefix
// honors TTL and does not return expired keys.
func TestTokenStoreService_KVScanPrefixWithTTL(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	// Set keys with and without TTL
	err := ts.KVSet(context.Background(), "prefix:permanent", "permanent-value", 0)
	require.NoError(t, err)

	err = ts.KVSet(context.Background(), "prefix:temporary", "temporary-value", 1)
	require.NoError(t, err)

	// Both should be found immediately
	result, err := ts.KVScanPrefix(context.Background(), "prefix:")
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Only permanent key should be found
	result, err = ts.KVScanPrefix(context.Background(), "prefix:")
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "permanent-value", result["prefix:permanent"])
	assert.NotContains(t, result, "prefix:temporary")
}

// TestTokenStoreService_KVScanPrefixEmpty verifies that KVScanPrefix
// returns empty map when no keys match the prefix.
func TestTokenStoreService_KVScanPrefixEmpty(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	result, err := ts.KVScanPrefix(context.Background(), "nonexistent:")
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestTokenStoreService_KVScanPrefixLockedVault verifies that KVScanPrefix
// skips keys that cannot be decrypted when vault is locked.
func TestTokenStoreService_KVScanPrefixLockedVault(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	// Set keys while unlocked
	err := ts.KVSet(context.Background(), "prefix:key1", "value1", 0)
	require.NoError(t, err)
	err = ts.KVSet(context.Background(), "prefix:key2", "value2", 0)
	require.NoError(t, err)

	// Lock the vault
	testVault.Lock()

	// Scan should return empty map (cannot decrypt)
	result, err := ts.KVScanPrefix(context.Background(), "prefix:")
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestTokenStoreService_NilService verifies that methods handle
// nil TokenStoreService gracefully (fail-closed behavior).
func TestTokenStoreService_NilService(t *testing.T) {
	t.Parallel()
	var ts *TokenStoreService

	// All methods should handle nil gracefully
	err := ts.KVSet(context.Background(), "key", "value", 0)
	assert.Error(t, err, "KVSet should error on nil service")

	_, err = ts.KVGet(context.Background(), "key")
	assert.Error(t, err)

	result, err := ts.KVScanPrefix(context.Background(), "prefix:")
	assert.Error(t, err)
	assert.Nil(t, result)

	err = ts.KVDelete("key")
	assert.Error(t, err, "KVDelete should error on nil service")

	assert.False(t, ts.IsEnabled())
}

// TestTokenStoreService_IsEnabled verifies that IsEnabled returns
// true for properly initialized service and false for nil.
func TestTokenStoreService_IsEnabled(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	assert.True(t, ts.IsEnabled())

	var nilTS *TokenStoreService
	assert.False(t, nilTS.IsEnabled())
}

// TestTokenStoreService_Wait verifies that Wait blocks until
// all background operations complete.
func TestTokenStoreService_Wait(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	// Perform some operations
	for i := 0; i < 10; i++ {
		err := ts.KVSet(context.Background(), "wait-key", "value", 0)
		require.NoError(t, err)
	}

	// Wait should complete without hanging
	ts.Wait()
}

// TestTokenStoreService_Close verifies that Close properly
// shuts down the service and stops the pruner.
func TestTokenStoreService_Close(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	assert.True(t, ts.IsEnabled())

	err := ts.Close()
	require.NoError(t, err)

	// After close, the service should still be considered enabled (has db reference)
	// but operations will fail due to closed database
	assert.True(t, ts.IsEnabled())
}

// TestTokenStoreService_CloseNil verifies that Close handles
// nil service gracefully.
func TestTokenStoreService_CloseNil(t *testing.T) {
	t.Parallel()
	var ts *TokenStoreService

	err := ts.Close()
	assert.NoError(t, err)
}

// TestTokenStoreService_ConcurrentOperations verifies that the service
// handles concurrent operations correctly with proper synchronization.
func TestTokenStoreService_ConcurrentOperations(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	// Perform concurrent sets
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			key := "concurrent-key"
			value := "value"
			err := ts.KVSet(context.Background(), key, value, 0)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Wait for background workers
	ts.Wait()

	// Verify the key exists
	retrieved, err := ts.KVGet(context.Background(), "concurrent-key")
	require.NoError(t, err)
	assert.Equal(t, "value", retrieved)
}

// TestTokenStoreService_LargeValue verifies that the service can
// handle large values (stress test).
func TestTokenStoreService_LargeValue(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	// Create a large value (1MB)
	largeValue := make([]byte, 1024*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	key := "large-value-key"
	err := ts.KVSet(context.Background(), key, string(largeValue), 0)
	require.NoError(t, err)

	retrieved, err := ts.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, string(largeValue), retrieved)
}

// TestTokenStoreService_SpecialCharacters verifies that the service
// handles keys and values with special characters.
func TestTokenStoreService_SpecialCharacters(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	cases := []struct {
		key   string
		value string
	}{
		{"key-with-dashes", "value with spaces"},
		{"key_with_underscores", "value\nwith\nnewlines"},
		{"key:with:colons", "value\twith\ttabs"},
		{"key/with/slashes", "value\"with\"quotes"},
		{"key.with.dots", "value'with'apostrophes"},
	}

	for _, tc := range cases {
		err := ts.KVSet(context.Background(), tc.key, tc.value, 0)
		require.NoError(t, err)

		retrieved, err := ts.KVGet(context.Background(), tc.key)
		require.NoError(t, err)
		assert.Equal(t, tc.value, retrieved)
	}
}

// TestTokenStoreService_EmptyKey verifies that the service handles
// empty keys gracefully.
func TestTokenStoreService_EmptyKey(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	err := ts.KVSet(context.Background(), "", "value", 0)
	require.NoError(t, err)

	retrieved, err := ts.KVGet(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "value", retrieved)
}

// TestTokenStoreService_EmptyValue verifies that the service handles
// empty values correctly.
func TestTokenStoreService_EmptyValue(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	key := "empty-value-key"
	err := ts.KVSet(context.Background(), key, "", 0)
	require.NoError(t, err)

	retrieved, err := ts.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, "", retrieved)
}

// TestTokenStoreService_NegativeTTL verifies that negative TTL
// is treated as no expiration (TTL=0).
func TestTokenStoreService_NegativeTTL(t *testing.T) {
	t.Parallel()
	ts, testVault, _ := setupTestTokenStore(t)
	defer testVault.Close()

	key := "negative-ttl-key"
	value := "value"

	err := ts.KVSet(context.Background(), key, value, -1)
	require.NoError(t, err)

	// Should be retrievable immediately
	retrieved, err := ts.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)

	// Should still be retrievable after time passes
	time.Sleep(1 * time.Second)
	retrieved, err = ts.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)
}
