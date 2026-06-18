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

package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// DBIsolatedReadTool executes SELECT statements in read-only mode.
type DBIsolatedReadTool struct{}

// Name returns the tool identifier.
func (t *DBIsolatedReadTool) Name() string {
	return "db_isolated_read"
}

// Description returns a human-readable description.
func (t *DBIsolatedReadTool) Description() string {
	return "Executes SELECT statements in read-only mode against a SQLite database."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *DBIsolatedReadTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"database_path": {
				Type:        "string",
				Description: "Path to the SQLite database file",
			},
			"query": {
				Type:        "string",
				Description: "SELECT query to execute",
			},
		},
		Required: []string{"database_path", "query"},
	}
}

// Execute implements the tool logic.
func (t *DBIsolatedReadTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		DatabasePath string `json:"database_path"`
		Query        string `json:"query"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.DatabasePath == "" || req.Query == "" {
		return CallToolResult{}, fmt.Errorf("database_path and query required")
	}

	queryUpper := strings.ToUpper(strings.TrimSpace(req.Query))
	if !strings.HasPrefix(queryUpper, "SELECT") {
		return dbErrorResult("Only SELECT queries are allowed in isolated read mode"), nil
	}

	if err := validateSQLQuery(req.Query); err != nil {
		return dbErrorResult(fmt.Sprintf("Query validation failed: %v", err)), nil
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", req.DatabasePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Query is validated by validateSQLQuery to satisfy CodeQL sql-injection rule.
	rows, err := db.Query(req.Query)
	if err != nil {
		return dbErrorResult(fmt.Sprintf("Query execution failed: %v", err)), nil
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to get columns: %w", err)
	}

	var resultRows []DBRow
	for rows.Next() {
		if ctx.Err() != nil {
			return CallToolResult{}, ctx.Err()
		}

		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		rowValues := make(map[string]DBValue)
		for i, col := range columns {
			val := values[i]
			rowValues[col] = convertToDBValue(val)
		}
		resultRows = append(resultRows, DBRow{Values: rowValues})
	}

	result := DBIsolatedReadResult{
		Rows:    resultRows,
		Columns: columns,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
	}

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// dbErrorResult builds an error CallToolResult.
func dbErrorResult(msg string) CallToolResult {
	return CallToolResult{
		Content: []TextContent{{Type: "text", Text: msg}},
		IsError: true,
	}
}

// convertToDBValue converts an interface{} value to a typed DBValue.
func convertToDBValue(val interface{}) DBValue {
	if val == nil {
		return DBValue{Null: true}
	}

	switch v := val.(type) {
	case []byte:
		s := string(v)
		return DBValue{String: &s}
	case string:
		return DBValue{String: &v}
	case int:
		i := int64(v)
		return DBValue{Int64: &i}
	case int64:
		return DBValue{Int64: &v}
	case float64:
		return DBValue{Float64: &v}
	case bool:
		return DBValue{Bool: &v}
	default:
		// Fallback to string representation for unknown types
		s := fmt.Sprintf("%v", v)
		return DBValue{String: &s}
	}
}
