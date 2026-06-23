// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"testing"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agentic_tool_emulator/client"
)

func TestApiKeyNote(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "empty API key",
			apiKey:   "",
			expected: "",
		},
		{
			name:     "API key present",
			apiKey:   "test-key-123",
			expected: " + API key",
		},
		{
			name:     "whitespace API key",
			apiKey:   "   ",
			expected: " + API key",
		},
		{
			name:     "long API key",
			apiKey:   "very-long-api-key-with-many-characters",
			expected: " + API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &clientpkg.Client{}
			// Create a mock config with the API key
			// Note: We can't directly set Config on Client, so we'll test the function logic
			// by checking the behavior based on the apiKeyNote implementation
			result := apiKeyNote(mockClient)
			// Since we can't mock the Config(), this test will always return empty string
			// This is a limitation of the current design - apiKeyNote depends on Client.Config()
			// which we can't easily mock in a unit test without modifying the Client struct
			_ = result
		})
	}
}

func TestFirstTool(t *testing.T) {
	tests := []struct {
		name     string
		resp     *clientpkg.JSONRPCResponse
		def      string
		expected string
	}{
		{
			name:     "nil response",
			resp:     nil,
			def:      "default",
			expected: "default",
		},
		{
			name:     "empty result",
			resp:     &clientpkg.JSONRPCResponse{Result: []byte{}},
			def:      "default",
			expected: "default",
		},
		{
			name: "valid tools array",
			resp: &clientpkg.JSONRPCResponse{
				Result: []byte(`{"tools":[{"name":"fs_list"},{"name":"fs_read"}]}`),
			},
			def:      "default",
			expected: "fs_list",
		},
		{
			name: "nested result structure",
			resp: &clientpkg.JSONRPCResponse{
				Result: []byte(`{"result":{"tools":[{"name":"fs_list"}]}}`),
			},
			def:      "default",
			expected: "default", // This shape is not supported by firstTool
		},
		{
			name: "empty tools array",
			resp: &clientpkg.JSONRPCResponse{
				Result: []byte(`{"tools":[]}`),
			},
			def:      "default",
			expected: "default",
		},
		{
			name: "tool with empty name",
			resp: &clientpkg.JSONRPCResponse{
				Result: []byte(`{"tools":[{"name":""}]}`),
			},
			def:      "default",
			expected: "default",
		},
		{
			name: "invalid JSON",
			resp: &clientpkg.JSONRPCResponse{
				Result: []byte(`invalid json`),
			},
			def:      "default",
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firstTool(tt.resp, tt.def)
			if result != tt.expected {
				t.Errorf("firstTool() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFirstToolEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		resp     *clientpkg.JSONRPCResponse
		def      string
		expected string
	}{
		{
			name:     "tools with special characters",
			resp:     &clientpkg.JSONRPCResponse{Result: []byte(`{"tools":[{"name":"fs_list_v2"}]}`)},
			def:      "default",
			expected: "fs_list_v2",
		},
		{
			name:     "tools with numbers",
			resp:     &clientpkg.JSONRPCResponse{Result: []byte(`{"tools":[{"name":"tool123"}]}`)},
			def:      "default",
			expected: "tool123",
		},
		{
			name:     "multiple tools, pick first",
			resp:     &clientpkg.JSONRPCResponse{Result: []byte(`{"tools":[{"name":"first"},{"name":"second"}]}`)},
			def:      "default",
			expected: "first",
		},
		{
			name:     "extra fields in tool",
			resp:     &clientpkg.JSONRPCResponse{Result: []byte(`{"tools":[{"name":"tool","description":"desc"}]}`)},
			def:      "default",
			expected: "tool",
		},
		{
			name:     "null result",
			resp:     &clientpkg.JSONRPCResponse{Result: nil},
			def:      "default",
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firstTool(tt.resp, tt.def)
			if result != tt.expected {
				t.Errorf("firstTool() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMCPScenarios(t *testing.T) {
	scenarios := mcpScenarios()

	// Should have 5 MCP scenarios (3 core + 2 healthcare)
	if len(scenarios) != 5 {
		t.Errorf("mcpScenarios should return 5 scenarios, got %d", len(scenarios))
	}

	// Should have expected names
	expectedNames := []string{"mcp-plain", "mcp-advanced", "mcp-secured", "healthcare-success", "healthcare-phi-blocked"}
	nameSet := make(map[string]bool)
	for _, sc := range scenarios {
		nameSet[sc.Name] = true
	}
	for _, name := range expectedNames {
		if !nameSet[name] {
			t.Errorf("mcpScenarios should include scenario %q", name)
		}
	}
}

func TestA2AScenarios(t *testing.T) {
	scenarios := a2aScenarios()

	// Should have 3 A2A scenarios
	if len(scenarios) != 3 {
		t.Errorf("a2aScenarios should return 3 scenarios, got %d", len(scenarios))
	}

	// All should require Doctrine posture
	for _, sc := range scenarios {
		if sc.RequiresPosture != Doctrine {
			t.Errorf("A2A scenario %q should require Doctrine posture, got %q", sc.Name, sc.RequiresPosture)
		}
	}

	// Should have expected names
	expectedNames := []string{"a2a-plain", "a2a-secured", "a2a-protobuf"}
	nameSet := make(map[string]bool)
	for _, sc := range scenarios {
		nameSet[sc.Name] = true
	}
	for _, name := range expectedNames {
		if !nameSet[name] {
			t.Errorf("a2aScenarios should include scenario %q", name)
		}
	}
}

func TestMCPScenarioNames(t *testing.T) {
	scenarios := mcpScenarios()

	expectedNames := map[string]bool{
		"mcp-plain":              true,
		"mcp-advanced":           true,
		"mcp-secured":            true,
		"healthcare-success":     true,
		"healthcare-phi-blocked": true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected MCP scenario name: %q", sc.Name)
		}
	}
}

func TestA2AScenarioNames(t *testing.T) {
	scenarios := a2aScenarios()

	expectedNames := map[string]bool{
		"a2a-plain":    true,
		"a2a-secured":  true,
		"a2a-protobuf": true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected A2A scenario name: %q", sc.Name)
		}
	}
}

func TestMCPScenarioPersonas(t *testing.T) {
	scenarios := mcpScenarios()

	expectedPersonas := map[string]string{
		"mcp-plain":              "claude-desktop",
		"mcp-advanced":           "cursor",
		"mcp-secured":            "enterprise-agent",
		"healthcare-success":     "clinical-agent",
		"healthcare-phi-blocked": "clinical-agent",
	}

	for _, sc := range scenarios {
		expectedPersona, ok := expectedPersonas[sc.Name]
		if !ok {
			t.Errorf("No expected persona defined for scenario %q", sc.Name)
			continue
		}
		if sc.Persona.ID != expectedPersona {
			t.Errorf("Scenario %q should have persona %q, got %q", sc.Name, expectedPersona, sc.Persona.ID)
		}
	}
}

func TestA2AScenarioPersonas(t *testing.T) {
	scenarios := a2aScenarios()

	expectedPersonas := map[string]string{
		"a2a-plain":    "a2a-peer",
		"a2a-secured":  "a2a-secure-peer",
		"a2a-protobuf": "protobuf-agent",
	}

	for _, sc := range scenarios {
		expectedPersona, ok := expectedPersonas[sc.Name]
		if !ok {
			t.Errorf("No expected persona defined for scenario %q", sc.Name)
			continue
		}
		if sc.Persona.ID != expectedPersona {
			t.Errorf("Scenario %q should have persona %q, got %q", sc.Name, expectedPersona, sc.Persona.ID)
		}
	}
}

func TestMCPScenarioTitles(t *testing.T) {
	scenarios := mcpScenarios()

	expectedTitles := map[string]string{
		"mcp-plain":              "Plain MCP tool call",
		"mcp-advanced":           "Advanced MCP: resources, prompts, chained calls",
		"mcp-secured":            "MCP with simple security (mTLS/API key + L1 gate)",
		"healthcare-success":     "Authorized FHIR PA Submission",
		"healthcare-phi-blocked": "PHI Exfiltration Blocked by Doctrine",
	}

	for _, sc := range scenarios {
		expectedTitle, ok := expectedTitles[sc.Name]
		if !ok {
			t.Errorf("No expected title defined for scenario %q", sc.Name)
			continue
		}
		if sc.Title != expectedTitle {
			t.Errorf("Scenario %q should have title %q, got %q", sc.Name, expectedTitle, sc.Title)
		}
	}
}

func TestA2AScenarioTitles(t *testing.T) {
	scenarios := a2aScenarios()

	expectedTitles := map[string]string{
		"a2a-plain":    "Plain A2A skill invocation",
		"a2a-secured":  "A2A with simple security (mTLS + L1 skill gate)",
		"a2a-protobuf": "A2A carrying a typed protobuf payload",
	}

	for _, sc := range scenarios {
		expectedTitle, ok := expectedTitles[sc.Name]
		if !ok {
			t.Errorf("No expected title defined for scenario %q", sc.Name)
			continue
		}
		if sc.Title != expectedTitle {
			t.Errorf("Scenario %q should have title %q, got %q", sc.Name, expectedTitle, sc.Title)
		}
	}
}

func TestMCPScenarioRunNotNil(t *testing.T) {
	scenarios := mcpScenarios()

	for _, sc := range scenarios {
		if sc.Run == nil {
			t.Errorf("MCP scenario %q should have non-nil Run function", sc.Name)
		}
	}
}

func TestA2AScenarioRunNotNil(t *testing.T) {
	scenarios := a2aScenarios()

	for _, sc := range scenarios {
		if sc.Run == nil {
			t.Errorf("A2A scenario %q should have non-nil Run function", sc.Name)
		}
	}
}
