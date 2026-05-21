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
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
)

// LoadOptions contains all configuration values passed explicitly from main
type LoadOptions struct {
	// Required
	APIKey           string
	OperatorEndpoint string
	HTTPPort         int // HTTP port to dial on operator for auth proxy (default: from paths.json)

	// Cloud Operator mode
	CloudMode     bool
	CloudProvider string

	// Local storage
	LocalStorageEnabled bool

	// Git / Ledger
	NoGit bool // --no-git flag: disables ledger (git-backed file versioning)

	// Working directory
	WorkDir string // Absolute path of the directory the operator was launched from (--working-dir or os.Getwd())

	// PKI and Secrets directories
	PKIDir     string
	SecretsDir string

	// Monitoring
	HeartbeatInterval time.Duration // --heartbeat-interval: overrides the 30s default when non-zero

	// Logging
	LogLevel string // Log level passed to --log flag (info, debug, error)

	// System / process context - sourced from Settings at startup
	Shell      string // SHELL value
	Lang       string // LANG value
	Term       string // TERM value
	TZ         string // TZ value
	IPService  string // G8E_IP_SERVICE value
	IPResolver string // G8E_IP_RESOLVER value
}

// ListenConfig holds configuration for --listen mode.
// In listen mode, the Operator binary becomes the persistence and messaging
// backbone for the entire g8e platform, replacing external databases.
// No outbound authentication is required - the Operator simply starts and listens.
type ListenConfig struct {
	Enabled          bool
	HTTPPort         int    // TLS/HTTPS port for internal g8ee/client traffic (default: from paths.json)
	BootstrapPort    int    // Plain-TLS port for bootstrap routes (/.well-known/, /api/auth/device-link/register) (default: from paths.json)
	PublicPort       int    // Plain-TLS port for browser-based auth and setup (default: from paths.json)
	DataDir          string // Root directory for SQLite database (default: .g8e/data in working directory)
	PKIDir           string // Directory for TLS certificates (default: .g8e/pki)
	SecretsDir       string // Directory for platform secrets (default: .g8e/secrets)
	PasskeyRpID      string // RP ID for passkey operations (default: localhost)
	PasskeyRpName    string // RP Name for passkey operations (default: g8e)
	MCPDownstreamURL string // URL of the downstream MCP server to proxy discovery and execution to
	A2ADownstreamURL string // URL of the downstream A2A server to proxy execution to
}

// OpenClawConfig holds configuration for --openclaw mode.
// In this mode the Operator connects to an OpenClaw Gateway as a Node Host,
// advertising system.run and system.which. No g8e infrastructure is needed.
type OpenClawConfig struct {
	GatewayURL  string // ws:// or wss:// URL of the OpenClaw Gateway
	Token       string // Shared-secret token for Gateway auth (optional)
	NodeID      string // Stable identifier for this node (defaults to hostname)
	DisplayName string // Human-readable label shown in OpenClaw UI
	PathEnv     string // PATH value to advertise to the Gateway
	LogLevel    string
}

// OpenClawOptions contains configuration values for LoadOpenClaw.
type OpenClawOptions struct {
	GatewayURL  string
	Token       string
	NodeID      string
	DisplayName string
	PathEnv     string
	LogLevel    string
}

// LoadOpenClaw creates configuration for --openclaw mode.
func LoadOpenClaw(opts OpenClawOptions) (*OpenClawConfig, error) {
	if opts.GatewayURL == "" {
		return nil, fmt.Errorf("gateway URL is required (--openclaw-url)")
	}
	logLevel := opts.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}
	return &OpenClawConfig{
		GatewayURL:  opts.GatewayURL,
		Token:       opts.Token,
		NodeID:      opts.NodeID,
		DisplayName: opts.DisplayName,
		PathEnv:     opts.PathEnv,
		LogLevel:    logLevel,
	}, nil
}

// Config holds all configuration for g8eo
type Config struct {
	// Basic configuration
	ProjectID     string
	ComponentName constants.ComponentName
	Version       string

	// Authentication
	APIKey string

	// Operator identification
	OperatorID        string
	OperatorSessionId string // Operator's unique operator session ID for authorization
	SystemFingerprint string // Unique system fingerprint for Operator tracking

	// Cloud Operator mode
	CloudMode     bool   // True if running as cloud Operator (--cloud flag)
	CloudProvider string // Cloud provider: 'aws', 'gcp', 'azure' (empty unless --cloud is set)

	// Endpoint is the client host or IP used for all HTTP and WebSocket connections.
	Endpoint string

	// TLSServerName overrides the hostname used for TLS certificate verification.
	// Set automatically when Endpoint is a raw IP address so the embedded CA cert
	// (which carries a hostname SAN, not an IP SAN) still validates correctly.
	TLSServerName string

	// operator connection ports (operator dials these on the remote host)
	PubSubURL string // WebSocket base URL for operator pub/sub (e.g., wss://192.168.1.10:443) - no path; client appends /ws/pubsub
	HTTPPort  int    // HTTPS port for auth/bootstrap requests via operator proxy (default: from paths.json)

	// Logging
	LogLevel string // Active log level (info, debug, error)

	// Execution configuration
	MaxConcurrentTasks int
	MaxMemoryMB        int

	// Monitoring configuration
	HeartbeatInterval time.Duration

	// WorkDir is the absolute path of the directory the operator was launched from.
	// All data storage and command execution is anchored here unless explicitly overridden.
	WorkDir string

	// PKI and Secrets directories
	PKIDir     string
	SecretsDir string

	// Local storage configuration. All paths are relative to WorkDir - the directory the operator was launched from.
	LocalStoreEnabled       bool
	LocalStoreDBPath        string
	LocalStoreMaxSizeMB     int64
	LocalStoreRetentionDays int

	// Git / Ledger
	NoGit        bool   // User explicitly disabled git via --no-git
	GitPath      string // Resolved path to git binary (empty if unavailable)
	GitAvailable bool   // True if a functional git binary was found

	// System / process context - injected from Settings at startup, never read again
	Shell      string // SHELL env var value (e.g. /bin/bash)
	Lang       string // LANG env var value
	Term       string // TERM env var value
	TZ         string // TZ env var value (IANA timezone name)
	IPService  string // G8E_IP_SERVICE - URL for public IP detection
	IPResolver string // G8E_IP_RESOLVER - UDP target for local IP detection

	// Listen mode configuration
	Listen ListenConfig
}

// FindProjectRoot locates the g8e project root by searching for the VERSION file.
func FindProjectRoot() string {
	curr, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(curr, "VERSION")); err == nil {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return ""
}

// ListenOptions contains configuration values for LoadListen.
type ListenOptions struct {
	HTTPPort         int
	BootstrapPort    int
	PublicPort       int
	DataDir          string
	PKIDir           string
	SecretsDir       string
	PasskeyRpID      string
	PasskeyRpName    string
	MCPDownstreamURL string
	A2ADownstreamURL string

	// AllowTestPortZero should be true only when called from Go tests; when false,
	// port 0 is rejected to prevent dynamic port assignment in production.
	AllowTestPortZero bool
}

// LoadListen creates configuration for --listen mode.
// Listen mode skips all operator-mode validation - no API key, no endpoint,
// no outbound connections. The Operator simply starts and listens locally.
func LoadListen(opts ListenOptions) (*Config, error) {
	projectRoot := FindProjectRoot()

	mcpDownstreamURL := opts.MCPDownstreamURL
	if mcpDownstreamURL == "" {
		mcpDownstreamURL = os.Getenv("G8E_MCP_DOWNSTREAM_URL")
	}
	a2aDownstreamURL := opts.A2ADownstreamURL
	if a2aDownstreamURL == "" {
		a2aDownstreamURL = os.Getenv("G8E_A2A_DOWNSTREAM_URL")
	}

	dataDir := opts.DataDir
	if dataDir == "" {
		if projectRoot != "" {
			dataDir = filepath.Join(projectRoot, ".g8e", "data")
		} else {
			cwd, _ := os.Getwd()
			dataDir = filepath.Join(cwd, ".g8e", "data")
		}
	}
	pkiDir := opts.PKIDir
	if pkiDir == "" {
		if projectRoot != "" {
			pkiDir = filepath.Join(projectRoot, ".g8e", "pki")
		} else {
			cwd, _ := os.Getwd()
			pkiDir = filepath.Join(cwd, ".g8e", "pki")
		}
	}
	secretsDir := opts.SecretsDir
	if secretsDir == "" {
		if projectRoot != "" {
			secretsDir = filepath.Join(projectRoot, ".g8e", "secrets")
		} else {
			cwd, _ := os.Getwd()
			secretsDir = filepath.Join(cwd, ".g8e", "secrets")
		}
	}

	httpPort := opts.HTTPPort
	bootstrapPort := opts.BootstrapPort
	publicPort := opts.PublicPort

	// Reject port 0 in production (only allowed for Go tests)
	// This check must happen before default assignment to validate actual input
	if !opts.AllowTestPortZero {

		if httpPort == 0 {
			return nil, fmt.Errorf("httpPort cannot be 0 in production")
		}
		if bootstrapPort == 0 {
			return nil, fmt.Errorf("bootstrapPort cannot be 0 in production")
		}
		if publicPort == 0 {
			return nil, fmt.Errorf("publicPort cannot be 0 in production")
		}
	}

	// Assign default ports only if they are still 0 AND we are not in test-port-zero mode.
	// If allowTestPortZero is true and ports are 0, we leave them as 0 so net.Listen can bind to a random port.
	// Default ports must match protocol/constants/paths.json (canonical source of truth).
	if !opts.AllowTestPortZero {

		if httpPort <= 0 {
			httpPort = constants.Ports.OperatorHttp
		}
		if bootstrapPort <= 0 {
			bootstrapPort = constants.Ports.OperatorBootstrap
		}
		if publicPort <= 0 {
			publicPort = constants.Ports.OperatorPublic
		}
	}
	passkeyRpID := opts.PasskeyRpID
	if passkeyRpID == "" {
		passkeyRpID = "localhost"
	}
	passkeyRpName := opts.PasskeyRpName
	if passkeyRpName == "" {
		passkeyRpName = "g8e"
	}

	return &Config{
		ComponentName: constants.Status.ComponentName.G8EOListen,
		PKIDir:        pkiDir,     // Also set top-level for services that use Config.PKIDir
		SecretsDir:    secretsDir, // Also set top-level for services that use Config.SecretsDir
		Listen: ListenConfig{
			Enabled: true,

			HTTPPort:         httpPort,
			BootstrapPort:    bootstrapPort,
			PublicPort:       publicPort,
			DataDir:          dataDir,
			PKIDir:           pkiDir,
			SecretsDir:       secretsDir,
			PasskeyRpID:      passkeyRpID,
			PasskeyRpName:    passkeyRpName,
			MCPDownstreamURL: mcpDownstreamURL,
			A2ADownstreamURL: a2aDownstreamURL,
		},
	}, nil
}

// Load creates configuration from explicit options passed by main
func Load(opts LoadOptions) (*Config, error) {
	// Resolve working directory - default to project root when not specified
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = FindProjectRoot()
		if workDir == "" {
			var err error
			workDir, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("failed to determine working directory: %w", err)
			}
		}
	} else {
		var err error
		workDir, err = filepath.Abs(workDir)
		if err != nil {
			return nil, fmt.Errorf("invalid --working-dir %q: %w", opts.WorkDir, err)
		}
	}

	if opts.APIKey == "" {
		return nil, fmt.Errorf("APIKey is required")
	}

	if opts.OperatorEndpoint == "" {
		return nil, fmt.Errorf("OperatorEndpoint is required")
	}

	// Build config from explicit options
	cfg := &Config{
		// From options
		APIKey:            opts.APIKey,
		CloudMode:         opts.CloudMode,
		CloudProvider:     opts.CloudProvider,
		LocalStoreEnabled: opts.LocalStorageEnabled,
		WorkDir:           workDir,
		PKIDir:            opts.PKIDir,
		SecretsDir:        opts.SecretsDir,

		// Derived values - ports default to values from paths.json
		Endpoint:  opts.OperatorEndpoint,
		PubSubURL: buildPubSubURL(opts.OperatorEndpoint, opts.HTTPPort),

		HTTPPort:      httpPortOrDefault(opts.HTTPPort),
		LogLevel:      opts.LogLevel,
		TLSServerName: tlsServerName(opts.OperatorEndpoint),
		ProjectID:     "g8e",

		// Fixed defaults
		ComponentName:      constants.Status.ComponentName.G8EO,
		MaxConcurrentTasks: 25,
		MaxMemoryMB:        2048,
		HeartbeatInterval:  heartbeatIntervalOrDefault(opts.HeartbeatInterval),

		// Local storage - all paths anchored to WorkDir
		LocalStoreDBPath:        filepath.Join(workDir, ".g8e", "local_state.db"),
		LocalStoreMaxSizeMB:     1024,
		LocalStoreRetentionDays: 30,

		// Git / Ledger
		NoGit: opts.NoGit,

		// System / process context
		Shell:      opts.Shell,
		Lang:       opts.Lang,
		Term:       opts.Term,
		TZ:         opts.TZ,
		IPService:  opts.IPService,
		IPResolver: opts.IPResolver,
	}

	// Default PKIDir to .g8e/pki if not explicitly set
	if cfg.PKIDir == "" {
		cfg.PKIDir = filepath.Join(workDir, ".g8e", "pki")
	}

	// Default SecretsDir to .g8e/secrets if not explicitly set
	if cfg.SecretsDir == "" {
		cfg.SecretsDir = filepath.Join(workDir, ".g8e", "secrets")
	}

	return cfg, nil
}

// heartbeatIntervalOrDefault returns d if positive, otherwise the 30-second default.
func heartbeatIntervalOrDefault(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return 30 * time.Second
}

// buildPubSubURL creates a WebSocket URL, omitting port 443 if it is the effective port.
func buildPubSubURL(endpoint string, httpPort int) string {
	port := httpPortOrDefault(httpPort)
	return fmt.Sprintf("wss://%s:%d", endpoint, port)
}

// httpPortOrDefault returns p if non-zero, otherwise the default from paths.json.
func httpPortOrDefault(p int) int {
	if p > 0 {
		return p
	}
	return constants.Ports.OperatorHttp
}

// tlsServerName returns the TLS ServerName override to use when endpoint is a
// raw IP address. The embedded CA cert is issued to "localhost",
// so TLS verification must use that hostname regardless of what IP is dialed.
// Returns an empty string when endpoint is already a hostname (no override needed).
func tlsServerName(endpoint string) string {
	if net.ParseIP(endpoint) != nil {
		return constants.DefaultEndpoint
	}
	return ""
}
