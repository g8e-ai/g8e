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

// FSDiskProfileTool recursively calculates directory sizes.
type FSDiskProfileTool struct{}

// Name returns the tool identifier.
func (t *FSDiskProfileTool) Name() string {
	return "fs_disk_profile"
}

// Description returns a human-readable description.
func (t *FSDiskProfileTool) Description() string {
	return "Recursively calculates directory sizes and disk usage."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *FSDiskProfileTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to profile",
			},
			"max_depth": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum directory depth (default 2)",
			},
		},
		"required": []string{"path"},
	}
}

// Execute implements the tool logic.
func (t *FSDiskProfileTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		Path     string `json:"path"`
		MaxDepth int    `json:"max_depth,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.Path == "" {
		return CallToolResult{}, fmt.Errorf("path required")
	}

	maxDepth := req.MaxDepth
	if maxDepth <= 0 {
		maxDepth = defaultDiskProfileDepth
	}

	var entries []map[string]interface{}
	var totalSize int64

	err := filepath.Walk(req.Path, func(path string, info os.FileInfo, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(req.Path, path)
		if err != nil {
			return nil
		}

		depth := len(strings.Split(relPath, string(filepath.Separator)))
		if depth > maxDepth+1 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		size := info.Size()
		totalSize += size

		entries = append(entries, map[string]interface{}{
			"path":     relPath,
			"size_mb":  size / (1024 * 1024),
			"is_dir":   info.IsDir(),
			"modified": info.ModTime().Unix(),
		})

		return nil
	})

	if err != nil {
		return CallToolResult{}, fmt.Errorf("error walking path: %w", err)
	}

	result := map[string]interface{}{
		"entries":  entries,
		"total_mb": totalSize / (1024 * 1024),
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
