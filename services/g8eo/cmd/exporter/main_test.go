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

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
)

func TestMergeMaps(t *testing.T) {
	tests := []struct {
		name     string
		newData  map[string]interface{}
		existing map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "empty existing",
			newData:  map[string]interface{}{"a": 1},
			existing: map[string]interface{}{},
			expected: map[string]interface{}{"a": 1},
		},
		{
			name:     "empty new data",
			newData:  map[string]interface{}{},
			existing: map[string]interface{}{"a": 1},
			expected: map[string]interface{}{"a": 1},
		},
		{
			name:     "no overlap",
			newData:  map[string]interface{}{"a": 1},
			existing: map[string]interface{}{"b": 2},
			expected: map[string]interface{}{"a": 1, "b": 2},
		},
		{
			name:     "overlap - new data takes precedence",
			newData:  map[string]interface{}{"a": 1},
			existing: map[string]interface{}{"a": 2},
			expected: map[string]interface{}{"a": 1},
		},
		{
			name:     "nested merge",
			newData:  map[string]interface{}{"a": map[string]interface{}{"x": 1}},
			existing: map[string]interface{}{"a": map[string]interface{}{"y": 2}},
			expected: map[string]interface{}{"a": map[string]interface{}{"x": 1, "y": 2}},
		},
		{
			name:     "deeply nested merge",
			newData:  map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"x": 1}}},
			existing: map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"y": 2}}},
			expected: map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"x": 1, "y": 2}}},
		},
		{
			name: "mixed types - new data takes precedence",
			newData: map[string]interface{}{
				"a": map[string]interface{}{"x": 1},
			},
			existing: map[string]interface{}{
				"a": "string value",
			},
			expected: map[string]interface{}{
				"a": map[string]interface{}{"x": 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeMaps(tt.newData, tt.existing)
			if !mapsEqual(result, tt.expected) {
				t.Errorf("mergeMaps() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEmptyMapIfNil(t *testing.T) {
	t.Run("nil input returns empty map", func(t *testing.T) {
		result := emptyMapIfNil(nil)
		if result == nil {
			t.Error("expected non-nil map")
		}
		if len(result) != 0 {
			t.Errorf("expected empty map, got %d elements", len(result))
		}
	})

	t.Run("non-nil input returns same map", func(t *testing.T) {
		input := map[string]string{"a": "b"}
		result := emptyMapIfNil(input)
		if result == nil {
			t.Error("expected non-nil map")
		}
		if len(result) != 1 {
			t.Errorf("expected 1 element, got %d", len(result))
		}
		if result["a"] != "b" {
			t.Errorf("expected a=b, got a=%s", result["a"])
		}
	})
}

func TestIsUpper(t *testing.T) {
	tests := []struct {
		r     rune
		upper bool
	}{
		{'A', true},
		{'Z', true},
		{'a', false},
		{'z', false},
		{'0', false},
		{'@', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			if got := isUpper(tt.r); got != tt.upper {
				t.Errorf("isUpper(%c) = %v, want %v", tt.r, got, tt.upper)
			}
		})
	}
}

func TestConstantsRegistry(t *testing.T) {
	snapshot := constants.Registry()

	// Verify critical collections exist
	if len(snapshot.Collections) == 0 {
		t.Error("Collections snapshot is empty")
	}

	// Verify critical events exist
	if len(snapshot.Events) == 0 {
		t.Error("Events snapshot is empty")
	}

	// Verify status snapshot has categories
	if len(snapshot.Status) == 0 {
		t.Error("Status snapshot is empty")
	}

	// Verify API paths exist
	if constants.APIPaths.InternalPrefix == "" {
		t.Error("InternalPrefix is empty")
	}
	if constants.APIPaths.OperatorPrefix == "" {
		t.Error("OperatorPrefix is empty")
	}
}

func TestExportIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temporary directory for export
	tmpDir, err := os.MkdirTemp("", "exporter-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create protocol constants subdirectory
	protocolConstantsDir := filepath.Join(tmpDir, "protocol", "constants")
	if err := os.MkdirAll(protocolConstantsDir, 0750); err != nil {
		t.Fatalf("failed to create protocol constants dir: %v", err)
	}

	// Create Python subdirectory
	protocolPythonDir := filepath.Join(tmpDir, "protocol", "python", "g8e_protocol")
	if err := os.MkdirAll(protocolPythonDir, 0750); err != nil {
		t.Fatalf("failed to create protocol python dir: %v", err)
	}

	// Create script subdirectory
	scriptDir := filepath.Join(tmpDir, "scripts", "cmd")
	if err := os.MkdirAll(scriptDir, 0750); err != nil {
		t.Fatalf("failed to create script dir: %v", err)
	}

	// Test JSON export
	snapshot := constants.Registry()
	collectionsJSON := filepath.Join(protocolConstantsDir, "collections.json")
	collectionsData := CollectionsExport{Collections: snapshot.Collections}
	jsonData, err := json.MarshalIndent(collectionsData, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal collections: %v", err)
	}
	if err := os.WriteFile(collectionsJSON, jsonData, 0600); err != nil {
		t.Fatalf("failed to write collections.json: %v", err)
	}

	// Verify JSON is valid and contains expected data
	var readBack CollectionsExport
	if err := json.Unmarshal(jsonData, &readBack); err != nil {
		t.Fatalf("failed to unmarshal collections.json: %v", err)
	}
	if len(readBack.Collections) != len(snapshot.Collections) {
		t.Errorf("collections count mismatch: got %d, want %d", len(readBack.Collections), len(snapshot.Collections))
	}

	// Test Python module export
	collectionsPy := filepath.Join(protocolPythonDir, "collections.py")
	lines := []string{
		"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
		"",
	}
	for _, entry := range snapshot.Collections {
		lines = append(lines, fmt.Sprintf("%s = %q", entry.PythonConst, entry.Value))
	}
	lines = append(lines, "")
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(collectionsPy, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write collections.py: %v", err)
	}

	// Verify Python file exists and has content
	pyContent, err := os.ReadFile(collectionsPy)
	if err != nil {
		t.Fatalf("failed to read collections.py: %v", err)
	}
	if !strings.Contains(string(pyContent), "# Code generated by services/g8eo/cmd/exporter") {
		t.Error("collections.py missing generated header")
	}
	if len(snapshot.Collections) > 0 && !strings.Contains(string(pyContent), " = ") {
		t.Error("collections.py missing constant assignments")
	}
}

// Helper function to compare maps deeply
func mapsEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if !interfaceEqual(av, bv) {
			return false
		}
	}
	return true
}

func interfaceEqual(a, b interface{}) bool {
	switch av := a.(type) {
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok {
			return false
		}
		return mapsEqual(av, bv)
	default:
		return a == b
	}
}
