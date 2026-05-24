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
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	t.Run("loads config from valid project root", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create protocol/constants directory structure
		protocolDir := filepath.Join(tempDir, "protocol", "constants")
		err := os.MkdirAll(protocolDir, 0755)
		require.NoError(t, err)

		// Create a valid paths.json
		pathsJSON := `{
			"g8ee": {
				"app_dir": "/app/services/g8ee",
				"cert_name": "g8ee",
				"config_dir": "/app/services/g8ee/config",
				"tests_dir": "/app/services/g8ee/tests"
			},
			"host": "localhost",
			"infra": {
				"app_cert_dir": ".g8e/pki/issued/apps",
				"ca_cert_path": ".g8e/pki/trust/hub-bundle.pem",
				"db_path": ".g8e/data/g8e.db",
				"docs_dir": "/docs",
				"pki_dir": ".g8e/pki",
				"protocol_constants_dir": "/home/bob/g8e/protocol/constants",
				"protocol_dir": "/home/bob/g8e/protocol",
				"protocol_models_dir": "/home/bob/g8e/protocol/models",
				"secrets_dir": ".g8e/secrets",
				"ssh_config_path": "/etc/g8e/ssh_config"
			},
			"ports": {
				"g8ee_https": 8443,
				"openclaw_gateway": 18789,
				"operator_bootstrap_https": 8441,
				"operator_https": 8440,
				"operator_public_https": 8442
			}
		}`
		err = os.WriteFile(filepath.Join(protocolDir, "paths.json"), []byte(pathsJSON), 0644)
		require.NoError(t, err)

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

		// Create protocol/constants directory structure
		protocolDir := filepath.Join(tempDir, "protocol", "constants")
		err := os.MkdirAll(protocolDir, 0755)
		require.NoError(t, err)

		// Create a valid paths.json
		pathsJSON := `{
			"g8ee": {
				"app_dir": "/app/services/g8ee",
				"cert_name": "g8ee",
				"config_dir": "/app/services/g8ee/config",
				"tests_dir": "/app/services/g8ee/tests"
			},
			"host": "localhost",
			"infra": {
				"app_cert_dir": ".g8e/pki/issued/apps",
				"ca_cert_path": ".g8e/pki/trust/hub-bundle.pem",
				"db_path": ".g8e/data/g8e.db",
				"docs_dir": "/docs",
				"pki_dir": ".g8e/pki",
				"protocol_constants_dir": "/home/bob/g8e/protocol/constants",
				"protocol_dir": "/home/bob/g8e/protocol",
				"protocol_models_dir": "/home/bob/g8e/protocol/models",
				"secrets_dir": ".g8e/secrets",
				"ssh_config_path": "/etc/g8e/ssh_config"
			},
			"ports": {
				"g8ee_https": 8443,
				"openclaw_gateway": 18789,
				"operator_bootstrap_https": 8441,
				"operator_https": 8440,
				"operator_public_https": 8442
			}
		}`
		err = os.WriteFile(filepath.Join(protocolDir, "paths.json"), []byte(pathsJSON), 0644)
		require.NoError(t, err)

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

	t.Run("returns error when paths.json does not exist", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create protocol/constants directory but no paths.json
		protocolDir := filepath.Join(tempDir, "protocol", "constants")
		err := os.MkdirAll(protocolDir, 0755)
		require.NoError(t, err)

		config, err := Load(tempDir)
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to read paths.json")
	})

	t.Run("returns error when paths.json is invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create protocol/constants directory structure
		protocolDir := filepath.Join(tempDir, "protocol", "constants")
		err := os.MkdirAll(protocolDir, 0755)
		require.NoError(t, err)

		// Create invalid paths.json
		err = os.WriteFile(filepath.Join(protocolDir, "paths.json"), []byte("invalid json"), 0644)
		require.NoError(t, err)

		config, err := Load(tempDir)
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to parse paths.json")
	})

	t.Run("returns error when protocol/constants directory does not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		// Don't create protocol/constants directory

		config, err := Load(tempDir)
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to read paths.json")
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
	t.Run("returns operator cert file path", func(t *testing.T) {
		config := &Config{
			CredentialsDir: "/credentials/dir",
		}

		result := config.OperatorCertFile()
		assert.Equal(t, "/credentials/dir/operator.crt", result)
	})
}

func TestConfig_OperatorKeyFile(t *testing.T) {
	t.Run("returns operator key file path", func(t *testing.T) {
		config := &Config{
			CredentialsDir: "/credentials/dir",
		}

		result := config.OperatorKeyFile()
		assert.Equal(t, "/credentials/dir/operator.key", result)
	})
}

func TestConfig_OperatorHTTPSPort(t *testing.T) {
	t.Run("returns operator HTTPS port", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{
				Ports: struct {
					OpenclawGateway        int `json:"openclaw_gateway"`
					OperatorBootstrapHTTPS int `json:"operator_bootstrap_https"`
					OperatorHTTPS          int `json:"operator_https"`
					OperatorPublicHTTPS    int `json:"operator_public_https"`
				}{
					OperatorHTTPS: 8440,
				},
			},
		}

		result := config.OperatorHTTPSPort()
		assert.Equal(t, 8440, result)
	})
}

func TestConfig_OperatorBootstrapHTTPSPort(t *testing.T) {
	t.Run("returns operator bootstrap HTTPS port", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{
				Ports: struct {
					OpenclawGateway        int `json:"openclaw_gateway"`
					OperatorBootstrapHTTPS int `json:"operator_bootstrap_https"`
					OperatorHTTPS          int `json:"operator_https"`
					OperatorPublicHTTPS    int `json:"operator_public_https"`
				}{
					OperatorBootstrapHTTPS: 8441,
				},
			},
		}

		result := config.OperatorBootstrapHTTPSPort()
		assert.Equal(t, 8441, result)
	})
}

func TestConfig_OperatorHTTPURL(t *testing.T) {
	t.Run("returns operator HTTPS URL", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{
				Ports: struct {
					OpenclawGateway        int `json:"openclaw_gateway"`
					OperatorBootstrapHTTPS int `json:"operator_bootstrap_https"`
					OperatorHTTPS          int `json:"operator_https"`
					OperatorPublicHTTPS    int `json:"operator_public_https"`
				}{
					OperatorHTTPS: 8440,
				},
			},
		}

		result := config.OperatorHTTPURL()
		assert.Equal(t, "https://localhost:8440", result)
	})
}

func TestConfig_OperatorBootstrapURL(t *testing.T) {
	t.Run("returns operator bootstrap HTTPS URL", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{
				Ports: struct {
					OpenclawGateway        int `json:"openclaw_gateway"`
					OperatorBootstrapHTTPS int `json:"operator_bootstrap_https"`
					OperatorHTTPS          int `json:"operator_https"`
					OperatorPublicHTTPS    int `json:"operator_public_https"`
				}{
					OperatorBootstrapHTTPS: 8441,
				},
			},
		}

		result := config.OperatorBootstrapURL()
		assert.Equal(t, "https://localhost:8441", result)
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
		assert.Equal(t, "~/.g8e", DefaultCredentialsDir)
	})
}

func TestLoadIntegration(t *testing.T) {
	// This is an integration test that uses the actual project structure
	// It verifies that Load works with the real paths.json file

	t.Run("loads real project config", func(t *testing.T) {
		// This test assumes it's run from the project root
		// Skip if not in the expected environment
		projectRoot := "/home/bob/g8e"
		if _, err := os.Stat(projectRoot); os.IsNotExist(err) {
			t.Skip("Project root not found, skipping integration test")
		}

		config, err := Load(projectRoot)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, projectRoot, config.ProjectRoot)
		assert.NotNil(t, config.Paths)

		// Verify port values match paths.json
		assert.Equal(t, 8440, config.OperatorHTTPSPort())
		assert.Equal(t, 8441, config.OperatorBootstrapHTTPSPort())

		// Verify URLs are formatted correctly
		assert.True(t, strings.HasPrefix(config.OperatorHTTPURL(), "https://localhost:"))
		assert.True(t, strings.HasPrefix(config.OperatorBootstrapURL(), "https://localhost:"))
	})
}
