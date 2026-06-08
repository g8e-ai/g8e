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

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
)

func TestGenerateMCPConfig(t *testing.T) {
	cfg := &config.Config{
		ProjectRoot: "/test/path",
	}

	config := generateMCPConfig(cfg)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(config), &parsed); err != nil {
		t.Fatalf("Failed to parse generated config: %v", err)
	}

	if _, ok := parsed["command"]; !ok {
		t.Error("Generated config missing 'command' field")
	}

	if parsed["command"] != "/test/path/g8e" {
		t.Errorf("Expected command '/test/path/g8e', got '%v'", parsed["command"])
	}

	if _, ok := parsed["args"]; !ok {
		t.Error("Generated config missing 'args' field")
	}

	// Ensure transport/capabilities nesting is NOT present (bug fix verification)
	if _, ok := parsed["transport"]; ok {
		t.Error("Generated config should not have 'transport' field (should be top-level command/args)")
	}

	if _, ok := parsed["capabilities"]; ok {
		t.Error("Generated config should not have 'capabilities' field (should be top-level command/args)")
	}
}

func TestStripJSONComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no comments",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "single line comment",
			input:    `{"key": "value"} // comment`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "comment in string",
			input:    `{"key": "value // not a comment"}`,
			expected: `{"key": "value // not a comment"}`,
		},
		{
			name:     "multi-line comment",
			input:    `{"key": "value"} /* comment */`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "escaped quote in string",
			input:    `{"key": "value with \"escaped\" quote"}`,
			expected: `{"key": "value with \"escaped\" quote"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripJSONComments(tt.input)
			if result != tt.expected {
				t.Errorf("stripJSONComments() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMergeMCPConfig(t *testing.T) {
	tests := []struct {
		name        string
		existing    map[string]interface{}
		mcpConfig   string
		wantErr     bool
		checkResult func(map[string]interface{}) error
	}{
		{
			name:      "merge into empty config",
			existing:  map[string]interface{}{},
			mcpConfig: `{"command": "/test/g8e", "args": ["mcp", "gov"]}`,
			wantErr:   false,
			checkResult: func(result map[string]interface{}) error {
				if result["mcpServers"] == nil {
					t.Error("Expected mcpServers to be set")
				}
				return nil
			},
		},
		{
			name: "merge into existing mcpServers",
			existing: map[string]interface{}{
				"mcpServers": map[string]interface{}{
					"existing-server": map[string]interface{}{
						"command": "/other/cmd",
					},
				},
			},
			mcpConfig: `{"command": "/test/g8e", "args": ["mcp", "gov"]}`,
			wantErr:   false,
			checkResult: func(result map[string]interface{}) error {
				mcpServers, ok := result["mcpServers"].(map[string]interface{})
				if !ok {
					t.Error("Expected mcpServers to be a map")
					return nil
				}
				if _, ok := mcpServers["existing-server"]; !ok {
					t.Error("Expected existing-server to be preserved")
				}
				if _, ok := mcpServers["g8e-gateway"]; !ok {
					t.Error("Expected g8e-gateway to be added")
				}
				return nil
			},
		},
		{
			name: "overwrite existing g8e-gateway config",
			existing: map[string]interface{}{
				"mcpServers": map[string]interface{}{
					"g8e-gateway": map[string]interface{}{
						"command": "/old/g8e",
					},
				},
			},
			mcpConfig: `{"command": "/new/g8e", "args": ["mcp", "gov"]}`,
			wantErr:   false,
			checkResult: func(result map[string]interface{}) error {
				mcpServers, ok := result["mcpServers"].(map[string]interface{})
				if !ok {
					t.Error("Expected mcpServers to be a map")
					return nil
				}
				g8eConfig, ok := mcpServers["g8e-gateway"].(map[string]interface{})
				if !ok {
					t.Error("Expected g8e-gateway to be a map")
					return nil
				}
				if g8eConfig["command"] != "/new/g8e" {
					t.Errorf("Expected command to be overwritten to /new/g8e, got %v", g8eConfig["command"])
				}
				return nil
			},
		},
		{
			name: "mcpServers is not a map (should be replaced)",
			existing: map[string]interface{}{
				"mcpServers": "invalid",
			},
			mcpConfig: `{"command": "/test/g8e", "args": ["mcp", "gov"]}`,
			wantErr:   false,
			checkResult: func(result map[string]interface{}) error {
				mcpServers, ok := result["mcpServers"].(map[string]interface{})
				if !ok {
					t.Error("Expected mcpServers to be replaced with a map")
					return nil
				}
				if _, ok := mcpServers["g8e-gateway"]; !ok {
					t.Error("Expected g8e-gateway to be added")
				}
				return nil
			},
		},
		{
			name: "config with other fields preserved",
			existing: map[string]interface{}{
				"otherField":   "value",
				"anotherField": 123,
			},
			mcpConfig: `{"command": "/test/g8e", "args": ["mcp", "gov"]}`,
			wantErr:   false,
			checkResult: func(result map[string]interface{}) error {
				if result["otherField"] != "value" {
					t.Error("Expected otherField to be preserved")
				}
				if result["anotherField"] != 123 {
					t.Error("Expected anotherField to be preserved")
				}
				return nil
			},
		},
		{
			name:      "invalid mcpConfig JSON",
			existing:  map[string]interface{}{},
			mcpConfig: `{invalid json}`,
			wantErr:   true,
		},
		{
			name:      "empty mcpConfig JSON",
			existing:  map[string]interface{}{},
			mcpConfig: `{}`,
			wantErr:   false,
			checkResult: func(result map[string]interface{}) error {
				mcpServers, ok := result["mcpServers"].(map[string]interface{})
				if !ok {
					t.Error("Expected mcpServers to be a map")
					return nil
				}
				if _, ok := mcpServers["g8e-gateway"]; !ok {
					t.Error("Expected g8e-gateway to be added even with empty config")
				}
				return nil
			},
		},
		{
			name:      "complex nested mcpConfig",
			existing:  map[string]interface{}{},
			mcpConfig: `{"command": "/test/g8e", "args": ["mcp", "gov"], "env": {"KEY": "value"}}`,
			wantErr:   false,
			checkResult: func(result map[string]interface{}) error {
				mcpServers, ok := result["mcpServers"].(map[string]interface{})
				if !ok {
					t.Error("Expected mcpServers to be a map")
					return nil
				}
				g8eConfig, ok := mcpServers["g8e-gateway"].(map[string]interface{})
				if !ok {
					t.Error("Expected g8e-gateway to be a map")
					return nil
				}
				if g8eConfig["env"] == nil {
					t.Error("Expected env field to be preserved")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mergeMCPConfig(tt.existing, tt.mcpConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("mergeMCPConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkResult != nil {
				tt.checkResult(tt.existing)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "expand tilde",
			input:    "~/test",
			expected: filepath.Join(home, "test"),
		},
		{
			name:     "expand tilde with nested path",
			input:    "~/config/file.json",
			expected: filepath.Join(home, "config/file.json"),
		},
		{
			name:     "absolute path unchanged",
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			name:     "relative path unchanged",
			input:    "relative/path",
			expected: "relative/path",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPath(tt.input)
			if result != tt.expected {
				t.Errorf("expandPath() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDiscoverTools(t *testing.T) {
	tools, err := discoverTools()
	if err != nil {
		t.Fatalf("discoverTools() error = %v", err)
	}

	// discoverTools should return a slice (may be empty if no tools installed)
	if tools == nil {
		t.Error("discoverTools() returned nil, expected slice")
	}
}

func TestGetToolConfigPaths(t *testing.T) {
	paths := getToolConfigPaths()

	if paths == nil {
		t.Fatal("getToolConfigPaths() returned nil")
	}

	// Verify all expected tools are present
	expectedTools := []string{"claude", "cursor", "code", "cline"}
	for _, tool := range expectedTools {
		if _, ok := paths[tool]; !ok {
			t.Errorf("getToolConfigPaths() missing tool %q", tool)
		}
	}

	// Verify platform-specific paths
	switch runtime.GOOS {
	case "windows":
		// Windows should use AppData paths
		for _, toolPaths := range paths {
			for _, path := range toolPaths {
				if !filepath.IsAbs(path) && path[0] != '~' {
					t.Errorf("Windows path should be absolute or start with ~, got %q", path)
				}
			}
		}
	case "darwin":
		// macOS should use Library/Application Support or ~/.cursor
		for tool, toolPaths := range paths {
			for _, path := range toolPaths {
				if tool == "cursor" {
					if !filepath.IsAbs(path) && path[0] != '~' {
						t.Errorf("macOS cursor path should be absolute or start with ~, got %q", path)
					}
				} else {
					if !filepath.IsAbs(path) && path[0] != '~' {
						t.Errorf("macOS path should be absolute or start with ~, got %q", path)
					}
				}
			}
		}
	default: // linux and others
		// Linux should use ~/.config paths
		for _, toolPaths := range paths {
			for _, path := range toolPaths {
				if !filepath.IsAbs(path) && path[0] != '~' {
					t.Errorf("Linux path should be absolute or start with ~, got %q", path)
				}
			}
		}
	}
}
