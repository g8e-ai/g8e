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

package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectRootConsistency(t *testing.T) {
	// Save original directory and restore after test
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	// Get the expected root from the current directory (should be services/g8eo)
	expectedRoot := ResolveProjectRoot()

	// Test from services/g8eo
	testDir := filepath.Join(expectedRoot, "services", "g8eo")
	if err := os.Chdir(testDir); err != nil {
		t.Fatalf("Failed to chdir to %s: %v", testDir, err)
	}
	rootFromG8eo := ResolveProjectRoot()
	if rootFromG8eo != expectedRoot {
		t.Errorf("ResolveProjectRoot from services/g8eo: got %s, want %s", rootFromG8eo, expectedRoot)
	}

	// Test from services/g8ee
	testDir = filepath.Join(expectedRoot, "services", "g8ee")
	if err := os.Chdir(testDir); err != nil {
		t.Fatalf("Failed to chdir to %s: %v", testDir, err)
	}
	rootFromG8ee := ResolveProjectRoot()
	if rootFromG8ee != expectedRoot {
		t.Errorf("ResolveProjectRoot from services/g8ee: got %s, want %s", rootFromG8ee, expectedRoot)
	}

	// Test from scripts
	testDir = filepath.Join(expectedRoot, "scripts")
	if err := os.Chdir(testDir); err != nil {
		t.Fatalf("Failed to chdir to %s: %v", testDir, err)
	}
	rootFromScripts := ResolveProjectRoot()
	if rootFromScripts != expectedRoot {
		t.Errorf("ResolveProjectRoot from scripts: got %s, want %s", rootFromScripts, expectedRoot)
	}

	// Test from project root
	if err := os.Chdir(expectedRoot); err != nil {
		t.Fatalf("Failed to chdir to %s: %v", expectedRoot, err)
	}
	rootFromRoot := ResolveProjectRoot()
	if rootFromRoot != expectedRoot {
		t.Errorf("ResolveProjectRoot from project root: got %s, want %s", rootFromRoot, expectedRoot)
	}
}

func TestResolveProjectRootWithEnvVar(t *testing.T) {
	// Save original value
	originalValue := os.Getenv("G8E_PROJECT_ROOT")
	defer func() {
		if originalValue != "" {
			os.Setenv("G8E_PROJECT_ROOT", originalValue)
		} else {
			os.Unsetenv("G8E_PROJECT_ROOT")
		}
	}()

	// Set custom root
	customRoot := "/custom/root"
	os.Setenv("G8E_PROJECT_ROOT", customRoot)

	root := ResolveProjectRoot()
	if root != customRoot {
		t.Errorf("ResolveProjectRoot with G8E_PROJECT_ROOT: got %s, want %s", root, customRoot)
	}
}
