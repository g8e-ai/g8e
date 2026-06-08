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

package constants

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProtocolDir_Resolution(t *testing.T) {
	// G8E_PROJECT_ROOT env var was removed - paths are now resolved
	// solely by walking up from current working directory.
	// This test is removed as the feature no longer exists.
	t.Skip("G8E_PROJECT_ROOT env var removed")
}

func TestInitPaths(t *testing.T) {
	t.Run("initializes paths relative to cwd", func(t *testing.T) {
		tmpDir := t.TempDir()
		subDir := filepath.Join(tmpDir, "subdir")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)

		if err := os.Chdir(subDir); err != nil {
			t.Fatalf("Failed to chdir: %v", err)
		}

		if err := InitPaths(); err != nil {
			t.Fatalf("InitPaths failed: %v", err)
		}

		// All paths should be relative to the current working directory (subDir)
		assert.Contains(t, Paths.Infra.RuntimeDir, subDir)
		assert.Contains(t, Paths.Infra.DataDir, subDir)
		assert.Contains(t, Paths.Infra.PkiDir, subDir)
		assert.Contains(t, Paths.Infra.SecretsDir, subDir)
	})
}
