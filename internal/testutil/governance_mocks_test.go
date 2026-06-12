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

package testutil

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockReplayStore(t *testing.T) {
	store := &MockReplayStore{}

	// ReserveNonce never detects replays
	isReplay, err := store.ReserveNonce("test-nonce", time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	require.False(t, isReplay)

	// FinalizeNonce is a no-op
	err = store.FinalizeNonce("test-nonce")
	require.NoError(t, err)

	// ReleaseNonce is a no-op
	err = store.ReleaseNonce("test-nonce")
	require.NoError(t, err)
}

func TestStatefulMockReplayStore(t *testing.T) {
	store := NewStatefulMockReplayStore()
	require.NotNil(t, store)
	require.NotNil(t, store.Nonces)

	nonce := "test-nonce-123"
	expiresAt := time.Now().Add(5 * time.Minute)

	// First use - should not be a replay
	isReplay, err := store.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	require.False(t, isReplay, "First use should not be a replay")

	// Second use - should be a replay
	isReplay, err = store.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	require.True(t, isReplay, "Second use should be detected as replay")

	// Different nonce - should not be a replay
	nonce2 := "test-nonce-456"
	isReplay, err = store.ReserveNonce(nonce2, expiresAt)
	require.NoError(t, err)
	require.False(t, isReplay, "Different nonce should not be a replay")

	// Test ReserveNonce
	nonce3 := "test-nonce-reserve"
	isReplay, err = store.ReserveNonce(nonce3, expiresAt)
	require.NoError(t, err)
	require.False(t, isReplay, "First reserve should not be a replay")

	isReplay, err = store.ReserveNonce(nonce3, expiresAt)
	require.NoError(t, err)
	require.True(t, isReplay, "Second reserve should be detected as replay")

	// Test ReleaseNonce
	err = store.ReleaseNonce(nonce3)
	require.NoError(t, err)

	// After release, nonce should be available again
	isReplay, err = store.ReserveNonce(nonce3, expiresAt)
	require.NoError(t, err)
	require.False(t, isReplay, "After release, nonce should not be a replay")

	// Test FinalizeNonce (no-op)
	err = store.FinalizeNonce(nonce3)
	require.NoError(t, err)
}

func TestStatefulMockReplayStore_Concurrent(t *testing.T) {
	store := NewStatefulMockReplayStore()
	nonce := "concurrent-nonce"
	expiresAt := time.Now().Add(5 * time.Minute)

	// Test concurrent access
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := store.ReserveNonce(nonce, expiresAt)
			assert.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Nonce should be marked as used
	isReplay, err := store.ReserveNonce(nonce, expiresAt)
	require.NoError(t, err)
	require.True(t, isReplay, "Nonce should be marked as used after concurrent access")
}

func TestMockStateRootProvider(t *testing.T) {
	root := "test-state-root-123"
	provider := NewMockStateRootProvider(root)

	require.Equal(t, root, provider.Root, "Root should be set correctly")

	retrievedRoot, err := provider.GetCurrentStateRoot()
	require.NoError(t, err)
	require.Equal(t, root, retrievedRoot, "Retrieved root should match set root")
}

func TestMockStateRootProvider_EmptyRoot(t *testing.T) {
	provider := NewMockStateRootProvider("")
	require.Empty(t, provider.Root)

	retrievedRoot, err := provider.GetCurrentStateRoot()
	require.NoError(t, err)
	require.Empty(t, retrievedRoot)
}

func TestMockL3Notary(t *testing.T) {
	notary := &MockL3Notary{}

	// MockL3Notary always approves
	approved, err := notary.VerifyL3Proof(context.Background(), "user-123", "tx-hash-456", "session-789", nil)
	require.NoError(t, err)
	require.True(t, approved, "MockL3Notary should always approve")
}

func TestConfigurableMockL3Notary(t *testing.T) {
	// Test pass behavior
	passNotary := NewConfigurableMockL3Notary(true)
	approved, err := passNotary.VerifyL3Proof(context.Background(), "user-123", "tx-hash-456", "session-789", nil)
	require.NoError(t, err)
	require.True(t, approved, "Configurable notary with ShouldPass=true should approve")

	// Test fail behavior
	failNotary := NewConfigurableMockL3Notary(false)
	approved, err = failNotary.VerifyL3Proof(context.Background(), "user-123", "tx-hash-456", "session-789", nil)
	require.NoError(t, err)
	require.False(t, approved, "Configurable notary with ShouldPass=false should reject")
}

func TestSlowMockL3Notary(t *testing.T) {
	delay := 50 * time.Millisecond
	notary := NewSlowMockL3Notary(delay)

	start := time.Now()
	approved, err := notary.VerifyL3Proof(context.Background(), "user-123", "tx-hash-456", "session-789", nil)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.True(t, approved, "SlowMockL3Notary should approve after delay")
	require.GreaterOrEqual(t, elapsed, delay, "SlowMockL3Notary should have waited at least the specified delay")
}

func TestMockTransactionAudit(t *testing.T) {
	audit := &MockTransactionAudit{}

	// MockTransactionAudit is a no-op
	err := audit.DocSet("test-collection", "test-id", json.RawMessage(`{"test": "data"}`))
	require.NoError(t, err, "MockTransactionAudit should not return errors")
}

func TestConfigurableMockAuditStore(t *testing.T) {
	calls := []struct {
		Collection string
		ID         string
		Data       json.RawMessage
	}{}

	docSetFunc := func(collection, id string, data json.RawMessage) error {
		calls = append(calls, struct {
			Collection string
			ID         string
			Data       json.RawMessage
		}{collection, id, data})
		return nil
	}

	store := NewConfigurableMockAuditStore(docSetFunc)
	require.NotNil(t, store)

	data := json.RawMessage(`{"test": "value"}`)
	err := store.DocSet("collection-1", "id-1", data)
	require.NoError(t, err)

	require.Len(t, store.Calls, 1, "Should have recorded one call")
	require.Equal(t, "collection-1", store.Calls[0].Collection)
	require.Equal(t, "id-1", store.Calls[0].ID)
	require.Equal(t, data, store.Calls[0].Data)

	// Test multiple calls
	err = store.DocSet("collection-2", "id-2", json.RawMessage(`{"test": "value2"}`))
	require.NoError(t, err)

	err = store.DocSet("collection-3", "id-3", json.RawMessage(`{"test": "value3"}`))
	require.NoError(t, err)

	require.Len(t, store.Calls, 3, "Should have recorded three calls")
}

func TestConfigurableMockAuditStore_ErrorFunc(t *testing.T) {
	expectedErr := errors.New("test error")
	docSetFunc := func(collection, id string, data json.RawMessage) error {
		return expectedErr
	}

	store := NewConfigurableMockAuditStore(docSetFunc)
	err := store.DocSet("collection-1", "id-1", json.RawMessage(`{}`))
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestConfigurableMockAuditStore_NilFunc(t *testing.T) {
	store := NewConfigurableMockAuditStore(nil)
	require.NotNil(t, store)

	err := store.DocSet("collection-1", "id-1", json.RawMessage(`{}`))
	require.NoError(t, err, "Nil DocSetFunc should not return error")

	require.Len(t, store.Calls, 1, "Should still record calls even with nil func")
}
