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
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
