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
)

// Paths defines canonical G8E filesystem paths.
var Paths = struct {
	Infra struct {
		DbPath               string
		PkiDir               string
		SecretsDir           string
		CaCertPath           string
		AppCertDir           string
		DocsDir              string
		ProtocolDir          string
		ProtocolConstantsDir string
		ProtocolModelsDir    string
		SshConfigPath        string
	}
}{
	Infra: struct {
		DbPath               string
		PkiDir               string
		SecretsDir           string
		CaCertPath           string
		AppCertDir           string
		DocsDir              string
		ProtocolDir          string
		ProtocolConstantsDir string
		ProtocolModelsDir    string
		SshConfigPath        string
	}{
		DbPath:               ".g8e/data/g8e.db",
		PkiDir:               ".g8e/pki",
		SecretsDir:           ".g8e/secrets",
		CaCertPath:           ".g8e/pki/trust/hub-bundle.pem",
		AppCertDir:           ".g8e/pki/issued/apps",
		DocsDir:              "/docs",
		ProtocolDir:          "/app/protocol",
		ProtocolConstantsDir: "/app/protocol/constants",
		ProtocolModelsDir:    "/app/protocol/models",
		SshConfigPath:        "/etc/g8e/ssh_config",
	},
}

func init() {
	resolvePaths()
}

// resolvePaths dynamically resolves filesystem paths from environment variables.
// Priority:
// 1. Explicit environment variable (e.g., G8E_PROTOCOL_DIR)
// 2. Relative to G8E_PROJECT_ROOT if set
// 3. Fallback to hardcoded default (container path)
func resolvePaths() {
	projectRoot := os.Getenv("G8E_PROJECT_ROOT")
	if projectRoot == "" {
		projectRoot = resolveProjectRoot()
	}

	// Resolve ProtocolDir
	if protocolDir := os.Getenv("G8E_PROTOCOL_DIR"); protocolDir != "" {
		if filepath.IsAbs(protocolDir) {
			Paths.Infra.ProtocolDir = protocolDir
		} else {
			Paths.Infra.ProtocolDir = filepath.Join(projectRoot, protocolDir)
		}
	} else {
		// Default to protocol/ relative to project root for host-native execution
		Paths.Infra.ProtocolDir = filepath.Join(projectRoot, "protocol")
	}

	// Update derived paths
	Paths.Infra.ProtocolConstantsDir = filepath.Join(Paths.Infra.ProtocolDir, "constants")
	Paths.Infra.ProtocolModelsDir = filepath.Join(Paths.Infra.ProtocolDir, "models")
}

// resolveProjectRoot returns the project root directory.
// This mirrors the logic in services/g8eo/internal/services/system/path.go
// but is duplicated here to avoid circular dependencies during init.
func resolveProjectRoot() string {
	if root := os.Getenv("G8E_PROJECT_ROOT"); root != "" {
		abs, err := filepath.Abs(root)
		if err == nil {
			return abs
		}
		return root
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "/home/g8e"
	}

	// Try to find the root by looking for services/g8eo or .git
	current := cwd
	for {
		if _, err := os.Stat(filepath.Join(current, "services")); err == nil {
			if _, err := os.Stat(filepath.Join(current, "g8e")); err == nil {
				return current
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Fallback to relative path from services/g8eo
	if contains(cwd, filepath.Join("services", "g8eo")) {
		current = cwd
		for {
			if filepath.Base(current) == "g8eo" && filepath.Base(filepath.Dir(current)) == "services" {
				return filepath.Dir(filepath.Dir(current))
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	return cwd
}

// contains checks if a string contains a substring (Go < 1.21 compatibility)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
