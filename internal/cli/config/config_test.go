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

// os.Chdir is used because these tests verify config.Load("") which reads
// from the current working directory by design. The config package is the
// layer that translates cwd into fileSvc baseDir. This is a legitimate cwd
// usage — not .g8e/ runtime state alignment.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandPath(t *testing.T) {
	t.Run("empty path returns empty", func(t *testing.T) {
		result, err := expandPath("")
		require.NoError(t, err)
		assert.Empty(t, result)
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
		tempDir := testutil.TempDir(t)

		config, err := Load(tempDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, tempDir, config.ProjectRoot)
		assert.Equal(t, filepath.Join(tempDir, DefaultRuntimeDir), config.RuntimeDir)
		assert.Equal(t, filepath.Join(tempDir, DefaultPKIDir), config.PKIDir)
		assert.Equal(t, filepath.Join(tempDir, DefaultSecretsDir), config.SecretsDir)
		assert.Contains(t, config.RuntimeDir, ".g8e")
		assert.NotNil(t, config.Paths)
	})

	t.Run("uses current directory when project root is empty", func(t *testing.T) {
		tempDir := testutil.TempDir(t)

		// Change to temp directory and load with empty project root
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		err = os.Chdir(tempDir)
		require.NoError(t, err)

		config, err := Load("")
		require.NoError(t, err)
		assert.NotNil(t, config)
		actualWd, err := os.Getwd()
		require.NoError(t, err)
		assert.Equal(t, actualWd, config.ProjectRoot)
	})

	t.Run("always uses embedded defaults regardless of file presence", func(t *testing.T) {
		tempDir := testutil.TempDir(t)

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

func TestConfig_DefaultTrustBundleRelPath(t *testing.T) {
	t.Run("returns canonical runtime-relative path", func(t *testing.T) {
		config := &Config{}
		result := config.DefaultTrustBundleRelPath()
		assert.Equal(t, constants.PkiDirname+"/"+constants.PkiSubdirTrust+"/"+constants.PkiFileGatewayBundle, result)
	})
}

func TestConfig_CustomTrustBundlePath(t *testing.T) {
	t.Run("returns absolute custom path when set", func(t *testing.T) {
		caPath := filepath.Join(testutil.TempDir(t), "absolute", "path", "to", "ca.pem")
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
					CACertPath: caPath,
				},
			},
		}

		result := config.CustomTrustBundlePath()
		assert.Equal(t, caPath, result)
	})

	t.Run("joins relative custom path with project root", func(t *testing.T) {
		projectRoot := filepath.Join(string(filepath.Separator), "project", "root")
		config := &Config{
			ProjectRoot: projectRoot,
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

		result := config.CustomTrustBundlePath()
		assert.Equal(t, filepath.Join(projectRoot, "relative", "path", "to", "ca.pem"), result)
	})

	t.Run("returns empty when CACertPath is empty", func(t *testing.T) {
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

		result := config.CustomTrustBundlePath()
		assert.Empty(t, result)
	})

	t.Run("returns empty when CACertPath matches default runtime location", func(t *testing.T) {
		runtimeDir := filepath.Join(string(filepath.Separator), "project", "root", ".g8e")
		defaultPath := filepath.Join(runtimeDir, constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
		config := &Config{
			ProjectRoot: "/project/root",
			RuntimeDir:  runtimeDir,
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
					CACertPath: defaultPath,
				},
			},
		}

		result := config.CustomTrustBundlePath()
		assert.Empty(t, result)
	})
}

func TestConfig_ResolvedTrustBundlePath(t *testing.T) {
	t.Run("returns custom path when set", func(t *testing.T) {
		caPath := filepath.Join(testutil.TempDir(t), "custom", "ca.pem")
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
					CACertPath: caPath,
				},
			},
		}

		result := config.ResolvedTrustBundlePath()
		assert.Equal(t, caPath, result)
	})

	t.Run("returns default runtime path when no custom path", func(t *testing.T) {
		runtimeDir := filepath.Join(string(filepath.Separator), "project", "root", ".g8e")
		config := &Config{
			ProjectRoot: "/project/root",
			RuntimeDir:  runtimeDir,
			Paths:       &PathsConfig{},
		}

		result := config.ResolvedTrustBundlePath()
		assert.Equal(t, filepath.Join(runtimeDir, constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle), result)
	})
}

func TestConfig_CredentialsFile(t *testing.T) {
	t.Run("returns credentials file path", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.CredentialsFile()
		assert.Equal(t, filepath.Join(credentialsDir, "credentials"), result)
	})
}

func TestConfig_CLICertFile(t *testing.T) {
	t.Run("returns CLI cert file path", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.CLICertFile()
		assert.Equal(t, filepath.Join(credentialsDir, "cli.crt"), result)
	})
}

func TestConfig_CLIKeyFile(t *testing.T) {
	t.Run("returns CLI key file path", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.CLIKeyFile()
		assert.Equal(t, filepath.Join(credentialsDir, "cli.key"), result)
	})
}

func TestConfig_OperatorCertFile(t *testing.T) {
	t.Run("returns Operator cert file path", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.OperatorCertFile()
		assert.Equal(t, filepath.Join(credentialsDir, "operator.crt"), result)
	})
}

func TestConfig_OperatorKeyFile(t *testing.T) {
	t.Run("returns Operator key file path", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.OperatorKeyFile()
		assert.Equal(t, filepath.Join(credentialsDir, "operator.key"), result)
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
		assert.Equal(t, network.LocalhostHTTPSURL(constants.Ports.OperatorHttps), result)
	})

}

func TestConfig_OperatorPublicURL(t *testing.T) {
	t.Run("returns Operator public TLS URL for CSR-based enrollment", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{},
		}

		result := config.OperatorPublicURL()
		assert.Equal(t, network.LocalhostHTTPSURL(constants.Ports.OperatorHttps), result)
	})
}

func TestConfig_OperatorDiscoveryURL(t *testing.T) {
	t.Run("returns Operator discovery URL for CA download over plain HTTP", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{},
		}

		result := config.OperatorDiscoveryURL()
		assert.Equal(t, network.LocalhostHTTPURL(constants.Ports.OperatorHttp), result)
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

}

func TestLoadWithPaths(t *testing.T) {
	t.Run("loads config with custom paths from struct", func(t *testing.T) {
		tempDir := testutil.TempDir(t)

		customPaths := DefaultInfraPaths()
		customPaths.Host = "test-host"
		customPaths.Infra.AppCertDir = "custom/app/certs"
		customPaths.Infra.CACertPath = "custom/ca.pem"
		customPaths.Infra.DBPath = "custom/data.db"
		customPaths.Infra.DocsDir = "custom/docs"
		customPaths.Infra.PKIDir = "custom/pki"
		customPaths.Infra.ProtocolConstantsDir = "custom/protocol/constants"
		customPaths.Infra.ProtocolDir = "custom/protocol"
		customPaths.Infra.ProtocolModelsDir = "custom/protocol/models"
		customPaths.Infra.SecretsDir = "custom/secrets"
		customPaths.Infra.SSHConfigPath = "custom/ssh_config"
		customPaths.Infra.VaultDir = "custom/vault"
		customPaths.Infra.VaultKeyPath = "custom/vault/key"

		// Convert to JSON for LoadWithPaths
		pathsData, err := json.Marshal(customPaths)
		require.NoError(t, err)

		config, err := LoadWithPaths(tempDir, pathsData)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, tempDir, config.ProjectRoot)
		assert.Equal(t, "test-host", config.Paths.Host)
		assert.Equal(t, filepath.Join(tempDir, "custom/app/certs"), config.Paths.Infra.AppCertDir)
		assert.Equal(t, filepath.Join(tempDir, "custom/ca.pem"), config.Paths.Infra.CACertPath)
		assert.Equal(t, filepath.Join(tempDir, "custom/data.db"), config.Paths.Infra.DBPath)
	})

	t.Run("uses current directory when project root is empty", func(t *testing.T) {
		tempDir := testutil.TempDir(t)

		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		err = os.Chdir(tempDir)
		require.NoError(t, err)

		customPaths := DefaultPathsConfig()
		pathsData, err := json.Marshal(customPaths)
		require.NoError(t, err)

		config, err := LoadWithPaths("", pathsData)
		require.NoError(t, err)
		assert.NotNil(t, config)
		actualWd, err := os.Getwd()
		require.NoError(t, err)
		assert.Equal(t, actualWd, config.ProjectRoot)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		tempDir := testutil.TempDir(t)

		invalidJSON := `{"host": invalid}`

		config, err := LoadWithPaths(tempDir, []byte(invalidJSON))
		require.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to parse paths")
	})

	t.Run("resolves absolute paths as-is", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		absPath := filepath.Join(testutil.TempDir(t), "absolute", "path", "to", "cert.pem")

		customPaths := DefaultInfraPaths()
		customPaths.Infra.CACertPath = absPath
		pathsData, err := json.Marshal(customPaths)
		require.NoError(t, err)

		config, err := LoadWithPaths(tempDir, pathsData)
		require.NoError(t, err)
		assert.Equal(t, absPath, config.Paths.Infra.CACertPath)
	})

	t.Run("resolves relative paths relative to project root", func(t *testing.T) {
		tempDir := testutil.TempDir(t)

		customPaths := DefaultInfraPaths()
		customPaths.Infra.CACertPath = "relative/ca.pem"
		pathsData, err := json.Marshal(customPaths)
		require.NoError(t, err)

		config, err := LoadWithPaths(tempDir, pathsData)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tempDir, "relative/ca.pem"), config.Paths.Infra.CACertPath)
	})

	t.Run("handles empty infra fields gracefully", func(t *testing.T) {
		tempDir := testutil.TempDir(t)

		customPaths := DefaultInfraPaths()
		customPaths.Infra.AppCertDir = ""
		customPaths.Infra.CACertPath = ""
		customPaths.Infra.DBPath = ""
		pathsData, err := json.Marshal(customPaths)
		require.NoError(t, err)

		config, err := LoadWithPaths(tempDir, pathsData)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Empty(t, config.Paths.Infra.AppCertDir)
		assert.Empty(t, config.Paths.Infra.CACertPath)
		assert.Empty(t, config.Paths.Infra.DBPath)
	})
}

func TestResolveInfraPaths(t *testing.T) {
	t.Run("resolves all relative paths relative to project root", func(t *testing.T) {
		projectRoot := "/project/root"
		paths := &PathsConfig{
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
				AppCertDir:           "relative/app/certs",
				CACertPath:           "relative/ca.pem",
				DBPath:               "relative/data.db",
				DocsDir:              "relative/docs",
				PKIDir:               "relative/pki",
				ProtocolConstantsDir: "relative/protocol/constants",
				ProtocolDir:          "relative/protocol",
				ProtocolModelsDir:    "relative/protocol/models",
				SecretsDir:           "relative/secrets",
				SSHConfigPath:        "relative/ssh_config",
				VaultDir:             "relative/vault",
				VaultKeyPath:         "relative/vault/key",
			},
		}

		resolveInfraPaths(paths, projectRoot)

		assert.Equal(t, filepath.Join(projectRoot, "relative/app/certs"), paths.Infra.AppCertDir)
		assert.Equal(t, filepath.Join(projectRoot, "relative/ca.pem"), paths.Infra.CACertPath)
		assert.Equal(t, filepath.Join(projectRoot, "relative/data.db"), paths.Infra.DBPath)
		assert.Equal(t, filepath.Join(projectRoot, "relative/docs"), paths.Infra.DocsDir)
		assert.Equal(t, filepath.Join(projectRoot, "relative/pki"), paths.Infra.PKIDir)
		assert.Equal(t, filepath.Join(projectRoot, "relative/protocol/constants"), paths.Infra.ProtocolConstantsDir)
		assert.Equal(t, filepath.Join(projectRoot, "relative/protocol"), paths.Infra.ProtocolDir)
		assert.Equal(t, filepath.Join(projectRoot, "relative/protocol/models"), paths.Infra.ProtocolModelsDir)
		assert.Equal(t, filepath.Join(projectRoot, "relative/secrets"), paths.Infra.SecretsDir)
		assert.Equal(t, filepath.Join(projectRoot, "relative/ssh_config"), paths.Infra.SSHConfigPath)
		assert.Equal(t, filepath.Join(projectRoot, "relative/vault"), paths.Infra.VaultDir)
		assert.Equal(t, filepath.Join(projectRoot, "relative/vault/key"), paths.Infra.VaultKeyPath)
	})

	t.Run("preserves absolute paths unchanged", func(t *testing.T) {
		projectRoot := "/project/root"
		cwd, _ := os.Getwd()
		absPath := filepath.Join(cwd, constants.TestTempDirname, "absolute")
		paths := &PathsConfig{
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
				AppCertDir: absPath,
				CACertPath: absPath,
				DBPath:     absPath,
			},
		}

		resolveInfraPaths(paths, projectRoot)

		assert.Equal(t, absPath, paths.Infra.AppCertDir)
		assert.Equal(t, absPath, paths.Infra.CACertPath)
		assert.Equal(t, absPath, paths.Infra.DBPath)
	})

	t.Run("handles empty strings gracefully", func(t *testing.T) {
		projectRoot := "/project/root"
		paths := &PathsConfig{
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
				AppCertDir: "",
				CACertPath: "",
				DBPath:     "",
			},
		}

		resolveInfraPaths(paths, projectRoot)

		assert.Empty(t, paths.Infra.AppCertDir)
		assert.Empty(t, paths.Infra.CACertPath)
		assert.Empty(t, paths.Infra.DBPath)
	})
}

func TestConfig_AppCertFile(t *testing.T) {
	t.Run("returns app cert file path with name", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.AppCertFile("myapp")
		assert.Equal(t, filepath.Join(credentialsDir, "apps", "myapp.crt"), result)
	})

	t.Run("handles empty app name", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.AppCertFile("")
		assert.Equal(t, filepath.Join(credentialsDir, "apps", ".crt"), result)
	})
}

func TestConfig_AppKeyFile(t *testing.T) {
	t.Run("returns app key file path with name", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.AppKeyFile("myapp")
		assert.Equal(t, filepath.Join(credentialsDir, "apps", "myapp.key"), result)
	})

	t.Run("handles empty app name", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.AppKeyFile("")
		assert.Equal(t, filepath.Join(credentialsDir, "apps", ".key"), result)
	})
}

func TestConfig_TrustBundleFile(t *testing.T) {
	t.Run("returns trust bundle file path", func(t *testing.T) {
		credentialsDir := filepath.Join(string(filepath.Separator), "credentials", "dir")
		config := &Config{
			RuntimeDir: credentialsDir,
		}

		result := config.TrustBundleFile()
		assert.Equal(t, filepath.Join(credentialsDir, "g8eg-ca-bundle.pem"), result)
	})
}

func TestConfig_OperatorHTTPURL_Override(t *testing.T) {
	t.Run("returns custom URL when Host contains protocol", func(t *testing.T) {
		customURL := "https://custom-test-server:8443"
		config := &Config{
			Paths: &PathsConfig{
				Host: customURL,
			},
		}

		result := config.OperatorHTTPURL()
		assert.Equal(t, customURL, result)
	})

	t.Run("returns default localhost URL when Host is simple hostname", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{
				Host: "localhost",
			},
		}

		result := config.OperatorHTTPURL()
		assert.Equal(t, network.LocalhostHTTPSURL(constants.Ports.OperatorHttps), result)
	})

	t.Run("returns default localhost URL when Host is IP address", func(t *testing.T) {
		config := &Config{
			Paths: &PathsConfig{
				Host: "127.0.0.1",
			},
		}

		result := config.OperatorHTTPURL()
		assert.Equal(t, network.LocalhostHTTPSURL(constants.Ports.OperatorHttps), result)
	})

	t.Run("returns default localhost URL when Paths is nil", func(t *testing.T) {
		config := &Config{
			Paths: nil,
		}

		result := config.OperatorHTTPURL()
		assert.Equal(t, network.LocalhostHTTPSURL(constants.Ports.OperatorHttps), result)
	})

	t.Run("handles http:// protocol override", func(t *testing.T) {
		customURL := "http://custom-test-server:8080"
		config := &Config{
			Paths: &PathsConfig{
				Host: customURL,
			},
		}

		result := config.OperatorHTTPURL()
		assert.Equal(t, customURL, result)
	})
}

func TestConfig_CustomTrustBundlePath_NilPaths(t *testing.T) {
	t.Run("returns empty when Paths is nil", func(t *testing.T) {
		config := &Config{
			ProjectRoot: "/project/root",
			Paths:       nil,
		}

		result := config.CustomTrustBundlePath()
		assert.Empty(t, result)
	})
}

func TestConfig_ResolvedTrustBundlePath_NilPaths(t *testing.T) {
	t.Run("returns default runtime path when Paths is nil", func(t *testing.T) {
		runtimeDir := filepath.Join(string(filepath.Separator), "project", "root", ".g8e")
		config := &Config{
			ProjectRoot: "/project/root",
			RuntimeDir:  runtimeDir,
			Paths:       nil,
		}

		result := config.ResolvedTrustBundlePath()
		assert.Equal(t, filepath.Join(runtimeDir, constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle), result)
	})
}

func TestLoadIntegration(t *testing.T) {
	// This is an integration test that verifies the embedded-only behavior

	t.Run("loads embedded default paths from any directory", func(t *testing.T) {
		// This test verifies the self-sovereign binary behavior:
		// The binary always uses embedded default paths regardless of directory structure.

		tempDir := testutil.TempDir(t)

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
		actualWd, err := os.Getwd()
		require.NoError(t, err)
		assert.Equal(t, actualWd, config.ProjectRoot)

		// Verify embedded default paths are resolved relative to tempDir
		assert.Equal(t, filepath.Join(actualWd, ".g8e"), config.RuntimeDir)
		assert.Equal(t, filepath.Join(actualWd, ".g8e/pki"), config.PKIDir)
		assert.Equal(t, filepath.Join(actualWd, ".g8e/secrets"), config.SecretsDir)

		// Verify protocol paths are relative
		assert.Equal(t, filepath.Join(actualWd, ".g8e/protocol"), config.Paths.Infra.ProtocolDir)
		assert.Equal(t, filepath.Join(actualWd, ".g8e/protocol/constants"), config.Paths.Infra.ProtocolConstantsDir)
		assert.Equal(t, filepath.Join(actualWd, ".g8e/protocol/models"), config.Paths.Infra.ProtocolModelsDir)

		// Verify port values from constants
		assert.Equal(t, constants.Ports.OperatorHttps, config.OperatorHTTPSPort())
	})
}

func TestSetHTTPEndpointOverride_OnlyAffectsDiscoveryURL(t *testing.T) {
	SetEndpointOverride("")
	t.Cleanup(func() { SetEndpointOverride("") })

	SetHTTPEndpointOverride("remote.example.com")

	cfg := &Config{Paths: &PathsConfig{}}
	assert.Contains(t, cfg.OperatorDiscoveryURL(), "remote.example.com")
	assert.Contains(t, cfg.OperatorDiscoveryURL(), "http://")
	assert.NotContains(t, cfg.OperatorPublicURL(), "remote.example.com")
	assert.NotContains(t, cfg.OperatorHTTPURL(), "remote.example.com")
}

func TestSetHTTPSEndpointOverride_OnlyAffectsPublicAndHTTPURL(t *testing.T) {
	SetEndpointOverride("")
	t.Cleanup(func() { SetEndpointOverride("") })

	SetHTTPSEndpointOverride("remote.example.com")

	cfg := &Config{Paths: &PathsConfig{}}
	assert.Contains(t, cfg.OperatorPublicURL(), "remote.example.com")
	assert.Contains(t, cfg.OperatorPublicURL(), "https://")
	assert.Contains(t, cfg.OperatorHTTPURL(), "remote.example.com")
	assert.Contains(t, cfg.OperatorHTTPURL(), "https://")
	assert.NotContains(t, cfg.OperatorDiscoveryURL(), "remote.example.com")
}

func TestSetEndpointOverride_AffectsBoth(t *testing.T) {
	SetEndpointOverride("")
	t.Cleanup(func() { SetEndpointOverride("") })

	SetEndpointOverride("remote.example.com")

	cfg := &Config{Paths: &PathsConfig{}}
	assert.Contains(t, cfg.OperatorDiscoveryURL(), "remote.example.com")
	assert.Contains(t, cfg.OperatorPublicURL(), "remote.example.com")
	assert.Contains(t, cfg.OperatorHTTPURL(), "remote.example.com")
}

func TestSetEndpointOverride_FullURLReturnedAsIs(t *testing.T) {
	SetEndpointOverride("")
	t.Cleanup(func() { SetEndpointOverride("") })

	fullURL := "http://test-server:9090"
	SetEndpointOverride(fullURL)

	cfg := &Config{Paths: &PathsConfig{}}
	assert.Equal(t, fullURL, cfg.OperatorDiscoveryURL())
	assert.Equal(t, fullURL, cfg.OperatorPublicURL())
	assert.Equal(t, fullURL, cfg.OperatorHTTPURL())
}

func TestSetEndpointOverrideWithPort_SplitsHTTPOffsetAndHTTPSPort(t *testing.T) {
	SetEndpointOverride("")
	t.Cleanup(func() { SetEndpointOverride("") })

	SetEndpointOverrideWithPort("remote.example.com", 9999)

	cfg := &Config{Paths: &PathsConfig{}}

	discoveryURL := cfg.OperatorDiscoveryURL()
	assert.Contains(t, discoveryURL, "remote.example.com")
	assert.NotContains(t, discoveryURL, "9999")
	assert.Contains(t, discoveryURL, "http://")

	publicURL := cfg.OperatorPublicURL()
	assert.Contains(t, publicURL, "remote.example.com:9999")
	assert.Contains(t, publicURL, "https://")
}

func TestSetEndpointOverrideWithPort_StripsExistingPortFromHost(t *testing.T) {
	SetEndpointOverride("")
	t.Cleanup(func() { SetEndpointOverride("") })

	SetEndpointOverrideWithPort("remote.example.com:8085", 9999)

	cfg := &Config{Paths: &PathsConfig{}}

	discoveryURL := cfg.OperatorDiscoveryURL()
	assert.Contains(t, discoveryURL, "remote.example.com")
	assert.NotContains(t, discoveryURL, "8085")
	assert.NotContains(t, discoveryURL, "9999")

	publicURL := cfg.OperatorPublicURL()
	assert.Contains(t, publicURL, "remote.example.com:9999")
	assert.NotContains(t, publicURL, "8085")
}

func TestSetEndpointOverrideWithPort_FullURLSetsBoth(t *testing.T) {
	SetEndpointOverride("")
	t.Cleanup(func() { SetEndpointOverride("") })

	fullURL := "http://test-server:9090"
	SetEndpointOverrideWithPort(fullURL, 9999)

	cfg := &Config{Paths: &PathsConfig{}}
	assert.Equal(t, fullURL, cfg.OperatorDiscoveryURL())
	assert.Equal(t, fullURL, cfg.OperatorPublicURL())
}

func TestOperatorPublicURL_HostFullURL(t *testing.T) {
	t.Run("returns custom URL when Host contains protocol", func(t *testing.T) {
		customURL := "https://custom-test-server:8443"
		cfg := &Config{
			Paths: &PathsConfig{
				Host: customURL,
			},
		}

		result := cfg.OperatorPublicURL()
		assert.Equal(t, customURL, result)
	})

	t.Run("returns default localhost URL when Host is simple hostname", func(t *testing.T) {
		cfg := &Config{
			Paths: &PathsConfig{
				Host: "localhost",
			},
		}

		result := cfg.OperatorPublicURL()
		assert.Equal(t, network.LocalhostHTTPSURL(constants.Ports.OperatorHttps), result)
	})

	t.Run("returns default localhost URL when Paths is nil", func(t *testing.T) {
		cfg := &Config{
			Paths: nil,
		}

		result := cfg.OperatorPublicURL()
		assert.Equal(t, network.LocalhostHTTPSURL(constants.Ports.OperatorHttps), result)
	})
}
