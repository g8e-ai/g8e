// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build ignore

// TEMPLATE: Copy this file to create a new native tool
//
// USAGE:
// 1. Copy this file to internal/services/mcp/your_tool_name.go
// 2. Replace your_tool_name with your tool name (snake_case)
// 3. Replace YourTool with your tool name (PascalCase)
// 4. Replace YourToolRequest/YourToolResult fields with your inputs/outputs
// 5. Implement the Execute() method; validate user input and respect ctx.Err()
// 6. Keep YourToolRequest and YourToolResult in this file (not models.go)
// 7. Add input validation helpers to validation.go for user-facing inputs
// 8. Add your tool to the tools list in RegisterNativeTools() in native_tool_registry.go
// 9. Create a test file internal/services/mcp/your_tool_name_test.go
// 10. Done! No init() function needed
//
// CONVENTIONS (see docs/devs/devs.md):
// - Three import blocks: standard library, external, internal
// - Wrap errors with context: fmt.Errorf("your_tool_name: %w: %w", constants.Err..., err)
// - Use constants from internal/constants/errors.go for known failure modes
// - Return Go errors for programming failures (unmarshal, marshal)
// - Return CallToolResult{IsError: true, ...} for operational failures (tool ran but action failed)
// - Use context.Context for cancellation; check ctx.Err() in long-running loops
// - Define filepaths in internal/constants/paths.go; use RuntimeFileService for .g8e/ I/O

package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
)

// YourToolRequest represents the input for your tool.
// Keep request/result structs in this tool file, not in models.go.
type YourToolRequest struct {
	Param1 string `json:"param1"`
	Param2 int    `json:"param2,omitempty"`
}

// YourToolResult represents the output of your tool.
// Keep request/result structs in this tool file, not in models.go.
type YourToolResult struct {
	Success bool   `json:"success"`
	Data    string `json:"data"`
	Error   string `json:"error,omitempty"`
}

// YourTool implements a custom native tool.
type YourTool struct{}

// Name returns the tool identifier.
func (t *YourTool) Name() string {
	return "your_tool_name"
}

// Description returns a human-readable description.
func (t *YourTool) Description() string {
	return "Brief description of what your tool does"
}

// InputSchema returns the JSON Schema for tool validation.
func (t *YourTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"param1": {
				Type:        "string",
				Description: "Description of param1",
			},
			"param2": {
				Type:        "integer",
				Description: "Description of param2 (optional)",
			},
		},
		Required: []string{"param1"},
	}
}

// Execute implements the tool logic.
// Return a Go error for programming failures (unmarshal, marshal).
// Return CallToolResult{IsError: true, ...} for operational failures (tool ran but action failed).
func (t *YourTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req YourToolRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("your_tool_name: %w: %w", constants.ErrMCPUnmarshalArguments, err)
	}

	// Validate required fields (replace with helpers in validation.go for user input).
	if req.Param1 == "" {
		return CallToolResult{}, fmt.Errorf("your_tool_name: param1 is required")
	}

	// Check context for cancellation.
	if err := ctx.Err(); err != nil {
		return CallToolResult{}, err
	}

	// YOUR TOOL LOGIC HERE
	// Implement your tool's functionality
	result := YourToolResult{
		Success: true,
		Data:    "Tool executed successfully",
	}

	// Example: operational error (tool ran but action failed)
	// The IsError flag tells the client the error is in the result, not a JSON-RPC error.
	//
	// result := YourToolResult{
	// 	Success: false,
	// 	Error:   "resource not found",
	// }
	// resultJSON, err := json.Marshal(result)
	// if err != nil {
	// 	return CallToolResult{}, fmt.Errorf("your_tool_name: %w: %w", constants.ErrMCPMarshalResult, err)
	// }
	// return CallToolResult{
	// 	IsError: true,
	// 	Content: []TextContent{
	// 		{
	// 			Type: "text",
	// 			Text: string(resultJSON),
	// 		},
	// 	},
	// }, nil

	// Marshal result to JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("your_tool_name: %w: %w", constants.ErrMCPMarshalResult, err)
	}

	// Return result
	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}
