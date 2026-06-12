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
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestReplayStoreService(t *testing.T) *ReplayStoreService {
	t.Helper()
	db := newTestDB(t)
	return NewReplayStoreService(db.GetDB(), testutil.NewTestLogger())
}

func TestReplayStoreService_ReserveNonce(t *testing.T) {
	t.Parallel()
	svc := newTestReplayStoreService(t)

	// Reserve a new nonce
	expiresAt := time.Now().Add(1 * time.Hour)
	replayed, err := svc.ReserveNonce("nonce1", expiresAt)
	require.NoError(t, err)
	assert.False(t, replayed, "first reservation should not detect replay")

	// Reserve the same nonce again - should detect replay
	replayed, err = svc.ReserveNonce("nonce1", expiresAt)
	require.NoError(t, err)
	assert.True(t, replayed, "second reservation should detect replay")
}

func TestReplayStoreService_ReserveNonce_Concurrent(t *testing.T) {
	t.Parallel()
	svc := newTestReplayStoreService(t)

	expiresAt := time.Now().Add(1 * time.Hour)
	nonce := "concurrent-nonce"

	// Multiple concurrent reservations should only succeed once
	done := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func() {
			replayed, err := svc.ReserveNonce(nonce, expiresAt)
			assert.NoError(t, err)
			done <- !replayed // true if succeeded (not replay)
		}()
	}

	successCount := 0
	for i := 0; i < 2; i++ {
		if <-done {
			successCount++
		}
	}
	assert.Equal(t, 1, successCount, "exactly one reservation should succeed")
}

func TestReplayStoreService_FinalizeNonce(t *testing.T) {
	t.Parallel()
	svc := newTestReplayStoreService(t)

	// Create a nonce in the nonces table with expires_at
	expiresAt := time.Now().Add(1 * time.Hour)
	replayed, err := svc.ReserveNonce("test123", expiresAt)
	require.NoError(t, err)
	require.False(t, replayed)

	// Finalize the nonce
	err = svc.FinalizeNonce("test123")
	require.NoError(t, err)

	// Verify nonce was updated
	db := svc.db
	var status string
	err = db.QueryRowWithRetry("SELECT status FROM nonces WHERE nonce = ?", "test123").Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "used", status)
}

func TestReplayStoreService_FinalizeNonce_NonExistent(t *testing.T) {
	t.Parallel()
	svc := newTestReplayStoreService(t)

	// Finalize non-existent nonce should not error
	err := svc.FinalizeNonce("nonexistent")
	require.NoError(t, err)
}

func TestReplayStoreService_ReleaseNonce(t *testing.T) {
	t.Parallel()
	svc := newTestReplayStoreService(t)

	// Create a nonce in the nonces table
	expiresAt := time.Now().Add(1 * time.Hour)
	replayed, err := svc.ReserveNonce("test456", expiresAt)
	require.NoError(t, err)
	require.False(t, replayed)

	// Release the nonce
	err = svc.ReleaseNonce("test456")
	require.NoError(t, err)

	// Verify nonce was deleted
	db := svc.db
	var count int
	err = db.QueryRowWithRetry("SELECT COUNT(*) FROM nonces WHERE nonce = ?", "test456").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestReplayStoreService_ReleaseNonce_NonExistent(t *testing.T) {
	t.Parallel()
	svc := newTestReplayStoreService(t)

	// Release non-existent nonce should not error
	err := svc.ReleaseNonce("nonexistent")
	require.NoError(t, err)
}

func TestReplayStoreService_CleanupExpiredNonces(t *testing.T) {
	t.Parallel()
	svc := newTestReplayStoreService(t)

	// Create an expired nonce
	expiresAt := time.Now().Add(-1 * time.Hour)
	replayed, err := svc.ReserveNonce("expired-nonce", expiresAt)
	require.NoError(t, err)
	require.False(t, replayed)

	// Create a non-expired nonce
	expiresAt = time.Now().Add(1 * time.Hour)
	replayed, err = svc.ReserveNonce("valid-nonce", expiresAt)
	require.NoError(t, err)
	require.False(t, replayed)

	// Cleanup expired nonces
	err = svc.CleanupExpiredNonces()
	require.NoError(t, err)

	// Verify expired nonce was deleted
	db := svc.db
	var count int
	err = db.QueryRowWithRetry("SELECT COUNT(*) FROM nonces WHERE nonce = ?", "expired-nonce").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify valid nonce still exists
	err = db.QueryRowWithRetry("SELECT COUNT(*) FROM nonces WHERE nonce = ?", "valid-nonce").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
