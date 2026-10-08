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
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"testing"

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

// oracleStateRoots recomputes both roots from a full scan of every committed
// row, building the whole tree in memory. It shares only the leaf/node encoding
// with the service, not its incremental path.
func oracleStateRoots(t *testing.T, db *CanonicalDBService) (bound, observed string) {
	t.Helper()
	leaves := map[string]map[int][][2][]byte{stateTierBound: {}, stateTierObserved: {}}
	add := func(tier string, key stateDirtyKey, fields ...[]byte) {
		id := stateLeafID(key.source, key.k1, key.k2)
		b := stateBucketOf(id)
		leaves[tier][b] = append(leaves[tier][b], [2][]byte{id, stateLeafDigest(key, fields...)})
	}

	rows, err := db.db.Query("SELECT collection, id, data FROM documents")
	require.NoError(t, err)
	for rows.Next() {
		var c, id, data string
		require.NoError(t, rows.Scan(&c, &id, &data))
		add(stateTierBound, stateDirtyKey{stateSourceDocuments, c, id}, []byte(data))
	}
	require.NoError(t, rows.Err())
	rows.Close()

	rows, err = db.db.Query("SELECT key, value, COALESCE(expires_at, ''), state_tier FROM kv_store WHERE key NOT LIKE 'g8e:cache:%'")
	require.NoError(t, err)
	for rows.Next() {
		var k, v, exp, tier string
		require.NoError(t, rows.Scan(&k, &v, &exp, &tier))
		add(tier, stateDirtyKey{stateSourceKV, k, ""}, []byte(v), []byte(exp))
	}
	require.NoError(t, rows.Err())
	rows.Close()

	rows, err = db.db.Query("SELECT namespace, id, size, content_type, data, COALESCE(expires_at, ''), state_tier FROM blobs")
	require.NoError(t, err)
	for rows.Next() {
		var ns, id, ct, exp, tier string
		var size int64
		var data []byte
		require.NoError(t, rows.Scan(&ns, &id, &size, &ct, &data, &exp, &tier))
		add(tier, stateDirtyKey{stateSourceBlobs, ns, id}, binary.BigEndian.AppendUint64(nil, uint64(size)), []byte(ct), data, []byte(exp))
	}
	require.NoError(t, rows.Err())
	rows.Close()

	treeRoot := func(tier string) []byte {
		width := 1
		for i := 0; i < stateTreeDepth; i++ {
			width *= stateTreeFanout
		}
		level := make([][]byte, width)
		for b := range level {
			bucket := leaves[tier][b]
			sortLeaves(bucket)
			level[b] = stateBucketDigest(bucket)
		}
		for l := stateTreeDepth - 1; l >= 0; l-- {
			next := make([][]byte, len(level)/stateTreeFanout)
			for i := range next {
				next[i] = stateNodeDigest(l, level[i*stateTreeFanout:(i+1)*stateTreeFanout])
			}
			level = next
		}
		return level[0]
	}
	return stateRootDigest(stateTierBound, treeRoot(stateTierBound), ""),
		stateRootDigest(stateTierObserved, treeRoot(stateTierObserved), "")
}

func sortLeaves(leaves [][2][]byte) {
	slices.SortFunc(leaves, func(a, b [2][]byte) int { return bytes.Compare(a[0], b[0]) })
}

func requireRootsMatchOracle(t *testing.T, db *CanonicalDBService) {
	t.Helper()
	wantBound, wantObserved := oracleStateRoots(t, db)
	bound, err := db.GetStateRootSvc().GetCurrentStateRoot()
	require.NoError(t, err)
	observed, err := db.GetStateRootSvc().GetObservedStateRoot()
	require.NoError(t, err)
	require.Equal(t, wantBound, bound, "incremental bound root must equal the full-rebuild oracle")
	require.Equal(t, wantObserved, observed, "incremental observed root must equal the full-rebuild oracle")
}

func TestStateRootService_GetCurrentStateRoot(t *testing.T) {
	svc := newStateRootService(t)

	root1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Len(t, root1, 64)

	root2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2)
}

func TestStateRootService_StateChangeDetection(t *testing.T) {
	db := newTestDB(t)
	svc := db.GetStateRootSvc()

	root1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)

	require.NoError(t, db.GetDocStore().DocSet("test", "doc1", mustDocJSON(t, map[string]interface{}{"key": "value"})))
	root2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root1, root2, "state root should change after document insertion")

	require.NoError(t, db.GetDocStore().DocDelete("test", "doc1"))
	root3, err := svc.GetCurrentStateRoot()
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
			require.NoError(t, db.GetDocStore().DocSet("oracle", fmt.Sprintf("d%d", k), json.RawMessage(fmt.Sprintf(`{"v":%d}`, rng.Int()))))
		case 2:
			_ = db.GetDocStore().DocDelete("oracle", fmt.Sprintf("d%d", k))
		case 3:
			require.NoError(t, db.GetKVStore().KVSet(fmt.Sprintf("k%d", k), fmt.Sprint(rng.Int()), 0))
		case 4:
			require.NoError(t, db.GetKVStore().KVSetObserved(fmt.Sprintf("k%d", k), fmt.Sprint(rng.Int()), 0))
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
	require.NoError(t, db.GetKVStore().KVSet("move:me", "v", 0))
	bound1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	observed1, err := svc.GetObservedStateRoot()
	require.NoError(t, err)

	require.NoError(t, db.GetKVStore().KVSetObserved("move:me", "v", 0))
	bound2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	observed2, err := svc.GetObservedStateRoot()
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
	root1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	flushed := svc.flushedLeaves.Load()

	rollback := errors.New("rollback")
	err = db.db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO documents (collection, id, data, created_at, updated_at) VALUES ('t', 'x', '{}', 'now', 'now')"); err != nil {
			return err
		}
		return rollback
	})
	require.ErrorIs(t, err, rollback)

	root2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2)
	assert.Equal(t, flushed, svc.flushedLeaves.Load(), "a rolled-back write must leave nothing to flush")
}

func TestStateRootService_MetadataOnlyUpdateIsNotDirty(t *testing.T) {
	db := newTestDB(t)
	svc := db.GetStateRootSvc()
	require.NoError(t, db.GetDocStore().DocSet("t", "x", json.RawMessage(`{"a":1}`)))
	_, err := svc.GetCurrentStateRoot()
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
	_, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		before := svc.flushedLeaves.Load()
		require.NoError(t, db.GetDocStore().DocSet("operators", "op-1", json.RawMessage(fmt.Sprintf(`{"n":%d}`, i))))
		_, err := svc.GetCurrentStateRoot()
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
	require.NoError(t, db.GetDocStore().DocSet("legacy", "d1", json.RawMessage(`{"v":1}`)))
	require.NoError(t, db.GetKVStore().KVSet("legacy:k", "v", 0))
	want, err := db.GetStateRootSvc().GetCurrentStateRoot()
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
	got, err := reopened.GetStateRootSvc().GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, want, got, "the rebuild must reproduce the incremental root")

	var legacy int
	require.NoError(t, reopened.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE name IN ('state_version', 'state_root', 'trg_documents_insert_version')").Scan(&legacy))
	assert.Zero(t, legacy, "legacy state_version objects must be dropped")
	require.NoError(t, reopened.GetDocStore().DocSet("legacy", "d2", json.RawMessage(`{"v":2}`)))
	requireRootsMatchOracle(t, reopened)
}

func TestStateRootService_MissingCommitmentTableFailsClosed(t *testing.T) {
	db := newTestDB(t)
	_, err := db.db.Exec("DROP TABLE state_nodes")
	require.NoError(t, err)

	_, err = db.GetStateRootSvc().GetCurrentStateRoot()
	require.ErrorIs(t, err, constants.ErrStateRootCalculate)
}

// TestStateRootService_NoCacheLeakOnDocumentWrite verifies that cache keys never
// contribute to the authoritative root.
func TestStateRootService_NoCacheLeakOnDocumentWrite(t *testing.T) {
	db := newTestDB(t)
	svc := db.GetStateRootSvc()

	root1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root1)

	require.NoError(t, db.GetKVStore().KVSet("g8e:cache:doc:test:doc1", "cached_value", 3600))
	root2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "state root should not change when cache keys are added")

	require.NoError(t, db.GetKVStore().KVSet("g8e:cache:query:SELECT * FROM test", "query_result", 3600))
	root3, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root2, root3, "state root should not change when more cache keys are added")

	require.NoError(t, db.GetKVStore().KVSet("authoritative:key", "value", 0))
	root4, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root3, root4, "state root should change when authoritative KV entries are added")

	require.NoError(t, db.GetDocStore().DocSet("test", "doc1", mustDocJSON(t, map[string]interface{}{"key": "value"})))
	root5, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root4, root5, "state root should change after document insertion")

	require.NoError(t, db.GetDocStore().DocSet("test", "doc2", mustDocJSON(t, map[string]interface{}{"key2": "value2"})))
	root6, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root5, root6, "state root should change after second document insertion")
	requireRootsMatchOracle(t, db)
}
