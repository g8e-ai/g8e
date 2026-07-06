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
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/netutil"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/pathutil"
)

const (
	DefaultRuntimeDir     = ".g8e"
	DefaultPKIDir         = ".g8e/pki"
	DefaultSecretsDir     = ".g8e/secrets"
	DefaultCredentialsDir = ".g8e"
)

// PathsConfig holds path configuration for test support.
// Production code should use constants.Paths directly.
type PathsConfig struct {
	Host  string `json:"host"`
	Infra struct {
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
	} `json:"infra"`
}

// DefaultPathsConfig returns the default path configuration.
// All paths are relative and resolved from the current working directory.
func DefaultPathsConfig() PathsConfig {
	return PathsConfig{
		Host: "localhost",
	}
}

// DefaultInfraPaths returns the default infra path configuration.
func DefaultInfraPaths() PathsConfig {
	paths := DefaultPathsConfig()
	paths.Infra.AppCertDir = ".g8e/pki/issued/apps"
	paths.Infra.CACertPath = ".g8e/pki/trust/g8eg-ca-bundle.pem"
	paths.Infra.DBPath = ".g8e/data/g8e.db"
	paths.Infra.DocsDir = ".g8e/docs"
	paths.Infra.PKIDir = ".g8e/pki"
	paths.Infra.ProtocolConstantsDir = ".g8e/protocol/constants"
	paths.Infra.ProtocolDir = ".g8e/protocol"
	paths.Infra.ProtocolModelsDir = ".g8e/protocol/models"
	paths.Infra.SecretsDir = ".g8e/secrets"
	paths.Infra.SSHConfigPath = ".g8e/ssh_config"
	paths.Infra.VaultDir = ".g8e/vault"
	paths.Infra.VaultKeyPath = ".g8e/vault/key"
	return paths
}

// Config holds CLI configuration resolved from constants.Paths.
// All paths are sourced from internal/constants/paths.go (SSOT).
type Config struct {
	ProjectRoot    string
	RuntimeDir     string
	PKIDir         string
	SecretsDir     string
	CredentialsDir string
	Paths          *PathsConfig
}

// expandPath expands ~ to the user home directory and environment variables in a path.
func expandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	path = os.ExpandEnv(path)
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: expandPath: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

// Load initializes CLI configuration from the current working directory.
// All paths are resolved using constants.InitPathsWithBase.
func Load(projectRoot string) (*Config, error) {
	if projectRoot == "" {
		var err error
		projectRoot, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cli config: failed to get working directory: %w", err)
		}
	}

	if err := paths.InitWithBase(projectRoot); err != nil {
		return nil, fmt.Errorf("cli config: failed to initialize paths: %w", err)
	}

	pathsCfg := DefaultInfraPaths()
	// paths.InitWithBase already makes all paths.Infra.* absolute
	// Copy them directly without re-joining with projectRoot
	pathsCfg.Infra.ProtocolDir = paths.Infra.ProtocolDir
	pathsCfg.Infra.ProtocolConstantsDir = paths.Infra.ProtocolConstantsDir
	pathsCfg.Infra.ProtocolModelsDir = paths.Infra.ProtocolModelsDir
	pathsCfg.Infra.DBPath = paths.Infra.DbPath
	pathsCfg.Infra.PKIDir = paths.Infra.PkiDir
	pathsCfg.Infra.CACertPath = paths.Infra.CaCertPath
	pathsCfg.Infra.SecretsDir = paths.Infra.SecretsDir
	pathsCfg.Infra.AppCertDir = paths.Infra.AppCertDir
	pathsCfg.Infra.DocsDir = paths.Infra.DocsDir
	pathsCfg.Infra.SSHConfigPath = paths.Infra.SshConfigPath
	pathsCfg.Infra.VaultDir = paths.Infra.VaultDir
	pathsCfg.Infra.VaultKeyPath = paths.Infra.VaultKeyPath

	return &Config{
		ProjectRoot:    projectRoot,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: paths.Infra.RuntimeDir,
		Paths:          &pathsCfg,
	}, nil
}

// LoadWithPaths loads config with custom paths configuration for testing.
// This allows hermetic test environments without relying on disk paths.
// Production code should use Load() instead.
func LoadWithPaths(projectRoot string, pathsData []byte) (*Config, error) {
	if projectRoot == "" {
		var err error
		projectRoot, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cli config: failed to get working directory: %w", err)
		}
	}

	runtimeDir := pathutil.SafeJoin(projectRoot, constants.RuntimeDirname)
	pkiDir := pathutil.SafeJoin(projectRoot, constants.DefaultPKIDir)
	secretsDir := pathutil.SafeJoin(projectRoot, constants.DefaultSecretsDir)
	credentialsDir := pathutil.SafeJoin(projectRoot, constants.RuntimeDirname)

	var paths PathsConfig
	if err := json.Unmarshal(pathsData, &paths); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToParsePaths, err)
	}

	resolveInfraPaths(&paths, projectRoot)

	return &Config{
		ProjectRoot:    projectRoot,
		RuntimeDir:     runtimeDir,
		PKIDir:         pkiDir,
		SecretsDir:     secretsDir,
		CredentialsDir: credentialsDir,
		Paths:          &paths,
	}, nil
}

// resolveInfraPaths resolves all relative paths in the infra section relative to projectRoot.
// This is test-only helper for LoadWithPaths.
// Uses pathutil.SafeJoin to handle cross-platform path joining correctly.
func resolveInfraPaths(paths *PathsConfig, projectRoot string) {
	if paths.Infra.AppCertDir != "" && !filepath.IsAbs(paths.Infra.AppCertDir) {
		paths.Infra.AppCertDir = pathutil.SafeJoin(projectRoot, paths.Infra.AppCertDir)
	}
	if paths.Infra.CACertPath != "" && !filepath.IsAbs(paths.Infra.CACertPath) {
		paths.Infra.CACertPath = pathutil.SafeJoin(projectRoot, paths.Infra.CACertPath)
	}
	if paths.Infra.DBPath != "" && !filepath.IsAbs(paths.Infra.DBPath) {
		paths.Infra.DBPath = pathutil.SafeJoin(projectRoot, paths.Infra.DBPath)
	}
	if paths.Infra.DocsDir != "" && !filepath.IsAbs(paths.Infra.DocsDir) {
		paths.Infra.DocsDir = pathutil.SafeJoin(projectRoot, paths.Infra.DocsDir)
	}
	if paths.Infra.PKIDir != "" && !filepath.IsAbs(paths.Infra.PKIDir) {
		paths.Infra.PKIDir = pathutil.SafeJoin(projectRoot, paths.Infra.PKIDir)
	}
	if paths.Infra.ProtocolConstantsDir != "" && !filepath.IsAbs(paths.Infra.ProtocolConstantsDir) {
		paths.Infra.ProtocolConstantsDir = pathutil.SafeJoin(projectRoot, paths.Infra.ProtocolConstantsDir)
	}
	if paths.Infra.ProtocolDir != "" && !filepath.IsAbs(paths.Infra.ProtocolDir) {
		paths.Infra.ProtocolDir = pathutil.SafeJoin(projectRoot, paths.Infra.ProtocolDir)
	}
	if paths.Infra.ProtocolModelsDir != "" && !filepath.IsAbs(paths.Infra.ProtocolModelsDir) {
		paths.Infra.ProtocolModelsDir = pathutil.SafeJoin(projectRoot, paths.Infra.ProtocolModelsDir)
	}
	if paths.Infra.SecretsDir != "" && !filepath.IsAbs(paths.Infra.SecretsDir) {
		paths.Infra.SecretsDir = pathutil.SafeJoin(projectRoot, paths.Infra.SecretsDir)
	}
	if paths.Infra.SSHConfigPath != "" && !filepath.IsAbs(paths.Infra.SSHConfigPath) {
		paths.Infra.SSHConfigPath = pathutil.SafeJoin(projectRoot, paths.Infra.SSHConfigPath)
	}
	if paths.Infra.VaultDir != "" && !filepath.IsAbs(paths.Infra.VaultDir) {
		paths.Infra.VaultDir = pathutil.SafeJoin(projectRoot, paths.Infra.VaultDir)
	}
	if paths.Infra.VaultKeyPath != "" && !filepath.IsAbs(paths.Infra.VaultKeyPath) {
		paths.Infra.VaultKeyPath = pathutil.SafeJoin(projectRoot, paths.Infra.VaultKeyPath)
	}
}

func (c *Config) TrustBundlePath() string {
	if c.Paths == nil {
		return paths.Infra.CaCertPath
	}
	if c.Paths.Infra.CACertPath == "" {
		return ""
	}
	if filepath.IsAbs(c.Paths.Infra.CACertPath) {
		return c.Paths.Infra.CACertPath
	}
	// Use pathutil.SafeJoin for cross-platform safety when joining relative paths
	return pathutil.SafeJoin(c.ProjectRoot, c.Paths.Infra.CACertPath)
}

func (c *Config) CredentialsFile() string {
	return filepath.Join(c.CredentialsDir, constants.CredentialsFilename)
}

func (c *Config) CLICertFile() string {
	return filepath.Join(c.CredentialsDir, constants.CliCertFilename)
}

func (c *Config) CLIKeyFile() string {
	return filepath.Join(c.CredentialsDir, constants.CliKeyFilename)
}

func (c *Config) AppCertFile(name string) string {
	return filepath.Join(c.CredentialsDir, constants.PkiSubdirApps, name+constants.FileExtCert)
}

func (c *Config) AppKeyFile(name string) string {
	return filepath.Join(c.CredentialsDir, constants.PkiSubdirApps, name+constants.FileExtKey)
}

func (c *Config) OperatorCertFile() string {
	return filepath.Join(c.CredentialsDir, constants.PkiFileOperatorCert)
}

func (c *Config) OperatorKeyFile() string {
	return filepath.Join(c.CredentialsDir, constants.PkiFileOperatorKey)
}

func (c *Config) TrustBundleFile() string {
	return filepath.Join(c.CredentialsDir, constants.PkiFileGatewayBundle)
}

func (c *Config) OperatorHTTPSPort() int {
	return constants.Ports.OperatorHttps
}

// OperatorHTTPURL returns the HTTPS URL for the operator API.
// When cfg.Paths.Host is a full URL (contains "://"), it is returned directly.
// This allows tests to override the URL by setting cfg.Paths.Host to the test server URL.
func (c *Config) OperatorHTTPURL() string {
	if endpointOverride != "" {
		if strings.Contains(endpointOverride, "://") {
			return endpointOverride
		}
		if _, _, err := net.SplitHostPort(endpointOverride); err != nil {
			return fmt.Sprintf("https://%s:%d", endpointOverride, constants.Ports.OperatorHttps)
		}
		return fmt.Sprintf("https://%s", endpointOverride)
	}
	if c.Paths != nil && strings.Contains(c.Paths.Host, "://") {
		return c.Paths.Host
	}
	return netutil.LocalhostHTTPSURL(c.OperatorHTTPSPort())
}

// endpointOverride is set by the -e/--endpoint persistent flag to allow
// connecting to a remote gateway instead of localhost.
var endpointOverride string

// SetEndpointOverride sets the gateway endpoint override (host or host:port).
func SetEndpointOverride(endpoint string) {
	endpointOverride = endpoint
}

// SetEndpointOverrideWithPort combines a host and port into a host:port endpoint override.
// If the host already contains a port, it is replaced with the specified port.
func SetEndpointOverrideWithPort(host string, port int) {
	if strings.Contains(host, "://") {
		// Full URL provided — use as-is
		endpointOverride = host
		return
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		// Host already has a port — replace it
		h, _, _ := net.SplitHostPort(host)
		host = h
	}
	endpointOverride = fmt.Sprintf("%s:%d", host, port)
}

// OperatorPublicURL returns the HTTPS port for mTLS API and public surface
func (c *Config) OperatorPublicURL() string {
	if endpointOverride != "" {
		if strings.Contains(endpointOverride, "://") {
			return endpointOverride
		}
		if _, _, err := net.SplitHostPort(endpointOverride); err != nil {
			return fmt.Sprintf("https://%s:%d", endpointOverride, constants.Ports.OperatorHttps)
		}
		return fmt.Sprintf("https://%s", endpointOverride)
	}
	return netutil.LocalhostHTTPSURL(c.OperatorHTTPSPort())
}

// OperatorDiscoveryURL returns the HTTP port for CA download and bootstrap routes
func (c *Config) OperatorDiscoveryURL() string {
	if endpointOverride != "" {
		if strings.Contains(endpointOverride, "://") {
			return endpointOverride
		}
		if _, _, err := net.SplitHostPort(endpointOverride); err != nil {
			return fmt.Sprintf("http://%s:%d", endpointOverride, constants.Ports.OperatorHttp)
		}
		return fmt.Sprintf("http://%s", endpointOverride)
	}
	return netutil.LocalhostHTTPURL(constants.Ports.OperatorHttp)
}
