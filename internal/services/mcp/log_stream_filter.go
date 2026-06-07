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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/g8e-ai/g8e/internal/security"
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
func (t *LogStreamFilterTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"log_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the log file",
			},
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Regex pattern to filter lines",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of lines to return (default 100)",
			},
		},
		"required": []string{"log_path", "pattern"},
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
		limit = defaultLogFilterLimit
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
