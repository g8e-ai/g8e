// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// setupTestReplayStore creates a real SQLReplayStore with a temporary database.
func setupTestReplayStore(t *testing.T) *SQLReplayStore {
	t.Helper()

	tempDir := testutil.TempDir(t)
	dbPath := filepath.Join(tempDir, "test_replay_store.db")

	config := &ReplayStoreConfig{
		DBPath: dbPath,
	}

	logger := testutil.NewTestLogger()
	rs, err := NewSQLReplayStore(config, logger)
	require.NoError(t, err)
	require.NotNil(t, rs)

	t.Cleanup(func() {
		rs.Close()
	})

	return rs
}

func TestReplayStore_ReserveNonce_Success(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-1"
	expiresAt := time.Now().UTC().Add(time.Hour)

	isReplay, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.False(t, isReplay, "first reservation should not be detected as replay")
}

func TestReplayStore_ReserveNonce_ReplayDetection(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-replay"
	expiresAt := time.Now().UTC().Add(time.Hour)

	// First reservation should succeed
	isReplay, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.False(t, isReplay)

	// Second reservation with same nonce should detect replay
	isReplay, err = rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.True(t, isReplay, "second reservation should be detected as replay")
}

func TestReplayStore_ReserveNonce_MultipleNonces(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Reserve multiple different nonces
	nonces := []string{"nonce-1", "nonce-2", "nonce-3"}
	for _, nonce := range nonces {
		isReplay, err := rs.ReserveNonce(nonce, expiresAt)
		require.NoError(t, err)
		assert.False(t, isReplay)
	}

	// Each nonce should still be detectable as replay
	for _, nonce := range nonces {
		isReplay, err := rs.ReserveNonce(nonce, expiresAt)
		require.NoError(t, err)
		assert.True(t, isReplay)
	}
}

func TestReplayStore_FinalizeNonce_Success(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-finalize"
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Reserve the nonce
	_, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)

	// Finalize the nonce
	err = rs.FinalizeNonce(nonce)
	require.NoError(t, err)
}

func TestReplayStore_FinalizeNonce_NotReserved(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-not-reserved"

	// Try to finalize a nonce that was never reserved
	err := rs.FinalizeNonce(nonce)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or not in reserved state")
}

func TestReplayStore_ReleaseNonce_Success(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-release"
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Reserve the nonce
	_, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)

	// Release the nonce
	err = rs.ReleaseNonce(nonce)
	require.NoError(t, err)

	// After release, the nonce should be available for reservation again
	isReplay, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.False(t, isReplay, "nonce should be available after release")
}

func TestReplayStore_ReleaseNonce_NotReserved(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-not-reserved-release"

	// Try to release a nonce that was never reserved - should not error (no-op)
	err := rs.ReleaseNonce(nonce)
	require.NoError(t, err)
}

func TestReplayStore_ReleaseNonce_AlreadyFinalized(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-already-finalized"
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Reserve and finalize the nonce
	_, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	err = rs.FinalizeNonce(nonce)
	require.NoError(t, err)

	// Try to release a finalized nonce - should not error (no-op)
	err = rs.ReleaseNonce(nonce)
	require.NoError(t, err)
}

func TestReplayStore_Prune(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-prune"
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Reserve and finalize the nonce
	_, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	err = rs.FinalizeNonce(nonce)
	require.NoError(t, err)

	// Prune with 1 retention day should not remove recently used records
	// Since we just finalized, the record won't be deleted.
	err = rs.Prune(1)
	require.NoError(t, err)

	// The nonce should still be a replay since it wasn't pruned (recently used)
	isReplay, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.True(t, isReplay, "nonce should still be a replay after prune (not old enough)")
}

func TestReplayStore_NewStore_NilConfig(t *testing.T) {
	t.Parallel()

	logger := testutil.NewTestLogger()
	rs, err := NewSQLReplayStore(nil, logger)
	require.NoError(t, err)
	assert.NotNil(t, rs)
	assert.NotNil(t, rs.config)
}

func TestReplayStore_Close_NilStore(t *testing.T) {
	t.Parallel()

	var rs *SQLReplayStore
	err := rs.Close()
	require.NoError(t, err)
}

func TestReplayStore_ReserveNonce_ExpiredNonce(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-expired"
	expiresAt := time.Now().UTC().Add(-time.Hour) // Already expired

	// Reserve with an already-expired timestamp
	isReplay, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.False(t, isReplay)

	// The expired nonce should be cleaned up on next reservation
	isReplay, err = rs.ReserveNonce(nonce, time.Now().UTC().Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, isReplay, "expired nonce should be cleaned up and available")
}

func TestReplayStore_FullWorkflow(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-workflow"
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Step 1: Reserve nonce
	isReplay, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.False(t, isReplay)

	// Step 2: Attempt replay (should be detected)
	isReplay, err = rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.True(t, isReplay)

	// Step 3: Release nonce (simulating transaction failure)
	err = rs.ReleaseNonce(nonce)
	require.NoError(t, err)

	// Step 4: Reserve again (should succeed after release)
	isReplay, err = rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.False(t, isReplay)

	// Step 5: Finalize nonce (simulating transaction success)
	err = rs.FinalizeNonce(nonce)
	require.NoError(t, err)

	// Step 6: Attempt to finalize again (should fail)
	err = rs.FinalizeNonce(nonce)
	require.Error(t, err)
}

func TestReplayStore_CleanupStaleReserved_Success(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-stale"
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Reserve the nonce
	_, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)

	// Brief sleep to ensure time.Now() in CleanupStaleReserved advances past
	// the reserved_at timestamp (Windows clock resolution is ~15ms).
	time.Sleep(20 * time.Millisecond)

	// Cleanup with a very short duration (should remove the reserved nonce)
	err = rs.CleanupStaleReserved(1 * time.Nanosecond)
	require.NoError(t, err)

	// The nonce should now be available for reservation again
	isReplay, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.False(t, isReplay, "nonce should be available after stale cleanup")
}

func TestReplayStore_CleanupStaleReserved_NoStaleNonces(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-fresh"
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Reserve the nonce
	_, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)

	// Cleanup with a long duration (should not remove the fresh nonce)
	err = rs.CleanupStaleReserved(24 * time.Hour)
	require.NoError(t, err)

	// The nonce should still be detected as replay
	isReplay, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.True(t, isReplay, "nonce should still be a replay after cleanup with long duration")
}

func TestReplayStore_CleanupStaleReserved_NilStore(t *testing.T) {
	t.Parallel()

	var rs *SQLReplayStore

	// CleanupStaleReserved on nil store will panic - this is expected behavior
	assert.Panics(t, func() {
		rs.CleanupStaleReserved(1 * time.Hour)
	})
}

func TestReplayStore_CleanupStaleReserved_MultipleNonces(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Reserve multiple nonces
	nonces := []string{"stale-1", "stale-2", "stale-3"}
	for _, nonce := range nonces {
		_, err := rs.ReserveNonce(nonce, expiresAt)
		require.NoError(t, err)
	}

	// Brief sleep to ensure time.Now() in CleanupStaleReserved advances past
	// all reserved_at timestamps (Windows clock resolution is ~15ms).
	time.Sleep(20 * time.Millisecond)

	// Cleanup all as stale
	err := rs.CleanupStaleReserved(1 * time.Nanosecond)
	require.NoError(t, err)

	// All nonces should be available again
	for _, nonce := range nonces {
		isReplay, err := rs.ReserveNonce(nonce, expiresAt)
		require.NoError(t, err)
		assert.False(t, isReplay, "nonce %s should be available after stale cleanup", nonce)
	}
}

func TestReplayStore_CleanupStaleReserved_FinalizedNonces(t *testing.T) {
	t.Parallel()

	rs := setupTestReplayStore(t)
	nonce := "test-nonce-finalized-stale"
	expiresAt := time.Now().UTC().Add(time.Hour)

	// Reserve and finalize the nonce
	_, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	err = rs.FinalizeNonce(nonce)
	require.NoError(t, err)

	// Cleanup should not affect finalized nonces (they have different status)
	err = rs.CleanupStaleReserved(1 * time.Nanosecond)
	require.NoError(t, err)

	// The nonce should still be a replay (finalized, not reserved)
	isReplay, err := rs.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	assert.True(t, isReplay, "finalized nonce should still be a replay after stale cleanup")
}
