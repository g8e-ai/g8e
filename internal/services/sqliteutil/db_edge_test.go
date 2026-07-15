package sqliteutil

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestDB_Backoff_DoesNotPanic(t *testing.T) {
	t.Parallel()
	dir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "test.db"))
	cfg.RetryBaseDelayMs = 1

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	start := time.Now()
	db.backoff(0)
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 100*time.Millisecond, "backoff(0) with 1ms base should be fast")
}

func TestDB_Backoff_HighAttempt(t *testing.T) {
	t.Parallel()
	dir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "test.db"))
	cfg.RetryBaseDelayMs = 1

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	start := time.Now()
	db.backoff(3)
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 500*time.Millisecond, "backoff(3) with 1ms base should complete quickly")
}

func TestExecInTxWithRetry_Success(t *testing.T) {
	t.Parallel()
	dir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "test.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test_tx (id INTEGER PRIMARY KEY, val TEXT)")
	require.NoError(t, err)

	err = db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test_tx (val) VALUES (?)", "hello")
		return err
	})
	require.NoError(t, err)

	var val string
	err = db.QueryRow("SELECT val FROM test_tx WHERE id = 1").Scan(&val)
	require.NoError(t, err)
	assert.Equal(t, "hello", val)
}

func TestExecInTxWithRetry_FnErrorRollsBack(t *testing.T) {
	t.Parallel()
	dir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "test.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("CREATE TABLE test_tx2 (id INTEGER PRIMARY KEY, val TEXT)")
	require.NoError(t, err)

	err = db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		_, _ = tx.Exec("INSERT INTO test_tx2 (val) VALUES (?)", "will-rollback")
		return fmt.Errorf("intentional error")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intentional error")

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_tx2").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "transaction should have been rolled back")
}

func TestExecInTxWithRetry_ClosedDB(t *testing.T) {
	t.Parallel()
	dir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "test.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	db.Close()

	err = db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		return nil
	})
	require.Error(t, err)
}
