// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestStateRootSemantics(t *testing.T) {
	db, stores := newTestDB(t)

	// 1. Initial state root must be deterministic
	root1, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	require.NotEmpty(t, root1)

	root1Again, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root1Again, "State root must be deterministic for identical state")

	// 1.5. Verify caching - second call should use cache (same version)
	// This is implicitly tested by the above, but we can verify the version tracking
	var version1 int64
	err = db.db.QueryRow("SELECT version FROM state_version WHERE id = 1").Scan(&version1)
	require.NoError(t, err)

	// 2. Document content change alters root
	err = stores.DocStore.DocSet("test", "d1", json.RawMessage(`{"val":1}`))
	require.NoError(t, err)
	root2, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root1, root2, "Content change must alter state root")

	// 2.5. Verify version incremented
	var version2 int64
	err = db.db.QueryRow("SELECT version FROM state_version WHERE id = 1").Scan(&version2)
	require.NoError(t, err)
	assert.Greater(t, version2, version1, "State version must increment on document change")

	// 3. Document metadata change (updated_at) does NOT alter root
	// Small delay to ensure updated_at timestamp changes
	time.Sleep(10 * time.Millisecond)
	err = stores.DocStore.DocSet("test", "d1", json.RawMessage(`{"val":1}`))
	require.NoError(t, err)
	root3, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root2, root3, "Metadata-only change (updated_at) must NOT alter state root")

	// 4. KV change alters root
	err = stores.KVStore.KVSet("k1", "v1", 0)
	require.NoError(t, err)
	root4, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root3, root4, "KV change must alter state root")

	// 5. Blob change alters root
	err = stores.BlobStore.BlobPut("ns", "b1", []byte("data"), "text/plain", 0)
	require.NoError(t, err)
	root5, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root4, root5, "Blob change must alter state root")

	// 6. Nonce insert does NOT alter root
	replayed, err := stores.ReplayStore.ReserveNonce("nonce1", time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, replayed)
	root6, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root5, root6, "Nonce insert must NOT alter state root")

	// 7. SSE event insert does NOT alter root
	_, err = stores.SSEStore.SSEEventsAppend(SSERoute{UserID: "u-state-root", WebSessionID: "session1"}, "type1", "payload1", "")
	require.NoError(t, err)
	root7, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root6, root7, "SSE event insert must NOT alter state root")

	// 8. Expired KV is excluded from root
	err = stores.KVStore.KVSet("exp1", "val", 1) // 1 second TTL
	require.NoError(t, err)
	rootWithExp, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, rootWithExp, root7)

	// Manually delete the expired entry to simulate maintenance job
	_, err = db.db.Exec("DELETE FROM kv_store WHERE key = 'exp1'")
	require.NoError(t, err)

	rootAfterExp, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, rootAfterExp, root7, "Expired KV must be excluded from state root calculation")
}

func TestStateRootDeterministicOrder(t *testing.T) {
	db1, stores1 := newTestDB(t)
	db2, stores2 := newTestDB(t)

	// Wipe initial random platform settings to have a clean slate for order comparison
	_, err := db1.db.Exec("DELETE FROM documents")
	require.NoError(t, err)
	_, err = db1.db.Exec("DELETE FROM kv_store")
	require.NoError(t, err)

	_, err = db2.db.Exec("DELETE FROM documents")
	require.NoError(t, err)
	_, err = db2.db.Exec("DELETE FROM kv_store")
	require.NoError(t, err)

	// Insert in one order into db1
	require.NoError(t, stores1.DocStore.DocSet("test", "a", json.RawMessage(`{"v":1}`)))
	require.NoError(t, stores1.DocStore.DocSet("test", "b", json.RawMessage(`{"v":2}`)))
	require.NoError(t, stores1.KVStore.KVSet("k1", "v1", 0))
	require.NoError(t, stores1.KVStore.KVSet("k2", "v2", 0))
	root1, err := stores1.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)

	// Insert in different order into db2
	require.NoError(t, stores2.KVStore.KVSet("k2", "v2", 0))
	require.NoError(t, stores2.DocStore.DocSet("test", "b", json.RawMessage(`{"v":2}`)))
	require.NoError(t, stores2.KVStore.KVSet("k1", "v1", 0))
	require.NoError(t, stores2.DocStore.DocSet("test", "a", json.RawMessage(`{"v":1}`)))
	root2, err := stores2.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)

	assert.Equal(t, root1, root2, "State root must be deterministic regardless of insertion order")
}

func TestStateRootCaching(t *testing.T) {
	db, stores := newTestDB(t)

	// Get initial root and version
	root1, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)

	var version1 int64
	err = db.db.QueryRow("SELECT version FROM state_version WHERE id = 1").Scan(&version1)
	require.NoError(t, err)

	// Call again without changes - should use cache
	root2, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "Cached root must match")

	// Verify cache is being used by checking internal state
	assert.Equal(t, root1, stores.StateRootSvc.cachedStateRoot, "Internal cache should be set")
	assert.Equal(t, version1, stores.StateRootSvc.cachedStateVersion, "Internal version should match")

	// Make a change
	err = stores.DocStore.DocSet("cache_test", "doc1", json.RawMessage(`{"data":1}`))
	require.NoError(t, err)

	// Version should have incremented
	var version2 int64
	err = db.db.QueryRow("SELECT version FROM state_version WHERE id = 1").Scan(&version2)
	require.NoError(t, err)
	assert.Greater(t, version2, version1, "Version must increment on change")

	// Get new root - should recalculate
	root3, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root1, root3, "Root must change after data change")

	// Cache should be updated
	assert.Equal(t, root3, stores.StateRootSvc.cachedStateRoot, "Cache should be updated")
	assert.Equal(t, version2, stores.StateRootSvc.cachedStateVersion, "Cache version should be updated")
}

func BenchmarkStateRootCalculation(b *testing.B) {
	dir := b.TempDir()
	baseDir := b.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(b, err)
	require.NoError(b, fileSvc.CreateRuntimeTree(context.Background()))
	ks := newTestKeystore(b, fileSvc, testutil.NewTestLogger())
	db, stores, err := OpenCanonicalDBService(dir, fileSvc.Resolve(constants.VaultDirname), testutil.NewTestLogger(), "", ks, fileSvc)
	require.NoError(b, err)
	defer db.Close()

	// Populate with realistic data
	for i := 0; i < 100; i++ {
		docData := fmt.Sprintf(`{"field1":"value%d","field2":%d}`, i, i*2)
		require.NoError(b, stores.DocStore.DocSet("benchmark", fmt.Sprintf("doc%d", i), json.RawMessage(docData)))
		require.NoError(b, stores.KVStore.KVSet(fmt.Sprintf("key%d", i), fmt.Sprintf("val%d", i), 0))
		require.NoError(b, stores.BlobStore.BlobPut("ns", fmt.Sprintf("blob%d", i), []byte(fmt.Sprintf("data%d", i)), "text/plain", 0))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := stores.StateRootSvc.GetCurrentStateRoot()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateRootLargeDataset(b *testing.B) {
	dir := b.TempDir()
	baseDir := b.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(b, err)
	require.NoError(b, fileSvc.CreateRuntimeTree(context.Background()))
	ks := newTestKeystore(b, fileSvc, testutil.NewTestLogger())
	db, stores, err := OpenCanonicalDBService(dir, fileSvc.Resolve(constants.VaultDirname), testutil.NewTestLogger(), "", ks, fileSvc)
	require.NoError(b, err)
	defer db.Close()

	// Populate with larger dataset to test scalability
	for i := 0; i < 1000; i++ {
		docData := fmt.Sprintf(`{"field1":"value%d","field2":%d,"field3":"%s"}`, i, i*2, strings.Repeat("x", 100))
		require.NoError(b, stores.DocStore.DocSet("benchmark", fmt.Sprintf("doc%d", i), json.RawMessage(docData)))
		require.NoError(b, stores.KVStore.KVSet(fmt.Sprintf("key%d", i), fmt.Sprintf("val%d", i), 0))
		require.NoError(b, stores.BlobStore.BlobPut("ns", fmt.Sprintf("blob%d", i), []byte(strings.Repeat("y", 500)), "text/plain", 0))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := stores.StateRootSvc.GetCurrentStateRoot()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestStateRoot_ObservedKVDoesNotChurnBoundRoot(t *testing.T) {
	_, stores := newTestDB(t)

	// Get initial bound root
	root1, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)

	// Write an observed-state KV entry
	err = stores.KVStore.KVSetObserved("observed:metric:cpu", "42.5", 0)
	require.NoError(t, err)

	// Bound root must NOT change — observed state is excluded
	root2, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "bound root must not change when observed-state KV is written")

	// Write a bound KV entry — root must change
	err = stores.KVStore.KVSet("bound:config:timeout", "30", 0)
	require.NoError(t, err)
	root3, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root2, root3, "bound root must change when bound-state KV is written")
}

func TestStateRoot_ObservedBlobDoesNotChurnBoundRoot(t *testing.T) {
	_, stores := newTestDB(t)

	// Get initial bound root
	root1, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)

	// Write an observed-state blob
	err = stores.BlobStore.BlobPutObserved("telemetry", "snapshot1", []byte("observed-data"), "application/octet-stream", 0)
	require.NoError(t, err)

	// Bound root must NOT change
	root2, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "bound root must not change when observed-state blob is written")

	// Write a bound blob — root must change
	err = stores.BlobStore.BlobPut("config", "binary1", []byte("bound-data"), "application/octet-stream", 0)
	require.NoError(t, err)
	root3, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root2, root3, "bound root must change when bound-state blob is written")
}

func TestStateRoot_ObservedStateRootIsSeparate(t *testing.T) {
	_, stores := newTestDB(t)

	// Get initial observed root (may be empty hash if no observed state)
	obsRoot1, err := stores.StateRootSvc.GetObservedStateRoot()
	require.NoError(t, err)

	// Write an observed-state KV entry
	err = stores.KVStore.KVSetObserved("observed:metric:memory", "8192", 0)
	require.NoError(t, err)

	// Observed root must change
	obsRoot2, err := stores.StateRootSvc.GetObservedStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, obsRoot1, obsRoot2, "observed root must change when observed-state KV is written")

	// Bound root must NOT change
	boundRoot1, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)

	err = stores.KVStore.KVSetObserved("observed:metric:disk", "512", 0)
	require.NoError(t, err)

	boundRoot2, err := stores.StateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, boundRoot1, boundRoot2, "bound root must not change when only observed state changes")

	// Observed root must change again
	obsRoot3, err := stores.StateRootSvc.GetObservedStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, obsRoot2, obsRoot3, "observed root must change when more observed state is written")
}

func TestStateRoot_ObservedStateRootCaching(t *testing.T) {
	_, stores := newTestDB(t)

	// Get observed root — should cache it
	root1, err := stores.StateRootSvc.GetObservedStateRoot()
	require.NoError(t, err)

	// Call again — should return cached value
	root2, err := stores.StateRootSvc.GetObservedStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "observed root should be cached")

	// Invalidate cache
	err = stores.StateRootSvc.InvalidateCache()
	require.NoError(t, err)

	// Call again — should recalculate but return same value (no state change)
	root3, err := stores.StateRootSvc.GetObservedStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root3, "observed root should be same after recalculation without changes")
}
