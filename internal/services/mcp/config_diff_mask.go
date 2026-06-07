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
	"strings"

	"github.com/g8e-ai/g8e/internal/security"
)

// ConfigDiffMaskTool compares configuration files with secret masking.
type ConfigDiffMaskTool struct{}

// Name returns the tool identifier.
func (t *ConfigDiffMaskTool) Name() string {
	return "config_diff_mask"
}

// Description returns a human-readable description.
func (t *ConfigDiffMaskTool) Description() string {
	return "Compares configuration files with automatic secret masking for sensitive values."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *ConfigDiffMaskTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"config_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the current configuration file",
			},
			"baseline": map[string]interface{}{
				"type":        "string",
				"description": "Baseline configuration content as string",
			},
		},
		"required": []string{"config_path", "baseline"},
	}
}

// Execute implements the tool logic.
func (t *ConfigDiffMaskTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req ConfigDiffMaskRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.ConfigPath == "" || req.Baseline == "" {
		return CallToolResult{}, fmt.Errorf("config_path and baseline required")
	}

	// Validate path to prevent directory traversal attacks
	// Use current working directory as root for relative paths
	cwd, err := os.Getwd()
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to get current working directory: %w", err)
	}
	safePath, err := security.ValidatePath(req.ConfigPath, cwd)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("invalid config path: %w", err)
	}

	currentBytes, err := os.ReadFile(safePath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to read config file: %w", err)
	}

	baselineBytes := []byte(req.Baseline)

	currentLines := strings.Split(string(currentBytes), "\n")
	baselineLines := strings.Split(string(baselineBytes), "\n")

	var differences []ConfigDiff

	maxLines := len(currentLines)
	if len(baselineLines) > maxLines {
		maxLines = len(baselineLines)
	}

	for i := 0; i < maxLines; i++ {
		var current, baseline string
		if i < len(currentLines) {
			current = strings.TrimSpace(currentLines[i])
		}
		if i < len(baselineLines) {
			baseline = strings.TrimSpace(baselineLines[i])
		}

		if current != baseline {
			diffType := "added"
			if baseline != "" && current == "" {
				diffType = "removed"
			} else if baseline != "" && current != "" {
				diffType = "changed"
			}

			differences = append(differences, ConfigDiff{
				Key:      fmt.Sprintf("line_%d", i),
				Current:  maskSecret(current),
				Baseline: maskSecret(baseline),
				Type:     diffType,
			})
		}
	}

	result := ConfigDiffMaskResult{
		Differences: differences,
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
