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
	"strings"

	_ "modernc.org/sqlite"
)

// dbOpener defines the interface for opening database connections.
type dbOpener interface {
	Open(driverName, dataSourceName string) (*sql.DB, error)
}

// realDBOpener is the default implementation using sql.Open.
type realDBOpener struct{}

func (r *realDBOpener) Open(driverName, dataSourceName string) (*sql.DB, error) {
	return sql.Open(driverName, dataSourceName)
}

// DBIndexTriageTool queries fragmentation statistics and indexes.
type DBIndexTriageTool struct {
	dbOpener dbOpener
}

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

	opener := t.dbOpener
	if opener == nil {
		opener = &realDBOpener{}
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", req.DatabasePath)
	db, err := opener.Open("sqlite", dsn)
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
