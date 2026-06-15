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

//go:build integration

package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/require"
)

func TestDBQueryValidateTool_Name(t *testing.T) {
	tool := &DBQueryValidateTool{}
	require.Equal(t, "db_query_validate", tool.Name())
}

func TestDBQueryValidateTool_Description(t *testing.T) {
	tool := &DBQueryValidateTool{}
	require.NotEmpty(t, tool.Description())
}

func TestDBQueryValidateTool_InputSchema(t *testing.T) {
	tool := &DBQueryValidateTool{}
	schema := tool.InputSchema()

	require.NotNil(t, schema)
	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Properties, "database_path")
	require.Contains(t, schema.Properties, "query")
	require.Equal(t, []string{"database_path", "query"}, schema.Required)
}

func TestDBQueryValidateTool_Execute_InvalidJSON(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	// Invalid JSON
	args := json.RawMessage(`{invalid json`)
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid arguments")
}

func TestDBQueryValidateTool_Execute_MissingDatabasePath(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"query": "SELECT * FROM users"}`)
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database_path and query required")
}

func TestDBQueryValidateTool_Execute_MissingQuery(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"database_path": "/tmp/test.db"}`)
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database_path and query required")
}

func TestDBQueryValidateTool_Execute_EmptyDatabasePath(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"database_path": "", "query": "SELECT * FROM users"}`)
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database_path and query required")
}

func TestDBQueryValidateTool_Execute_EmptyQuery(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"database_path": "/tmp/test.db", "query": ""}`)
	_, err := tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database_path and query required")
}

func TestDBQueryValidateTool_Execute_NonSELECTQuery(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tests := []struct {
		name  string
		query string
	}{
		{"DROP TABLE", "DROP TABLE users"},
		{"DELETE", "DELETE FROM users WHERE id = 1"},
		{"INSERT", "INSERT INTO users (name) VALUES ('test')"},
		{"UPDATE", "UPDATE users SET name = 'test' WHERE id = 1"},
		{"CREATE TABLE", "CREATE TABLE test (id INTEGER)"},
		{"ALTER TABLE", "ALTER TABLE users ADD COLUMN age INTEGER"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := json.RawMessage(`{"database_path": "/tmp/test.db", "query": "` + tt.query + `"}`)
			result, err := tool.Execute(ctx, args)
			require.NoError(t, err)
			require.Len(t, result.Content, 1)

			var res map[string]interface{}
			err = json.Unmarshal([]byte(result.Content[0].Text), &res)
			require.NoError(t, err)

			require.False(t, res["valid"].(bool))
			require.True(t, res["rejected"].(bool))
			require.Contains(t, res["reason"], "Only SELECT queries are allowed")
		})
	}
}

func TestDBQueryValidateTool_Execute_NonSELECTLowercase(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"database_path": "/tmp/test.db", "query": "delete from users"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.False(t, res["valid"].(bool))
	require.True(t, res["rejected"].(bool))
	require.Contains(t, res["reason"], "Only SELECT queries are allowed")
}

func TestDBQueryValidateTool_Execute_TrailingSemicolon(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"database_path": "/tmp/test.db", "query": "SELECT * FROM users;"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.False(t, res["valid"].(bool))
	require.True(t, res["rejected"].(bool))
	require.Contains(t, res["reason"], "Query validation failed")
	require.Contains(t, res["reason"], "must not end with semicolon")
}

func TestDBQueryValidateTool_Execute_TrailingSemicolonWithSpaces(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"database_path": "/tmp/test.db", "query": "SELECT * FROM users;   "}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.False(t, res["valid"].(bool))
	require.True(t, res["rejected"].(bool))
	require.Contains(t, res["reason"], "must not end with semicolon")
}

func TestDBQueryValidateTool_Execute_DatabaseOpenFailure(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	// SQLite creates databases on the fly, so testing open failure is difficult.
	// Instead, test that the tool handles read-only mode correctly.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a valid database first
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
	require.NoError(t, err)
	db.Close()

	// Now test that the tool can open it in read-only mode
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users WHERE id = 1"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Contains(t, res, "valid")
}

func TestDBQueryValidateTool_Execute_ValidIndexedQuery(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database with indexed table
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT UNIQUE
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE INDEX idx_users_email ON users(email)`)
	require.NoError(t, err)

	// Test indexed query
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users WHERE email = 'test@example.com'"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.True(t, res["valid"].(bool))
	require.False(t, res["rejected"].(bool))
	require.Contains(t, res, "plan")
}

func TestDBQueryValidateTool_Execute_PrimaryKeyLookup(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	// Test primary key lookup
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users WHERE id = 1"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.True(t, res["valid"].(bool))
	require.False(t, res["rejected"].(bool))
	require.Contains(t, res, "plan")
}

func TestDBQueryValidateTool_Execute_FullTableScan(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER,
		name TEXT NOT NULL,
		email TEXT
	)`)
	require.NoError(t, err)

	// Test query that may cause full table scan
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	// Check that we get a plan back (regardless of whether it's rejected)
	require.Contains(t, res, "plan")
	require.Contains(t, res, "valid")
	require.Contains(t, res, "rejected")
}

func TestDBQueryValidateTool_Execute_FullTableScanWithNonIndexedWhere(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database without primary key or indexes
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	// Test query with WHERE on non-indexed column
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users WHERE name = 'John'"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	// Check that we get a plan back (regardless of whether it's rejected)
	require.Contains(t, res, "plan")
	require.Contains(t, res, "valid")
	require.Contains(t, res, "rejected")
}

func TestDBQueryValidateTool_Execute_ComplexQueryWithJoin(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database with indexed foreign keys
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE TABLE orders (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		total DECIMAL(10,2),
		FOREIGN KEY (user_id) REFERENCES users(id)
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE INDEX idx_orders_user_id ON orders(user_id)`)
	require.NoError(t, err)

	// Test indexed join query
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE u.id = 1"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.True(t, res["valid"].(bool))
	require.False(t, res["rejected"].(bool))
	require.Contains(t, res, "plan")
}

func TestDBQueryValidateTool_Execute_QueryWithOrderBy(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	// Test query with ORDER BY on indexed column
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users WHERE id = 1 ORDER BY name"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.True(t, res["valid"].(bool))
	require.False(t, res["rejected"].(bool))
}

func TestDBQueryValidateTool_Execute_QueryWithLimit(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	// Test query with LIMIT (even without WHERE, LIMIT can prevent full scan in some cases)
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users LIMIT 10"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	// This may still be rejected depending on SQLite's query plan
	// but we should get a valid response
	require.Contains(t, res, "valid")
	require.Contains(t, res, "plan")
}

func TestDBQueryValidateTool_Execute_InvalidSQLSyntax(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	// Test invalid SQL syntax
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM nonexistent_table"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.False(t, res["valid"].(bool))
	require.True(t, res["rejected"].(bool))
	require.Contains(t, res["reason"], "Query parse error")
}

func TestDBQueryValidateTool_Execute_ContextCancellation(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx, cancel := context.WithCancel(context.Background())

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	// Cancel context before executing
	cancel()

	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users"}`)
	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
}

func TestDBQueryValidateTool_Execute_WhitespaceInQuery(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	// Test query with leading/trailing whitespace
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "  SELECT * FROM users WHERE id = 1  "}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.True(t, res["valid"].(bool))
	require.False(t, res["rejected"].(bool))
}

func TestDBQueryValidateTool_Execute_SelectWithSubquery(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE TABLE orders (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		total DECIMAL(10,2)
	)`)
	require.NoError(t, err)

	// Test query with subquery
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users WHERE id IN (SELECT user_id FROM orders WHERE total > 100)"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Contains(t, res, "valid")
	require.Contains(t, res, "plan")
}

func TestDBQueryValidateTool_Execute_AggregateQuery(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE INDEX idx_users_status ON users(status)`)
	require.NoError(t, err)

	// Test aggregate query with GROUP BY on indexed column
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT status, COUNT(*) FROM users WHERE status = 'active' GROUP BY status"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.True(t, res["valid"].(bool))
	require.False(t, res["rejected"].(bool))
}

func TestDBQueryValidateTool_Execute_ReadOnlyDatabase(t *testing.T) {
	tool := &DBQueryValidateTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test database
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	// Make database read-only
	err = os.Chmod(dbPath, 0444)
	require.NoError(t, err)
	defer os.Chmod(dbPath, 0644)

	// Test that read-only mode works
	args := json.RawMessage(`{"database_path": "` + dbPath + `", "query": "SELECT * FROM users WHERE id = 1"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.True(t, res["valid"].(bool))
	require.False(t, res["rejected"].(bool))
}
