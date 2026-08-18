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
	"fmt"

	_ "modernc.org/sqlite"
)

// DBDiscoverTopologyTool scans database schemas, tables, and column data types.
type DBDiscoverTopologyTool struct{}

// Name returns the tool identifier.
func (t *DBDiscoverTopologyTool) Name() string {
	return "db_discover_topology"
}

// Description returns a human-readable description.
func (t *DBDiscoverTopologyTool) Description() string {
	return "Automatically scans database schemas, tables, and column data types, returning a highly compressed JSON map."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *DBDiscoverTopologyTool) InputSchema() *InputSchema {
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
func (t *DBDiscoverTopologyTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		DatabasePath string `json:"database_path"`
	}
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

	schema := make(map[string]map[string]string)

	tables, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to query tables: %w", err)
	}
	defer tables.Close()

	for tables.Next() {
		if ctx.Err() != nil {
			return CallToolResult{}, ctx.Err()
		}

		var tableName string
		if err := tables.Scan(&tableName); err != nil {
			continue
		}

		schema[tableName] = make(map[string]string)

		if !isValidIdentifier(tableName) {
			continue
		}

		columns, err := db.Query("PRAGMA table_info(" + tableName + ")")
		if err != nil {
			continue
		}

		for columns.Next() {
			var cid int
			var name, datatype string
			var notnull int
			var dfltValue interface{}
			var pk int

			if err := columns.Scan(&cid, &name, &datatype, &notnull, &dfltValue, &pk); err != nil {
				continue
			}

			schema[tableName][name] = datatype
		}
		columns.Close()
	}

	result := map[string]interface{}{
		"schema": schema,
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
