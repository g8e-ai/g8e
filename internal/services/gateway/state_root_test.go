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

	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestStateRootSemantics(t *testing.T) {
	db := newTestDB(t)

	// 1. Initial state root must be deterministic
	root1, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, root1)

	root1Again, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root1, root1Again, "State root must be deterministic for identical state")

	// 2. Document content change alters root
	// The first write is stamped in the past so step 3's updated_at differs.
	past := time.Now().UTC().Add(-time.Hour)
	err = db.GetDocStore().DocSetWithTimestamps(t.Context(), "test", "d1", json.RawMessage(`{"val":1}`), past, past)
	require.NoError(t, err)
	root2, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, root1, root2, "Content change must alter state root")

	// 3. Document metadata change (updated_at) does NOT alter root
	err = db.GetDocStore().DocSet(t.Context(), "test", "d1", json.RawMessage(`{"val":1}`))
	require.NoError(t, err)
	root3, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root2, root3, "Metadata-only change (updated_at) must NOT alter state root")

	// 4. KV change alters root
	err = db.GetKVStore().KVSet(t.Context(), "k1", "v1", 0)
	require.NoError(t, err)
	root4, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, root3, root4, "KV change must alter state root")

	// 5. Blob change alters root
	err = db.GetBlobStore().BlobPut("ns", "b1", []byte("data"), "text/plain", 0)
	require.NoError(t, err)
	root5, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, root4, root5, "Blob change must alter state root")

	// 6. Nonce insert does NOT alter root
	replayed, err := db.GetReplayStore().ReserveNonce("nonce1", time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, replayed)
	root6, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root5, root6, "Nonce insert must NOT alter state root")

	// 7. SSE event insert does NOT alter root
	_, err = db.GetSSEStore().SSEEventsAppend(SSERoute{UserID: "u-state-root", WebSessionID: "session1"}, "type1", "payload1", "")
	require.NoError(t, err)
	root7, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root6, root7, "SSE event insert must NOT alter state root")

	// 8. Expired KV is excluded from root
	err = db.GetKVStore().KVSet(t.Context(), "exp1", "val", 1) // 1 second TTL
	require.NoError(t, err)
	rootWithExp, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, rootWithExp, root7)

	// Manually delete the expired entry to simulate maintenance job
	_, err = db.db.Exec("DELETE FROM kv_store WHERE key = 'exp1'")
	require.NoError(t, err)

	rootAfterExp, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, rootAfterExp, root7, "Expired KV must be excluded from state root calculation")
}

func TestStateRootDeterministicOrder(t *testing.T) {
	db1 := newTestDB(t)
	db2 := newTestDB(t)

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
	require.NoError(t, db1.GetDocStore().DocSet(t.Context(), "test", "a", json.RawMessage(`{"v":1}`)))
	require.NoError(t, db1.GetDocStore().DocSet(t.Context(), "test", "b", json.RawMessage(`{"v":2}`)))
	require.NoError(t, db1.GetKVStore().KVSet(t.Context(), "k1", "v1", 0))
	require.NoError(t, db1.GetKVStore().KVSet(t.Context(), "k2", "v2", 0))
	root1, err := db1.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)

	// Insert in different order into db2
	require.NoError(t, db2.GetKVStore().KVSet(t.Context(), "k2", "v2", 0))
	require.NoError(t, db2.GetDocStore().DocSet(t.Context(), "test", "b", json.RawMessage(`{"v":2}`)))
	require.NoError(t, db2.GetKVStore().KVSet(t.Context(), "k1", "v1", 0))
	require.NoError(t, db2.GetDocStore().DocSet(t.Context(), "test", "a", json.RawMessage(`{"v":1}`)))
	root2, err := db2.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)

	assert.Equal(t, root1, root2, "State root must be deterministic regardless of insertion order")
}

func TestStateRootUnchangedReadsDoNoFlushWork(t *testing.T) {
	db := newTestDB(t)
	svc := db.GetStateRootSvc()

	root1, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	flushed := svc.flushedLeaves.Load()

	root2, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root1, root2)
	assert.Equal(t, flushed, svc.flushedLeaves.Load(), "an unchanged root read must not rehash any leaf")

	require.NoError(t, db.GetDocStore().DocSet(t.Context(), "cache_test", "doc1", json.RawMessage(`{"data":1}`)))
	root3, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, root1, root3, "Root must change after data change")
	assert.Equal(t, flushed+1, svc.flushedLeaves.Load(), "one write must rehash exactly one leaf")
}

func BenchmarkStateRootCalculation(b *testing.B) {
	baseDir := b.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(b, err)
	require.NoError(b, fileSvc.CreateRuntimeTree(context.Background()))
	ks := newTestKeystore(b, fileSvc, testutil.NewTestLogger())
	db, err := OpenCanonicalDBService(testutil.NewTestLogger(), "", ks, fileSvc)
	require.NoError(b, err)
	defer db.Close()

	// Populate with realistic data
	for i := 0; i < 100; i++ {
		docData := fmt.Sprintf(`{"field1":"value%d","field2":%d}`, i, i*2)
		require.NoError(b, db.GetDocStore().DocSet(context.Background(), "benchmark", fmt.Sprintf("doc%d", i), json.RawMessage(docData)))
		require.NoError(b, db.GetKVStore().KVSet(context.Background(), fmt.Sprintf("key%d", i), fmt.Sprintf("val%d", i), 0))
		require.NoError(b, db.GetBlobStore().BlobPut("ns", fmt.Sprintf("blob%d", i), []byte(fmt.Sprintf("data%d", i)), "text/plain", 0))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := db.GetStateRootSvc().GetCurrentStateRoot(b.Context())
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateRootLargeDataset(b *testing.B) {
	baseDir := b.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(b, err)
	require.NoError(b, fileSvc.CreateRuntimeTree(context.Background()))
	ks := newTestKeystore(b, fileSvc, testutil.NewTestLogger())
	db, err := OpenCanonicalDBService(testutil.NewTestLogger(), "", ks, fileSvc)
	require.NoError(b, err)
	defer db.Close()

	// Populate with larger dataset to test scalability
	for i := 0; i < 1000; i++ {
		docData := fmt.Sprintf(`{"field1":"value%d","field2":%d,"field3":"%s"}`, i, i*2, strings.Repeat("x", 100))
		require.NoError(b, db.GetDocStore().DocSet(context.Background(), "benchmark", fmt.Sprintf("doc%d", i), json.RawMessage(docData)))
		require.NoError(b, db.GetKVStore().KVSet(context.Background(), fmt.Sprintf("key%d", i), fmt.Sprintf("val%d", i), 0))
		require.NoError(b, db.GetBlobStore().BlobPut("ns", fmt.Sprintf("blob%d", i), []byte(strings.Repeat("y", 500)), "text/plain", 0))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := db.GetStateRootSvc().GetCurrentStateRoot(b.Context())
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestStateRoot_ObservedKVDoesNotChurnBoundRoot(t *testing.T) {
	db := newTestDB(t)

	// Get initial bound root
	root1, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)

	// Write an observed-state KV entry
	err = db.GetKVStore().KVSetObserved(t.Context(), "observed:metric:cpu", "42.5", 0)
	require.NoError(t, err)

	// Bound root must NOT change — observed state is excluded
	root2, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "bound root must not change when observed-state KV is written")

	// Write a bound KV entry — root must change
	err = db.GetKVStore().KVSet(t.Context(), "bound:config:timeout", "30", 0)
	require.NoError(t, err)
	root3, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, root2, root3, "bound root must change when bound-state KV is written")
}

func TestStateRoot_ObservedBlobDoesNotChurnBoundRoot(t *testing.T) {
	db := newTestDB(t)

	// Get initial bound root
	root1, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)

	// Write an observed-state blob
	err = db.GetBlobStore().BlobPutObserved("telemetry", "snapshot1", []byte("observed-data"), "application/octet-stream", 0)
	require.NoError(t, err)

	// Bound root must NOT change
	root2, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "bound root must not change when observed-state blob is written")

	// Write a bound blob — root must change
	err = db.GetBlobStore().BlobPut("config", "binary1", []byte("bound-data"), "application/octet-stream", 0)
	require.NoError(t, err)
	root3, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, root2, root3, "bound root must change when bound-state blob is written")
}

func TestStateRoot_ObservedStateRootIsSeparate(t *testing.T) {
	db := newTestDB(t)

	// Get initial observed root (may be empty hash if no observed state)
	obsRoot1, err := db.GetStateRootSvc().GetObservedStateRoot(t.Context())
	require.NoError(t, err)

	// Write an observed-state KV entry
	err = db.GetKVStore().KVSetObserved(t.Context(), "observed:metric:memory", "8192", 0)
	require.NoError(t, err)

	// Observed root must change
	obsRoot2, err := db.GetStateRootSvc().GetObservedStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, obsRoot1, obsRoot2, "observed root must change when observed-state KV is written")

	// Bound root must NOT change
	boundRoot1, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)

	err = db.GetKVStore().KVSetObserved(t.Context(), "observed:metric:disk", "512", 0)
	require.NoError(t, err)

	boundRoot2, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, boundRoot1, boundRoot2, "bound root must not change when only observed state changes")

	// Observed root must change again
	obsRoot3, err := db.GetStateRootSvc().GetObservedStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, obsRoot2, obsRoot3, "observed root must change when more observed state is written")
}

func TestStateRoot_ObservedStateRootSurvivesRebuild(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.GetKVStore().KVSetObserved(t.Context(), "observed:metric:load", "0.5", 0))

	root1, err := db.GetStateRootSvc().GetObservedStateRoot(t.Context())
	require.NoError(t, err)
	root2, err := db.GetStateRootSvc().GetObservedStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "observed root must be stable without changes")

	require.NoError(t, db.GetStateRootSvc().RebuildCommitment(t.Context()))
	root3, err := db.GetStateRootSvc().GetObservedStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root1, root3, "a full rebuild must reproduce the incremental observed root")
}
