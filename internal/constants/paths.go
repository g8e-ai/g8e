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
		CaCertPath:           ".g8e/pki/trust/g8e-gw-ca-bundle.pem",
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
// This mirrors the logic in internal/services/system/path.go
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

	// Try to find the root by looking for protocol or .git
	current := cwd
	for {
		_, protocolErr := os.Stat(filepath.Join(current, "protocol"))
		_, gitErr := os.Stat(filepath.Join(current, ".git"))

		if protocolErr == nil || gitErr == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return cwd
}
