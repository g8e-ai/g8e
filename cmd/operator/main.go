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
	"sync"
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

	if dispatchCLI(version) {
		return
	}

	launchDir := initializePaths()
	flags := parseFlags()

	if flags.showVersion {
		printVersion()
		os.Exit(constants.ExitSuccess)
	}

	if dispatchCLIAfterFlags(version) {
		return
	}

	posture, postureCount := validatePostureFlags(flags)

	if shouldShowUsageHelp(postureCount, flags.endpointURL) {
		showUsageHelp()
		os.Exit(constants.ExitConfigError)
	}

	if flags.rekeyVault || flags.verifyVault || flags.resetVault {
		handleVaultCommand(flags.rekeyVault, flags.verifyVault, flags.resetVault, flags.privateKey, flags.oldPrivateKeyStr, flags.logLevel)
		return
	}

	if postureCount > 1 {
		fmt.Fprintf(os.Stderr, "%s\n", constants.ErrMutuallyExclusiveFlags)
		os.Exit(constants.ExitConfigError)
	}

	if postureCount > 0 {
		runGatewayModeFromFlags(flags, posture)
		return
	}

	runOperatorMode(flags, launchDir)
}

func printVersion() {
	fmt.Printf("g8e\n  Version:   %s\n  Build ID:  %s\n  Build Time: %s\n  Platform:  %s\n", version, buildID, buildTime, platform)
}

// cliSubcommands is the consolidated map of all CLI subcommands
var cliSubcommands = map[string]bool{
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
	"demos":                 true,
	"help":                  true,
	"--help":                true,
	"-h":                    true,
}

// dispatchCLI checks if the first argument is a CLI subcommand and dispatches to clicmd.Execute
func dispatchCLI(version string) bool {
	if len(os.Args) > 1 && cliSubcommands[os.Args[1]] {
		clicmd.Execute(version)
		return true
	}
	return false
}

// initializePaths initializes paths and returns the launch directory
func initializePaths() string {
	if err := paths.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrPathsInitFailed, err)
		os.Exit(constants.ExitConfigError)
	}
	launchDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrWorkingDirFailed, err)
		os.Exit(constants.ExitConfigError)
	}
	return launchDir
}

// flagValues holds all parsed command-line flags
type flagValues struct {
	privateKey                string
	clientCert                string
	endpointURL               string
	trustBundlePath           string
	workingDir                string
	cloudMode                 bool
	cloudProvider             string
	executionVault            bool
	logLevel                  string
	showVersion               bool
	noGit                     bool
	doctrineMode              bool
	consensusMode             bool
	notaryMode                bool
	gatewayHTTPPort           int
	gatewayHTTPSPort          int
	gatewayDataDir            string
	gatewayPKIDir             string
	gatewaySecretsDir         string
	gatewayVaultDir           string
	gatewayVaultKeyPath       string
	gatewayVaultRequireUnlock bool
	gatewayPasskeyRpID        string
	gatewayPasskeyRpName      string
	gatewayRateLimitRPS       float64
	gatewayRateLimitBurst     int
	gatewayCertIdentityMode   string
	gatewayNetworkIdentityFile string
	gatewayTribunalID         string
	gatewayTribunalURL        string
	heartbeatInterval         time.Duration
	rekeyVault                bool
	oldPrivateKeyStr          string
	verifyVault               bool
	resetVault                bool
}

// parseFlags defines and parses all command-line flags
func parseFlags() flagValues {
	var flags flagValues

	flag.StringVar(&flags.privateKey, "k", "", "Private key")
	flag.StringVar(&flags.privateKey, "key", "", "Private key")
	flag.StringVar(&flags.clientCert, "cert", "", "Client certificate (for mTLS)")
	flag.StringVar(&flags.clientCert, "client-cert", "", "Client certificate (for mTLS)")
	flag.StringVar(&flags.endpointURL, "e", "", "Endpoint (hostname or IP)")
	flag.StringVar(&flags.endpointURL, "endpoint", "", "Endpoint (hostname or IP)")
	flag.StringVar(&flags.trustBundlePath, "trust-bundle", "", "Path to trust bundle PEM file (default: from paths.Infra.CaCertPath or fetch from WellKnownPKICABundle endpoint)")
	flag.StringVar(&flags.workingDir, "working-dir", "", "Working directory (default: directory Operator was launched from)")
	flag.BoolVar(&flags.cloudMode, "c", true, "Cloud mode")
	flag.BoolVar(&flags.cloudMode, string(constants.OperatorTypeCloud), true, "Cloud mode")
	flag.StringVar(&flags.cloudProvider, "p", "", "Cloud provider")
	flag.StringVar(&flags.cloudProvider, "provider", "", "Cloud provider")
	flag.BoolVar(&flags.executionVault, "s", true, "Enable execution vault (stores execution data in current directory)")
	flag.BoolVar(&flags.executionVault, "execution-vault", true, "Enable execution vault (stores execution data in current directory)")
	flag.StringVar(&flags.logLevel, "l", "info", "Log level")
	flag.StringVar(&flags.logLevel, "log", "info", "Log level")
	flag.BoolVar(&flags.noGit, "G", false, "Disable git (ledger)")
	flag.BoolVar(&flags.noGit, "no-git", false, "Disable git (ledger)")
	flag.BoolVar(&flags.showVersion, "v", false, "Version")
	flag.BoolVar(&flags.showVersion, "version", false, "Version")

	flag.BoolVar(&flags.doctrineMode, "doctrine", false, "Gateway mode: L1 enforced, L2/L3 audited (default)")
	flag.BoolVar(&flags.consensusMode, "consensus", false, "Gateway mode: L1/L2 enforced, L3 audited")
	flag.BoolVar(&flags.notaryMode, "notary", false, "Gateway mode: L1/L2/L3 strictly enforced")
	flag.IntVar(&flags.gatewayHTTPPort, "http-port", constants.Ports.OperatorHttp, "HTTP port for bootstrap and MCP routes (default: from paths.json)")
	flag.IntVar(&flags.gatewayHTTPSPort, "https-port", constants.Ports.OperatorHttps, "HTTPS port for mTLS API and public surface (default: from paths.json)")
	flag.StringVar(&flags.gatewayDataDir, "data-dir", "", "Data directory for SQLite database (default: from paths.Infra.DataDir in working directory)")
	flag.StringVar(&flags.gatewayPKIDir, "pki-dir", "", "Directory for TLS certificates (default: from paths.Infra.PkiDir)")
	flag.StringVar(&flags.gatewaySecretsDir, "secrets-dir", "", "Directory for platform secrets (default: from paths.Infra.SecretsDir)")
	flag.StringVar(&flags.gatewayVaultDir, "vault-dir", "", "Directory for vault data (default: from constants.DefaultVaultDirDesc)")
	flag.StringVar(&flags.gatewayVaultKeyPath, "vault-key", "", "Path to vault private key (default: from constants.DefaultVaultKeyDesc)")
	flag.BoolVar(&flags.gatewayVaultRequireUnlock, "vault-require-unlock", false, "Require vault to be unlocked at startup (fail if vault cannot be unlocked)")
	flag.StringVar(&flags.gatewayPasskeyRpID, "passkey-rp-id", "", "RP ID for passkey operations (default: localhost)")
	flag.StringVar(&flags.gatewayPasskeyRpName, "passkey-rp-name", "", "RP Name for passkey operations (default: g8e)")
	flag.Float64Var(&flags.gatewayRateLimitRPS, "rate-limit-rps", 5.0, "Gateway requests per second limit (set to 0 to disable)")
	flag.IntVar(&flags.gatewayRateLimitBurst, "rate-limit-burst", 10, "Gateway rate limit burst size")
	flag.StringVar(&flags.gatewayCertIdentityMode, "cert-mode", "", "Certificate mode: full (all hostnames/IPs), localhost (only localhost)")
	flag.StringVar(&flags.gatewayNetworkIdentityFile, "network-identity-file", "", "Path to JSON file containing pre-detected network identity")
	flag.StringVar(&flags.gatewayTribunalID, "tribunal-id", "", "ID of the TribunalPolicy for L2 consensus (required for --consensus)")
	flag.StringVar(&flags.gatewayTribunalURL, "tribunal-url", "", "URL of the Tribunal service for L2 deliberation (e.g. https://localhost:8443/tribunal/v1/deliberate)")
	flag.BoolVar(&flags.rekeyVault, "rekey-vault", false, "Re-encrypt vault with new private key (requires --old-key)")
	flag.StringVar(&flags.oldPrivateKeyStr, "old-key", "", "Old private key for vault re-keying")
	flag.BoolVar(&flags.verifyVault, "verify-vault", false, "Verify vault integrity")
	flag.BoolVar(&flags.resetVault, "reset-vault", false, "Reset vault (DESTROYS ALL DATA)")

	flag.DurationVar(&flags.heartbeatInterval, "heartbeat-interval", 0, "Heartbeat interval (e.g. 60s, 2m); overrides the 30s default")

	flag.Parse()
	return flags
}

// dispatchCLIAfterFlags checks for CLI commands after flag parsing
func dispatchCLIAfterFlags(version string) bool {
	if len(os.Args) == 1 || (len(os.Args) > 1 && cliSubcommands[os.Args[1]]) {
		clicmd.Execute(version)
		return true
	}
	return false
}

// validatePostureFlags validates and returns the selected posture and count
func validatePostureFlags(flags flagValues) (config.GatewayPosture, int) {
	postureCount := 0
	var posture config.GatewayPosture
	if flags.doctrineMode {
		postureCount++
		posture = config.PostureDoctrine
	}
	if flags.consensusMode {
		postureCount++
		posture = config.PostureConsensus
	}
	if flags.notaryMode {
		postureCount++
		posture = config.PostureNotary
	}
	return posture, postureCount
}

// shouldShowUsageHelp determines if usage help should be displayed
func shouldShowUsageHelp(postureCount int, endpointURL string) bool {
	return len(os.Args) > 1 && !cliSubcommands[os.Args[1]] && endpointURL == "" && postureCount == 0
}

// showUsageHelp displays the usage help message
func showUsageHelp() {
	fmt.Fprintf(os.Stderr, "%s: '%s'\n\n", constants.ErrUnrecognizedCommand, os.Args[1])
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
}

// runGatewayModeFromFlags runs gateway mode with environment variable overrides
func runGatewayModeFromFlags(flags flagValues, posture config.GatewayPosture) {
	gatewayVaultDir := flags.gatewayVaultDir
	gatewayVaultKeyPath := flags.gatewayVaultKeyPath
	gatewayVaultRequireUnlock := flags.gatewayVaultRequireUnlock
	gatewayTribunalID := flags.gatewayTribunalID
	gatewayTribunalURL := flags.gatewayTribunalURL

	if gatewayVaultDir == "" {
		gatewayVaultDir = os.Getenv(string(constants.EnvVar.VaultDir))
	}
	if gatewayVaultKeyPath == "" {
		gatewayVaultKeyPath = os.Getenv(string(constants.EnvVar.VaultKey))
	}
	if !gatewayVaultRequireUnlock {
		gatewayVaultRequireUnlock = os.Getenv(string(constants.EnvVar.VaultRequireUnlock)) == "true"
	}
	if gatewayTribunalID == "" {
		gatewayTribunalID = os.Getenv(string(constants.EnvVar.TribunalID))
	}
	if gatewayTribunalURL == "" {
		gatewayTribunalURL = os.Getenv(string(constants.EnvVar.TribunalURL))
	}

	runGatewayMode(GatewayStartConfig{
		Posture:             posture,
		HTTPPort:            flags.gatewayHTTPPort,
		HTTPSPort:           flags.gatewayHTTPSPort,
		DataDir:             flags.gatewayDataDir,
		PKIDir:              flags.gatewayPKIDir,
		SecretsDir:          flags.gatewaySecretsDir,
		VaultDir:            gatewayVaultDir,
		VaultKeyPath:        gatewayVaultKeyPath,
		VaultRequireUnlock:  gatewayVaultRequireUnlock,
		PasskeyRpID:         flags.gatewayPasskeyRpID,
		PasskeyRpName:       flags.gatewayPasskeyRpName,
		RateLimitRPS:        flags.gatewayRateLimitRPS,
		RateLimitBurst:      flags.gatewayRateLimitBurst,
		LogLevel:            flags.logLevel,
		CertIdentityMode:    flags.gatewayCertIdentityMode,
		NetworkIdentityFile: flags.gatewayNetworkIdentityFile,
		TribunalID:          gatewayTribunalID,
		TribunalURL:         gatewayTribunalURL,
	})
}

// runOperatorMode runs the operator mode with the given flags and launch directory
func runOperatorMode(flags flagValues, launchDir string) {
	logger, err := configureLogger(flags.logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", flags.logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	operatorEndpoint := constants.DefaultEndpoint
	if strings.TrimSpace(flags.endpointURL) != "" {
		operatorEndpoint = strings.TrimSpace(flags.endpointURL)
	}

	logger.Info("g8e", "version", version, "build", buildID)
	logger.Info("Using Operator endpoint", "endpoint", operatorEndpoint)

	trustStore := certs.NewTrustStore(nil)
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})

	trustLoaded := loadTrustBundle(logger, flags.trustBundlePath, trustStore)
	if !trustLoaded {
		if flags.endpointURL != "" {
			trustURL := fmt.Sprintf("http://%s:%d%s", flags.endpointURL, constants.Ports.OperatorHttp, constants.WellKnownPKICABundle)
			logger.Info("Fetching trust bundle from Operator PKI endpoint", "url", trustURL)
			pemData, err := certs.FetchTrustBundle(context.Background(), trustURL, "")
			if err != nil {
				logger.Error("Failed to fetch trust bundle from Operator", "url", trustURL, string(constants.ConnectionStateError), err)
				fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrFetchTrustBundle, err)
				fmt.Fprintf(os.Stderr, "  Ensure the platform is running: ./g8e gw start\n")
				os.Exit(constants.ExitConfigError)
			}
			logCertBundle(logger, "fetched-trust-bundle", pemData)
			trustStore.SetCA(pemData)
		} else {
			logger.Error("No trust bundle available and no endpoint specified")
			fmt.Fprintf(os.Stderr, "%s. Provide --trust-bundle or --endpoint\n", constants.ErrNoTrustBundle)
			os.Exit(constants.ExitConfigError)
		}
	}
	logger.Info("Trust bundle loaded")

	privateKey := flags.privateKey
	clientCert := flags.clientCert

	if privateKey == "" {
		if _, err := os.Stat(paths.Infra.OperatorKeyPath); err == nil {
			privateKey = paths.Infra.OperatorKeyPath
			logger.Info("Using default Operator key from project directory", "path", privateKey)
		} else {
			if _, err := os.Stat(paths.Infra.ClientOperatorKeyPath); err == nil {
				privateKey = paths.Infra.ClientOperatorKeyPath
				logger.Info("Using default client key from project directory", "path", privateKey)
			}
		}
	}

	if clientCert == "" {
		if _, err := os.Stat(paths.Infra.OperatorCertPath); err == nil {
			clientCert = paths.Infra.OperatorCertPath
			logger.Info("Using default Operator certificate from project directory", "path", clientCert)
		} else {
			if _, err := os.Stat(paths.Infra.ClientOperatorCertPath); err == nil {
				clientCert = paths.Infra.ClientOperatorCertPath
				logger.Info("Using default client certificate from project directory", "path", clientCert)
			}
		}
	}

	if flags.endpointURL != "" {
		logger.Info("Performing automatic enrollment with Gateway", "endpoint", flags.endpointURL)
		if err := performAutomaticEnrollment(flags.endpointURL, launchDir, logger); err != nil {
			logger.Error("Automatic enrollment failed", string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "Automatic enrollment failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "  Ensure the Gateway is running and accessible at %s\n", flags.endpointURL)
			os.Exit(constants.ExitConfigError)
		}

		privateKey = paths.Infra.OperatorKeyPath
		clientCert = paths.Infra.OperatorCertPath

		if pemData, err := os.ReadFile(paths.Infra.CaCertPath); err == nil {
			trustStore.SetCA(pemData)
			logger.Info("Trust bundle reloaded after enrollment", "path", paths.Infra.CaCertPath)
		}

		logger.Info("Automatic enrollment completed, using enrolled certificates")
	}

	if privateKey == "" {
		fmt.Fprintf(os.Stderr, "%s (-k or --key). Expected locations:\n", constants.ErrPrivateKeyRequired)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultOperatorKeyDesc)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultClientKeyDesc)
		fmt.Fprintf(os.Stderr, "Or provide --endpoint to perform automatic enrollment\n")
		os.Exit(constants.ExitConfigError)
	}

	if clientCert == "" {
		fmt.Fprintf(os.Stderr, "%s (--cert or --client-cert). Expected locations:\n", constants.ErrClientCertRequired)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultOperatorCertDesc)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultClientCertDesc)
		fmt.Fprintf(os.Stderr, "Or provide --endpoint to perform automatic enrollment\n")
		os.Exit(constants.ExitConfigError)
	}

	tlsConfig := certs.NewTLSConfig(trustStore, clientIdentity)

	certPEM, err := os.ReadFile(clientCert)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrReadClientCert, err)
		os.Exit(constants.ExitConfigError)
	}

	keyPEM, err := os.ReadFile(privateKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrReadPrivateKey, err)
		os.Exit(constants.ExitConfigError)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrLoadCertKeyPair, err)
		os.Exit(constants.ExitConfigError)
	}

	clientIdentity.SetCertificate(cert)
	logCertBundle(logger, "client-cert", certPEM)
	logger.Info("[TLS-DEBUG] client cert loaded",
		"cert_file", clientCert,
		"key_file", privateKey,
	)

	effectiveWorkDir := launchDir
	if flags.workingDir != "" {
		effectiveWorkDir = flags.workingDir
	}

	cfg, err := config.Load(config.LoadOptions{
		OperatorEndpoint: operatorEndpoint,
		HTTPPort:              0,
		HTTPSPort:             0,
		CloudMode:             flags.cloudMode,
		CloudProvider:         flags.cloudProvider,
		ExecutionVaultEnabled: flags.executionVault,
		NoGit:                 flags.noGit,
		LogLevel:              flags.logLevel,
		WorkDir:               effectiveWorkDir,
		PKIDir:                "",
		SecretsDir:            "",
		HeartbeatInterval:     flags.heartbeatInterval,
		Shell:                 os.Getenv(string(constants.EnvVar.Shell)),
		Lang:                  os.Getenv(string(constants.EnvVar.Lang)),
		Term:                  os.Getenv(string(constants.EnvVar.Term)),
		TZ:                    os.Getenv(string(constants.EnvVar.TZ)),
		Posture:               "",
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

	var wg sync.WaitGroup
	serviceErr := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := g8eoService.Start(ctx); err != nil {
			logger.Error("Failed to start g8e", string(constants.ConnectionStateError), err)
			serviceErr <- err
		}
	}()

	if clientCert != "" && privateKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runClientCertRenewalLoop(ctx, cfg, clientCert, privateKey, logger, clientIdentity)
		}()
	}

	select {
	case err := <-serviceErr:
		logger.Error("Service failed, shutting down", string(constants.ConnectionStateError), err)
		cancel()
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig.String())
		cancel()
	}

	wg.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Duration(constants.ShutdownTimeout)*time.Second)

	if err := g8eoService.Stop(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", string(constants.ConnectionStateError), err)
	}
	shutdownCancel()

	os.Exit(constants.ExitSuccess)
}
