// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newStateRootService(t *testing.T) *StateRootService {
	t.Helper()
	db := newTestDB(t)
	return db.GetStateRootSvc()
}

// oracleLeaf is one committed row as the oracle sees it.
type oracleLeaf struct {
	tier       string
	id, digest []byte
}

// oracleStateRoots recomputes both roots from a full scan of every committed
// row, building the whole tree in memory. It shares only the leaf/node encoding
// with the service, not its incremental path.
func oracleStateRoots(t *testing.T, db *CanonicalDBService) (bound, observed string) {
	t.Helper()
	leaves := oracleLeafSet(t, db)
	return oracleRoot(leaves, stateTierBound), oracleRoot(leaves, stateTierObserved)
}

// oracleLeafSet scans every committed row into leaves keyed by leaf identity.
func oracleLeafSet(t *testing.T, db *CanonicalDBService) map[string]oracleLeaf {
	t.Helper()
	leaves := make(map[string]oracleLeaf)
	add := func(tier string, key stateDirtyKey, fields ...[]byte) {
		id := stateLeafID(key.source, key.k1, key.k2)
		leaves[string(id)] = oracleLeaf{tier: tier, id: id, digest: stateLeafDigest(key, fields...)}
	}

	rows, err := db.db.Query("SELECT collection, id, data FROM documents")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var c, id, data string
		require.NoError(t, rows.Scan(&c, &id, &data))
		add(stateTierBound, stateDirtyKey{stateSourceDocuments, c, id}, []byte(data))
	}
	require.NoError(t, rows.Err())

	rows, err = db.db.Query("SELECT key, value, COALESCE(expires_at, ''), state_tier FROM kv_store")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var k, v, exp, tier string
		require.NoError(t, rows.Scan(&k, &v, &exp, &tier))
		add(tier, stateDirtyKey{stateSourceKV, k, ""}, []byte(v), []byte(exp))
	}
	require.NoError(t, rows.Err())

	rows, err = db.db.Query("SELECT namespace, id, size, content_type, data, COALESCE(expires_at, ''), state_tier FROM blobs")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var ns, id, ct, exp, tier string
		var size int64
		var data []byte
		require.NoError(t, rows.Scan(&ns, &id, &size, &ct, &data, &exp, &tier))
		add(tier, stateDirtyKey{stateSourceBlobs, ns, id}, binary.BigEndian.AppendUint64(nil, uint64(size)), []byte(ct), data, []byte(exp))
	}
	require.NoError(t, rows.Err())
	return leaves
}

// oracleRoot builds the tier's whole tree in memory, hashing every bucket.
func oracleRoot(leaves map[string]oracleLeaf, tier string) string {
	buckets := make(map[int][][2][]byte)
	for _, leaf := range leaves {
		if leaf.tier == tier {
			b := stateBucketOf(leaf.id)
			buckets[b] = append(buckets[b], [2][]byte{leaf.id, leaf.digest})
		}
	}
	width := 1
	for i := 0; i < stateTreeDepth; i++ {
		width *= stateTreeFanout
	}
	emptyBucket := stateBucketDigest(nil)
	level := make([][]byte, width)
	for b := range level {
		level[b] = emptyBucket
		if bucket, ok := buckets[b]; ok {
			sortLeaves(bucket)
			level[b] = stateBucketDigest(bucket)
		}
	}
	for l := stateTreeDepth - 1; l >= 0; l-- {
		next := make([][]byte, len(level)/stateTreeFanout)
		for i := range next {
			next[i] = stateNodeDigest(l, level[i*stateTreeFanout:(i+1)*stateTreeFanout])
		}
		level = next
	}
	return stateRootDigest(tier, level[0], "")
}

func sortLeaves(leaves [][2][]byte) {
	slices.SortFunc(leaves, func(a, b [2][]byte) int { return bytes.Compare(a[0], b[0]) })
}

func requireRootsMatchOracle(t *testing.T, db *CanonicalDBService) {
	t.Helper()
	wantBound, wantObserved := oracleStateRoots(t, db)
	bound, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	observed, err := db.GetStateRootSvc().GetObservedStateRoot(t.Context())
	require.NoError(t, err)
	require.Equal(t, wantBound, bound, "incremental bound root must equal the full-rebuild oracle")
	require.Equal(t, wantObserved, observed, "incremental observed root must equal the full-rebuild oracle")
}

func TestStateRootService_GetCurrentStateRoot(t *testing.T) {
	svc := newStateRootService(t)

	root1, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Len(t, root1, 64)

	root2, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root1, root2)
}

// TestStateRootService_CancellationInterruptsPooledRead occupies the sole
// SQLite connection and proves that a root read waiting for it returns when its
// caller's context ends, instead of blocking until the connection is released.
func TestStateRootService_CancellationInterruptsPooledRead(t *testing.T) {
	svc := newStateRootService(t)
	pool := svc.db
	pool.SetMaxOpenConns(1)
	conn, err := pool.Conn(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := svc.GetCurrentStateRoot(ctx)
		done <- err
	}()
	// Always release the connection and join the reader before fixture cleanup,
	// including when the read ignores cancellation.
	defer func() {
		cancel()
		require.NoError(t, conn.Close())
		<-done
	}()
	select {
	case err := <-done:
		done <- err
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorIs(t, err, constants.ErrStateRootCalculate)
	case <-time.After(3 * time.Second):
		t.Fatal("state root read remained blocked after caller cancellation")
	}
	require.Positive(t, pool.Stats().WaitCount, "read must have waited for the occupied connection")
}

func TestStateRootService_StateChangeDetection(t *testing.T) {
	db := newTestDB(t)
	svc := db.GetStateRootSvc()

	root1, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)

	require.NoError(t, db.GetDocStore().DocSet(t.Context(), "test", "doc1", mustDocJSON(t, map[string]interface{}{"key": "value"})))
	root2, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, root1, root2, "state root should change after document insertion")

	require.NoError(t, db.GetDocStore().DocDelete(t.Context(), "test", "doc1"))
	root3, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root1, root3, "deleting the only change must restore the previous root")
}

// TestStateRootService_IncrementalMatchesOracle applies a random sequence of
// inserts, updates, deletes and tier moves across every committed table and
// compares the incremental roots with an independent full rebuild throughout.
func TestStateRootService_IncrementalMatchesOracle(t *testing.T) {
	db := newTestDB(t)
	requireRootsMatchOracle(t, db)

	rng := rand.New(rand.NewSource(7))
	for step := 0; step < 300; step++ {
		k := rng.Intn(40)
		switch rng.Intn(8) {
		case 0, 1:
			require.NoError(t, db.GetDocStore().DocSet(t.Context(), "oracle", fmt.Sprintf("d%d", k), json.RawMessage(fmt.Sprintf(`{"v":%d}`, rng.Int()))))
		case 2:
			_ = db.GetDocStore().DocDelete(t.Context(), "oracle", fmt.Sprintf("d%d", k))
		case 3:
			require.NoError(t, db.GetKVStore().KVSet(t.Context(), fmt.Sprintf("k%d", k), fmt.Sprint(rng.Int()), 0))
		case 4:
			require.NoError(t, db.GetKVStore().KVSetObserved(t.Context(), fmt.Sprintf("k%d", k), fmt.Sprint(rng.Int()), 0))
		case 5:
			_, err := db.db.Exec("DELETE FROM kv_store WHERE key = ?", fmt.Sprintf("k%d", k))
			require.NoError(t, err)
		case 6:
			require.NoError(t, db.GetBlobStore().BlobPut("ns", fmt.Sprintf("b%d", k), []byte(fmt.Sprint(rng.Int())), "text/plain", 0))
		case 7:
			require.NoError(t, db.GetBlobStore().BlobPutObserved("ns", fmt.Sprintf("b%d", k), []byte(fmt.Sprint(rng.Int())), "text/plain", 0))
		}
		// Read roots at irregular intervals so flushes cover batches of changes.
		if rng.Intn(5) == 0 {
			requireRootsMatchOracle(t, db)
		}
	}
	requireRootsMatchOracle(t, db)
}

func TestStateRootService_TierMoveUpdatesBothRoots(t *testing.T) {
	db := newTestDB(t)
	svc := db.GetStateRootSvc()
	require.NoError(t, db.GetKVStore().KVSet(t.Context(), "move:me", "v", 0))
	bound1, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	observed1, err := svc.GetObservedStateRoot(t.Context())
	require.NoError(t, err)

	require.NoError(t, db.GetKVStore().KVSetObserved(t.Context(), "move:me", "v", 0))
	bound2, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	observed2, err := svc.GetObservedStateRoot(t.Context())
	require.NoError(t, err)

	assert.NotEqual(t, bound1, bound2, "the leaf must leave the bound tree")
	assert.NotEqual(t, observed1, observed2, "the leaf must enter the observed tree")
	requireRootsMatchOracle(t, db)
}

// TestStateRootService_RolledBackWriteChangesNothing verifies that the dirty
// marker shares the writer's transaction.
func TestStateRootService_RolledBackWriteChangesNothing(t *testing.T) {
	db := newTestDB(t)
	svc := db.GetStateRootSvc()
	root1, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	flushed := svc.flushedLeaves.Load()

	rollback := errors.New("rollback")
	err = db.db.ExecInTxWithRetry(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO documents (collection, id, data, created_at, updated_at) VALUES ('t', 'x', '{}', 'now', 'now')"); err != nil {
			return err
		}
		return rollback
	})
	require.ErrorIs(t, err, rollback)

	root2, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root1, root2)
	assert.Equal(t, flushed, svc.flushedLeaves.Load(), "a rolled-back write must leave nothing to flush")
}

func TestStateRootService_MetadataOnlyUpdateIsNotDirty(t *testing.T) {
	db := newTestDB(t)
	svc := db.GetStateRootSvc()
	require.NoError(t, db.GetDocStore().DocSet(t.Context(), "t", "x", json.RawMessage(`{"a":1}`)))
	_, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)

	_, err = db.db.Exec("UPDATE documents SET updated_at = 'later' WHERE collection = 't' AND id = 'x'")
	require.NoError(t, err)
	var dirty int
	require.NoError(t, db.db.QueryRow("SELECT COUNT(*) FROM state_commitment_dirty").Scan(&dirty))
	assert.Zero(t, dirty)
}

// TestStateRootService_OneWriteWorkIsIndependentOfHistory is the bounded-work
// gate: with 10,000 committed history rows, the root read after one write
// rehashes exactly one leaf.
func TestStateRootService_OneWriteWorkIsIndependentOfHistory(t *testing.T) {
	db := newTestDB(t)
	seedHistoryDocuments(t, db, 10000)
	svc := db.GetStateRootSvc()
	_, err := svc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		before := svc.flushedLeaves.Load()
		require.NoError(t, db.GetDocStore().DocSet(t.Context(), "operators", "op-1", json.RawMessage(fmt.Sprintf(`{"n":%d}`, i))))
		_, err := svc.GetCurrentStateRoot(t.Context())
		require.NoError(t, err)
		assert.EqualValues(t, 1, svc.flushedLeaves.Load()-before)
	}
	requireRootsMatchOracle(t, db)
}

// TestStateRootService_LegacyDatabaseIsRebuiltOnOpen opens a database carrying
// the algorithm-1 state_version triggers and no commitment, and verifies the
// legacy objects are dropped and the commitment is rebuilt exactly once.
func TestStateRootService_LegacyDatabaseIsRebuiltOnOpen(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	logger := testutil.NewTestLogger()
	ks := newTestKeystore(t, fileSvc, logger)

	db, err := OpenCanonicalDBService(logger, "", ks, fileSvc)
	require.NoError(t, err)
	require.NoError(t, db.GetDocStore().DocSet(t.Context(), "legacy", "d1", json.RawMessage(`{"v":1}`)))
	require.NoError(t, db.GetKVStore().KVSet(t.Context(), "legacy:k", "v", 0))
	want, err := db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	for _, stmt := range []string{
		"DELETE FROM state_commitment",
		"DELETE FROM state_leaves",
		"DELETE FROM state_nodes",
		"CREATE TABLE state_version (id INTEGER PRIMARY KEY CHECK (id = 1), version INTEGER NOT NULL DEFAULT 0)",
		"INSERT INTO state_version (id, version) VALUES (1, 0)",
		"CREATE TABLE state_root (id INTEGER PRIMARY KEY CHECK (id = 1), root TEXT NOT NULL, updated_at TEXT NOT NULL)",
		"CREATE TRIGGER trg_documents_insert_version AFTER INSERT ON documents BEGIN UPDATE state_version SET version = version + 1 WHERE id = 1; END",
	} {
		_, err := db.db.Exec(stmt)
		require.NoError(t, err, stmt)
	}
	db.Close()

	reopened, err := OpenCanonicalDBService(logger, "", ks, fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { reopened.Close() })

	algorithm, err := reopened.GetStateRootSvc().CommitmentAlgorithm()
	require.NoError(t, err)
	assert.Equal(t, stateCommitmentAlgorithm, algorithm)
	got, err := reopened.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, want, got, "the rebuild must reproduce the incremental root")

	var legacy int
	require.NoError(t, reopened.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE name IN ('state_version', 'state_root', 'trg_documents_insert_version')").Scan(&legacy))
	assert.Zero(t, legacy, "legacy state_version objects must be dropped")
	require.NoError(t, reopened.GetDocStore().DocSet(t.Context(), "legacy", "d2", json.RawMessage(`{"v":2}`)))
	requireRootsMatchOracle(t, reopened)
}

func TestStateRootService_MissingCommitmentTableFailsClosed(t *testing.T) {
	db := newTestDB(t)
	_, err := db.db.Exec("DROP TABLE state_nodes")
	require.NoError(t, err)

	_, err = db.GetStateRootSvc().GetCurrentStateRoot(t.Context())
	require.ErrorIs(t, err, constants.ErrStateRootCalculate)
}

// TestStateRootService_LegacyDocumentCacheIsRemovedOnOpen opens a database that
// still carries the document cache invalidation triggers and cache rows, and
// verifies both are removed and document writes no longer touch kv_store.
func TestStateRootService_LegacyDocumentCacheIsRemovedOnOpen(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	logger := testutil.NewTestLogger()
	ks := newTestKeystore(t, fileSvc, logger)

	db, err := OpenCanonicalDBService(logger, "", ks, fileSvc)
	require.NoError(t, err)
	for _, stmt := range []string{
		"CREATE TRIGGER trg_documents_insert_kv AFTER INSERT ON documents BEGIN DELETE FROM kv_store WHERE key GLOB 'g8e:cache:query:' || NEW.collection || ':*'; END",
		"INSERT INTO kv_store (key, value, created_at) VALUES ('g8e:cache:doc:test:doc1', 'cached', '2026-01-01T00:00:00Z')",
		"INSERT INTO kv_store (key, value, created_at) VALUES ('g8e:cache:query:test:q1', '[]', '2026-01-01T00:00:00Z')",
	} {
		_, err := db.db.Exec(stmt)
		require.NoError(t, err, stmt)
	}
	require.NoError(t, db.GetKVStore().KVSet(t.Context(), "authoritative:key", "value", 0))
	db.Close()

	reopened, err := OpenCanonicalDBService(logger, "", ks, fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { reopened.Close() })

	var legacy int
	require.NoError(t, reopened.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE name IN ('trg_documents_insert_kv', 'trg_documents_update_kv', 'trg_documents_delete_kv')").Scan(&legacy))
	assert.Zero(t, legacy, "cache invalidation triggers must be dropped")
	var keys []string
	rows, err := reopened.db.Query("SELECT key FROM kv_store ORDER BY key")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var k string
		require.NoError(t, rows.Scan(&k))
		keys = append(keys, k)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"authoritative:key"}, keys, "only committed KV rows survive")

	require.NoError(t, reopened.GetDocStore().DocSet(t.Context(), "test", "doc1", mustDocJSON(t, map[string]interface{}{"key": "value"})))
	var kvCount int
	require.NoError(t, reopened.db.QueryRow("SELECT COUNT(*) FROM kv_store").Scan(&kvCount))
	assert.Equal(t, 1, kvCount, "a document write must not touch kv_store")
	requireRootsMatchOracle(t, reopened)
}
