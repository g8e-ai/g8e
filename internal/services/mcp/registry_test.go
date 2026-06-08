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
	"testing"
)

// mockTool is a test implementation of NativeTool for testing purposes.
type mockTool struct {
	name        string
	description string
	schema      *InputSchema
	executeFunc func(ctx context.Context, args json.RawMessage) (CallToolResult, error)
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) InputSchema() *InputSchema {
	return m.schema
}

func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, args)
	}
	return CallToolResult{}, nil
}

func TestNewToolRegistry(t *testing.T) {
	registry := NewToolRegistry()
	if registry == nil {
		t.Fatal("NewToolRegistry returned nil")
	}
	if registry.Count() != 0 {
		t.Errorf("Expected empty registry, got %d tools", registry.Count())
	}
}

func TestToolRegistry_Register(t *testing.T) {
	registry := NewToolRegistry()

	tool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		schema: &InputSchema{
			Type: "object",
		},
	}

	err := registry.Register(tool)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	if registry.Count() != 1 {
		t.Errorf("Expected 1 tool, got %d", registry.Count())
	}

	// Verify tool can be retrieved
	retrieved, ok := registry.Get("test_tool")
	if !ok {
		t.Fatal("Tool not found after registration")
	}
	if retrieved.Name() != "test_tool" {
		t.Errorf("Retrieved wrong tool: got %s, want test_tool", retrieved.Name())
	}
}

func TestToolRegistry_RegisterDuplicate(t *testing.T) {
	registry := NewToolRegistry()

	tool1 := &mockTool{
		name:        "duplicate_tool",
		description: "First tool",
		schema: &InputSchema{
			Type: "object",
		},
	}

	tool2 := &mockTool{
		name:        "duplicate_tool",
		description: "Second tool with same name",
		schema: &InputSchema{
			Type: "object",
		},
	}

	err := registry.Register(tool1)
	if err != nil {
		t.Fatalf("Failed to register first tool: %v", err)
	}

	err = registry.Register(tool2)
	if err == nil {
		t.Fatal("Expected error when registering duplicate tool, got nil")
	}

	expectedError := "registry: tool 'duplicate_tool' is already registered"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestToolRegistry_RegisterNil(t *testing.T) {
	registry := NewToolRegistry()

	err := registry.Register(nil)
	if err == nil {
		t.Fatal("Expected error when registering nil tool, got nil")
	}

	expectedError := "registry: cannot register nil tool"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestToolRegistry_RegisterEmptyName(t *testing.T) {
	registry := NewToolRegistry()

	tool := &mockTool{
		name:        "",
		description: "Tool with empty name",
		schema: &InputSchema{
			Type: "object",
		},
	}

	err := registry.Register(tool)
	if err == nil {
		t.Fatal("Expected error when registering tool with empty name, got nil")
	}

	expectedError := "registry: tool name cannot be empty"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestToolRegistry_RegisterInvalidName(t *testing.T) {
	registry := NewToolRegistry()

	testCases := []struct {
		name        string
		toolName    string
		expectedErr string
	}{
		{
			name:        "uppercase letters",
			toolName:    "InvalidTool",
			expectedErr: "registry: invalid tool name 'InvalidTool'",
		},
		{
			name:        "hyphens",
			toolName:    "invalid-tool",
			expectedErr: "registry: invalid tool name 'invalid-tool'",
		},
		{
			name:        "spaces",
			toolName:    "invalid tool",
			expectedErr: "registry: invalid tool name 'invalid tool'",
		},
		{
			name:        "starts with digit",
			toolName:    "1invalid",
			expectedErr: "registry: invalid tool name '1invalid'",
		},
		{
			name:        "starts with underscore",
			toolName:    "_invalid",
			expectedErr: "registry: invalid tool name '_invalid'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &mockTool{
				name:        tc.toolName,
				description: "Tool with invalid name",
				schema: &InputSchema{
					Type: "object",
				},
			}

			err := registry.Register(tool)
			if err == nil {
				t.Fatalf("Expected error for tool name '%s', got nil", tc.toolName)
			}

			if err.Error()[:len(tc.expectedErr)] != tc.expectedErr {
				t.Errorf("Expected error to start with '%s', got '%s'", tc.expectedErr, err.Error())
			}
		})
	}
}

func TestToolRegistry_RegisterValidNames(t *testing.T) {
	registry := NewToolRegistry()

	validNames := []string{
		"valid_tool",
		"tool123",
		"a",
		"tool_with_underscores_and_123",
	}

	for _, name := range validNames {
		tool := &mockTool{
			name:        name,
			description: "Valid tool",
			schema: &InputSchema{
				Type: "object",
			},
		}

		err := registry.Register(tool)
		if err != nil {
			t.Errorf("Expected no error for valid name '%s', got: %v", name, err)
		}
	}
}

func TestToolRegistry_RegisterInvalidSchema(t *testing.T) {
	registry := NewToolRegistry()

	testCases := []struct {
		name        string
		schema      *InputSchema
		expectedErr string
	}{
		{
			name:        "nil schema",
			schema:      nil,
			expectedErr: "registry: invalid input schema for tool 'test_tool': registry: schema cannot be nil",
		},
		{
			name:        "missing type",
			schema:      &InputSchema{},
			expectedErr: "registry: invalid input schema for tool 'test_tool': registry: schema missing required 'type' field",
		},
		{
			name:        "invalid type",
			schema:      &InputSchema{Type: "array"},
			expectedErr: "registry: invalid input schema for tool 'test_tool': registry: schema 'type' must be 'object'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &mockTool{
				name:        "test_tool",
				description: "Test tool",
				schema:      tc.schema,
			}

			err := registry.Register(tool)
			if err == nil {
				t.Fatal("Expected error for invalid schema, got nil")
			}

			if err.Error() != tc.expectedErr {
				t.Errorf("Expected error '%s', got '%s'", tc.expectedErr, err.Error())
			}
		})
	}
}

func TestToolRegistry_Get(t *testing.T) {
	registry := NewToolRegistry()

	tool := &mockTool{
		name:        "get_test",
		description: "Tool for get test",
		schema: &InputSchema{
			Type: "object",
		},
	}

	registry.Register(tool)

	// Test getting existing tool
	retrieved, ok := registry.Get("get_test")
	if !ok {
		t.Fatal("Failed to get existing tool")
	}
	if retrieved.Name() != "get_test" {
		t.Errorf("Got wrong tool: %s", retrieved.Name())
	}

	// Test getting non-existent tool
	_, ok = registry.Get("non_existent")
	if ok {
		t.Fatal("Expected false for non-existent tool")
	}
}

func TestToolRegistry_List(t *testing.T) {
	registry := NewToolRegistry()

	tools := []*mockTool{
		{name: "tool1", description: "First", schema: &InputSchema{Type: "object"}},
		{name: "tool2", description: "Second", schema: &InputSchema{Type: "object"}},
		{name: "tool3", description: "Third", schema: &InputSchema{Type: "object"}},
	}

	for _, tool := range tools {
		registry.Register(tool)
	}

	listed := registry.List()
	if len(listed) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(listed))
	}

	// Verify all tools are present
	names := make(map[string]bool)
	for _, tool := range listed {
		names[tool.Name()] = true
	}

	for _, expectedName := range []string{"tool1", "tool2", "tool3"} {
		if !names[expectedName] {
			t.Errorf("Expected tool '%s' in list", expectedName)
		}
	}
}

func TestToolRegistry_Count(t *testing.T) {
	registry := NewToolRegistry()

	if registry.Count() != 0 {
		t.Errorf("Expected count 0, got %d", registry.Count())
	}

	registry.Register(&mockTool{
		name:        "tool1",
		description: "First",
		schema:      &InputSchema{Type: "object"},
	})

	if registry.Count() != 1 {
		t.Errorf("Expected count 1, got %d", registry.Count())
	}

	registry.Register(&mockTool{
		name:        "tool2",
		description: "Second",
		schema:      &InputSchema{Type: "object"},
	})

	if registry.Count() != 2 {
		t.Errorf("Expected count 2, got %d", registry.Count())
	}
}

func TestToolRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewToolRegistry()

	// Register tools concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			tool := &mockTool{
				name:        "concurrent_tool",
				description: "Concurrent test",
				schema:      &InputSchema{Type: "object"},
			}
			registry.Register(tool)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Only one should have succeeded (no duplicates)
	if registry.Count() != 1 {
		t.Errorf("Expected 1 tool after concurrent registration, got %d", registry.Count())
	}
}

func TestIsValidToolName(t *testing.T) {
	validNames := []string{
		"valid_tool",
		"tool123",
		"a",
		"tool_with_underscores_and_123",
	}

	for _, name := range validNames {
		if !isValidToolName(name) {
			t.Errorf("Expected '%s' to be valid", name)
		}
	}

	invalidNames := []string{
		"",
		"InvalidTool",
		"invalid-tool",
		"invalid tool",
		"1invalid",
		"_invalid",
		"tool@",
		"tool!",
	}

	for _, name := range invalidNames {
		if isValidToolName(name) {
			t.Errorf("Expected '%s' to be invalid", name)
		}
	}
}

func TestValidateInputSchema(t *testing.T) {
	validSchemas := []*InputSchema{
		{Type: "object"},
		{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"param": {Type: "string"},
			},
		},
		{
			Type:     "object",
			Required: []string{"param1", "param2"},
		},
	}

	for _, schema := range validSchemas {
		err := validateInputSchema(schema)
		if err != nil {
			t.Errorf("Expected valid schema to pass validation: %v", err)
		}
	}

	invalidSchemas := []struct {
		schema      *InputSchema
		expectedErr string
	}{
		{nil, ErrSchemaNil},
		{&InputSchema{Type: ""}, ErrSchemaMissingType},
		{&InputSchema{Type: "array"}, ErrSchemaInvalidType},
	}

	for _, tc := range invalidSchemas {
		err := validateInputSchema(tc.schema)
		if err == nil {
			t.Fatal("Expected error for invalid schema, got nil")
		}
		if err.Error() != tc.expectedErr {
			t.Errorf("Expected error '%s', got '%s'", tc.expectedErr, err.Error())
		}
	}
}
