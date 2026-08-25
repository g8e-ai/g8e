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
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.NotNil(t, registry, "NewToolRegistry returned nil")
	assert.Equal(t, 0, registry.Count(), "Expected empty registry")
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
	require.NoError(t, err, "Failed to register tool")

	assert.Equal(t, 1, registry.Count(), "Expected 1 tool")

	// Verify tool can be retrieved
	retrieved, ok := registry.Get("test_tool")
	assert.True(t, ok, "Tool not found after registration")
	assert.Equal(t, "test_tool", retrieved.Name(), "Retrieved wrong tool")
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
	require.NoError(t, err, "Failed to register first tool")

	err = registry.Register(tool2)
	assert.Error(t, err, "Expected error when registering duplicate tool")
	assert.ErrorIs(t, err, constants.ErrMCPToolAlreadyRegistered, "Error should wrap ErrMCPToolAlreadyRegistered")
}

func TestToolRegistry_RegisterNil(t *testing.T) {
	registry := NewToolRegistry()

	err := registry.Register(nil)
	assert.Error(t, err, "Expected error when registering nil tool")
	assert.ErrorIs(t, err, constants.ErrMCPToolNil, "Error should be ErrMCPToolNil")
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
	assert.Error(t, err, "Expected error when registering tool with empty name")
	assert.ErrorIs(t, err, constants.ErrMCPToolNameEmpty, "Error should be ErrMCPToolNameEmpty")
}

func TestToolRegistry_RegisterInvalidName(t *testing.T) {
	registry := NewToolRegistry()

	testCases := []struct {
		name     string
		toolName string
	}{
		{"uppercase letters", "InvalidTool"},
		{"hyphens", "invalid-tool"},
		{"spaces", "invalid tool"},
		{"starts with digit", "1invalid"},
		{"starts with underscore", "_invalid"},
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
			assert.Error(t, err, "Expected error for tool name '%s'", tc.toolName)
			assert.ErrorIs(t, err, constants.ErrMCPToolNameInvalid, "Error should wrap ErrMCPToolNameInvalid")
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
		assert.NoError(t, err, "Expected no error for valid name '%s'", name)
	}
}

func TestToolRegistry_RegisterInvalidSchema(t *testing.T) {
	registry := NewToolRegistry()

	testCases := []struct {
		name   string
		schema *InputSchema
		err    error
	}{
		{"nil schema", nil, constants.ErrMCPSchemaNil},
		{"missing type", &InputSchema{}, constants.ErrMCPSchemaMissingType},
		{"invalid type", &InputSchema{Type: "array"}, constants.ErrMCPSchemaInvalidType},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &mockTool{
				name:        "test_tool",
				description: "Test tool",
				schema:      tc.schema,
			}

			err := registry.Register(tool)
			assert.Error(t, err, "Expected error for invalid schema")
			assert.ErrorIs(t, err, tc.err, "Error should wrap %v", tc.err)
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
	assert.True(t, ok, "Failed to get existing tool")
	assert.Equal(t, "get_test", retrieved.Name(), "Got wrong tool")

	// Test getting non-existent tool
	_, ok = registry.Get("non_existent")
	assert.False(t, ok, "Expected false for non-existent tool")
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
	assert.Equal(t, 3, len(listed), "Expected 3 tools")

	// Verify all tools are present
	names := make(map[string]bool)
	for _, tool := range listed {
		names[tool.Name()] = true
	}

	for _, expectedName := range []string{"tool1", "tool2", "tool3"} {
		assert.True(t, names[expectedName], "Expected tool '%s' in list", expectedName)
	}
}

func TestToolRegistry_Count(t *testing.T) {
	registry := NewToolRegistry()

	assert.Equal(t, 0, registry.Count(), "Expected count 0")

	registry.Register(&mockTool{
		name:        "tool1",
		description: "First",
		schema:      &InputSchema{Type: "object"},
	})

	assert.Equal(t, 1, registry.Count(), "Expected count 1")

	registry.Register(&mockTool{
		name:        "tool2",
		description: "Second",
		schema:      &InputSchema{Type: "object"},
	})

	assert.Equal(t, 2, registry.Count(), "Expected count 2")
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
	assert.Equal(t, 1, registry.Count(), "Expected 1 tool after concurrent registration")
}

func TestIsValidToolName(t *testing.T) {
	validNames := []string{
		"valid_tool",
		"tool123",
		"a",
		"tool_with_underscores_and_123",
	}

	for _, name := range validNames {
		assert.True(t, isValidToolName(name), "Expected '%s' to be valid", name)
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
		assert.False(t, isValidToolName(name), "Expected '%s' to be invalid", name)
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
		assert.NoError(t, err, "Expected valid schema to pass validation")
	}

	invalidSchemas := []struct {
		schema *InputSchema
		err    error
	}{
		{nil, constants.ErrMCPSchemaNil},
		{&InputSchema{Type: ""}, constants.ErrMCPSchemaMissingType},
		{&InputSchema{Type: "array"}, constants.ErrMCPSchemaInvalidType},
	}

	for _, tc := range invalidSchemas {
		err := validateInputSchema(tc.schema)
		assert.Error(t, err, "Expected error for invalid schema")
		assert.ErrorIs(t, err, tc.err, "Error should be %v", tc.err)
	}
}
