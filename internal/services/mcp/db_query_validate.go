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

// DBQueryValidateTool validates SQL queries using EXPLAIN QUERY PLAN.
type DBQueryValidateTool struct{}

// Name returns the tool identifier.
func (t *DBQueryValidateTool) Name() string {
	return "db_query_validate"
}

// Description returns a human-readable description.
func (t *DBQueryValidateTool) Description() string {
	return "Validates SQL queries using EXPLAIN QUERY PLAN to detect full table scans and performance issues."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *DBQueryValidateTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"database_path": {
				Type:        "string",
				Description: "Path to the SQLite database file",
			},
			"query": {
				Type:        "string",
				Description: "SQL query to validate (SELECT only)",
			},
		},
		Required: []string{"database_path", "query"},
	}
}

// Execute implements the tool logic.
func (t *DBQueryValidateTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
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

	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(req.Query)), "SELECT") {
		result := DBQueryValidateResult{
			Valid:    false,
			Rejected: true,
			Reason:   "Only SELECT queries are allowed for validation",
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

	if err := validateSQLQuery(req.Query); err != nil {
		result := DBQueryValidateResult{
			Valid:    false,
			Rejected: true,
			Reason:   fmt.Sprintf("Query validation failed: %v", err),
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

	dsn := fmt.Sprintf("file:%s?mode=ro", req.DatabasePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Query is validated by validateSQLQuery to satisfy CodeQL sql-injection rule.
	planRows, err := db.Query("EXPLAIN QUERY PLAN " + req.Query)
	if err != nil {
		result := DBQueryValidateResult{
			Valid:    false,
			Rejected: true,
			Reason:   fmt.Sprintf("Query parse error: %v", err),
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
	defer planRows.Close()

	var planLines []string
	hasFullScan := false

	for planRows.Next() {
		if ctx.Err() != nil {
			return CallToolResult{}, ctx.Err()
		}

		var id, parent, notused int
		var detail string
		if err := planRows.Scan(&id, &parent, &notused, &detail); err != nil {
			continue
		}
		planLines = append(planLines, detail)

		if strings.Contains(strings.ToLower(detail), "scan") &&
			(strings.Contains(strings.ToLower(detail), "table") ||
				strings.Contains(strings.ToLower(detail), "using index")) {
			hasFullScan = true
		}
	}

	planStr := strings.Join(planLines, "\n")
	result := DBQueryValidateResult{
		Valid:    true,
		Plan:     planStr,
		Rejected: false,
	}

	if hasFullScan {
		result.Valid = false
		result.Rejected = true
		result.Reason = "Query performs a full table scan, which may be slow on large datasets"
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
