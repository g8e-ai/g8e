// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/pathutil"
	"github.com/g8e-ai/g8e/internal/services/network"
)

const (
	DefaultRuntimeDir = ".g8e"
	DefaultPKIDir     = ".g8e/pki"
	DefaultSecretsDir = ".g8e/secrets"
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
	ProjectRoot string
	RuntimeDir  string
	PKIDir      string
	SecretsDir  string
	Paths       *PathsConfig
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
		ProjectRoot: projectRoot,
		RuntimeDir:  paths.Infra.RuntimeDir,
		PKIDir:      paths.Infra.PkiDir,
		SecretsDir:  paths.Infra.SecretsDir,
		Paths:       &pathsCfg,
	}, nil
}

// DefaultTrustBundleRelPath returns the trust bundle path relative to the
// .g8e/ runtime directory root. This is the canonical location for the
// gateway CA bundle and should be used with RuntimeFileService.
func (c *Config) DefaultTrustBundleRelPath() string {
	return constants.PkiDirname + "/" + constants.PkiSubdirTrust + "/" + constants.PkiFileGatewayBundle
}

// ResolvedTrustBundlePath returns the absolute trust bundle path for display,
// env-var propagation, or subprocess config. It returns the custom external
// path when configured, or the default runtime path joined with RuntimeDir.
// Callers that perform file I/O should use ReadTrustBundle/WriteTrustBundleFS/
// RemoveTrustBundleFS with fileSvc instead.
func (c *Config) ResolvedTrustBundlePath() string {
	if custom := c.CustomTrustBundlePath(); custom != "" {
		return custom
	}
	return filepath.Join(c.RuntimeDir, c.DefaultTrustBundleRelPath())
}

// CustomTrustBundlePath returns a user-specified external trust bundle path
// if one has been configured that differs from the default runtime location.
// Returns an empty string when the default runtime path should be used.
func (c *Config) CustomTrustBundlePath() string {
	if c.Paths == nil || c.Paths.Infra.CACertPath == "" {
		return ""
	}
	configured := c.Paths.Infra.CACertPath
	if !filepath.IsAbs(configured) {
		configured = pathutil.SafeJoin(c.ProjectRoot, configured)
	}
	defaultAbs := filepath.Join(c.RuntimeDir, constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
	if configured == defaultAbs {
		return ""
	}
	return configured
}

func (c *Config) CredentialsFile() string {
	return filepath.Join(c.RuntimeDir, constants.CredentialsFilename)
}

func (c *Config) CLICertFile() string {
	return filepath.Join(c.RuntimeDir, constants.CliCertFilename)
}

func (c *Config) CLIKeyFile() string {
	return filepath.Join(c.RuntimeDir, constants.CliKeyFilename)
}

func (c *Config) AppCertFile(name string) string {
	return filepath.Join(c.RuntimeDir, constants.PkiSubdirApps, name+constants.FileExtCert)
}

func (c *Config) AppKeyFile(name string) string {
	return filepath.Join(c.RuntimeDir, constants.PkiSubdirApps, name+constants.FileExtKey)
}

func (c *Config) OperatorCertFile() string {
	return filepath.Join(c.RuntimeDir, constants.PkiFileOperatorCert)
}

func (c *Config) OperatorKeyFile() string {
	return filepath.Join(c.RuntimeDir, constants.PkiFileOperatorKey)
}

func (c *Config) TrustBundleFile() string {
	return filepath.Join(c.RuntimeDir, constants.PkiFileGatewayBundle)
}

func (c *Config) OperatorHTTPSPort() int {
	return constants.Ports.OperatorHttps
}

// OperatorHTTPURL returns the HTTPS URL for the operator API.
// When cfg.Paths.Host is a full URL (contains "://"), it is returned directly.
// This allows tests to override the URL by setting cfg.Paths.Host to the test server URL.
func (c *Config) OperatorHTTPURL() string {
	endpointMu.RLock()
	override := httpsEndpointOverride
	endpointMu.RUnlock()
	if override != "" {
		if strings.Contains(override, "://") {
			return override
		}
		if _, _, err := net.SplitHostPort(override); err != nil {
			return fmt.Sprintf("https://%s:%d", override, constants.Ports.OperatorHttps)
		}
		return fmt.Sprintf("https://%s", override)
	}
	if c.Paths != nil && strings.Contains(c.Paths.Host, "://") {
		return c.Paths.Host
	}
	return network.LocalhostHTTPSURL(c.OperatorHTTPSPort())
}

// endpointMu guards httpEndpointOverride and httpsEndpointOverride against
// concurrent read/write races between enrollment (which sets and clears
// overrides) and request handlers (which read them via OperatorHTTPURL etc).
var endpointMu sync.RWMutex

// httpEndpointOverride is set by the -e/--endpoint flag to allow
// connecting to a remote gateway HTTP discovery endpoint instead of localhost.
var httpEndpointOverride string

// httpsEndpointOverride is set by the --port flag (combined with --endpoint host)
// to allow connecting to a remote gateway HTTPS/mTLS endpoint on a different port.
var httpsEndpointOverride string

// SetEndpointOverride sets both HTTP and HTTPS endpoint overrides to the same value.
// Backward-compatible — used by tests that pass full URLs.
func SetEndpointOverride(endpoint string) {
	endpointMu.Lock()
	defer endpointMu.Unlock()
	httpEndpointOverride = endpoint
	httpsEndpointOverride = endpoint
}

// SetHTTPEndpointOverride sets only the HTTP discovery override.
func SetHTTPEndpointOverride(endpoint string) {
	endpointMu.Lock()
	defer endpointMu.Unlock()
	httpEndpointOverride = endpoint
}

// SetHTTPSEndpointOverride sets only the HTTPS/mTLS override.
func SetHTTPSEndpointOverride(endpoint string) {
	endpointMu.Lock()
	defer endpointMu.Unlock()
	httpsEndpointOverride = endpoint
}

// HasHTTPSEndpointOverride reports whether the HTTPS endpoint override was set
// by the -e/--endpoint flag (optionally combined with --port). Callers use this
// to decide whether to rewrite a gateway-returned URL (e.g., the recovery
// approval URL) to match the user-supplied endpoint instead of the gateway's
// own PublicBaseURL.
func HasHTTPSEndpointOverride() bool {
	endpointMu.RLock()
	defer endpointMu.RUnlock()
	return httpsEndpointOverride != ""
}

// OperatorPublicURL returns the HTTPS URL for mTLS API and public surface.
// When cfg.Paths.Host is a full URL (contains "://"), it is returned directly,
// matching OperatorHTTPURL behavior for test overrides.
func (c *Config) OperatorPublicURL() string {
	endpointMu.RLock()
	override := httpsEndpointOverride
	endpointMu.RUnlock()
	if override != "" {
		if strings.Contains(override, "://") {
			return override
		}
		if _, _, err := net.SplitHostPort(override); err != nil {
			return fmt.Sprintf("https://%s:%d", override, constants.Ports.OperatorHttps)
		}
		return fmt.Sprintf("https://%s", override)
	}
	if c.Paths != nil && strings.Contains(c.Paths.Host, "://") {
		return c.Paths.Host
	}
	return network.LocalhostHTTPSURL(c.OperatorHTTPSPort())
}

// OperatorDiscoveryURL returns the HTTP URL for CA download and bootstrap routes.
// When cfg.Paths.Host is a full URL (contains "://"), it is returned directly,
// allowing tests to override the discovery URL via cfg.Paths.Host.
func (c *Config) OperatorDiscoveryURL() string {
	endpointMu.RLock()
	override := httpEndpointOverride
	endpointMu.RUnlock()
	if override != "" {
		if strings.Contains(override, "://") {
			return override
		}
		if _, _, err := net.SplitHostPort(override); err != nil {
			return fmt.Sprintf("http://%s:%d", override, constants.Ports.OperatorHttp)
		}
		return fmt.Sprintf("http://%s", override)
	}
	if c.Paths != nil && strings.Contains(c.Paths.Host, "://") {
		return c.Paths.Host
	}
	return network.LocalhostHTTPURL(constants.Ports.OperatorHttp)
}
