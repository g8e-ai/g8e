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
	t.Run("with G8E_PROJECT_ROOT set", func(t *testing.T) {
		t.Setenv("G8E_PROJECT_ROOT", "/test/root")
		t.Setenv("G8E_PROTOCOL_DIR", "")
		resolvePaths()

		expected := "/test/root/protocol"
		assert.Equal(t, expected, Paths.Infra.ProtocolDir)
		assert.Equal(t, expected+"/constants", Paths.Infra.ProtocolConstantsDir)
		assert.Equal(t, expected+"/models", Paths.Infra.ProtocolModelsDir)
	})

	t.Run("with G8E_PROTOCOL_DIR absolute", func(t *testing.T) {
		t.Setenv("G8E_PROTOCOL_DIR", "/custom/protocol")
		resolvePaths()

		assert.Equal(t, "/custom/protocol", Paths.Infra.ProtocolDir)
		assert.Equal(t, "/custom/protocol/constants", Paths.Infra.ProtocolConstantsDir)
		assert.Equal(t, "/custom/protocol/models", Paths.Infra.ProtocolModelsDir)
	})

	t.Run("with G8E_PROTOCOL_DIR relative", func(t *testing.T) {
		t.Setenv("G8E_PROJECT_ROOT", "/test/root")
		t.Setenv("G8E_PROTOCOL_DIR", "custom/protocol")
		resolvePaths()

		assert.Equal(t, "/test/root/custom/protocol", Paths.Infra.ProtocolDir)
		assert.Equal(t, "/test/root/custom/protocol/constants", Paths.Infra.ProtocolConstantsDir)
		assert.Equal(t, "/test/root/custom/protocol/models", Paths.Infra.ProtocolModelsDir)
	})

	t.Run("G8E_PROTOCOL_DIR takes precedence over G8E_PROJECT_ROOT", func(t *testing.T) {
		t.Setenv("G8E_PROJECT_ROOT", "/test/root")
		t.Setenv("G8E_PROTOCOL_DIR", "/override/protocol")
		resolvePaths()

		assert.Equal(t, "/override/protocol", Paths.Infra.ProtocolDir)
	})
}

func TestResolveProjectRoot(t *testing.T) {
	t.Run("with G8E_PROJECT_ROOT set", func(t *testing.T) {
		t.Setenv("G8E_PROJECT_ROOT", "/custom/root")
		result := resolveProjectRoot()
		assert.Equal(t, "/custom/root", result)
	})

	t.Run("resolves to absolute path", func(t *testing.T) {
		t.Setenv("G8E_PROJECT_ROOT", ".")
		result := resolveProjectRoot()
		assert.True(t, filepath.IsAbs(result), "resolveProjectRoot should return absolute path")
	})

	t.Run("walks up to find protocol marker", func(t *testing.T) {
		tmpDir := t.TempDir()
		protocolDir := filepath.Join(tmpDir, "protocol")
		if err := os.Mkdir(protocolDir, 0755); err != nil {
			t.Fatalf("Failed to create protocol dir: %v", err)
		}

		subDir := filepath.Join(tmpDir, "internal", "services")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirs: %v", err)
		}

		t.Setenv("G8E_PROJECT_ROOT", "")

		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)

		if err := os.Chdir(subDir); err != nil {
			t.Fatalf("Failed to chdir: %v", err)
		}

		result := resolveProjectRoot()
		assert.Equal(t, tmpDir, result)
	})

	t.Run("walks up to find .git marker", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		if err := os.Mkdir(gitDir, 0755); err != nil {
			t.Fatalf("Failed to create .git dir: %v", err)
		}

		subDir := filepath.Join(tmpDir, "deep", "nested")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirs: %v", err)
		}

		t.Setenv("G8E_PROJECT_ROOT", "")

		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)

		if err := os.Chdir(subDir); err != nil {
			t.Fatalf("Failed to chdir: %v", err)
		}

		result := resolveProjectRoot()
		assert.Equal(t, tmpDir, result)
	})

	t.Run("returns CWD when no markers found", func(t *testing.T) {
		tmpDir := t.TempDir()
		subDir := filepath.Join(tmpDir, "some", "path")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirs: %v", err)
		}

		t.Setenv("G8E_PROJECT_ROOT", "")

		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)

		if err := os.Chdir(subDir); err != nil {
			t.Fatalf("Failed to chdir: %v", err)
		}

		result := resolveProjectRoot()
		assert.Equal(t, subDir, result)
	})
}
