// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
func (t *ConfigDiffMaskTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"config_path": {
				Type:        "string",
				Description: "Path to the current configuration file",
			},
			"baseline": {
				Type:        "string",
				Description: "Baseline configuration content as string",
			},
		},
		Required: []string{"config_path", "baseline"},
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
