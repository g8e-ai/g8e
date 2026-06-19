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
	"sync"

	"github.com/g8e-ai/g8e/internal/constants"
)

// PropertySchema represents a JSON Schema property definition.
type PropertySchema struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// InputSchema represents a JSON Schema for tool input validation.
type InputSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]*PropertySchema `json:"properties"`
	Required   []string                   `json:"required"`
}

// ToMap converts InputSchema to map[string]interface{} for JSON serialization.
// This is required for MCP protocol compatibility with the JSON Schema specification.
func (s *InputSchema) ToMap() map[string]interface{} {
	if s == nil {
		return nil
	}
	result := map[string]interface{}{
		"type": s.Type,
	}
	if s.Properties != nil {
		props := make(map[string]interface{})
		for k, v := range s.Properties {
			propMap := map[string]interface{}{
				"type":        v.Type,
				"description": v.Description,
			}
			if v.Enum != nil {
				propMap["enum"] = v.Enum
			}
			props[k] = propMap
		}
		result["properties"] = props
	}
	if s.Required != nil {
		result["required"] = s.Required
	}
	return result
}

// NativeTool defines the interface that all native tools must implement.
type NativeTool interface {
	// Name returns the unique identifier for this tool.
	Name() string

	// Description returns a human-readable description of what the tool does.
	Description() string

	// InputSchema returns the JSON Schema for validating tool input arguments.
	InputSchema() *InputSchema

	// Execute runs the tool with the provided arguments and returns the result.
	Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error)
}

// ToolRegistry manages tool registration and lookup in a thread-safe manner.
type ToolRegistry struct {
	tools map[string]NativeTool
	mu    sync.RWMutex
}

// NewToolRegistry creates a new empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]NativeTool),
	}
}

// Register adds a tool to the registry.
// Returns an error if a tool with the same name is already registered.
func (r *ToolRegistry) Register(tool NativeTool) error {
	if tool == nil {
		return constants.ErrMCPToolNil
	}

	name := tool.Name()
	if name == "" {
		return constants.ErrMCPToolNameEmpty
	}

	// Validate tool name format (must be valid identifier)
	if !isValidToolName(name) {
		return fmt.Errorf("mcp: register: invalid tool name '%s': %w", name, constants.ErrMCPToolNameInvalid)
	}

	// Validate input schema
	if err := validateInputSchema(tool.InputSchema()); err != nil {
		return fmt.Errorf("mcp: register: validate schema for tool '%s': %w", name, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate tool name
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("mcp: register: tool '%s': %w", name, constants.ErrMCPToolAlreadyRegistered)
	}

	r.tools[name] = tool
	return nil
}

// Get retrieves a tool by name.
// Returns the tool and true if found, nil and false otherwise.
func (r *ToolRegistry) Get(name string) (NativeTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tools.
func (r *ToolRegistry) List() []NativeTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]NativeTool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// Count returns the number of registered tools.
func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tools)
}

// isValidToolName validates that a tool name is a valid identifier.
// Tool names must be lowercase, alphanumeric with underscores only.
func isValidToolName(name string) bool {
	if len(name) == 0 {
		return false
	}

	for i, r := range name {
		if i == 0 {
			// First character must be a letter
			if r < 'a' || r > 'z' {
				return false
			}
		} else {
			// Subsequent characters can be letters, digits, or underscores
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
				return false
			}
		}
	}
	return true
}

// validateInputSchema performs basic validation of a tool's input schema.
func validateInputSchema(schema *InputSchema) error {
	if schema == nil {
		return constants.ErrMCPSchemaNil
	}

	// Check for required "type" field
	if schema.Type == "" {
		return constants.ErrMCPSchemaMissingType
	}

	// Check that type is "object"
	if schema.Type != "object" {
		return constants.ErrMCPSchemaInvalidType
	}

	return nil
}
