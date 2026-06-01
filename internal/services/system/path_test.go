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
	// Get the expected root from the current directory (should be project root)
	expectedRoot := ResolveProjectRoot()

	// Test from internal
	t.Run("from internal", func(t *testing.T) {
		internalPath := filepath.Join(expectedRoot, "internal")
		if _, err := os.Stat(internalPath); os.IsNotExist(err) {
			t.Skip("internal directory does not exist, skipping test")
		}
		t.Chdir(internalPath)
		rootFromInternal := ResolveProjectRoot()
		if rootFromInternal != expectedRoot {
			t.Errorf("ResolveProjectRoot from internal: got %s, want %s", rootFromInternal, expectedRoot)
		}
	})

	// Test from scripts
	t.Run("from scripts", func(t *testing.T) {
		scriptsPath := filepath.Join(expectedRoot, "scripts")
		if _, err := os.Stat(scriptsPath); os.IsNotExist(err) {
			t.Skip("scripts directory does not exist, skipping test")
		}
		t.Chdir(scriptsPath)
		rootFromScripts := ResolveProjectRoot()
		if rootFromScripts != expectedRoot {
			t.Errorf("ResolveProjectRoot from scripts: got %s, want %s", rootFromScripts, expectedRoot)
		}
	})

	// Test from project root
	t.Run("from project root", func(t *testing.T) {
		t.Chdir(expectedRoot)
		rootFromRoot := ResolveProjectRoot()
		if rootFromRoot != expectedRoot {
			t.Errorf("ResolveProjectRoot from project root: got %s, want %s", rootFromRoot, expectedRoot)
		}
	})
}

func TestResolveProjectRootWithEnvVar(t *testing.T) {
	// G8E_PROJECT_ROOT env var was removed - project root is now resolved
	// solely by walking up from current working directory.
	// This test is removed as the feature no longer exists.
	t.Skip("G8E_PROJECT_ROOT env var removed")
}

func TestResolveProjectRootWithRelativeEnvVar(t *testing.T) {
	// G8E_PROJECT_ROOT env var was removed - project root is now resolved
	// solely by walking up from current working directory.
	// This test is removed as the feature no longer exists.
	t.Skip("G8E_PROJECT_ROOT env var removed")
}

func TestResolveProjectRootWalksWithProtocolMarker(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	protocolDir := filepath.Join(tmpDir, "protocol")
	if err := os.Mkdir(protocolDir, 0755); err != nil {
		t.Fatalf("Failed to create protocol dir: %v", err)
	}

	// Change to a subdirectory
	subDir := filepath.Join(tmpDir, "internal", "services", "system")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirs: %v", err)
	}

	// No env var to unset - walking is the only method

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	root := ResolveProjectRoot()
	if root != tmpDir {
		t.Errorf("ResolveProjectRoot walking with protocol marker: got %s, want %s", root, tmpDir)
	}
}

func TestResolveProjectRootWalksWithGitMarker(t *testing.T) {
	// Create a temporary directory structure with .git marker
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git dir: %v", err)
	}

	// Change to a subdirectory
	subDir := filepath.Join(tmpDir, "deep", "nested", "path")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirs: %v", err)
	}

	// No env var to unset - walking is the only method

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	root := ResolveProjectRoot()
	if root != tmpDir {
		t.Errorf("ResolveProjectRoot walking with .git marker: got %s, want %s", root, tmpDir)
	}
}

func TestResolveProjectRootPrefersCloserMarker(t *testing.T) {
	// Create nested directory structures with markers at different levels
	tmpDir := t.TempDir()
	parentMarker := filepath.Join(tmpDir, "protocol")
	if err := os.Mkdir(parentMarker, 0755); err != nil {
		t.Fatalf("Failed to create parent protocol dir: %v", err)
	}

	// Create a subdirectory with its own marker
	subDir := filepath.Join(tmpDir, "subdir")
	subMarker := filepath.Join(subDir, ".git")
	if err := os.MkdirAll(subMarker, 0755); err != nil {
		t.Fatalf("Failed to create subdir with marker: %v", err)
	}

	// Change to the subdirectory
	t.Setenv("G8E_PROJECT_ROOT", "")

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	root := ResolveProjectRoot()
	// Should find the closer marker (subdir with .git)
	if root != subDir {
		t.Errorf("ResolveProjectRoot should prefer closer marker: got %s, want %s", root, subDir)
	}
}

func TestResolveProjectRootNoMarkerReturnsCWD(t *testing.T) {
	// Create a temporary directory with no markers
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "some", "path")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirs: %v", err)
	}

	// No env var to unset - walking is the only method

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	root := ResolveProjectRoot()
	// Should return CWD when no markers found
	if root != subDir {
		t.Errorf("ResolveProjectRoot with no markers: got %s, want %s (CWD)", root, subDir)
	}
}

func TestResolveProjectRootAtFilesystemRoot(t *testing.T) {
	// This test verifies the loop termination condition
	// When parent == current (filesystem root), it should return CWD

	// Create a temp dir and chdir to it
	tmpDir := t.TempDir()
	t.Setenv("G8E_PROJECT_ROOT", "")

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	root := ResolveProjectRoot()
	// Should return tmpDir (CWD) since no markers exist
	if root != tmpDir {
		t.Errorf("ResolveProjectRoot at filesystem boundary: got %s, want %s", root, tmpDir)
	}
}

func TestResolveProjectRootBothMarkers(t *testing.T) {
	// Create a directory with both protocol and .git markers
	tmpDir := t.TempDir()
	protocolDir := filepath.Join(tmpDir, "protocol")
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(protocolDir, 0755); err != nil {
		t.Fatalf("Failed to create protocol dir: %v", err)
	}
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git dir: %v", err)
	}

	// Change to a subdirectory
	subDir := filepath.Join(tmpDir, "internal")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	t.Setenv("G8E_PROJECT_ROOT", "")

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	root := ResolveProjectRoot()
	if root != tmpDir {
		t.Errorf("ResolveProjectRoot with both markers: got %s, want %s", root, tmpDir)
	}
}
