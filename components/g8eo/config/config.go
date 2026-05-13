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

	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

// LoadOptions contains all configuration values passed explicitly from main
type LoadOptions struct {
	// Required
	APIKey           string
	OperatorEndpoint string
	WSSPort          int // WSS port to dial on operator (default: 443)
	HTTPPort         int // HTTP port to dial on operator for auth proxy (default: 443)

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

	// System / process context — sourced from Settings at startup
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
// No outbound authentication is required — the Operator simply starts and listens.
type ListenConfig struct {
	Enabled       bool
	WSSPort       int    // WSS/TLS port for operator pub/sub connections (default: 443)
	HTTPPort      int    // TLS/HTTPS port for internal g8ee/g8ed traffic (default: 443)
	BootstrapPort int    // Plain-TLS port for bootstrap routes (/.well-known/, /api/auth/device-link/register) (default: 8080)
	PublicPort    int    // Plain-TLS port for browser-based auth and setup (default: 8081)
	DataDir       string // Root directory for SQLite database (default: .g8e/data in working directory)
	PKIDir        string // Directory for TLS certificates (default: .g8e/pki)
	SecretsDir    string // Directory for platform secrets (default: .g8e/secrets)
	PasskeyRpID   string // RP ID for passkey operations (default: localhost)
	PasskeyRpName string // RP Name for passkey operations (default: g8e)
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

// LoadOpenClaw creates configuration for --openclaw mode.
func LoadOpenClaw(gatewayURL, token, nodeID, displayName, pathEnv, logLevel string) (*OpenClawConfig, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gateway URL is required (--openclaw-url)")
	}
	if logLevel == "" {
		logLevel = "info"
	}
	return &OpenClawConfig{
		GatewayURL:  gatewayURL,
		Token:       token,
		NodeID:      nodeID,
		DisplayName: displayName,
		PathEnv:     pathEnv,
		LogLevel:    logLevel,
	}, nil
}

// Config holds all configuration for g8eo
type Config struct {
	// Basic configuration
	ProjectID     string
	ComponentName string
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

	// Endpoint is the g8ed host or IP used for all HTTP and WebSocket connections.
	Endpoint string

	// TLSServerName overrides the hostname used for TLS certificate verification.
	// Set automatically when Endpoint is a raw IP address so the embedded CA cert
	// (which carries a hostname SAN, not an IP SAN) still validates correctly.
	TLSServerName string

	// operator connection ports (operator dials these on the remote host)
	PubSubURL string // WebSocket base URL for operator pub/sub (e.g., wss://192.168.1.10:443) — no path; client appends /ws/pubsub
	WSSPort   int    // WSS port used to build PubSubURL (default: 443)
	HTTPPort  int    // HTTPS port for auth/bootstrap requests via operator proxy (default: 443)

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

	// Local storage configuration. All paths are relative to WorkDir — the directory the operator was launched from.
	LocalStoreEnabled       bool
	LocalStoreDBPath        string
	LocalStoreMaxSizeMB     int64
	LocalStoreRetentionDays int

	// Git / Ledger
	NoGit        bool   // User explicitly disabled git via --no-git
	GitPath      string // Resolved path to git binary (empty if unavailable)
	GitAvailable bool   // True if a functional git binary was found

	// System / process context — injected from Settings at startup, never read again
	Shell      string // SHELL env var value (e.g. /bin/bash)
	Lang       string // LANG env var value
	Term       string // TERM env var value
	TZ         string // TZ env var value (IANA timezone name)
	IPService  string // G8E_IP_SERVICE — URL for public IP detection
	IPResolver string // G8E_IP_RESOLVER — UDP target for local IP detection

	// Listen mode configuration
	Listen ListenConfig
}

// LoadListen creates configuration for --listen mode.
// Listen mode skips all operator-mode validation — no API key, no endpoint,
// no outbound connections. The Operator simply starts and listens locally.
// allowTestPortZero should be true only when called from Go tests; when false,
// port 0 is rejected to prevent dynamic port assignment in production.
func LoadListen(wssPort, httpPort, bootstrapPort, publicPort int, dataDir, pkiDir, secretsDir string, passkeyRpID, passkeyRpName string, allowTestPortZero bool) (*Config, error) {
	if dataDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to determine working directory: %w", err)
		}
		dataDir = filepath.Join(cwd, ".g8e", "data")
	}
	if pkiDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to determine working directory: %w", err)
		}
		pkiDir = filepath.Join(cwd, ".g8e", "pki")
	}
	if secretsDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to determine working directory: %w", err)
		}
		secretsDir = filepath.Join(cwd, ".g8e", "secrets")
	}

	// Reject port 0 in production (only allowed for Go tests)
	// This check must happen before default assignment to validate actual input
	if !allowTestPortZero {
		if wssPort == 0 {
			return nil, fmt.Errorf("wssPort cannot be 0 in production")
		}
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

	if wssPort <= 0 {
		wssPort = 9001 // default WSS port
	}
	if httpPort <= 0 {
		httpPort = 9000 // default HTTPS API port
	}
	if bootstrapPort <= 0 {
		bootstrapPort = 8080 // default bootstrap port
	}
	if publicPort <= 0 {
		publicPort = 8081 // default public port
	}
	if passkeyRpID == "" {
		passkeyRpID = "localhost"
	}
	if passkeyRpName == "" {
		passkeyRpName = "g8e"
	}

	return &Config{
		ComponentName: "g8eo-listen",
		PKIDir:        pkiDir,     // Also set top-level for services that use Config.PKIDir
		SecretsDir:    secretsDir, // Also set top-level for services that use Config.SecretsDir
		Listen: ListenConfig{
			Enabled:       true,
			WSSPort:       wssPort,
			HTTPPort:      httpPort,
			BootstrapPort: bootstrapPort,
			PublicPort:    publicPort,
			DataDir:       dataDir,
			PKIDir:        pkiDir,
			SecretsDir:    secretsDir,
			PasskeyRpID:   passkeyRpID,
			PasskeyRpName: passkeyRpName,
		},
	}, nil
}

// Load creates configuration from explicit options passed by main
func Load(opts LoadOptions) (*Config, error) {
	// Resolve working directory — default to process cwd when not specified
	workDir := opts.WorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to determine working directory: %w", err)
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

		// Derived values — ports default to 443
		Endpoint:      opts.OperatorEndpoint,
		PubSubURL:     buildPubSubURL(opts.OperatorEndpoint, opts.WSSPort),
		WSSPort:       wssPortOrDefault(opts.WSSPort),
		HTTPPort:      httpPortOrDefault(opts.HTTPPort),
		LogLevel:      opts.LogLevel,
		TLSServerName: tlsServerName(opts.OperatorEndpoint),
		ProjectID:     "g8e",

		// Fixed defaults
		ComponentName:      constants.Status.ComponentName.G8EO,
		MaxConcurrentTasks: 25,
		MaxMemoryMB:        2048,
		HeartbeatInterval:  heartbeatIntervalOrDefault(opts.HeartbeatInterval),

		// Local storage — all paths anchored to WorkDir
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

// buildPubSubURL creates a WebSocket URL, omitting port 443 only when explicitly set
func buildPubSubURL(endpoint string, wssPort int) string {
	if wssPort == 443 {
		// Port 443 explicitly set - omit from URL
		return fmt.Sprintf("wss://%s", endpoint)
	}
	port := wssPortOrDefault(wssPort)
	return fmt.Sprintf("wss://%s:%d", endpoint, port)
}

// wssPortOrDefault returns p if non-zero, otherwise 443.
func wssPortOrDefault(p int) int {
	if p > 0 {
		return p
	}
	return 443
}

// httpPortOrDefault returns p if non-zero, otherwise 443.
func httpPortOrDefault(p int) int {
	if p > 0 {
		return p
	}
	return 443
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
