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

package main

//	@title			g8e Gateway API
//	@version		1.0
//	@description	API documentation for the g8e Gateway public endpoints
//	@termsOfService	https://github.com/g8e-ai/g8e

//	@contact.name	g8e Team
//	@contact.url	https://github.com/g8e-ai/g8e
//	@contact.email	support@g8e.ai

//	@license.name	Apache 2.0
//	@license.url	http://www.apache.org/licenses/LICENSE-2.0.html

//	@host		localhost:8443
//	@BasePath	/api/v1

//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Bearer token authentication (JWT or mTLS certificate)

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	clicmd "github.com/g8e-ai/g8e/internal/cli/cmd"
	"github.com/g8e-ai/g8e/internal/cmd"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/exitcode"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services"
)

// Version information (set via ldflags during build)
var (
	version   string = string(constants.VersionStabilityDev)
	buildID   string = string(constants.SystemHealthUnknown)
	buildTime string = string(constants.SystemHealthUnknown)
	platform  string = string(constants.SystemHealthUnknown)
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == string(constants.ApprovalTypeStream) {
		cmd.RunStream(os.Args[2:])
		return
	}

	// Check for CLI subcommands
	cliSubcommands := map[string]bool{
		"gw":                    true,
		"gateway":               true,
		"agentic-tool-emulator": true,
		"chaos":                 true,
		"mcp":                   true,
		"operator":              true,
		"agent":                 true,
		"claude":                true,
		"vault":                 true,
		"test":                  true,
		"setup":                 true,
		"auth":                  true,
		"audit":                 true,
		"swagger":               true,
	}

	if len(os.Args) > 1 && cliSubcommands[os.Args[1]] {
		// Delegate to CLI commands
		clicmd.Execute(version)
		os.Exit(0)
	}

	// Initialize paths relative to current working directory
	// This must be done before any path operations to ensure paths.Infra.* are absolute
	if err := paths.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize paths: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	// Capture the launch directory before any flag parsing or os.Chdir calls.
	launchDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to determine working directory: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	var privateKey string
	var clientCert string
	var endpointURL string
	var trustBundlePath string
	var workingDir string
	var cloudMode bool
	var cloudProvider string
	var executionVault bool
	var logLevel string
	var showVersion bool

	var noGit bool

	var doctrineMode bool
	var consensusMode bool
	var notaryMode bool
	var gatewayHTTPPort int
	var gatewayHTTPSPort int
	var gatewayDataDir string
	var gatewayPKIDir string
	var gatewaySecretsDir string
	var gatewayVaultDir string
	var gatewayVaultKeyPath string
	var gatewayVaultRequireUnlock bool
	var gatewayPasskeyRpID string
	var gatewayPasskeyRpName string
	var gatewayRateLimitRPS float64
	var gatewayRateLimitBurst int
	var gatewayCertIdentityMode string
	var gatewayNetworkIdentityFile string

	var heartbeatInterval time.Duration

	var rekeyVault bool
	var oldPrivateKeyStr string
	var verifyVault bool
	var resetVault bool
	flag.StringVar(&privateKey, "k", "", "Private key")
	flag.StringVar(&privateKey, "key", "", "Private key")
	flag.StringVar(&clientCert, "cert", "", "Client certificate (for mTLS)")
	flag.StringVar(&clientCert, "client-cert", "", "Client certificate (for mTLS)")
	flag.StringVar(&endpointURL, "e", "", "Endpoint (hostname or IP)")
	flag.StringVar(&endpointURL, "endpoint", "", "Endpoint (hostname or IP)")
	flag.StringVar(&trustBundlePath, "trust-bundle", "", "Path to trust bundle PEM file (default: from paths.Infra.CaCertPath or fetch from WellKnownPKICABundle endpoint)")
	flag.StringVar(&workingDir, "working-dir", "", "Working directory (default: directory Operator was launched from)")
	flag.BoolVar(&cloudMode, "c", true, "Cloud mode")
	flag.BoolVar(&cloudMode, string(constants.OperatorTypeCloud), true, "Cloud mode")
	flag.StringVar(&cloudProvider, "p", "", "Cloud provider")
	flag.StringVar(&cloudProvider, "provider", "", "Cloud provider")
	flag.BoolVar(&executionVault, "s", true, "Enable execution vault (stores execution data in current directory)")
	flag.BoolVar(&executionVault, "execution-vault", true, "Enable execution vault (stores execution data in current directory)")
	flag.StringVar(&logLevel, "l", "info", "Log level")
	flag.StringVar(&logLevel, "log", "info", "Log level")
	flag.BoolVar(&noGit, "G", false, "Disable git (ledger)")
	flag.BoolVar(&noGit, "no-git", false, "Disable git (ledger)")
	flag.BoolVar(&showVersion, "v", false, "Version")
	flag.BoolVar(&showVersion, "version", false, "Version")

	flag.BoolVar(&doctrineMode, "doctrine", false, "Gateway mode: L1 enforced, L2/L3 audited (default)")
	flag.BoolVar(&consensusMode, "consensus", false, "Gateway mode: L1/L2 enforced, L3 audited")
	flag.BoolVar(&notaryMode, "notary", false, "Gateway mode: L1/L2/L3 strictly enforced")
	flag.IntVar(&gatewayHTTPPort, "http-port", constants.Ports.OperatorHttp, "HTTP port for bootstrap and MCP routes (default: from paths.json)")
	flag.IntVar(&gatewayHTTPSPort, "https-port", constants.Ports.OperatorHttps, "HTTPS port for mTLS API and public surface (default: from paths.json)")
	flag.StringVar(&gatewayDataDir, "data-dir", "", "Data directory for SQLite database (default: from paths.Infra.DataDir in working directory)")
	flag.StringVar(&gatewayPKIDir, "pki-dir", "", "Directory for TLS certificates (default: from paths.Infra.PkiDir)")
	flag.StringVar(&gatewaySecretsDir, "secrets-dir", "", "Directory for platform secrets (default: from paths.Infra.SecretsDir)")
	flag.StringVar(&gatewayVaultDir, "vault-dir", "", "Directory for vault data (default: from constants.DefaultVaultDirDesc)")
	flag.StringVar(&gatewayVaultKeyPath, "vault-key", "", "Path to vault private key (default: from constants.DefaultVaultKeyDesc)")
	flag.BoolVar(&gatewayVaultRequireUnlock, "vault-require-unlock", false, "Require vault to be unlocked at startup (fail if vault cannot be unlocked)")
	flag.StringVar(&gatewayPasskeyRpID, "passkey-rp-id", "", "RP ID for passkey operations (default: localhost)")
	flag.StringVar(&gatewayPasskeyRpName, "passkey-rp-name", "", "RP Name for passkey operations (default: g8e)")
	flag.Float64Var(&gatewayRateLimitRPS, "rate-limit-rps", 5.0, "Gateway requests per second limit (set to 0 to disable)")
	flag.IntVar(&gatewayRateLimitBurst, "rate-limit-burst", 10, "Gateway rate limit burst size")
	flag.StringVar(&gatewayCertIdentityMode, "cert-mode", "", "Certificate mode: full (all hostnames/IPs), localhost (only localhost)")
	flag.StringVar(&gatewayNetworkIdentityFile, "network-identity-file", "", "Path to JSON file containing pre-detected network identity")
	flag.BoolVar(&rekeyVault, "rekey-vault", false, "Re-encrypt vault with new private key (requires --old-key)")
	flag.StringVar(&oldPrivateKeyStr, "old-key", "", "Old private key for vault re-keying")
	flag.BoolVar(&verifyVault, "verify-vault", false, "Verify vault integrity")
	flag.BoolVar(&resetVault, "reset-vault", false, "Reset vault (DESTROYS ALL DATA)")

	flag.DurationVar(&heartbeatInterval, "heartbeat-interval", 0, "Heartbeat interval (e.g. 60s, 2m); overrides the 30s default")

	flag.Parse()

	// Check for version flag before other processing
	if showVersion {
		printVersion()
		os.Exit(constants.ExitSuccess)
	}

	// Check if this is a CLI command (known subcommands)
	cliCommands := map[string]bool{
		"gw":       true,
		"gateway":  true,
		"mcp":      true,
		"operator": true,
		"vault":    true,
		"test":     true,
		"setup":    true,
		"demos":    true,
		"auth":     true,
		"audit":    true,
		"swagger":  true,
		"help":     true,
		"--help":   true,
		"-h":       true,
	}

	// Show help if no arguments provided, or if first arg is a CLI command
	if len(os.Args) == 1 || (len(os.Args) > 1 && cliCommands[os.Args[1]]) {
		clicmd.Execute(version)
		return
	}

	// Check for mutually exclusive posture flags
	postureCount := 0
	var posture config.GatewayPosture
	if doctrineMode {
		postureCount++
		posture = config.PostureDoctrine
	}
	if consensusMode {
		postureCount++
		posture = config.PostureConsensus
	}
	if notaryMode {
		postureCount++
		posture = config.PostureNotary
	}

	// If we have arguments after flag parsing but they weren't recognized as CLI commands,
	// and we're not in operator mode (no -e, no posture flags), show usage help
	if len(os.Args) > 1 && !cliCommands[os.Args[1]] && endpointURL == "" && postureCount == 0 {
		fmt.Fprintf(os.Stderr, "Error: unrecognized command or flag '%s'\n\n", os.Args[1])
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  ./g8e [command] [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Available Commands:\n")
		fmt.Fprintf(os.Stderr, "  gw, gateway    Gateway management (start, stop, status, logs)\n")
		fmt.Fprintf(os.Stderr, "  auth           Authentication (login, logout)\n")
		fmt.Fprintf(os.Stderr, "  mcp            MCP configuration and proxy\n")
		fmt.Fprintf(os.Stderr, "  operator       Operator management (list, deploy, stream)\n")
		fmt.Fprintf(os.Stderr, "  vault          Vault operations (encrypt, decrypt, rekey)\n")
		fmt.Fprintf(os.Stderr, "  test           Run tests\n")
		fmt.Fprintf(os.Stderr, "  setup          Configure AI IDE integrations\n")
		fmt.Fprintf(os.Stderr, "  demos          Run demo applications\n")
		fmt.Fprintf(os.Stderr, "  audit          Run audit reports for compliance\n")
		fmt.Fprintf(os.Stderr, "  swagger        Manage Swagger/OpenAPI documentation\n\n")
		fmt.Fprintf(os.Stderr, "Operator Mode Flags:\n")
		fmt.Fprintf(os.Stderr, "  -e, --endpoint <host>    Gateway endpoint (for operator mode)\n")
		fmt.Fprintf(os.Stderr, "  -k, --key <path>        Private key path\n")
		fmt.Fprintf(os.Stderr, "  --cert <path>           Client certificate path\n")
		fmt.Fprintf(os.Stderr, "  --trust-bundle <path>   Trust bundle path\n\n")
		fmt.Fprintf(os.Stderr, "Gateway Mode Flags:\n")
		fmt.Fprintf(os.Stderr, "  --doctrine               Gateway mode: L1 enforced, L2/L3 audited\n")
		fmt.Fprintf(os.Stderr, "  --consensus             Gateway mode: L1/L2 enforced, L3 audited\n")
		fmt.Fprintf(os.Stderr, "  --notary                Gateway mode: L1/L2/L3 strictly enforced\n\n")
		fmt.Fprintf(os.Stderr, "Run './g8e --help' for more information\n")
		os.Exit(constants.ExitConfigError)
	}

	if rekeyVault || verifyVault || resetVault {
		vaultWorkDir := launchDir
		if workingDir != "" {
			vaultWorkDir = workingDir
		}
		handleVaultCommand(rekeyVault, verifyVault, resetVault, privateKey, oldPrivateKeyStr, logLevel, vaultWorkDir)
		return
	}

	if postureCount > 1 {
		fmt.Fprintf(os.Stderr, "Error: Only one of --doctrine, --consensus, or --notary may be specified\n")
		os.Exit(constants.ExitConfigError)
	}

	if postureCount > 0 {
		// Environment variables override CLI flags
		if gatewayVaultDir == "" {
			gatewayVaultDir = os.Getenv("G8E_VAULT_DIR")
		}
		if gatewayVaultKeyPath == "" {
			gatewayVaultKeyPath = os.Getenv("G8E_VAULT_KEY")
		}
		if !gatewayVaultRequireUnlock {
			gatewayVaultRequireUnlock = os.Getenv("G8E_VAULT_REQUIRE_UNLOCK") == "true"
		}
		runGatewayMode(posture, gatewayHTTPPort, gatewayHTTPSPort, gatewayDataDir, gatewayPKIDir, gatewaySecretsDir, gatewayVaultDir, gatewayVaultKeyPath, gatewayVaultRequireUnlock, gatewayPasskeyRpID, gatewayPasskeyRpName, gatewayRateLimitRPS, gatewayRateLimitBurst, logLevel, gatewayCertIdentityMode, gatewayNetworkIdentityFile)
		return
	}

	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	operatorEndpoint := constants.DefaultEndpoint
	if strings.TrimSpace(endpointURL) != "" {
		operatorEndpoint = strings.TrimSpace(endpointURL)
	}

	logger.Info("g8e", "version", version, "build", buildID)
	logger.Info("Using Operator endpoint", "endpoint", operatorEndpoint)

	// Instantiate DI types for trust and client identity
	trustStore := certs.NewTrustStore(nil)
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})

	// Load trust bundle for TLS verification. Priority:
	// 1. Explicit --trust-bundle path
	// 2. Local PKI directory (from paths.Infra.CaCertPath)
	// 3. Fetch from Operator WellKnownPKICABundle endpoint
	trustLoaded := loadTrustBundle(logger, trustBundlePath, trustStore)
	if !trustLoaded {
		if endpointURL != "" {
			trustURL := fmt.Sprintf("http://%s:%d%s", endpointURL, constants.Ports.OperatorHttp, constants.WellKnownPKICABundle)
			logger.Info("Fetching trust bundle from Operator PKI endpoint", "url", trustURL)
			pemData, err := certs.FetchTrustBundle(context.Background(), trustURL, "")
			if err != nil {
				logger.Error("Failed to fetch trust bundle from Operator", "url", trustURL, string(constants.ConnectionStateError), err)
				fmt.Fprintf(os.Stderr, "Failed to fetch trust bundle from Operator: %v\n", err)
				fmt.Fprintf(os.Stderr, "  Ensure the platform is running: ./g8e gw start\n")
				os.Exit(constants.ExitConfigError)
			}
			logCertBundle(logger, "fetched-trust-bundle", pemData)
			trustStore.SetCA(pemData)
		} else {
			logger.Error("No trust bundle available and no endpoint specified")
			fmt.Fprintf(os.Stderr, "Error: No trust bundle available. Provide --trust-bundle or --endpoint\n")
			os.Exit(constants.ExitConfigError)
		}
	}
	logger.Info("Trust bundle loaded")

	// Resolve default client certificate paths if not explicitly provided
	// Priority: 1. Explicit flags, 2. Project-local operator certificates, 3. Project-local client certificates
	if privateKey == "" {
		// Try project-local Operator key (created by enrollment)
		if _, err := os.Stat(paths.Infra.OperatorKeyPath); err == nil {
			privateKey = paths.Infra.OperatorKeyPath
			logger.Info("Using default Operator key from project directory", "path", privateKey)
		} else {
			// Try project-local client key
			if _, err := os.Stat(paths.Infra.ClientOperatorKeyPath); err == nil {
				privateKey = paths.Infra.ClientOperatorKeyPath
				logger.Info("Using default client key from project directory", "path", privateKey)
			}
		}
	}

	if clientCert == "" {
		// Try project-local Operator cert (created by enrollment)
		if _, err := os.Stat(paths.Infra.OperatorCertPath); err == nil {
			clientCert = paths.Infra.OperatorCertPath
			logger.Info("Using default Operator certificate from project directory", "path", clientCert)
		} else {
			// Try project-local client cert
			if _, err := os.Stat(paths.Infra.ClientOperatorCertPath); err == nil {
				clientCert = paths.Infra.ClientOperatorCertPath
				logger.Info("Using default client certificate from project directory", "path", clientCert)
			}
		}
	}

	// When -e is given, always re-enroll so we get certs from the current gateway PKI.
	// Without -e, fall back to existing certs only if both are present.
	if endpointURL != "" {
		logger.Info("Performing automatic enrollment with Gateway", "endpoint", endpointURL)
		if err := performAutomaticEnrollment(endpointURL, launchDir, logger); err != nil {
			logger.Error("Automatic enrollment failed", string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "Automatic enrollment failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "  Ensure the Gateway is running and accessible at %s\n", endpointURL)
			os.Exit(constants.ExitConfigError)
		}

		// After enrollment, set the certificate paths
		privateKey = paths.Infra.OperatorKeyPath
		clientCert = paths.Infra.OperatorCertPath

		// Reload trust bundle after enrollment (enrollment may have updated it)
		if pemData, err := os.ReadFile(paths.Infra.CaCertPath); err == nil {
			trustStore.SetCA(pemData)
			logger.Info("Trust bundle reloaded after enrollment", "path", paths.Infra.CaCertPath)
		}

		// Keep using the original endpoint (localhost or provided IP) for Gateway connections
		logger.Info("Automatic enrollment completed, using enrolled certificates")
	}

	if privateKey == "" {
		fmt.Fprintf(os.Stderr, "Private key is required (-k or --key). Expected locations:\n")
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultOperatorKeyDesc)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultClientKeyDesc)
		fmt.Fprintf(os.Stderr, "Or provide --endpoint to perform automatic enrollment\n")
		os.Exit(constants.ExitConfigError)
	}

	if clientCert == "" {
		fmt.Fprintf(os.Stderr, "Client certificate is required (--cert or --client-cert). Expected locations:\n")
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultOperatorCertDesc)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultClientCertDesc)
		fmt.Fprintf(os.Stderr, "Or provide --endpoint to perform automatic enrollment\n")
		os.Exit(constants.ExitConfigError)
	}

	// Create DI-based TLS config from trust store and client identity
	tlsConfig := certs.NewTLSConfig(trustStore, clientIdentity)

	// Load client certificate for mTLS
	certPEM, err := os.ReadFile(clientCert)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read client certificate: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	keyPEM, err := os.ReadFile(privateKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read private key: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load client certificate/key pair: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	clientIdentity.SetCertificate(cert)
	logCertBundle(logger, "client-cert", certPEM)
	logger.Info("[TLS-DEBUG] client cert loaded",
		"cert_file", clientCert,
		"key_file", privateKey,
	)

	// Probe the gateway's TLS cert chain before the real connection.
	logger.Info("[TLS-DEBUG] probing gateway TLS cert chain", "endpoint", operatorEndpoint, "tls_server_name", constants.GatewayInternalHostname)
	probeGatewayTLS(logger, operatorEndpoint, trustStore)

	// Resolve the effective working directory: flag overrides launch dir.
	effectiveWorkDir := launchDir
	if workingDir != "" {
		effectiveWorkDir = workingDir
	}

	cfg, err := config.Load(config.LoadOptions{
		OperatorEndpoint: operatorEndpoint,

		HTTPPort:              0, // Will default to constants.Ports.OperatorHttp (8080)
		HTTPSPort:             0, // Will default to constants.Ports.OperatorHttps (8443)
		CloudMode:             cloudMode,
		CloudProvider:         cloudProvider,
		ExecutionVaultEnabled: executionVault,
		NoGit:                 noGit,
		LogLevel:              logLevel,
		WorkDir:               effectiveWorkDir,
		PKIDir:                "",
		SecretsDir:            "",
		HeartbeatInterval:     heartbeatInterval,
		Shell:                 os.Getenv("SHELL"),
		Lang:                  os.Getenv("LANG"),
		Term:                  os.Getenv("TERM"),
		TZ:                    os.Getenv("TZ"),
		Posture:               "", // Will default to PostureNotary in Load() since L3Notary is nil
	})
	if err != nil {
		logger.Error("Failed to load configuration", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitConfigError)
	}

	cfg.Version = version

	if cfg.CloudMode {
		logger.Info("Cloud Operator mode enabled", "provider", cfg.CloudProvider)
	}

	if cfg.ExecutionVaultEnabled {
		logger.Info("Execution vault enabled - data stays in working directory", "working_dir", cfg.WorkDir)
	} else {
		logger.Info("Execution vault disabled (command output sent to cloud)")
	}

	g8eoService, err := services.NewG8eoService(cfg, logger, tlsConfig)
	if err != nil {
		logger.Error("Failed to create Operator service", string(constants.ConnectionStateError), err)
		os.Exit(exitcode.FromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := g8eoService.Start(ctx); err != nil {
			logger.Error("Failed to start g8e", string(constants.ConnectionStateError), err)
			os.Exit(exitcode.FromError(err))
		}
	}()

	// Start background client certificate renewal loop
	if clientCert != "" && privateKey != "" {
		go runClientCertRenewalLoop(ctx, cfg, clientCert, privateKey, logger, clientIdentity)
	}

	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)

	if err := g8eoService.Stop(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", string(constants.ConnectionStateError), err)
	}
	shutdownCancel()

	cancel()
	os.Exit(constants.ExitSuccess)
}

func printVersion() {
	fmt.Printf("g8e\n  Version:   %s\n  Build ID:  %s\n  Build Time: %s\n  Platform:  %s\n", version, buildID, buildTime, platform)
}
