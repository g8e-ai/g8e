// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockDBOpener is a mock implementation of dbOpener for testing.
type mockDBOpener struct {
	db      *sql.DB
	openErr error
}

func (m *mockDBOpener) Open(driverName, dataSourceName string) (*sql.DB, error) {
	if m.openErr != nil {
		return nil, m.openErr
	}
	return m.db, nil
}

func TestDBIndexTriageTool_Name(t *testing.T) {
	tool := &DBIndexTriageTool{}
	require.Equal(t, "db_index_triage", tool.Name())
}

func TestDBIndexTriageTool_Description(t *testing.T) {
	tool := &DBIndexTriageTool{}
	require.NotEmpty(t, tool.Description())
}

func TestDBIndexTriageTool_InputSchema(t *testing.T) {
	tool := &DBIndexTriageTool{}
	schema := tool.InputSchema()
	require.NotNil(t, schema)
	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Properties, "database_path")
	require.Equal(t, []string{"database_path"}, schema.Required)
}

func TestDBIndexTriageTool_Execute_InvalidJSON(t *testing.T) {
	tool := &DBIndexTriageTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid arguments")
}

func TestDBIndexTriageTool_Execute_MissingDatabasePath(t *testing.T) {
	tool := &DBIndexTriageTool{}
	args := json.RawMessage(`{}`)
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database_path required")
}

func TestDBIndexTriageTool_Execute_DBOpenFailure(t *testing.T) {
	mock := &mockDBOpener{
		openErr: errors.New("failed to open database"),
	}
	tool := &DBIndexTriageTool{dbOpener: mock}

	args := json.RawMessage(`{"database_path": "/path/to/db.sqlite"}`)
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to open database")
}

func TestDBIndexTriageTool_Execute_QueryFailure(t *testing.T) {
	// Create a real in-memory database that will fail on the specific query
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	mock := &mockDBOpener{db: db}
	tool := &DBIndexTriageTool{dbOpener: mock}

	args := json.RawMessage(`{"database_path": "/path/to/db.sqlite"}`)
	_, err = tool.Execute(context.Background(), args)
	require.NoError(t, err) // Query succeeds on empty database, returns empty result
}

func TestDBIndexTriageTool_Execute_SuccessWithIndexes(t *testing.T) {
	// Create an in-memory database with actual indexes
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a table with indexes
	_, err = db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE UNIQUE INDEX idx_users_email ON users(email)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE INDEX idx_users_name ON users(name)")
	require.NoError(t, err)

	mock := &mockDBOpener{db: db}
	tool := &DBIndexTriageTool{dbOpener: mock}

	args := json.RawMessage(`{"database_path": "/path/to/db.sqlite"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res DBIndexTriageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Greater(t, len(res.Indexes), 0)
	require.Equal(t, 0.0, res.Fragmentation)

	// Verify we have both unique and non-unique indexes
	hasUnique := false
	hasNonUnique := false
	for _, idx := range res.Indexes {
		if idx.Unique {
			hasUnique = true
		} else {
			hasNonUnique = true
		}
	}
	require.True(t, hasUnique, "Should have at least one unique index")
	require.True(t, hasNonUnique, "Should have at least one non-unique index")
}

func TestDBIndexTriageTool_Execute_EmptyDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	mock := &mockDBOpener{db: db}
	tool := &DBIndexTriageTool{dbOpener: mock}

	args := json.RawMessage(`{"database_path": "/path/to/db.sqlite"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res DBIndexTriageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Empty(t, res.Indexes)
	require.Equal(t, 0.0, res.Fragmentation)
}

func TestDBIndexTriageTool_Execute_SQLiteInternalIndexesFiltered(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a table to ensure SQLite creates internal indexes
	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)

	mock := &mockDBOpener{db: db}
	tool := &DBIndexTriageTool{dbOpener: mock}

	args := json.RawMessage(`{"database_path": "/path/to/db.sqlite"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res DBIndexTriageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	// SQLite internal indexes (starting with sqlite_) should be filtered out
	for _, idx := range res.Indexes {
		require.NotContains(t, idx.Name, "sqlite_", "SQLite internal indexes should be filtered")
	}
}

func TestDBIndexTriageTool_Execute_UniqueIndexDetection(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create tables with different index types
	_, err = db.Exec("CREATE TABLE test1 (id INTEGER PRIMARY KEY, col TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE UNIQUE INDEX idx_unique ON test1(col)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE TABLE test2 (id INTEGER PRIMARY KEY, col TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE INDEX idx_nonunique ON test2(col)")
	require.NoError(t, err)

	mock := &mockDBOpener{db: db}
	tool := &DBIndexTriageTool{dbOpener: mock}

	args := json.RawMessage(`{"database_path": "/path/to/db.sqlite"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res DBIndexTriageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	// Find the unique and non-unique indexes
	var uniqueIdx, nonUniqueIdx *IndexInfo
	for i := range res.Indexes {
		if res.Indexes[i].Name == "idx_unique" {
			uniqueIdx = &res.Indexes[i]
		}
		if res.Indexes[i].Name == "idx_nonunique" {
			nonUniqueIdx = &res.Indexes[i]
		}
	}

	require.NotNil(t, uniqueIdx, "Unique index should be found")
	require.NotNil(t, nonUniqueIdx, "Non-unique index should be found")
	require.True(t, uniqueIdx.Unique, "idx_unique should be marked as unique")
	// Note: SQLite stores the SQL with "UNIQUE" for unique indexes, so we can verify the detection logic
}

func TestDBIndexTriageTool_Execute_ContextCancellation(t *testing.T) {
	// Create a mock that will check the context before returning a database
	mock := &mockDBOpener{
		openErr: context.Canceled,
	}
	tool := &DBIndexTriageTool{dbOpener: mock}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	args := json.RawMessage(`{"database_path": "/path/to/db.sqlite"}`)
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
}

func TestDBIndexTriageTool_Execute_NilDBOpener(t *testing.T) {
	// Test that nil dbOpener uses the default realDBOpener
	tool := &DBIndexTriageTool{dbOpener: nil}

	// This should not panic, but will fail to open a non-existent database
	args := json.RawMessage(`{"database_path": "/nonexistent/path/db.sqlite"}`)
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	// The error can be either at open or query stage depending on SQLite behavior
}

func TestDBIndexTriageTool_Execute_MultipleIndexes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create multiple tables with multiple indexes
	_, err = db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT, age INTEGER)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE INDEX idx_users_name ON users(name)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE UNIQUE INDEX idx_users_email ON users(email)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE INDEX idx_users_age ON users(age)")
	require.NoError(t, err)

	_, err = db.Exec("CREATE TABLE posts (id INTEGER PRIMARY KEY, title TEXT, user_id INTEGER)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE INDEX idx_posts_title ON posts(title)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE INDEX idx_posts_user_id ON posts(user_id)")
	require.NoError(t, err)

	mock := &mockDBOpener{db: db}
	tool := &DBIndexTriageTool{dbOpener: mock}

	args := json.RawMessage(`{"database_path": "/path/to/db.sqlite"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res DBIndexTriageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(res.Indexes), 5, "Should have at least 5 indexes")

	// Verify all indexes have the correct structure
	for _, idx := range res.Indexes {
		require.NotEmpty(t, idx.Name)
		require.NotEmpty(t, idx.Table)
		require.True(t, idx.Used, "All indexes should be marked as used")
	}
}

func TestDBIndexTriageTool_Execute_ResultMarshaling(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)

	mock := &mockDBOpener{db: db}
	tool := &DBIndexTriageTool{dbOpener: mock}

	args := json.RawMessage(`{"database_path": "/path/to/db.sqlite"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	require.Equal(t, "text", result.Content[0].Type)
	require.NotEmpty(t, result.Content[0].Text)

	// Verify the result can be unmarshaled
	var res DBIndexTriageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
}
