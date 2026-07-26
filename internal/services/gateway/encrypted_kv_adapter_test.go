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
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupEncryptedKVTest creates a KVStoreService with gateway schema and an
// unlocked vault for testing EncryptedKVAdapter.
func setupEncryptedKVTest(t *testing.T) (*KVStoreService, *vault.Vault) {
	t.Helper()
	dir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()

	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(filepath.Join(dir, "test.db")), logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)

	kv := NewKVStoreService(db, logger)

	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	header, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(vaultDir))

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: logger})
	require.NoError(t, err)
	require.NoError(t, v.Unlock(privKey))
	t.Cleanup(func() { v.Close() })

	return kv, v
}

func TestEncryptedKVAdapter_SetGet(t *testing.T) {
	kv, v := setupEncryptedKVTest(t)
	adapter := NewEncryptedKVAdapter(kv, v)

	ctx := context.Background()
	err := adapter.KVSet(ctx, "test-key", "test-value", 0)
	require.NoError(t, err)

	val, err := adapter.KVGet(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, "test-value", val)
}

func TestEncryptedKVAdapter_GetNotFound(t *testing.T) {
	kv, v := setupEncryptedKVTest(t)
	adapter := NewEncryptedKVAdapter(kv, v)

	_, err := adapter.KVGet(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyNotFound)
}

func TestEncryptedKVAdapter_SetLockedVault(t *testing.T) {
	kv, _ := setupEncryptedKVTest(t)

	// Create a locked vault (no unlock)
	dir := testutil.TempDir(t)
	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	header, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(vaultDir))
	lockedVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	t.Cleanup(func() { lockedVault.Close() })

	adapter := NewEncryptedKVAdapter(kv, lockedVault)
	err = adapter.KVSet(context.Background(), "key", "value", 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrVaultLocked)
}

func TestEncryptedKVAdapter_GetLockedVault(t *testing.T) {
	kv, v := setupEncryptedKVTest(t)
	adapter := NewEncryptedKVAdapter(kv, v)

	ctx := context.Background()
	require.NoError(t, adapter.KVSet(ctx, "key", "value", 0))

	// Create a locked vault and swap it in
	dir := testutil.TempDir(t)
	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	header, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(vaultDir))
	lockedVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	t.Cleanup(func() { lockedVault.Close() })

	lockedAdapter := NewEncryptedKVAdapter(kv, lockedVault)
	_, err = lockedAdapter.KVGet(ctx, "key")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrVaultLocked)
}

func TestEncryptedKVAdapter_ScanPrefix(t *testing.T) {
	kv, v := setupEncryptedKVTest(t)
	adapter := NewEncryptedKVAdapter(kv, v)

	ctx := context.Background()
	require.NoError(t, adapter.KVSet(ctx, "prefix:key1", "val1", 0))
	require.NoError(t, adapter.KVSet(ctx, "prefix:key2", "val2", 0))
	require.NoError(t, adapter.KVSet(ctx, "other:key3", "val3", 0))

	result, err := adapter.KVScanPrefix(ctx, "prefix:")
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "val1", result["prefix:key1"])
	assert.Equal(t, "val2", result["prefix:key2"])
}

func TestEncryptedKVAdapter_ScanPrefixEmpty(t *testing.T) {
	kv, v := setupEncryptedKVTest(t)
	adapter := NewEncryptedKVAdapter(kv, v)

	result, err := adapter.KVScanPrefix(context.Background(), "nonexistent:")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestEncryptedKVAdapter_ScanPrefixLockedVault(t *testing.T) {
	kv, v := setupEncryptedKVTest(t)
	adapter := NewEncryptedKVAdapter(kv, v)

	ctx := context.Background()
	require.NoError(t, adapter.KVSet(ctx, "prefix:key1", "val1", 0))

	// Create a locked vault
	dir := testutil.TempDir(t)
	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	header, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(vaultDir))
	lockedVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	t.Cleanup(func() { lockedVault.Close() })

	lockedAdapter := NewEncryptedKVAdapter(kv, lockedVault)
	_, err = lockedAdapter.KVScanPrefix(ctx, "prefix:")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrVaultLocked)
}

func TestEncryptedKVAdapter_SetWithTTL(t *testing.T) {
	kv, v := setupEncryptedKVTest(t)
	adapter := NewEncryptedKVAdapter(kv, v)

	ctx := context.Background()
	err := adapter.KVSet(ctx, "ttl-key", "ttl-value", 3600)
	require.NoError(t, err)

	val, err := adapter.KVGet(ctx, "ttl-key")
	require.NoError(t, err)
	assert.Equal(t, "ttl-value", val)
}

func TestEncryptedKVAdapter_RoundtripEncryption(t *testing.T) {
	kv, v := setupEncryptedKVTest(t)
	adapter := NewEncryptedKVAdapter(kv, v)

	ctx := context.Background()
	testValue := "sensitive-data-that-should-be-encrypted"
	require.NoError(t, adapter.KVSet(ctx, "secret", testValue, 0))

	// Verify the raw stored value is NOT the plaintext
	raw, found := kv.KVGet(constants.SentinelKeyPrefix + "secret")
	require.True(t, found)
	assert.NotEqual(t, testValue, raw, "stored value should be encrypted")

	// Verify decrypt returns the original
	decrypted, err := adapter.KVGet(ctx, "secret")
	require.NoError(t, err)
	assert.Equal(t, testValue, decrypted)
}
