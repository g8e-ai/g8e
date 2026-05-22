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
	// Test with G8E_PROJECT_ROOT set
	originalRoot := os.Getenv("G8E_PROJECT_ROOT")
	originalProtocolDir := os.Getenv("G8E_PROTOCOL_DIR")
	defer func() {
		if originalRoot != "" {
			os.Setenv("G8E_PROJECT_ROOT", originalRoot)
		} else {
			os.Unsetenv("G8E_PROJECT_ROOT")
		}
		if originalProtocolDir != "" {
			os.Setenv("G8E_PROTOCOL_DIR", originalProtocolDir)
		} else {
			os.Unsetenv("G8E_PROTOCOL_DIR")
		}
		// Re-resolve paths to restore original state
		resolvePaths()
	}()

	t.Run("with G8E_PROJECT_ROOT set", func(t *testing.T) {
		os.Setenv("G8E_PROJECT_ROOT", "/test/root")
		os.Unsetenv("G8E_PROTOCOL_DIR")
		resolvePaths()

		expected := "/test/root/protocol"
		assert.Equal(t, expected, Paths.Infra.ProtocolDir)
		assert.Equal(t, expected+"/constants", Paths.Infra.ProtocolConstantsDir)
		assert.Equal(t, expected+"/models", Paths.Infra.ProtocolModelsDir)
	})

	t.Run("with G8E_PROTOCOL_DIR absolute", func(t *testing.T) {
		os.Setenv("G8E_PROTOCOL_DIR", "/custom/protocol")
		resolvePaths()

		assert.Equal(t, "/custom/protocol", Paths.Infra.ProtocolDir)
		assert.Equal(t, "/custom/protocol/constants", Paths.Infra.ProtocolConstantsDir)
		assert.Equal(t, "/custom/protocol/models", Paths.Infra.ProtocolModelsDir)
	})

	t.Run("with G8E_PROTOCOL_DIR relative", func(t *testing.T) {
		os.Setenv("G8E_PROJECT_ROOT", "/test/root")
		os.Setenv("G8E_PROTOCOL_DIR", "custom/protocol")
		resolvePaths()

		assert.Equal(t, "/test/root/custom/protocol", Paths.Infra.ProtocolDir)
		assert.Equal(t, "/test/root/custom/protocol/constants", Paths.Infra.ProtocolConstantsDir)
		assert.Equal(t, "/test/root/custom/protocol/models", Paths.Infra.ProtocolModelsDir)
	})

	t.Run("G8E_PROTOCOL_DIR takes precedence over G8E_PROJECT_ROOT", func(t *testing.T) {
		os.Setenv("G8E_PROJECT_ROOT", "/test/root")
		os.Setenv("G8E_PROTOCOL_DIR", "/override/protocol")
		resolvePaths()

		assert.Equal(t, "/override/protocol", Paths.Infra.ProtocolDir)
	})
}

func TestResolveProjectRoot(t *testing.T) {
	t.Run("with G8E_PROJECT_ROOT set", func(t *testing.T) {
		original := os.Getenv("G8E_PROJECT_ROOT")
		defer func() {
			if original != "" {
				os.Setenv("G8E_PROJECT_ROOT", original)
			} else {
				os.Unsetenv("G8E_PROJECT_ROOT")
			}
		}()

		os.Setenv("G8E_PROJECT_ROOT", "/custom/root")
		result := resolveProjectRoot()
		assert.Equal(t, "/custom/root", result)
	})

	t.Run("resolves to absolute path", func(t *testing.T) {
		original := os.Getenv("G8E_PROJECT_ROOT")
		defer func() {
			if original != "" {
				os.Setenv("G8E_PROJECT_ROOT", original)
			} else {
				os.Unsetenv("G8E_PROJECT_ROOT")
			}
		}()

		os.Setenv("G8E_PROJECT_ROOT", ".")
		result := resolveProjectRoot()
		assert.True(t, filepath.IsAbs(result), "resolveProjectRoot should return absolute path")
	})
}
