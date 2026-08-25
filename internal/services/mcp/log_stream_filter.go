// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/security"
)

// LogStreamFilterTool reads log files and applies regex filtering with scrubbing.
type LogStreamFilterTool struct{}

// Name returns the tool identifier.
func (t *LogStreamFilterTool) Name() string {
	return "log_stream_filter"
}

// Description returns a human-readable description.
func (t *LogStreamFilterTool) Description() string {
	return "Reads log files and applies regex filtering with sensitive data scrubbing."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *LogStreamFilterTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"log_path": {
				Type:        "string",
				Description: "Path to the log file",
			},
			"pattern": {
				Type:        "string",
				Description: "Regex pattern to filter lines",
			},
			"limit": {
				Type:        "integer",
				Description: "Maximum number of lines to return (default 100)",
			},
		},
		Required: []string{"log_path", "pattern"},
	}
}

// Execute implements the tool logic.
func (t *LogStreamFilterTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req LogStreamFilterRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.LogPath == "" || req.Pattern == "" {
		return CallToolResult{}, fmt.Errorf("log_path and pattern required")
	}

	// Validate path to prevent directory traversal attacks
	// Use current working directory as root for relative paths
	cwd, err := os.Getwd()
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to get current working directory: %w", err)
	}
	safePath, err := security.ValidatePath(req.LogPath, cwd)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("invalid log path: %w", err)
	}

	file, err := os.Open(safePath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	regex, err := regexp.Compile(req.Pattern)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("invalid regex pattern: %w", err)
	}

	var lines []string
	limit := req.Limit
	if limit <= 0 {
		limit = constants.DefaultLogFilterLimit
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() && len(lines) < limit {
		if ctx.Err() != nil {
			return CallToolResult{}, ctx.Err()
		}

		line := scanner.Text()
		if regex.MatchString(line) {
			scrubbed := scrubLine(line)
			lines = append(lines, scrubbed)
		}
	}

	if err := scanner.Err(); err != nil {
		return CallToolResult{}, fmt.Errorf("error reading log file: %w", err)
	}

	result := LogStreamFilterResult{
		Lines: lines,
		Count: len(lines),
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
