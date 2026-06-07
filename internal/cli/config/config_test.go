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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandPath(t *testing.T) {
	t.Run("empty path returns empty", func(t *testing.T) {
		result, err := expandPath("")
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("tilde alone expands to home directory", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		result, err := expandPath("~")
		require.NoError(t, err)
		assert.Equal(t, homeDir, result)
	})

	t.Run("tilde with path expands to home subdirectory", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		result, err := expandPath("~/subdir")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(homeDir, "subdir"), result)
	})

	t.Run("environment variable expansion", func(t *testing.T) {
		os.Setenv("TEST_VAR", "test_value")
		defer os.Unsetenv("TEST_VAR")

		result, err := expandPath("$TEST_VAR")
		require.NoError(t, err)
		assert.Equal(t, "test_value", result)
	})

	t.Run("mixed tilde and environment variable", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		os.Setenv("TEST_VAR", "test_value")
		defer os.Unsetenv("TEST_VAR")

		result, err := expandPath("~/subdir/$TEST_VAR")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(homeDir, "subdir", "test_value"), result)
	})

	t.Run("absolute path unchanged", func(t *testing.T) {
		result, err := expandPath("/absolute/path")
		require.NoError(t, err)
		assert.Equal(t, "/absolute/path", result)
	})

	t.Run("relative path unchanged", func(t *testing.T) {
		result, err := expandPath("relative/path")
		require.NoError(t, err)
		assert.Equal(t, "relative/path", result)
	})

	t.Run("no tilde prefix returns path as-is", func(t *testing.T) {
		result, err := expandPath("/not/tilde/path")
		require.NoError(t, err)
		assert.Equal(t, "/not/tilde/path", result)
	})
}

func TestLoad(t *testing.T) {
	t.Run("loads config from any directory using embedded defaults", func(t *testing.T) {
		tempDir := t.TempDir()

		config, err := Load(tempDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, tempDir, config.ProjectRoot)
		assert.Equal(t, filepath.Join(tempDir, DefaultRuntimeDir), config.RuntimeDir)
		assert.Equal(t, filepath.Join(tempDir, DefaultPKIDir), config.PKIDir)
		assert.Equal(t, filepath.Join(tempDir, DefaultSecretsDir), config.SecretsDir)
		assert.Contains(t, config.CredentialsDir, ".g8e")
		assert.NotNil(t, config.Paths)
	})

	t.Run("uses current directory when project root is empty", func(t *testing.T) {
		tempDir := t.TempDir()

		// Change to temp directory and load with empty project root
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		err = os.Chdir(tempDir)
		require.NoError(t, err)

		config, err := Load("")
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, tempDir, config.ProjectRoot)
	})

	t.Run("always uses embedded defaults regardless of file presence", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create protocol/constants directory with a paths.json file
		// This should be ignored since we always use embedded defaults
		protocolDir := filepath.Join(tempDir, "protocol", "constants")
		err := os.MkdirAll(protocolDir, 0755)
		require.NoError(t, err)

		// Create a paths.json with different values to verify it's ignored
		pathsJSON := `{
			"host": "should-be-ignored",
			"ports": {
				"operator_https": 9999
			}
		}`
		err = os.WriteFile(filepath.Join(protocolDir, "paths.json"), []byte(pathsJSON), 0644)
		require.NoError(t, err)

		config, err := Load(tempDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		// Should use embedded defaults, not the file
		assert.Equal(t, "localhost", config.Paths.Host)
	})
}

func TestConfig_TrustBundlePath(t *testing.T) {
	t.Run("returns absolute path as-is", func(t *testing.T) {
		config := &Config{
			ProjectRoot: "/project/root",
			Paths: &PathsConfig{
				Infra: struct {
					AppCertDir           string `json:"app_cert_dir"`
					CACertPath           string `json:"ca_cert_path"`
					DBPath               string `json:"db_path"`
					DocsDir              string `json:"docs_dir"`
					PKIDir               string `json:"pki_dir"`
					ProtocolConstantsDir string `json:"protocol_constants_dir"`
					ProtocolDir          string `json:"protocol_dir"`
					ProtocolModelsDir    string `json:"protocol_models_dir"`
					SecretsDir           string `json:"secrets_dir"`
					SSHConfigPath        string `json:"ssh_config_path"`
					VaultDir             string `json:"vault_dir"`
					VaultKeyPath         string `json:"vault_key_path"`
				}{
					CACertPath: "/absolute/path/to/ca.pem",
				},
			},
		}

		result := config.TrustBundlePath()
		assert.Equal(t, "/absolute/path/to/ca.pem", result)
	})

	t.Run("joins relative path with project root", func(t *testing.T) {
		config := &Config{
			ProjectRoot: "/project/root",
			Paths: &PathsConfig{
				Infra: struct {
					AppCertDir           string `json:"app_cert_dir"`
					CACertPath           string `json:"ca_cert_path"`
					DBPath               string `json:"db_path"`
					DocsDir              string `json:"docs_dir"`
					PKIDir               string `json:"pki_dir"`
					ProtocolConstantsDir string `json:"protocol_constants_dir"`
					ProtocolDir          string `json:"protocol_dir"`
					ProtocolModelsDir    string `json:"protocol_models_dir"`
					SecretsDir           string `json:"secrets_dir"`
					SSHConfigPath        string `json:"ssh_config_path"`
					VaultDir             string `json:"vault_dir"`
					VaultKeyPath         string `json:"vault_key_path"`
				}{
					CACertPath: "relative/path/to/ca.pem",
				},
			},
		}

		result := config.TrustBundlePath()
		assert.Equal(t, "/project/root/relative/path/to/ca.pem", result)
	})

	t.Run("returns empty string when CACertPath is empty", func(t *testing.T) {
		config := &Config{
			ProjectRoot: "/project/root",
			Paths: &PathsConfig{
				Infra: struct {
					AppCertDir           string `json:"app_cert_dir"`
					CACertPath           string `json:"ca_cert_path"`
					DBPath               string `json:"db_path"`
					DocsDir              string `json:"docs_dir"`
					PKIDir               string `json:"pki_dir"`
					ProtocolConstantsDir string `json:"protocol_constants_dir"`
					ProtocolDir          string `json:"protocol_dir"`
					ProtocolModelsDir    string `json:"protocol_models_dir"`
					SecretsDir           string `json:"secrets_dir"`
					SSHConfigPath        string `json:"ssh_config_path"`
					VaultDir             string `json:"vault_dir"`
					VaultKeyPath         string `json:"vault_key_path"`
				}{
					CACertPath: "",
				},
			},
		}

		result := config.TrustBundlePath()
		assert.Equal(t, "", result)
	})
}

func TestConfig_CredentialsFile(t *testing.T) {
	t.Run("returns credentials file path", func(t *testing.T) {
		config := &Config{
			CredentialsDir: "/credentials/dir",
		}

		result := config.CredentialsFile()
		assert.Equal(t, "/credentials/dir/credentials", result)
	})
}

func TestConfig_CLICertFile(t *testing.T) {
	t.Run("returns CLI cert file path", func(t *testing.T) {
		config := &Config{
			CredentialsDir: "/credentials/dir",
		}

		result := config.CLICertFile()
		assert.Equal(t, "/credentials/dir/cli.crt", result)
	})
}

func TestConfig_CLIKeyFile(t *testing.T) {
	t.Run("returns CLI key file path", func(t *testing.T) {
		config := &Config{
			CredentialsDir: "/credentials/dir",
		}

		result := config.CLIKeyFile()
		assert.Equal(t, "/credentials/dir/cli.key", result)
	})
}

func TestConfig_OperatorCertFile(t *testing.T) {
	t.Run("returns Operator cert file path", func(t *testing.T) {
		config := &Config{
			CredentialsDir: "/credentials/dir",
		}

		result := config.OperatorCertFile()
		assert.Equal(t, "/credentials/dir/operator.crt", result)
	})
}

func TestConfig_OperatorKeyFile(t *testing.T) {
	t.Run("returns Operator key file path", func(t *testing.T) {
		config := &Config{
			CredentialsDir: "/credentials/dir",
		}

		result := config.OperatorKeyFile()
		assert.Equal(t, "/credentials/dir/operator.key", result)
	})
}

func TestConfig_OperatorHTTPSPort(t *testing.T) {
	t.Run("returns Operator HTTPS port from constants", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{},
		}

		result := config.OperatorHTTPSPort()
		assert.Equal(t, constants.Ports.OperatorHttps, result)
	})
}

func TestConfig_OperatorHTTPURL(t *testing.T) {
	t.Run("returns Operator HTTPS URL from constants", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{},
		}

		result := config.OperatorHTTPURL()
		assert.Equal(t, fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps), result)
	})

	t.Run("returns Operator HTTPS URL with test port override", func(t *testing.T) {
		config := &Config{
			Paths:            &PathsConfig{},
			TestPortOverride: 9999,
		}

		result := config.OperatorHTTPURL()
		assert.Equal(t, "https://localhost:9999", result)
	})
}

func TestConfig_OperatorBootstrapURL(t *testing.T) {
	t.Run("returns Operator bootstrap HTTPS URL (deprecated, delegates to OperatorPublicURL)", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{},
		}

		result := config.OperatorBootstrapURL()
		assert.Equal(t, fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps), result)
	})
}

func TestConfig_OperatorPublicURL(t *testing.T) {
	t.Run("returns Operator public TLS URL for CSR-based enrollment", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{},
		}

		result := config.OperatorPublicURL()
		assert.Equal(t, fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps), result)
	})
}

func TestConfig_OperatorDiscoveryURL(t *testing.T) {
	t.Run("returns Operator discovery URL for CA download over plain HTTP", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{},
		}

		result := config.OperatorDiscoveryURL()
		assert.Equal(t, fmt.Sprintf("http://localhost:%d", constants.Ports.OperatorHttp), result)
	})
}

func TestDefaultConstants(t *testing.T) {
	t.Run("default runtime dir constant", func(t *testing.T) {
		assert.Equal(t, ".g8e", DefaultRuntimeDir)
	})

	t.Run("default PKI dir constant", func(t *testing.T) {
		assert.Equal(t, ".g8e/pki", DefaultPKIDir)
	})

	t.Run("default secrets dir constant", func(t *testing.T) {
		assert.Equal(t, ".g8e/secrets", DefaultSecretsDir)
	})

	t.Run("default credentials dir constant", func(t *testing.T) {
		assert.Equal(t, ".g8e", DefaultCredentialsDir)
	})
}

func TestLoadIntegration(t *testing.T) {
	// This is an integration test that verifies the embedded-only behavior

	t.Run("loads embedded default paths from any directory", func(t *testing.T) {
		// This test verifies the self-sovereign binary behavior:
		// The binary always uses embedded default paths regardless of directory structure.

		tempDir := t.TempDir()

		// Change to temp directory (simulating running binary from empty directory)
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		err = os.Chdir(tempDir)
		require.NoError(t, err)

		// Load config from empty directory (no source tree)
		config, err := Load("")
		require.NoError(t, err)
		assert.NotNil(t, config)

		// Verify it loaded from embedded defaults
		assert.NotNil(t, config.Paths)
		assert.Equal(t, tempDir, config.ProjectRoot)

		// Verify embedded default paths are resolved relative to tempDir
		assert.Equal(t, filepath.Join(tempDir, ".g8e"), config.RuntimeDir)
		assert.Equal(t, filepath.Join(tempDir, ".g8e/pki"), config.PKIDir)
		assert.Equal(t, filepath.Join(tempDir, ".g8e/secrets"), config.SecretsDir)

		// Verify protocol paths are relative
		assert.Equal(t, filepath.Join(tempDir, ".g8e/protocol"), config.Paths.Infra.ProtocolDir)
		assert.Equal(t, filepath.Join(tempDir, ".g8e/protocol/constants"), config.Paths.Infra.ProtocolConstantsDir)
		assert.Equal(t, filepath.Join(tempDir, ".g8e/protocol/models"), config.Paths.Infra.ProtocolModelsDir)

		// Verify port values from constants
		assert.Equal(t, constants.Ports.OperatorHttps, config.OperatorHTTPSPort())
		assert.Equal(t, constants.Ports.OperatorHttp, constants.Ports.OperatorHttp)
	})
}
