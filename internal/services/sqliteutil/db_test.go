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

package sqliteutil

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestDefaultDBConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultDBConfig("/some/path/db.sqlite")
	assert.Equal(t, "/some/path/db.sqlite", cfg.Path)
	assert.Equal(t, 64, cfg.CacheSizeMB)
	assert.Equal(t, 30000, cfg.BusyTimeoutMs)
	assert.True(t, cfg.SetFilePermissions)
	assert.Equal(t, 10, cfg.MaxRetries)
	assert.Equal(t, 50, cfg.RetryBaseDelayMs)
}

func TestOpenDB_CreatesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "test.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	var result int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&result))
	assert.Equal(t, 1, result)
}

func TestOpenDB_CreatesParentDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "nested", "deep", "test.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	var result int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&result))
	assert.Equal(t, 1, result)
}

func TestOpenDB_WALModeEnabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "wal.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	var journalMode string
	require.NoError(t, db.QueryRow("PRAGMA journal_mode").Scan(&journalMode))
	assert.Equal(t, "wal", journalMode)
}

func TestOpenDB_ForeignKeysEnabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "fk.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	var fkEnabled int
	require.NoError(t, db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled))
	assert.Equal(t, 1, fkEnabled)
}

func TestOpenDB_SingleConnectionPool(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "pool.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	stats := db.Stats()
	assert.Equal(t, 20, stats.MaxOpenConnections)
}

func TestOpenDB_SetFilePermissions_False(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "noperm.db"))
	cfg.SetFilePermissions = false

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	var result int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&result))
	assert.Equal(t, 1, result)
}

func TestRunIncrementalVacuum(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "vacuum.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	err = db.RunIncrementalVacuum(100)
	require.NoError(t, err)
}

func TestRunIncrementalVacuum_ZeroPages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "vacuum0.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	err = db.RunIncrementalVacuum(0)
	require.NoError(t, err)
}

func TestRunIncrementalVacuum_WrapsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "wraperrdb.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	db.Close()

	err = db.RunIncrementalVacuum(100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incremental vacuum")
}

func TestGetPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	path := filepath.Join(dir, "getpath.db")
	cfg := DefaultDBConfig(path)

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	assert.Equal(t, path, db.GetPath())
}

func TestGetDBSizeBytes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "size.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	size, err := db.GetSizeBytes()
	require.NoError(t, err)
	assert.Greater(t, size, int64(0))
}

func TestGetDBSizeBytes_GrowsWithData(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "grow.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	sizeBefore, err := db.GetSizeBytes()
	require.NoError(t, err)

	_, err = db.Exec(`CREATE TABLE t (data TEXT)`)
	require.NoError(t, err)
	for i := 0; i < 500; i++ {
		_, insertErr := db.Exec(`INSERT INTO t VALUES (?)`, "x-data-payload-to-grow-the-db")
		require.NoError(t, insertErr)
	}

	sizeAfter, err := db.GetSizeBytes()
	require.NoError(t, err)

	assert.GreaterOrEqual(t, sizeAfter, sizeBefore)
}

func TestGetDBSizeBytes_ReturnsErrorOnClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "closederr.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	db.Close()

	_, err = db.GetSizeBytes()
	require.Error(t, err)
}

func TestHealthCheck_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "health.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	err = db.HealthCheck(ctx)
	require.NoError(t, err)
}

func TestHealthCheck_ClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "healthclosed.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	db.Close()

	ctx := context.Background()
	err = db.HealthCheck(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health check")
}

func TestHealthCheck_CancelledContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "healthcancel.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = db.HealthCheck(ctx)
	require.Error(t, err)
}

func TestExecWithRetry_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "exec.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	result, err := db.ExecWithRetry("CREATE TABLE test (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestExecWithRetry_InvalidSQL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "execinvalid.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.ExecWithRetry("INVALID SQL STATEMENT")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "retry", "non-busy errors should not retry")
}

func TestExecWithRetry_WithParameters(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "execparams.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.ExecWithRetry("CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	result, err := db.ExecWithRetry("INSERT INTO test (value) VALUES (?)", "test-value")
	require.NoError(t, err)

	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestQueryWithRetry_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "query.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO test (value) VALUES ('a'), ('b'), ('c')")
	require.NoError(t, err)

	rows, err := db.QueryWithRetry("SELECT value FROM test ORDER BY value")
	require.NoError(t, err)
	t.Cleanup(func() { rows.Close() })

	var values []string
	for rows.Next() {
		var v string
		require.NoError(t, rows.Scan(&v))
		values = append(values, v)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []string{"a", "b", "c"}, values)
}

func TestQueryWithRetry_InvalidSQL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "queryinvalid.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.QueryWithRetry("INVALID QUERY")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "retry", "non-busy errors should not retry")
}

func TestQueryRowWithRetry_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "queryrow.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO test (value) VALUES ('single-row')")
	require.NoError(t, err)

	row := db.QueryRowWithRetry("SELECT value FROM test WHERE id = 1")
	var value string
	err = row.Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "single-row", value)
}

func TestQueryRowWithRetry_NoRows(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "queryrowempty.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)

	row := db.QueryRowWithRetry("SELECT id FROM test WHERE id = 999")
	var id int
	err = row.Scan(&id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no rows")
}

func TestQueryRowWithRetry_InvalidSQL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "queryrowinvalid.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	row := db.QueryRowWithRetry("INVALID QUERY")
	err = row.Err()
	require.Error(t, err)
}

func TestExecInTxWithRetry_Commit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "txcommit.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	err = db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (value) VALUES (?)", "committed")
		return err
	})
	require.NoError(t, err)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test WHERE value = 'committed'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestExecInTxWithRetry_RollbackOnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "txrollback.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	err = db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (value) VALUES (?)", "rolled-back")
		if err != nil {
			return err
		}
		return assert.AnError
	})
	require.Error(t, err)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test WHERE value = 'rolled-back'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "transaction should have been rolled back")
}

func TestExecInTxWithRetry_MultipleStatements(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "txmulti.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	err = db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		for i := 0; i < 5; i++ {
			_, err := tx.Exec("INSERT INTO test (value) VALUES (?)", i)
			if err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
}

func TestExecInTxWithRetry_InvalidSQL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "txinvalid.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	err = db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		_, err := tx.Exec("INVALID SQL")
		return err
	})
	require.Error(t, err)
}

func TestMaterializeRows_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "materialize.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO test (value) VALUES ('a'), ('b'), ('c')")
	require.NoError(t, err)

	results, err := MaterializeRows(db, "SELECT value FROM test ORDER BY value", nil, func(rows *sql.Rows) (string, error) {
		var v string
		err := rows.Scan(&v)
		return v, err
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, results)
}

func TestMaterializeRows_EmptyResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "materializeempty.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)

	results, err := MaterializeRows(db, "SELECT id FROM test", nil, func(rows *sql.Rows) (int, error) {
		var id int
		err := rows.Scan(&id)
		return id, err
	})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestMaterializeRows_WithParameters(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "materializeparams.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO test (value) VALUES ('a'), ('b'), ('c')")
	require.NoError(t, err)

	results, err := MaterializeRows(db, "SELECT value FROM test WHERE value > ? ORDER BY value", []interface{}{"a"}, func(rows *sql.Rows) (string, error) {
		var v string
		err := rows.Scan(&v)
		return v, err
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "c"}, results)
}

func TestMaterializeRows_ScanError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "materializescan.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO test (value) VALUES ('a')")
	require.NoError(t, err)

	_, err = MaterializeRows(db, "SELECT value FROM test", nil, func(rows *sql.Rows) (int, error) {
		var v string
		err := rows.Scan(&v)
		if err != nil {
			return 0, err
		}
		return 0, assert.AnError
	})
	require.Error(t, err)
}

func TestMaterializeRows_QueryError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "materializequery.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = MaterializeRows(db, "INVALID QUERY", nil, func(rows *sql.Rows) (int, error) {
		return 0, nil
	})
	require.Error(t, err)
}

func TestContains_EmptyString(t *testing.T) {
	t.Parallel()
	assert.False(t, contains("", "test"))
}

func TestContains_EmptySubstring(t *testing.T) {
	t.Parallel()
	assert.True(t, contains("test", ""))
}

func TestContains_ExactMatch(t *testing.T) {
	t.Parallel()
	assert.True(t, contains("test", "test"))
}

func TestContains_SubstringPresent(t *testing.T) {
	t.Parallel()
	assert.True(t, contains("hello world", "world"))
}

func TestContains_SubstringAbsent(t *testing.T) {
	t.Parallel()
	assert.False(t, contains("hello world", "xyz"))
}

func TestContains_SubstringLongerThanString(t *testing.T) {
	t.Parallel()
	assert.False(t, contains("hi", "hello"))
}

func TestFindSubstring_Found(t *testing.T) {
	t.Parallel()
	assert.True(t, findSubstring("hello world", "world"))
}

func TestFindSubstring_NotFound(t *testing.T) {
	t.Parallel()
	assert.False(t, findSubstring("hello world", "xyz"))
}

func TestFindSubstring_EmptySubstring(t *testing.T) {
	t.Parallel()
	assert.True(t, findSubstring("test", ""))
}

func TestFindSubstring_AtStart(t *testing.T) {
	t.Parallel()
	assert.True(t, findSubstring("hello world", "hello"))
}

func TestFindSubstring_AtEnd(t *testing.T) {
	t.Parallel()
	assert.True(t, findSubstring("hello world", "world"))
}

func TestFindSubstring_SingleChar(t *testing.T) {
	t.Parallel()
	assert.True(t, findSubstring("abc", "b"))
}
