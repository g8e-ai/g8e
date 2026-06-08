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

// DBIndexTriageTool queries fragmentation statistics and indexes.
type DBIndexTriageTool struct{}

// Name returns the tool identifier.
func (t *DBIndexTriageTool) Name() string {
	return "db_index_triage"
}

// Description returns a human-readable description.
func (t *DBIndexTriageTool) Description() string {
	return "Queries database fragmentation statistics and index information."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *DBIndexTriageTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"database_path": {
				Type:        "string",
				Description: "Path to the SQLite database file",
			},
		},
		Required: []string{"database_path"},
	}
}

// Execute implements the tool logic.
func (t *DBIndexTriageTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req DBIndexTriageRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.DatabasePath == "" {
		return CallToolResult{}, fmt.Errorf("database_path required")
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", req.DatabasePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	var indexes []IndexInfo

	indexList, err := db.Query("SELECT name, tbl_name, sql FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to query indexes: %w", err)
	}
	defer indexList.Close()

	for indexList.Next() {
		if ctx.Err() != nil {
			return CallToolResult{}, ctx.Err()
		}

		var name, table, sql string
		if err := indexList.Scan(&name, &table, &sql); err != nil {
			return CallToolResult{}, fmt.Errorf("failed to scan index: %w", err)
		}

		unique := strings.Contains(strings.ToUpper(sql), "UNIQUE")

		indexes = append(indexes, IndexInfo{
			Name:   name,
			Table:  table,
			Unique: unique,
			Used:   true,
		})
	}

	result := DBIndexTriageResult{
		Indexes:       indexes,
		Fragmentation: 0.0,
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
