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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/g8eo/testutil"
)

func TestDefaultDBConfig(t *testing.T) {
	cfg := DefaultDBConfig("/some/path/db.sqlite")
	assert.Equal(t, "/some/path/db.sqlite", cfg.Path)
	assert.Equal(t, 64, cfg.CacheSizeMB)
	assert.Equal(t, 5000, cfg.BusyTimeoutMs)
	assert.True(t, cfg.SetFilePermissions)
}

func TestOpenDB_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "test.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var result int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&result))
	assert.Equal(t, 1, result)
}

func TestOpenDB_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "nested", "deep", "test.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var result int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&result))
	assert.Equal(t, 1, result)
}

func TestOpenDB_WALModeEnabled(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "wal.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var journalMode string
	require.NoError(t, db.QueryRow("PRAGMA journal_mode").Scan(&journalMode))
	assert.Equal(t, "wal", journalMode)
}

func TestOpenDB_ForeignKeysEnabled(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "fk.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var fkEnabled int
	require.NoError(t, db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled))
	assert.Equal(t, 1, fkEnabled)
}

func TestOpenDB_SingleConnectionPool(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "pool.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	stats := db.Stats()
	assert.Equal(t, 1, stats.MaxOpenConnections)
}

func TestOpenDB_SetFilePermissions_False(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "noperm.db"))
	cfg.SetFilePermissions = false

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var result int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&result))
	assert.Equal(t, 1, result)
}

func TestRunIncrementalVacuum(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "vacuum.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	err = db.RunIncrementalVacuum(100)
	require.NoError(t, err)
}

func TestRunIncrementalVacuum_ZeroPages(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "vacuum0.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	err = db.RunIncrementalVacuum(0)
	require.NoError(t, err)
}

func TestRunIncrementalVacuum_WrapsError(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "wraperrdb.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	db.Close()

	err = db.RunIncrementalVacuum(100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incremental vacuum failed")
}

func TestGetPath(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	path := filepath.Join(dir, "getpath.db")
	cfg := DefaultDBConfig(path)

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	assert.Equal(t, path, db.GetPath())
}

func TestGetDBSizeBytes(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "size.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	size, err := db.GetSizeBytes()
	require.NoError(t, err)
	assert.Greater(t, size, int64(0))
}

func TestGetDBSizeBytes_GrowsWithData(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "grow.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

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
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "closederr.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	db.Close()

	_, err = db.GetSizeBytes()
	require.Error(t, err)
}
