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

//go:build ignore
// +build ignore

// TEMPLATE: Copy this file to create a new native tool
//
// USAGE:
// 1. Copy this file to internal/services/mcp/your_tool_name.go
// 2. Replace YOUR_TOOL_NAME with your tool name (snake_case)
// 3. Replace YourTool with your tool name (PascalCase)
// 4. Implement the Execute() method
// 5. Add YourToolRequest and YourToolResult structs to internal/services/mcp/models.go
// 6. Add your tool to the tools list in RegisterNativeTools() in native_tool_registry.go
// 7. Create a test file internal/services/mcp/your_tool_name_test.go
// 8. Done! No init() function needed

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// YourToolRequest represents the input for your tool.
// Ideally, move this to internal/services/mcp/models.go
type YourToolRequest struct {
	Param1 string `json:"param1"`
	Param2 int    `json:"param2,omitempty"`
}

// YourToolResult represents the output of your tool.
// Ideally, move this to internal/services/mcp/models.go
type YourToolResult struct {
	Success bool   `json:"success"`
	Data    string `json:"data"`
	Error   string `json:"error,omitempty"`
}

// YourTool implements a custom native tool
type YourTool struct{}

// Name returns the tool identifier
func (t *YourTool) Name() string {
	return "your_tool_name"
}

// Description returns a human-readable description
func (t *YourTool) Description() string {
	return "Brief description of what your tool does"
}

// InputSchema returns the JSON Schema for tool validation
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

// Execute implements the tool logic
func (t *YourTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req YourToolRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("your_tool_name: unmarshal arguments: %w", err)
	}

	// Validate required fields (optional, but good practice for clarity)
	if req.Param1 == "" {
		return CallToolResult{}, fmt.Errorf("your_tool_name: param1 is required")
	}

	// Example: Check context for cancellation
	if err := ctx.Err(); err != nil {
		return CallToolResult{}, err
	}

	// YOUR TOOL LOGIC HERE
	// Implement your tool's functionality
	result := YourToolResult{
		Success: true,
		Data:    "Tool executed successfully",
	}

	// Marshal result to JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("your_tool_name: marshal result: %w", err)
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
