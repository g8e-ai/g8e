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
	"path/filepath"
	"testing"
)

func TestResolveProjectRootConsistency(t *testing.T) {
	// Get the expected root from the current directory (should be services/g8eo)
	expectedRoot := ResolveProjectRoot()

	// Test from services/g8eo
	t.Run("from services/g8eo", func(t *testing.T) {
		t.Chdir(filepath.Join(expectedRoot, "services", "g8eo"))
		rootFromG8eo := ResolveProjectRoot()
		if rootFromG8eo != expectedRoot {
			t.Errorf("ResolveProjectRoot from services/g8eo: got %s, want %s", rootFromG8eo, expectedRoot)
		}
	})

	// Test from services/g8ee
	t.Run("from services/g8ee", func(t *testing.T) {
		t.Chdir(filepath.Join(expectedRoot, "services", "g8ee"))
		rootFromG8ee := ResolveProjectRoot()
		if rootFromG8ee != expectedRoot {
			t.Errorf("ResolveProjectRoot from services/g8ee: got %s, want %s", rootFromG8ee, expectedRoot)
		}
	})

	// Test from scripts
	t.Run("from scripts", func(t *testing.T) {
		t.Chdir(filepath.Join(expectedRoot, "scripts"))
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
	t.Setenv("G8E_PROJECT_ROOT", "/custom/root")

	root := ResolveProjectRoot()
	if root != "/custom/root" {
		t.Errorf("ResolveProjectRoot with G8E_PROJECT_ROOT: got %s, want /custom/root", root)
	}
}
