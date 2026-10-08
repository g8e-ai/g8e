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
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

const (
	stateRootCrashDBEnv    = "G8E_STATE_ROOT_CRASH_DB"
	stateRootCrashPointEnv = "G8E_STATE_ROOT_CRASH_POINT"
	stateRootCrashMarker   = "state-root-crash-child: killing"

	stateRootCrashAfterWrite = "after-write"
	stateRootCrashMidFlush   = "mid-flush"
)

// TestStateRootService_CrashRecovery kills a separate process holding the
// canonical database, then reopens it and compares the roots with the oracle.
//   - after-write: the process commits writes (and their dirty marks) and is
//     killed before any root read flushes them.
//   - mid-flush: the process is killed inside the flush transaction, after the
//     leaves, buckets and lower tree levels were rewritten and the dirty set
//     cleared, but before COMMIT.
//
// In both cases the reopened database must still hold the pre-crash tree and
// the full dirty set, and the next root read must equal the oracle.
func TestStateRootService_CrashRecovery(t *testing.T) {
	for _, point := range []string{stateRootCrashAfterWrite, stateRootCrashMidFlush} {
		t.Run(point, func(t *testing.T) {
			fileSvc := newTestFileSvc(t)
			logger := testutil.NewTestLogger()
			ks := newTestKeystore(t, fileSvc, logger)

			db, err := OpenCanonicalDBService(logger, "", ks, fileSvc)
			require.NoError(t, err)
			require.NoError(t, db.GetDocStore().DocSet("crash", "a", mustDocJSON(t, map[string]int{"v": 1})))
			require.NoError(t, db.GetDocStore().DocSet("crash", "b", mustDocJSON(t, map[string]int{"v": 1})))
			require.NoError(t, db.GetKVStore().KVSet("crash:k", "v", 0))
			_, err = db.GetStateRootSvc().GetCurrentStateRoot()
			require.NoError(t, err)
			treeBefore := committedBoundTreeRoot(t, db.db)
			db.Close()

			runStateRootCrashChild(t, fileSvc.Resolve(constants.CanonicalDBRelPath), point)

			reopened, err := OpenCanonicalDBService(logger, "", ks, fileSvc)
			require.NoError(t, err)
			t.Cleanup(func() { reopened.Close() })

			assert.Equal(t, treeBefore, committedBoundTreeRoot(t, reopened.db),
				"the crashed process must not leave a partly updated tree")
			var dirty int
			require.NoError(t, reopened.db.QueryRow("SELECT COUNT(*) FROM state_commitment_dirty").Scan(&dirty))
			assert.Equal(t, 4, dirty, "every committed write must keep its dirty mark across the crash")

			requireRootsMatchOracle(t, reopened)
			require.NoError(t, reopened.db.QueryRow("SELECT COUNT(*) FROM state_commitment_dirty").Scan(&dirty))
			assert.Zero(t, dirty)
		})
	}
}

// TestStateRootService_CrashChild is the process killed by
// TestStateRootService_CrashRecovery. It opens the database file directly, as a
// second runtime would, and is skipped in normal runs.
func TestStateRootService_CrashChild(t *testing.T) {
	dbPath := os.Getenv(stateRootCrashDBEnv)
	if dbPath == "" {
		t.Skip("helper process for TestStateRootService_CrashRecovery")
	}
	logger := testutil.NewTestLogger()
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), logger)
	require.NoError(t, err)

	require.NoError(t, db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`INSERT INTO documents (collection, id, data, created_at, updated_at) VALUES ('crash', 'c', '{"v":1}', 'now', 'now')`,
			`UPDATE documents SET data = '{"v":2}' WHERE collection = 'crash' AND id = 'a'`,
			`DELETE FROM documents WHERE collection = 'crash' AND id = 'b'`,
			`UPDATE kv_store SET value = 'changed' WHERE key = 'crash:k'`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	}))

	if os.Getenv(stateRootCrashPointEnv) == stateRootCrashMidFlush {
		ctx := context.Background()
		conn, err := db.Conn(ctx)
		require.NoError(t, err)
		_, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE")
		require.NoError(t, err)
		require.NoError(t, NewStateRootService(db, logger).flush(conn))
	}

	fmt.Println(stateRootCrashMarker)
	self, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, self.Kill())
	time.Sleep(time.Minute)
	t.Fatal("crash child was not killed")
}

func runStateRootCrashChild(t *testing.T, dbPath, point string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	// #nosec G204 -- re-executes this test binary with a fixed test selector.
	child := exec.Command(executable, "-test.run=^TestStateRootService_CrashChild$", "-test.count=1")
	child.Env = append(os.Environ(), stateRootCrashDBEnv+"="+dbPath, stateRootCrashPointEnv+"="+point)
	out, err := child.CombinedOutput()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "crash child must not exit cleanly: %s", out)
	require.Contains(t, string(out), stateRootCrashMarker, "crash child must reach the crash point: %s", out)
}

func committedBoundTreeRoot(t *testing.T, db *sqliteutil.DB) []byte {
	t.Helper()
	var digest []byte
	err := db.QueryRow("SELECT digest FROM state_nodes WHERE tier = ? AND level = 0 AND idx = 0", stateTierBound).Scan(&digest)
	require.NoError(t, err)
	return digest
}

// TestStateRootService_ConcurrentWritersAndReaders runs writers and root readers
// together. Each write records itself in a commit log inside its own
// transaction, so the log order is the commit order. Every root a reader
// observes must be the oracle root of some committed prefix, each reader's
// sequence must never move back to an earlier prefix, and the final root must
// equal the oracle.
func TestStateRootService_ConcurrentWritersAndReaders(t *testing.T) {
	const (
		writers         = 4
		readers         = 4
		writesPerWriter = 25
		keys            = 12
	)
	db := newTestDB(t)
	svc := db.GetStateRootSvc()
	_, err := db.db.Exec(`CREATE TABLE test_commit_log (
		seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL, data TEXT)`)
	require.NoError(t, err)
	_, err = svc.GetCurrentStateRoot()
	require.NoError(t, err)
	initial := oracleLeafSet(t, db)

	var writersWG, readersWG sync.WaitGroup
	writeErrs := make(chan error, writers)
	for w := 0; w < writers; w++ {
		writersWG.Add(1)
		go func(w int) {
			defer writersWG.Done()
			rng := rand.New(rand.NewSource(int64(w + 1)))
			for i := 0; i < writesPerWriter; i++ {
				id := fmt.Sprintf("d%d", rng.Intn(keys))
				var data sql.NullString
				if rng.Intn(4) != 0 {
					data = sql.NullString{String: fmt.Sprintf(`{"w":%d,"i":%d}`, w, i), Valid: true}
				}
				err := db.db.ExecInTxWithRetry(func(tx *sql.Tx) error {
					var err error
					if data.Valid {
						_, err = tx.Exec(`INSERT INTO documents (collection, id, data, created_at, updated_at)
							VALUES ('concurrent', ?, ?, 'now', 'now')
							ON CONFLICT(collection, id) DO UPDATE SET data = excluded.data`, id, data.String)
					} else {
						_, err = tx.Exec("DELETE FROM documents WHERE collection = 'concurrent' AND id = ?", id)
					}
					if err != nil {
						return err
					}
					_, err = tx.Exec("INSERT INTO test_commit_log (id, data) VALUES (?, ?)", id, data)
					return err
				})
				if err != nil {
					writeErrs <- fmt.Errorf("writer %d write %d: %w", w, i, err)
					return
				}
			}
		}(w)
	}

	done := make(chan struct{})
	observed := make([][]string, readers)
	readErrs := make(chan error, readers)
	for r := 0; r < readers; r++ {
		readersWG.Add(1)
		go func(r int) {
			defer readersWG.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				root, err := svc.GetCurrentStateRoot()
				if err != nil {
					readErrs <- fmt.Errorf("reader %d: %w", r, err)
					return
				}
				observed[r] = append(observed[r], root)
			}
		}(r)
	}

	writersWG.Wait()
	close(done)
	readersWG.Wait()
	close(writeErrs)
	close(readErrs)
	require.NoError(t, errors.Join(drainErrs(writeErrs)...))
	require.NoError(t, errors.Join(drainErrs(readErrs)...))

	// Replay the commit log over the initial leaf set; prefixByRoot lists every
	// prefix index at which each root was the committed root.
	leaves := maps.Clone(initial)
	prefixByRoot := map[string][]int{oracleRoot(leaves, stateTierBound): {0}}
	rows, err := db.db.Query("SELECT id, data FROM test_commit_log ORDER BY seq")
	require.NoError(t, err)
	defer rows.Close()
	prefix := 0
	for rows.Next() {
		var id string
		var data sql.NullString
		require.NoError(t, rows.Scan(&id, &data))
		key := stateDirtyKey{stateSourceDocuments, "concurrent", id}
		leafID := stateLeafID(key.source, key.k1, key.k2)
		if data.Valid {
			leaves[string(leafID)] = oracleLeaf{tier: stateTierBound, id: leafID, digest: stateLeafDigest(key, []byte(data.String))}
		} else {
			delete(leaves, string(leafID))
		}
		prefix++
		root := oracleRoot(leaves, stateTierBound)
		prefixByRoot[root] = append(prefixByRoot[root], prefix)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, writers*writesPerWriter, prefix)

	reads := 0
	for r, roots := range observed {
		lowest := 0
		for i, root := range roots {
			next := -1
			for _, p := range prefixByRoot[root] {
				if p >= lowest {
					next = p
					break
				}
			}
			require.GreaterOrEqual(t, next, 0,
				"reader %d read %d: root is not the oracle root of any committed prefix at or after %d", r, i, lowest)
			lowest = next
		}
		reads += len(roots)
	}
	t.Logf("%d writes, %d root reads, %d distinct committed roots", prefix, reads, len(prefixByRoot))
	assert.Positive(t, reads)

	final, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Contains(t, prefixByRoot[final], prefix, "the final root must be the root of the full commit log")
	requireRootsMatchOracle(t, db)
}

func drainErrs(ch <-chan error) []error {
	var errs []error
	for err := range ch {
		errs = append(errs, err)
	}
	return errs
}
