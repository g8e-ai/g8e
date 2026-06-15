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

//go:build integration || e2e

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/mcp"
)

func TestRegisterNativeTools(t *testing.T) {
	registry := mcp.NewToolRegistry()

	err := mcp.RegisterNativeTools(registry)
	if err != nil {
		t.Fatalf("RegisterNativeTools failed: %v", err)
	}

	// Verify all expected tools are registered
	expectedTools := []string{
		"db_discover_topology",
		"db_query_validate",
		"db_isolated_read",
		"db_index_triage",
		"log_stream_filter",
		"sys_oom_detect",
		"config_diff_mask",
		"proc_metric_top",
		"fs_disk_profile",
		"proc_signal_safe",
		"net_socket_audit",
		"net_endpoint_ping",
		"net_http_probe",
		"sys_info",
		"net_dns_resolve",
		"tls_cert_inspect",
		"sys_env_vars",
		"fs_file_checksum",
		"sys_service_status",
		"sys_container_status",
		"fs_disk_usage",
		"sys_time_clock",
		"proc_tree",
		"git_ops",
		"cloud_metadata",
		"k8s_inspect",
		"run_shell_command",
		"net_ssh_known_hosts",
		"operator_deploy",
		"read_file",
	}

	for _, toolName := range expectedTools {
		tool, ok := registry.Get(toolName)
		if !ok {
			t.Errorf("Expected tool '%s' to be registered", toolName)
			continue
		}

		// Verify tool has required interface methods
		if tool.Name() == "" {
			t.Errorf("Tool '%s' has empty name", toolName)
		}
		if tool.Description() == "" {
			t.Errorf("Tool '%s' has empty description", toolName)
		}
		if tool.InputSchema() == nil {
			t.Errorf("Tool '%s' has nil input schema", toolName)
		}
	}

	// Verify count matches expected
	count := registry.Count()
	if count != len(expectedTools) {
		t.Errorf("Expected %d registered tools, got %d", len(expectedTools), count)
	}
}

func TestRegisterNativeTools_DuplicateRegistration(t *testing.T) {
	registry := mcp.NewToolRegistry()

	// First registration should succeed
	err := mcp.RegisterNativeTools(registry)
	if err != nil {
		t.Fatalf("First RegisterNativeTools failed: %v", err)
	}

	// Second registration should fail due to duplicates
	err = mcp.RegisterNativeTools(registry)
	if err == nil {
		t.Fatal("Expected error when registering tools twice, got nil")
	}

	// Verify the error mentions duplicate registration
	expectedError := "already registered"
	if err.Error() == "" || !containsSubstring(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestRegisterNativeTools_NilRegistry(t *testing.T) {
	err := mcp.RegisterNativeTools(nil)
	if err == nil {
		t.Fatal("Expected error when registering to nil registry, got nil")
	}
	expectedError := "cannot register to nil registry"
	if !containsSubstring(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestRegisterNativeTools_ToolNameConsistency(t *testing.T) {
	registry := mcp.NewToolRegistry()

	err := mcp.RegisterNativeTools(registry)
	if err != nil {
		t.Fatalf("RegisterNativeTools failed: %v", err)
	}

	// Verify all tool names follow the naming convention (lowercase with underscores)
	tools := registry.List()
	for _, tool := range tools {
		name := tool.Name()
		if !isValidToolName(name) {
			t.Errorf("Tool name '%s' does not follow naming convention", name)
		}
	}
}

func TestRegisterNativeTools_SchemaValidity(t *testing.T) {
	registry := mcp.NewToolRegistry()

	err := mcp.RegisterNativeTools(registry)
	if err != nil {
		t.Fatalf("RegisterNativeTools failed: %v", err)
	}

	// Verify all tool schemas are valid
	tools := registry.List()
	for _, tool := range tools {
		schema := tool.InputSchema()
		err := validateInputSchema(schema)
		if err != nil {
			t.Errorf("Tool '%s' has invalid schema: %v", tool.Name(), err)
		}
	}
}

func TestRegisterNativeTools_PartialRegistration(t *testing.T) {
	registry := mcp.NewToolRegistry()

	// Manually register one tool first
	manualTool := &mockTool{
		name:        "db_discover_topology",
		description: "Manually registered",
		schema:      &mcp.InputSchema{Type: "object"},
	}
	err := registry.Register(manualTool)
	if err != nil {
		t.Fatalf("Failed to manually register tool: %v", err)
	}

	// Attempting to register all native tools should fail due to duplicate
	err = mcp.RegisterNativeTools(registry)
	if err == nil {
		t.Fatal("Expected error when some tools already registered, got nil")
	}

	// Verify the manually registered tool is still present
	_, ok := registry.Get("db_discover_topology")
	if !ok {
		t.Error("Manually registered tool should still be present")
	}
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper types for testing
type mockTool struct {
	name        string
	description string
	schema      *mcp.InputSchema
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.description }
func (m *mockTool) InputSchema() *mcp.InputSchema {
	return m.schema
}
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (mcp.CallToolResult, error) {
	return mcp.CallToolResult{
		Content: []mcp.TextContent{
			{Type: "text", Text: "mock execution"},
		},
	}, nil
}

func isValidToolName(name string) bool {
	// Simple validation: tool names should be lowercase alphanumeric with underscores
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func validateInputSchema(schema *mcp.InputSchema) error {
	// Basic validation: schema should not be nil and should have a type
	if schema == nil {
		return fmt.Errorf("schema is nil")
	}
	if schema.Type == "" {
		return fmt.Errorf("schema type is empty")
	}
	return nil
}
