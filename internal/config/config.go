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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
)

// GatewayPosture defines the governance enforcement posture for the Gateway.
type GatewayPosture string

const (
	// PostureDoctrine: L1 enforced, L2/L3 signature not required (default)
	PostureDoctrine GatewayPosture = "doctrine"
	// PostureConsensus: L1/L2 enforced, L3 signature not required
	PostureConsensus GatewayPosture = "consensus"
	// PostureNotary: L1/L2/L3 strictly enforced
	PostureNotary GatewayPosture = "notary"
)

// LoadOptions contains all configuration values passed explicitly from main
type LoadOptions struct {
	// Required
	OperatorEndpoint string
	HTTPPort         int // HTTP port to dial on Operator for bootstrap and trust bundle fetch (default: from paths.json)
	HTTPSPort        int // HTTPS port to dial on Operator for auth proxy (default: from paths.json)

	// Cloud Operator mode
	CloudMode     bool
	CloudProvider string

	// Governance posture for outbound mode (doctrine, consensus, or notary)
	// Defaults to "notary" for outbound mode - L1/L2/L3 strictly enforced
	// Outbound mode requires L3 (human) authorization before sending mutations
	Posture GatewayPosture

	// Execution vault
	ExecutionVaultEnabled bool

	// Git / Ledger
	NoGit bool // --no-git flag: disables ledger (git-backed file versioning)

	// Working directory
	WorkDir string // Absolute path of the directory the Operator was launched from (--working-dir or os.Getwd())

	// PKI and Secrets directories
	PKIDir     string
	SecretsDir string

	// Logging
	LogLevel string // Log level passed to --log flag (info, debug, error)

	// System / process context - sourced from Settings at startup
	Shell string // SHELL value
	Lang  string // LANG value
	Term  string // TERM value
	TZ    string // TZ value

	// Monitoring
	HeartbeatInterval time.Duration // --heartbeat-interval: overrides the 30s default when non-zero
}

// GatewayConfig holds configuration for gateway mode.
// In gateway mode, the Node binary becomes the persistence and messaging
// backbone for the entire g8e platform, replacing external databases.
// No outbound authentication is required - the Operator simply starts and listens.
type GatewayConfig struct {
	Enabled            bool
	Posture            GatewayPosture // Governance enforcement posture (doctrine, consensus, notary)
	HTTPPort           int            // Plain HTTP port for bootstrap and MCP (default: constants.Ports.OperatorHttp)
	HTTPSPort          int            // HTTPS port for mTLS API (default: constants.Ports.OperatorHttps)
	DataDir            string         // Root directory for SQLite database (default: .g8e/data in working directory)
	PKIDir             string         // Directory for TLS certificates (default: .g8e/pki)
	SecretsDir         string         // Directory for platform secrets (default: .g8e/secrets)
	VaultDir           string         // Directory for encryption vault (default: .g8e/vault)
	VaultKeyPath       string         // Path to vault key file (default: .g8e/vault/key)
	VaultRequireUnlock bool           // Require vault to be unlocked before starting (default: true)
	PasskeyRpID        string         // RP ID for passkey operations (default: localhost)
	PasskeyRpName      string         // RP Name for passkey operations (default: g8e)
	PasskeyRpOrigins   []string       // Additional RP origins for passkey operations (e.g. demo remapped ports)
	MCPDownstreamURL   string         // URL of the downstream MCP server to proxy discovery and execution to
	A2ADownstreamURL   string         // URL of the downstream A2A server to proxy execution to
	PublicBaseURL      string         // Public base URL for L3 approval links (e.g., https://localhost:8443)
	JWKSURL            string         // URL to fetch JWKS for JWT validation
	JWTRoleClaim       string         // The claim in JWT that contains roles (default: "roles")
	JWTIssuer          string         // Expected issuer claim in JWT (optional, for multi-audience IdP deployments)
	JWTAudience        string         // Expected audience claim in JWT (optional, for multi-audience IdP deployments)

	// HTTP server limits
	MaxPayloadBytes   int64         // Maximum request payload size in bytes (default: 10MB)
	ReadHeaderTimeout time.Duration // Timeout for reading request headers (default: 10s)
	ReadTimeout       time.Duration // Timeout for reading entire request (default: 30s)
	WriteTimeout      time.Duration // Timeout for writing response (default: 30s)
	IdleTimeout       time.Duration // Timeout for idle connections (default: 120s)
	MaxHeaderBytes    int           // Maximum size of request headers in bytes (default: 1MB)

	// Rate limiting
	RateLimitRPS   float64 // Requests per second limit (default: 5)
	RateLimitBurst int     // Burst size for rate limiter (default: 10)

	// Certificate mode
	CertMode            string // "full" for all hostnames/IPs, "localhost" for minimal
	NetworkIdentityFile string // Path to JSON file containing pre-detected network identity

	// Tribunal configuration for consensus posture
	TribunalID  string // ID of the TribunalPolicy to use for L2 deliberation (required for --consensus)
	TribunalURL string // URL of the Tribunal service for L2 deliberation (e.g. https://localhost:8443/tribunal/v1/deliberate)

	// CORS allowed origins for cross-origin browser access (e.g. https://lovable.dev)
	AllowedOrigins []string

	// Distributed lock retry configuration
	LockMaxRetries int           // Maximum retry attempts for distributed lock acquisition (default: 30)
	LockRetryDelay time.Duration // Base delay for lock retry backoff (default: 50ms)
}

// Config holds all configuration for g8eo
type Config struct {
	// Basic configuration
	ProjectID     string
	ComponentName constants.ComponentName
	Version       string

	// Operator identification
	OperatorID        string
	OperatorSessionId string // Operator's unique Operator session ID for authorization
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

	// Operator connection ports (operator dials these on the remote host)
	PubSubURL string // WebSocket base URL for Operator pub/sub (e.g., wss://192.168.1.10:443) - no path; client appends /ws/pubsub
	HTTPPort  int    // HTTP port for bootstrap and trust bundle fetch (default: from paths.json)
	HTTPSPort int    // HTTPS port for auth/bootstrap requests via Operator proxy (default: from paths.json)

	// Logging
	LogLevel string // Active log level (info, debug, error)

	// Execution configuration
	MaxConcurrentTasks int
	MaxMemoryMB        int

	// Monitoring configuration
	HeartbeatInterval time.Duration

	// WorkDir is the absolute path of the directory the Operator was launched from.
	// All data storage and command execution is anchored here unless explicitly overridden.
	WorkDir string

	// PKI and Secrets directories
	PKIDir     string
	SecretsDir string

	// Vault configuration for encryption at rest
	VaultDir           string // Directory for encryption vault (default: .g8e/vault)
	VaultKeyPath       string // Path to vault key file (default: .g8e/vault/key)
	VaultRequireUnlock bool   // Require vault to be unlocked before starting (default: true)

	// Execution vault configuration. All paths are relative to WorkDir - the directory the Operator was launched from.
	ExecutionVaultEnabled       bool
	ExecutionVaultMaxSizeMB     int64
	ExecutionVaultRetentionDays int

	// Git / Ledger
	NoGit        bool   // User explicitly disabled git via --no-git
	GitPath      string // Resolved path to git binary (empty if unavailable)
	GitAvailable bool   // True if a functional git binary was found

	// System / process context - injected from Settings at startup, never read again
	Shell string // SHELL env var value (e.g. /bin/bash)
	Lang  string // LANG env var value
	Term  string // TERM env var value
	TZ    string // TZ env var value (IANA timezone name)

	// Governance posture for outbound mode (doctrine, consensus, or notary)
	// Defaults to "notary" since L3Notary is nil and mutations must fail-closed
	Posture GatewayPosture

	// Gateway mode configuration
	Gateway GatewayConfig
}

// FindProjectRoot returns the current working directory.
func FindProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// GatewayOptions contains configuration values for LoadGateway.
type GatewayOptions struct {
	Posture          GatewayPosture
	HTTPPort         int
	HTTPSPort        int
	DataDir          string
	PKIDir           string
	SecretsDir       string
	PasskeyRpID      string
	PasskeyRpName    string
	PasskeyRpOrigins []string
	MCPDownstreamURL string
	A2ADownstreamURL string
	PublicBaseURL    string
	JWKSURL          string
	JWTRoleClaim     string
	JWTIssuer        string
	JWTAudience      string

	RateLimitRPS   float64
	RateLimitBurst int

	CertMode            string
	NetworkIdentityFile string

	TribunalID  string
	TribunalURL string

	AllowedOrigins []string

	// AllowTestPortZero should be true only when called from Go tests; when false,
	// port 0 is rejected to prevent dynamic port assignment in production.
	AllowTestPortZero bool
}

// ResolveGatewayPorts finds two available ports incrementally, starting from the
// requested ports. This allows multiple operators to run on the same host.
func ResolveGatewayPorts(httpPort, httpsPort int) (int, int) {
	if httpPort <= 0 {
		httpPort = constants.Ports.OperatorHttp
	}
	if httpsPort <= 0 {
		httpsPort = constants.Ports.OperatorHttps
	}

	// Try up to 100 offsets
	for offset := 0; offset < 100; offset++ {
		h := httpPort + offset
		s := httpsPort + offset

		if isPortAvailable(h) && isPortAvailable(s) {
			return h, s
		}
	}

	// Fallback to original if we can't find a free block (let it fail during bind)
	return httpPort, httpsPort
}

func isPortAvailable(port int) bool {
	ln, err := net.Listen(string(constants.NetworkProtocolTCP), fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// ValidateL2PostureStartup checks the startup validation rules for postures
// that require L2 signatures (consensus and notary). If posture is neither,
// it returns nil (no validation needed). For consensus/notary posture, the
// tribunalID must be non-empty and the quorum must be >= 1.
//
// This is a pure function extracted from the gateway startup path so it
// can be tested without os.Exit. The gateway startup calls this after
// loading the tribunal policy from the database to validate the quorum.
func ValidateL2PostureStartup(posture string, tribunalID string, quorum int) error {
	if posture != string(PostureConsensus) && posture != string(PostureNotary) {
		return nil
	}
	if tribunalID == "" {
		return constants.ErrConfigTribunalIDRequired
	}
	if quorum < 1 {
		return constants.ErrConfigTribunalQuorumLow
	}
	return nil
}

// validateAndResolveGatewayPorts validates and resolves gateway port configuration.
// It handles:
// - Port zero validation (rejects explicit zero in production unless all ports are zero)
// - Port resolution to available ports (with offset fallback)
// - Port uniqueness checks (ignoring zero-valued ports)
//
// Returns validated and resolved ports, or an error if validation fails.
func validateAndResolveGatewayPorts(httpPort, httpsPort int, allowTestPortZero bool) (int, int, error) {
	// Reject port 0 in production
	// This check must happen before default assignment to validate actual input
	if !allowTestPortZero {
		if httpPort == 0 && httpsPort == 0 {
			// All zero means "use defaults and resolve"
		} else {
			if httpPort == 0 {
				return 0, 0, constants.ErrConfigHTTPPortZero
			}
			if httpsPort == 0 {
				return 0, 0, constants.ErrConfigHTTPSPortZero
			}
		}
	}

	// Resolve available ports if they are not 0 (dynamic test ports)
	if !allowTestPortZero {
		httpPort, httpsPort = ResolveGatewayPorts(httpPort, httpsPort)
	}

	// Validate that all ports are unique to prevent conflicts.
	// Zero-valued ports are ignored so test/default configurations can leave
	// optional ports unset without tripping false conflicts.
	if httpPort > 0 && httpsPort > 0 && httpPort == httpsPort {
		return 0, 0, constants.ErrConfigPortsMustDiffer
	}

	return httpPort, httpsPort, nil
}

// LoadGateway creates configuration for gateway mode.
// Gateway mode skips all operator-mode validation - no endpoint,
// no outbound connections. The Operator simply starts and listens locally.
func LoadGateway(opts GatewayOptions) (*Config, error) {
	// Initialize paths relative to current working directory
	projectRoot := FindProjectRoot()
	if projectRoot == "" {
		projectRoot = "."
	}
	if err := paths.InitWithBase(projectRoot); err != nil {
		return nil, fmt.Errorf("config: failed to initialize paths: %w", err)
	}

	// Resolve paths using canonical constants
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = paths.Infra.DataDir
	}
	pkiDir := opts.PKIDir
	if pkiDir == "" {
		pkiDir = paths.Infra.PkiDir
	}

	mcpDownstreamURL := opts.MCPDownstreamURL
	a2aDownstreamURL := opts.A2ADownstreamURL
	secretsDir := opts.SecretsDir
	if secretsDir == "" {
		secretsDir = paths.Infra.SecretsDir
	}

	vaultDir := paths.Infra.VaultDir
	vaultKeyPath := paths.Infra.VaultKeyPath

	// Validate and resolve gateway ports
	httpPort, httpsPort, err := validateAndResolveGatewayPorts(
		opts.HTTPPort,
		opts.HTTPSPort,
		opts.AllowTestPortZero,
	)
	if err != nil {
		return nil, err
	}

	passkeyRpID := opts.PasskeyRpID
	if passkeyRpID == "" {
		passkeyRpID = "localhost"
	}
	// Normalize 127.0.0.1 to localhost for passkey RP ID
	// WebAuthn requires RP ID to be a valid domain, not an IP address
	if passkeyRpID == "127.0.0.1" {
		passkeyRpID = "localhost"
	}
	passkeyRpName := opts.PasskeyRpName
	if passkeyRpName == "" {
		passkeyRpName = "g8e"
	}

	posture := opts.Posture
	if posture == "" {
		posture = PostureDoctrine
	}

	jwksURL := opts.JWKSURL
	jwtRoleClaim := opts.JWTRoleClaim
	if jwtRoleClaim == "" {
		jwtRoleClaim = "roles"
	}
	jwtIssuer := opts.JWTIssuer
	jwtAudience := opts.JWTAudience

	return &Config{
		ComponentName:      constants.ComponentNameG8EOGateway,
		PKIDir:             pkiDir,
		SecretsDir:         secretsDir,
		MaxConcurrentTasks: 25,
		MaxMemoryMB:        2048,
		Gateway: GatewayConfig{
			Enabled: true,
			Posture: posture,

			HTTPPort:           httpPort,
			HTTPSPort:          httpsPort,
			DataDir:            dataDir,
			PKIDir:             pkiDir,
			SecretsDir:         secretsDir,
			VaultDir:           vaultDir,
			VaultKeyPath:       vaultKeyPath,
			VaultRequireUnlock: false,
			PasskeyRpID:        passkeyRpID,
			PasskeyRpName:      passkeyRpName,
			PasskeyRpOrigins:   opts.PasskeyRpOrigins,
			MCPDownstreamURL:   mcpDownstreamURL,
			A2ADownstreamURL:   a2aDownstreamURL,
			PublicBaseURL:      opts.PublicBaseURL,
			JWKSURL:            jwksURL,
			JWTRoleClaim:       jwtRoleClaim,
			JWTIssuer:          jwtIssuer,
			JWTAudience:        jwtAudience,

			// HTTP server limits with fail-closed defaults
			MaxPayloadBytes:   512 * 1024, // 512KB
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20, // 1MB

			// Rate limiting disabled for gateway mode (local-only, no external clients)
			RateLimitRPS:   0,
			RateLimitBurst: 0,

			// Certificate mode
			CertMode:            opts.CertMode,
			NetworkIdentityFile: opts.NetworkIdentityFile,

			// Tribunal configuration
			TribunalID:  opts.TribunalID,
			TribunalURL: opts.TribunalURL,

			AllowedOrigins: opts.AllowedOrigins,

			// Distributed lock retry defaults
			LockMaxRetries: 30,                    // 30 retry attempts
			LockRetryDelay: 50 * time.Millisecond, // 50ms base delay
		},
	}, nil
}

// Load creates configuration from explicit options passed by main
func Load(opts LoadOptions) (*Config, error) {
	// Initialize paths relative to project root
	projectRoot := FindProjectRoot()
	if projectRoot == "" {
		projectRoot = "."
	}
	if err := paths.InitWithBase(projectRoot); err != nil {
		return nil, fmt.Errorf("config: failed to initialize paths: %w", err)
	}

	// Resolve working directory - default to project root when not specified
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = projectRoot
	} else {
		var err error
		workDir, err = filepath.Abs(workDir)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", constants.ErrConfigInvalidWorkingDir, opts.WorkDir)
		}
	}

	if opts.OperatorEndpoint == "" {
		return nil, constants.ErrEndpointRequired
	}

	// Build config from explicit options
	tlsServerName := tlsServerName(opts.OperatorEndpoint)
	cfg := &Config{
		// From options
		CloudMode:             opts.CloudMode,
		CloudProvider:         opts.CloudProvider,
		ExecutionVaultEnabled: opts.ExecutionVaultEnabled,
		WorkDir:               workDir,
		PKIDir:                opts.PKIDir,
		SecretsDir:            opts.SecretsDir,

		// Derived values - ports default to values from paths.json
		Endpoint:  opts.OperatorEndpoint,
		PubSubURL: buildPubSubURL(opts.OperatorEndpoint, tlsServerName, opts.HTTPSPort),

		HTTPPort:      httpPortOrDefault(opts.HTTPPort),
		HTTPSPort:     httpsPortOrDefault(opts.HTTPSPort),
		LogLevel:      opts.LogLevel,
		TLSServerName: tlsServerName,
		ProjectID:     "g8e",

		// Fixed defaults
		ComponentName:      constants.ComponentNameG8EO,
		MaxConcurrentTasks: 25,
		MaxMemoryMB:        2048,
		HeartbeatInterval:  heartbeatIntervalOrDefault(opts.HeartbeatInterval),

		// Execution vault defaults
		ExecutionVaultMaxSizeMB:     1024,
		ExecutionVaultRetentionDays: 30,

		// Git / Ledger
		NoGit: opts.NoGit,

		// System / process context
		Shell: opts.Shell,
		Lang:  opts.Lang,
		Term:  opts.Term,
		TZ:    opts.TZ,

		// Governance posture - default to notary for outbound mode (L1/L2/L3 strictly enforced)
		Posture: opts.Posture,
	}
	if cfg.Posture == "" {
		cfg.Posture = PostureNotary
	}

	// Default PKIDir to .g8e/pki if not explicitly set
	if cfg.PKIDir == "" {
		cfg.PKIDir = paths.Infra.PkiDir
	}

	// Default SecretsDir to .g8e/secrets if not explicitly set
	if cfg.SecretsDir == "" {
		cfg.SecretsDir = paths.Infra.SecretsDir
	}

	// Default VaultDir to .g8e/vault if not explicitly set
	if cfg.VaultDir == "" {
		cfg.VaultDir = paths.Infra.VaultDir
	}

	// Default VaultKeyPath to .g8e/vault/key if not explicitly set
	if cfg.VaultKeyPath == "" {
		cfg.VaultKeyPath = paths.Infra.VaultKeyPath
	}

	// Default VaultRequireUnlock to false (matches CLI flag default)
	// Gateway can start with vault locked; vault key is optional
	cfg.VaultRequireUnlock = false

	// Read operator session ID from environment variable (in-memory only, never persisted)
	// This is set by the deploy script after enrollment to track the operator's session
	cfg.OperatorSessionId = os.Getenv(string(constants.EnvVar.OperatorSessionID))

	return cfg, nil
}

// heartbeatIntervalOrDefault returns d if positive, otherwise the 30-second default.
func heartbeatIntervalOrDefault(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return 30 * time.Second
}

// buildPubSubURL creates a WebSocket URL using the HTTPS port (WSS runs over TLS).
// Uses tlsServerName for the hostname when provided (for IP-to-g8e.local mapping).
func buildPubSubURL(endpoint string, tlsServerName string, httpsPort int) string {
	port := httpsPortOrDefault(httpsPort)
	hostname := endpoint
	if tlsServerName != "" {
		hostname = tlsServerName
	}
	return fmt.Sprintf("wss://%s:%d", hostname, port)
}

// httpPortOrDefault returns p if non-zero, otherwise the default from paths.json.
func httpPortOrDefault(p int) int {
	if p > 0 {
		return p
	}
	return constants.Ports.OperatorHttp
}

// httpsPortOrDefault returns p if non-zero, otherwise the default from paths.json.
func httpsPortOrDefault(p int) int {
	if p > 0 {
		return p
	}
	return constants.Ports.OperatorHttps
}

// tlsServerName returns the TLS ServerName override to use when endpoint is a
// raw IP address. When connecting to a Gateway via IP, we use the internal
// Gateway hostname (g8e.local) for TLS verification since the Gateway's
// certificate is issued to this name.
// Returns an empty string when endpoint is already a hostname (no override needed).
func tlsServerName(endpoint string) string {
	if net.ParseIP(endpoint) != nil {
		return constants.GatewayInternalHostname
	}
	return ""
}
