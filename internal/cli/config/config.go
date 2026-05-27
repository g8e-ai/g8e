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
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// defaultPathsJSON contains embedded default path configuration. This is the sole source of truth
// for path configuration in the g8e binary. All paths are relative and resolved from the current
// working directory. The binary is fully self-sovereign and requires no external configuration files.
//
//go:embed paths_default.json
var defaultPathsJSON []byte

const (
	DefaultRuntimeDir     = ".g8e"
	DefaultPKIDir         = ".g8e/pki"
	DefaultSecretsDir     = ".g8e/secrets"
	DefaultCredentialsDir = "~/.g8e"
)

// expandPath expands tilde (~) to the user's home directory and expands environment variables
func expandPath(path string) (string, error) {
	if path == "" {
		return path, nil
	}

	// Expand tilde to home directory
	if strings.HasPrefix(path, "~/") || path == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		if path == "~" {
			return homeDir, nil
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	return path, nil
}

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
	} `json:"infra"`
	Ports struct {
		G8eeHTTPS              int `json:"g8ee_https"`
		InsecureMcpGateway     int `json:"insecure_mcp_gateway"`
		OperatorBootstrapHTTPS int `json:"operator_bootstrap_https"`
		OperatorHTTPS          int `json:"operator_https"`
		OperatorPublicHTTPS    int `json:"operator_public_https"`
	} `json:"ports"`
}

type Config struct {
	ProjectRoot    string
	RuntimeDir     string
	PKIDir         string
	SecretsDir     string
	CredentialsDir string
	Paths          *PathsConfig
}

func Load(projectRoot string) (*Config, error) {
	if projectRoot == "" {
		var err error
		projectRoot, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	runtimeDir := filepath.Join(projectRoot, DefaultRuntimeDir)
	pkiDir := filepath.Join(projectRoot, DefaultPKIDir)
	secretsDir := filepath.Join(projectRoot, DefaultSecretsDir)

	credentialsDir, err := expandPath(DefaultCredentialsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to expand credentials directory: %w", err)
	}

	// Always use embedded default paths configuration
	pathsData := defaultPathsJSON

	var paths PathsConfig
	if err := json.Unmarshal(pathsData, &paths); err != nil {
		return nil, fmt.Errorf("failed to parse embedded paths.json: %w", err)
	}

	// Resolve all relative paths in infra section relative to projectRoot
	if paths.Infra.AppCertDir != "" && !filepath.IsAbs(paths.Infra.AppCertDir) {
		paths.Infra.AppCertDir = filepath.Join(projectRoot, paths.Infra.AppCertDir)
	}
	if paths.Infra.CACertPath != "" && !filepath.IsAbs(paths.Infra.CACertPath) {
		paths.Infra.CACertPath = filepath.Join(projectRoot, paths.Infra.CACertPath)
	}
	if paths.Infra.DBPath != "" && !filepath.IsAbs(paths.Infra.DBPath) {
		paths.Infra.DBPath = filepath.Join(projectRoot, paths.Infra.DBPath)
	}
	if paths.Infra.DocsDir != "" && !filepath.IsAbs(paths.Infra.DocsDir) {
		paths.Infra.DocsDir = filepath.Join(projectRoot, paths.Infra.DocsDir)
	}
	if paths.Infra.PKIDir != "" && !filepath.IsAbs(paths.Infra.PKIDir) {
		paths.Infra.PKIDir = filepath.Join(projectRoot, paths.Infra.PKIDir)
	}
	if paths.Infra.ProtocolConstantsDir != "" && !filepath.IsAbs(paths.Infra.ProtocolConstantsDir) {
		paths.Infra.ProtocolConstantsDir = filepath.Join(projectRoot, paths.Infra.ProtocolConstantsDir)
	}
	if paths.Infra.ProtocolDir != "" && !filepath.IsAbs(paths.Infra.ProtocolDir) {
		paths.Infra.ProtocolDir = filepath.Join(projectRoot, paths.Infra.ProtocolDir)
	}
	if paths.Infra.ProtocolModelsDir != "" && !filepath.IsAbs(paths.Infra.ProtocolModelsDir) {
		paths.Infra.ProtocolModelsDir = filepath.Join(projectRoot, paths.Infra.ProtocolModelsDir)
	}
	if paths.Infra.SecretsDir != "" && !filepath.IsAbs(paths.Infra.SecretsDir) {
		paths.Infra.SecretsDir = filepath.Join(projectRoot, paths.Infra.SecretsDir)
	}
	if paths.Infra.SSHConfigPath != "" && !filepath.IsAbs(paths.Infra.SSHConfigPath) {
		paths.Infra.SSHConfigPath = filepath.Join(projectRoot, paths.Infra.SSHConfigPath)
	}

	return &Config{
		ProjectRoot:    projectRoot,
		RuntimeDir:     runtimeDir,
		PKIDir:         pkiDir,
		SecretsDir:     secretsDir,
		CredentialsDir: credentialsDir,
		Paths:          &paths,
	}, nil
}

func (c *Config) TrustBundlePath() string {
	if c.Paths.Infra.CACertPath != "" && !filepath.IsAbs(c.Paths.Infra.CACertPath) {
		return filepath.Join(c.ProjectRoot, c.Paths.Infra.CACertPath)
	}
	return c.Paths.Infra.CACertPath
}

func (c *Config) CredentialsFile() string {
	return filepath.Join(c.CredentialsDir, "credentials")
}

func (c *Config) CLICertFile() string {
	return filepath.Join(c.CredentialsDir, "cli.crt")
}

func (c *Config) CLIKeyFile() string {
	return filepath.Join(c.CredentialsDir, "cli.key")
}

func (c *Config) OperatorCertFile() string {
	return filepath.Join(c.CredentialsDir, "operator.crt")
}

func (c *Config) OperatorKeyFile() string {
	return filepath.Join(c.CredentialsDir, "operator.key")
}

func (c *Config) OperatorHTTPSPort() int {
	return c.Paths.Ports.OperatorHTTPS
}

func (c *Config) OperatorBootstrapHTTPSPort() int {
	return c.Paths.Ports.OperatorBootstrapHTTPS
}

func (c *Config) OperatorPublicHTTPSPort() int {
	return c.Paths.Ports.OperatorPublicHTTPS
}

func (c *Config) G8eeHTTPSPort() int {
	return c.Paths.Ports.G8eeHTTPS
}

func (c *Config) OperatorHTTPURL() string {
	return fmt.Sprintf("https://localhost:%d", c.OperatorHTTPSPort())
}

// OperatorPublicURL returns the Public TLS port (8442) for device-link enrollment
func (c *Config) OperatorPublicURL() string {
	return fmt.Sprintf("https://localhost:%d", c.Paths.Ports.OperatorPublicHTTPS)
}

// OperatorDiscoveryURL returns the bootstrap port (8441) for CA download over plain HTTP
func (c *Config) OperatorDiscoveryURL() string {
	return fmt.Sprintf("http://localhost:%d", c.Paths.Ports.OperatorBootstrapHTTPS)
}

// OperatorBootstrapURL is deprecated; use OperatorPublicURL for device-link enrollment
func (c *Config) OperatorBootstrapURL() string {
	return c.OperatorPublicURL()
}

// GetExternalInterfaceIP returns the first non-loopback IPv4 address found on the host
// This is used for the Operator Bootstrap endpoint which remote operators rely on
func GetExternalInterfaceIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "localhost"
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				return ip.String()
			}
		}
	}

	return "localhost"
}
