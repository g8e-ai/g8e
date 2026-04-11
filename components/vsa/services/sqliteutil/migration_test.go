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

	"github.com/g8e-ai/g8e/components/vsa/testutil"
)

func TestRunMigrations_AppliesInOrder(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "migrations.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	migrations := []Migration{
		{Version: 1, Description: "create table a", SQL: `CREATE TABLE a (id INTEGER PRIMARY KEY)`},
		{Version: 2, Description: "create table b", SQL: `CREATE TABLE b (id INTEGER PRIMARY KEY)`},
	}

	err = db.RunMigrations(migrations)
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count))
	assert.Equal(t, 2, count)

	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='a'").Scan(&count))
	assert.Equal(t, 1, count)

	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='b'").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestRunMigrations_IdempotentOnRerun(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "idempotent.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	migrations := []Migration{
		{Version: 1, Description: "create table x", SQL: `CREATE TABLE x (id INTEGER PRIMARY KEY)`},
	}

	require.NoError(t, db.RunMigrations(migrations))
	require.NoError(t, db.RunMigrations(migrations))

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestRunMigrations_SkipsAlreadyApplied(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "skip.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	first := []Migration{
		{Version: 1, Description: "create table c", SQL: `CREATE TABLE c (id INTEGER PRIMARY KEY)`},
	}
	require.NoError(t, db.RunMigrations(first))

	second := []Migration{
		{Version: 1, Description: "create table c", SQL: `CREATE TABLE c (id INTEGER PRIMARY KEY)`},
		{Version: 2, Description: "create table d", SQL: `CREATE TABLE d (id INTEGER PRIMARY KEY)`},
	}
	require.NoError(t, db.RunMigrations(second))

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count))
	assert.Equal(t, 2, count)
}

func TestRunMigrations_EmptyListIsNoOp(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "empty.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	err = db.RunMigrations([]Migration{})
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count))
	assert.Equal(t, 0, count)
}

func TestRunMigrations_RecordsDescriptionAndTimestamp(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "meta.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	migrations := []Migration{
		{Version: 1, Description: "initial schema", SQL: `CREATE TABLE e (id INTEGER PRIMARY KEY)`},
	}
	require.NoError(t, db.RunMigrations(migrations))

	var desc, appliedAt string
	require.NoError(t, db.QueryRow("SELECT description, applied_at FROM schema_version WHERE version = 1").Scan(&desc, &appliedAt))
	assert.Equal(t, "initial schema", desc)
	assert.NotEmpty(t, appliedAt)
}

func TestRunMigrations_InvalidSQLReturnsError(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "invalid.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	migrations := []Migration{
		{Version: 1, Description: "bad sql", SQL: `THIS IS NOT VALID SQL`},
	}

	err = db.RunMigrations(migrations)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration 1")
}

func TestColumnExists_ReturnsTrueForExistingColumn(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "colexist.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE things (id INTEGER PRIMARY KEY, name TEXT)`)
	require.NoError(t, err)

	exists, err := db.ColumnExists("things", "name")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestColumnExists_ReturnsFalseForMissingColumn(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "colmissing.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE things (id INTEGER PRIMARY KEY)`)
	require.NoError(t, err)

	exists, err := db.ColumnExists("things", "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestColumnExists_ReturnsFalseForMissingTable(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "notable.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	exists, err := db.ColumnExists("no_such_table", "col")
	require.NoError(t, err)
	assert.False(t, exists)
}
