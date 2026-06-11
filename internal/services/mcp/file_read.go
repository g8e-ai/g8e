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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileReadTool reads file contents with path validation and safety checks.
type FileReadTool struct{}

// Name returns the tool identifier.
func (t *FileReadTool) Name() string {
	return "read_file"
}

// Description returns a human-readable description.
func (t *FileReadTool) Description() string {
	return "Reads the contents of a file. Does not respect .gitignore patterns - can read any file on the filesystem."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *FileReadTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"path": {
				Type:        "string",
				Description: "Absolute or relative path to the file to read",
			},
			"offset": {
				Type:        "integer",
				Description: "Line number to start reading from (1-indexed, optional)",
			},
			"limit": {
				Type:        "integer",
				Description: "Maximum number of lines to read (optional)",
			},
		},
		Required: []string{"path"},
	}
}

// FileReadRequest is the params for the "read_file" tool.
type FileReadRequest struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// FileReadResult is the result for the "read_file" tool.
type FileReadResult struct {
	Content string `json:"content"`
	Path    string `json:"path"`
	Offset  int    `json:"offset,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Execute implements the tool logic.
func (t *FileReadTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req FileReadRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("read_file: unmarshal arguments: %w", err)
	}

	if req.Path == "" {
		return CallToolResult{}, fmt.Errorf("read_file: path is required")
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		result := FileReadResult{
			Path:  req.Path,
			Error: fmt.Sprintf("failed to resolve absolute path: %v", err),
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	// Security check: prevent reading sensitive system files
	dangerousPaths := []string{
		"/etc/shadow",
		"/etc/passwd",
		"/etc/gshadow",
		"/etc/sudoers",
		".ssh/id_rsa",
		".ssh/id_ed25519",
		".ssh/id_ecdsa",
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
	}
	for _, dangerous := range dangerousPaths {
		if strings.Contains(absPath, dangerous) {
			result := FileReadResult{
				Path:  absPath,
				Error: "access denied: sensitive file",
			}
			resultJSON, _ := json.Marshal(result)
			return CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: string(resultJSON),
					},
				},
			}, nil
		}
	}

	// Read file
	content, err := os.ReadFile(absPath)
	if err != nil {
		result := FileReadResult{
			Path:  absPath,
			Error: fmt.Sprintf("failed to read file: %v", err),
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	// Convert to string and handle offset/limit
	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")

	if req.Offset > 0 || req.Limit > 0 {
		start := 0
		if req.Offset > 0 {
			start = req.Offset - 1 // Convert to 0-indexed
			if start < 0 {
				start = 0
			}
			if start >= len(lines) {
				start = len(lines)
			}
		}

		end := len(lines)
		if req.Limit > 0 {
			end = start + req.Limit
			if end > len(lines) {
				end = len(lines)
			}
		}

		if start >= end {
			lines = []string{}
		} else {
			lines = lines[start:end]
		}
	}

	// Rejoin with newlines
	resultContent := strings.Join(lines, "\n")

	result := FileReadResult{
		Content: resultContent,
		Path:    absPath,
		Offset:  req.Offset,
		Limit:   req.Limit,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("read_file: marshal result: %w", err)
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
