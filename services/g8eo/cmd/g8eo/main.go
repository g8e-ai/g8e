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

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/certs"
	"github.com/g8e-ai/g8e/services/g8eo/internal/cmd"
	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services"
	auth "github.com/g8e-ai/g8e/services/g8eo/internal/services/auth"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/execution"
	listen "github.com/g8e-ai/g8e/services/g8eo/internal/services/listen"
	openclaw "github.com/g8e-ai/g8e/services/g8eo/internal/services/openclaw"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/pubsub"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sentinel"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/system"
	vault "github.com/g8e-ai/g8e/services/g8eo/internal/services/vault"
)

// Version information (set via ldflags during build)
var (
	version  string = string(constants.Status.VersionStability.Dev)
	buildID  string = string(constants.SystemHealthUnknown)
	platform string = string(constants.SystemHealthUnknown)
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == constants.ApprovalTypeStreamStr {
		cmd.RunStream(os.Args[2:])
		return
	}

	settings := config.LoadSettings()

	// Capture the launch directory before any flag parsing or os.Chdir calls.
	launchDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to determine working directory: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	var apiKey string
	var deviceToken string
	var endpointURL string
	var trustBundlePath string
	var workingDir string
	var cloudMode bool
	var cloudProvider string
	var localStorage bool
	var logLevel string
	var showVersion bool

	var noGit bool

	var httpPort int

	var listenMode bool
	var listenHTTPPort int
	var listenBootstrapPort int
	var listenPublicPort int
	var listenDataDir string
	var listenPKIDir string
	var listenSecretsDir string
	var listenPasskeyRpID string
	var listenPasskeyRpName string
	var openclawMode bool
	var openclawURL string
	var openclawToken string
	var openclawNodeID string
	var openclawDisplayName string

	var heartbeatInterval time.Duration

	var rekeyVault bool
	var oldAPIKey string
	var verifyVault bool
	var resetVault bool
	var mcpServe bool
	flag.BoolVar(&mcpServe, "mcp-serve", false, "Expose MCP gateway over stdio (JSON-RPC)")
	flag.StringVar(&apiKey, "k", "", "API key")
	flag.StringVar(&deviceToken, "D", "", "Device link token for operator deployment")
	flag.StringVar(&endpointURL, "e", "", "Endpoint (hostname or IP)")
	flag.BoolVar(&cloudMode, "c", true, "Cloud mode")
	flag.StringVar(&cloudProvider, "p", "", "Cloud provider")
	flag.BoolVar(&localStorage, "s", true, "Enable local storage (stores data in current directory)")
	flag.StringVar(&logLevel, "l", "info", "Log level")
	flag.BoolVar(&noGit, "G", false, "Disable git (ledger)")
	flag.BoolVar(&showVersion, "v", false, "Version")
	flag.IntVar(&httpPort, "http-port", constants.Ports.OperatorHttps, "HTTPS port for auth/bootstrap via operator proxy (default: from paths.json)")
	flag.StringVar(&apiKey, "key", "", "API key")
	flag.StringVar(&deviceToken, "device-token", "", "Device link token for operator deployment")
	flag.StringVar(&endpointURL, "endpoint", "", "Endpoint (hostname or IP)")
	flag.StringVar(&trustBundlePath, "trust-bundle", "", "Path to trust bundle PEM file (default: .g8e/pki/hub-bundle.pem or fetch from /.well-known/g8e/pki/hub-bundle.pem)")
	flag.StringVar(&workingDir, "working-dir", "", "Working directory (default: directory operator was launched from)")
	flag.BoolVar(&cloudMode, constants.OperatorTypeCloudStr, true, "Cloud mode")
	flag.StringVar(&cloudProvider, "provider", "", "Cloud provider")
	flag.BoolVar(&localStorage, "local-storage", true, "Enable local storage (stores data in current directory)")
	flag.StringVar(&logLevel, "log", "info", "Log level")
	flag.BoolVar(&noGit, "no-git", false, "Disable git (ledger)")
	flag.BoolVar(&showVersion, "version", false, "Version")

	flag.BoolVar(&listenMode, "listen", false, "Listen mode: platform persistence + pub/sub broker")
	flag.IntVar(&listenHTTPPort, "http-listen-port", constants.Ports.OperatorHttps, "HTTPS port for mTLS API (default: from paths.json)")
	flag.IntVar(&listenBootstrapPort, "bootstrap-listen-port", constants.Ports.OperatorBootstrapHttps, "Bootstrap TLS port for device-link enrollment (default: from paths.json)")
	flag.IntVar(&listenPublicPort, "public-listen-port", constants.Ports.OperatorPublicHttps, "Public browser/BYO bootstrap port (default: from paths.json)")
	flag.StringVar(&listenDataDir, "data-dir", "", "Data directory for SQLite database (default: .g8e/data in working directory)")
	flag.StringVar(&listenPKIDir, "pki-dir", "", "Directory for TLS certificates (default: .g8e/pki)")
	flag.StringVar(&listenSecretsDir, "secrets-dir", "", "Directory for platform secrets (default: .g8e/secrets)")
	flag.StringVar(&listenPasskeyRpID, "passkey-rp-id", "", "RP ID for passkey operations (default: localhost)")
	flag.StringVar(&listenPasskeyRpName, "passkey-rp-name", "", "RP Name for passkey operations (default: g8e)")
	flag.BoolVar(&rekeyVault, "rekey-vault", false, "Re-encrypt vault with new API key (requires --old-key)")
	flag.StringVar(&oldAPIKey, "old-key", "", "Old API key for vault re-keying")
	flag.BoolVar(&verifyVault, "verify-vault", false, "Verify vault integrity")
	flag.BoolVar(&resetVault, "reset-vault", false, "Reset vault (DESTROYS ALL DATA)")

	flag.DurationVar(&heartbeatInterval, "heartbeat-interval", 0, "Heartbeat interval (e.g. 60s, 2m); overrides the 30s default")

	flag.BoolVar(&openclawMode, "openclaw", false, "OpenClaw mode: connect to OpenClaw Gateway as a node host")
	flag.StringVar(&openclawURL, "openclaw-url", "", "OpenClaw Gateway WebSocket URL (e.g. ws://"+constants.DefaultEndpoint+":18789)")
	flag.StringVar(&openclawToken, "openclaw-token", "", "OpenClaw Gateway auth token (or set OPENCLAW_GATEWAY_TOKEN)")
	flag.StringVar(&openclawNodeID, "openclaw-node-id", "", "Node ID to advertise (default: hostname)")
	flag.StringVar(&openclawDisplayName, "openclaw-name", "", "Display name shown in OpenClaw UI (default: node ID)")

	// Customize usage
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: g8e.operator [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -k, --key <key>         API key (or set G8E_OPERATOR_API_KEY)\n")
		fmt.Fprintf(os.Stderr, "  -D, --device-token <tok> Device link token for operator deployment\n")
		fmt.Fprintf(os.Stderr, "  -e, --endpoint <host>     Operator endpoint: IP address of the Docker host running operator\n")
		fmt.Fprintf(os.Stderr, "      --trust-bundle <path> Path to trust bundle PEM file (default: .g8e/pki/hub-bundle.pem or fetch from /.well-known/g8e/pki/hub-bundle.pem)\n")
		fmt.Fprintf(os.Stderr, "      --working-dir <dir>   Working directory (default: directory operator was launched from)\n")
		fmt.Fprintf(os.Stderr, "                            All commands and data storage are anchored to this directory\n")
		fmt.Fprintf(os.Stderr, "      --http-port <port>    HTTPS port to dial for auth/bootstrap (default: %d)\n", constants.Ports.OperatorHttps)
		fmt.Fprintf(os.Stderr, "  -c, --cloud             Cloud Operator mode (for AWS/cloud CLI)\n")
		fmt.Fprintf(os.Stderr, "  -p, --provider <name>   Cloud provider: aws, gcp, azure\n")
		fmt.Fprintf(os.Stderr, "  -s, --local-storage     Store audit data locally instead of cloud (default: on)\n")
		fmt.Fprintf(os.Stderr, "                          When enabled, data is stored in ./.g8e/ relative to launch directory\n")
		fmt.Fprintf(os.Stderr, "  -l, --log <level>       Log level: info, error, debug (default: info)\n")
		fmt.Fprintf(os.Stderr, "  -G, --no-git            Disable ledger (git-backed file versioning)\n")
		fmt.Fprintf(os.Stderr, "      --heartbeat-interval <dur> Heartbeat interval (e.g. 60s, 2m); overrides the 30s default\n")
		fmt.Fprintf(os.Stderr, "  -v, --version           Show version\n")
		fmt.Fprintf(os.Stderr, "\nListen Mode (platform persistence + pub/sub broker):\n")
		fmt.Fprintf(os.Stderr, "  --listen                    Listen mode: local persistence + pub/sub broker\n")
		fmt.Fprintf(os.Stderr, "  --http-listen-port <port>   HTTPS port for mTLS API (default: %d)\n", constants.Ports.OperatorHttps)
		fmt.Fprintf(os.Stderr, "  --bootstrap-listen-port <port> Bootstrap TLS port for device-link enrollment (default: %d)\n", constants.Ports.OperatorBootstrapHttps)
		fmt.Fprintf(os.Stderr, "  --public-listen-port <port> Public browser/BYO bootstrap port (default: %d)\n", constants.Ports.OperatorPublicHttps)
		fmt.Fprintf(os.Stderr, "  --data-dir <dir>            Data directory for SQLite (default: .g8e/data in working directory)\n")
		fmt.Fprintf(os.Stderr, "  --pki-dir <dir>             Directory for TLS certificates (default: .g8e/pki)\n")
		fmt.Fprintf(os.Stderr, "  --secrets-dir <dir>         Directory for platform secrets (default: .g8e/secrets)\n")
		fmt.Fprintf(os.Stderr, "  --passkey-rp-id <id>        RP ID for passkey operations (default: localhost)\n")
		fmt.Fprintf(os.Stderr, "  --passkey-rp-name <name>    RP Name for passkey operations (default: g8e)\n")
		fmt.Fprintf(os.Stderr, "\nVault Management:\n")
		fmt.Fprintf(os.Stderr, "  --rekey-vault           Re-encrypt vault with new API key\n")
		fmt.Fprintf(os.Stderr, "  --old-key <key>         Old API key (required for --rekey-vault)\n")
		fmt.Fprintf(os.Stderr, "  --verify-vault          Verify vault integrity\n")
		fmt.Fprintf(os.Stderr, "  --reset-vault           Reset vault (DESTROYS ALL DATA)\n")
		fmt.Fprintf(os.Stderr, "\nOpenClaw Node Host Mode:\n")
		fmt.Fprintf(os.Stderr, "  --openclaw              Connect to an OpenClaw Gateway as a node host\n")
		fmt.Fprintf(os.Stderr, "  --openclaw-url <url>    OpenClaw Gateway WebSocket URL (e.g. ws://"+constants.DefaultEndpoint+":18789)\n")
		fmt.Fprintf(os.Stderr, "  --openclaw-token <tok>  Auth token (or set OPENCLAW_GATEWAY_TOKEN)\n")
		fmt.Fprintf(os.Stderr, "  --openclaw-node-id <id> Node ID advertised to the Gateway (default: hostname)\n")
		fmt.Fprintf(os.Stderr, "  --openclaw-name <name>  Display name shown in OpenClaw UI (default: node ID)\n")
	}

	flag.Parse()

	if showVersion {
		printVersion()
		os.Exit(constants.ExitSuccess)
	}

	if rekeyVault || verifyVault || resetVault {
		vaultWorkDir := launchDir
		if workingDir != "" {
			vaultWorkDir = workingDir
		}
		handleVaultCommand(rekeyVault, verifyVault, resetVault, apiKey, oldAPIKey, logLevel, vaultWorkDir)
		return
	}

	if listenMode {
		runListenMode(listenHTTPPort, listenBootstrapPort, listenPublicPort, listenDataDir, listenPKIDir, listenSecretsDir, listenPasskeyRpID, listenPasskeyRpName, logLevel)
		return
	}

	if mcpServe {
		runMCPServe(endpointURL, listenPKIDir, logLevel)
		return
	}

	if openclawMode {
		if openclawToken == "" {
			openclawToken = settings.OpenClawGatewayToken
		}
		runOpenClawMode(openclawURL, openclawToken, openclawNodeID, openclawDisplayName, settings.Path, logLevel)
		return
	}

	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	operatorEndpoint := constants.DefaultEndpoint
	if endpointURL == "" {
		endpointURL = settings.OperatorEndpoint
	}
	if strings.TrimSpace(endpointURL) != "" {
		operatorEndpoint = strings.TrimSpace(endpointURL)
	}

	logger.Info("g8e Operator", "version", version, "build", buildID)
	logger.Info("Using Operator endpoint", "endpoint", operatorEndpoint)

	// Load trust bundle for TLS verification. Priority:
	// 1. Explicit --trust-bundle path
	// 2. Local PKI directory (.g8e/pki/hub-bundle.pem)
	// 3. Fetch from Operator /.well-known/g8e/pki/hub-bundle.pem endpoint
	trustLoaded := loadTrustBundle(logger, trustBundlePath, workingDir)
	if !trustLoaded {
		if endpointURL != "" {
			trustURL := fmt.Sprintf("http://%s:%d/.well-known/g8e/pki/hub-bundle.pem", endpointURL, constants.Ports.OperatorBootstrapHttps)
			logger.Info("Fetching trust bundle from Operator PKI endpoint", "url", trustURL)
			if err := certs.FetchAndSetCA(context.Background(), trustURL); err != nil {
				logger.Error("Failed to fetch trust bundle from Operator", "url", trustURL, constants.ConnectionStateErrorStr, err)
				fmt.Fprintf(os.Stderr, "Failed to fetch trust bundle from Operator: %v\n", err)
				fmt.Fprintf(os.Stderr, "  Ensure the platform is running: ./g8e platform start\n")
				os.Exit(constants.ExitConfigError)
			}
		} else {
			logger.Error("No trust bundle available and no endpoint specified")
			fmt.Fprintf(os.Stderr, "Error: No trust bundle available. Provide --trust-bundle or --endpoint\n")
			os.Exit(constants.ExitConfigError)
		}
	}
	logger.Info("Trust bundle loaded")

	var deviceAuthResult *auth.DeviceAuthResult

	if deviceToken == "" {
		deviceToken = settings.DeviceToken
	}
	if deviceToken != "" {
		logger.Info("Device link token provided, authenticating...")
		deviceResult, err := auth.AuthenticateWithDeviceToken(deviceToken, operatorEndpoint, logger, settings.User)
		if err != nil {
			logger.Error("Device link authentication failed", constants.ConnectionStateErrorStr, err)
			fmt.Fprintf(os.Stderr, "Device authentication failed: %v\n", err)
			os.Exit(constants.ExitAuthFailure)
		}
		logger.Info("Device authentication successful", "operator_id", deviceResult.OperatorID)

		// Store device result for later bootstrap config application
		deviceAuthResult = deviceResult
	}

	if deviceAuthResult != nil && deviceAuthResult.Config != nil {
		logger.Info("Applying bootstrap config from device-link registration")
		// We still need to apply other config fields (like certs) once 'cfg' is created
	}

	if logLevel == "info" {
		if settings.LogLevel != "" {
			logLevel = settings.LogLevel
		}
	}

	if apiKey == "" {
		apiKey = settings.OperatorAPIKey
	}
	if apiKey == "" {
		var err error
		apiKey, err = promptForAPIKey()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading API key: %v\n", err)
			os.Exit(constants.ExitConfigError)
		}
		if apiKey == "" {
			fmt.Fprintf(os.Stderr, "API key is required\n")
			os.Exit(constants.ExitConfigError)
		}
	}

	// Resolve the effective working directory: flag overrides launch dir.
	effectiveWorkDir := launchDir
	if workingDir != "" {
		effectiveWorkDir = workingDir
	}

	cfg, err := config.Load(config.LoadOptions{
		APIKey:           apiKey,
		OperatorEndpoint: operatorEndpoint,

		HTTPPort:            httpPort,
		CloudMode:           cloudMode,
		CloudProvider:       cloudProvider,
		LocalStorageEnabled: localStorage,
		NoGit:               noGit,
		LogLevel:            logLevel,
		WorkDir:             effectiveWorkDir,
		PKIDir:              settings.PKIDir,
		SecretsDir:          settings.SecretsDir,
		HeartbeatInterval:   heartbeatInterval,
		Shell:               settings.Shell,
		Lang:                settings.Lang,
		Term:                settings.Term,
		TZ:                  settings.TZ,
		IPService:           settings.IPService,
		IPResolver:          settings.IPResolver,
	})
	if err != nil {
		logger.Error("Failed to load configuration", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitConfigError)
	}

	cfg.Version = version

	// Apply remaining bootstrap config from device-link registration if available
	if deviceAuthResult != nil && deviceAuthResult.Config != nil {
		logger.Info("Applying remaining bootstrap config from device-link registration")
		bootstrapService, err := auth.NewBootstrapService(cfg, logger)
		if err != nil {
			logger.Error("Failed to create bootstrap service", constants.ConnectionStateErrorStr, err)
			os.Exit(constants.ExitConfigError)
		}
		if err := bootstrapService.ApplyBootstrapConfig(deviceAuthResult.Config); err != nil {
			logger.Error("Failed to apply bootstrap config", constants.ConnectionStateErrorStr, err)
			os.Exit(constants.ExitCodeFromError(err))
		}
		logger.Info("Bootstrap config applied successfully")
	}

	if cfg.CloudMode {
		logger.Info("Cloud Operator mode enabled", "provider", cfg.CloudProvider)
	}

	if cfg.LocalStoreEnabled {
		logger.Info("Local storage enabled - data stays in working directory", "db_path", cfg.LocalStoreDBPath, "working_dir", cfg.WorkDir)
	} else {
		logger.Info("Local storage disabled (command output sent to cloud)")
	}

	g8eoService, err := services.NewG8eoService(cfg, logger)
	if err != nil {
		logger.Error("Failed to create Operator service", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := g8eoService.Start(ctx); err != nil {
			logger.Error("Failed to start g8e Operator", constants.ConnectionStateErrorStr, err)
			os.Exit(constants.ExitCodeFromError(err))
		}
	}()

	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)

	if err := g8eoService.Stop(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", constants.ConnectionStateErrorStr, err)
	}
	shutdownCancel()

	os.Exit(constants.ExitSuccess)
}

func printVersion() {
	fmt.Printf("g8e Operator\n  Version:  %s\n  Build ID: %s\n  Platform: %s\n", version, buildID, platform)
}

// loadTrustBundle attempts to read a trust bundle from:
// 1. Explicit path provided via --trust-bundle
// 2. Working directory PKI path (.g8e/pki/hub-bundle.pem)
// Returns true on the first valid PEM found, which is installed via
// certs.SetCA. Returns false if no valid trust bundle is found.
func loadTrustBundle(logger *slog.Logger, explicitPath, workingDir string) bool {
	pathsToCheck := []string{}

	if explicitPath != "" {
		pathsToCheck = append(pathsToCheck, explicitPath)
	}

	if workingDir != "" {
		pkiPath := filepath.Join(workingDir, ".g8e", "pki", "hub-bundle.pem")
		pathsToCheck = append(pathsToCheck, pkiPath)
	}

	for _, path := range pathsToCheck {
		pemData, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		logger.Info("Loading trust bundle from local path", "path", path)
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			logger.Warn("CA file exists but contains invalid certificate", "path", path)
			continue
		}
		certs.SetCA(pemData)
		logger.Info("CA certificate loaded from local file")
		return true
	}
	return false
}

// configureLogger returns a slog logger configured with operator-friendly formatting
func configureLogger(level string) (*slog.Logger, error) {
	parsedLevel, err := parseLogLevel(level)
	if err != nil {
		return nil, err
	}

	handler := newOperatorHandler(os.Stdout, parsedLevel)
	logger := slog.New(handler)

	return logger, nil
}

// parseLogLevel validates and converts CLI input into slog levels
func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info":
		return slog.LevelInfo, nil
	case constants.ConnectionStateErrorStr:
		return slog.LevelError, nil
	case "debug":
		return slog.LevelDebug, nil
	default:
		return slog.LevelInfo, fmt.Errorf("supported values are: info, error, debug")
	}
}

// operatorHandler is a custom slog.Handler for operator-friendly log formatting
type operatorHandler struct {
	level  slog.Level
	output io.Writer
	attrs  []slog.Attr
	groups []string
}

func newOperatorHandler(output io.Writer, level slog.Level) *operatorHandler {
	return &operatorHandler{
		level:  level,
		output: output,
	}
}

func (h *operatorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *operatorHandler) Handle(_ context.Context, r slog.Record) error {
	timestamp := r.Time.In(time.Local).Format(time.RFC3339)
	levelStr := strings.ToUpper(r.Level.String())

	msg := fmt.Sprintf("%s %s: %s", timestamp, levelStr, r.Message)

	attrs := make([]slog.Attr, 0, r.NumAttrs()+len(h.attrs))
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	if len(attrs) > 0 {
		for _, attr := range attrs {
			msg += fmt.Sprintf("\n  - %s: %v", attr.Key, attr.Value.Any())
		}
	}

	msg += "\n"
	_, err := h.output.Write([]byte(msg))
	return err
}

func (h *operatorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &operatorHandler{
		level:  h.level,
		output: h.output,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

func (h *operatorHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)
	return &operatorHandler{
		level:  h.level,
		output: h.output,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

// runListenMode starts the Operator in listen mode - the platform's central
// persistence (operator) and pub/sub broker. In this mode, the Operator also
// runs an in-process command service to act as the sovereign execution Gateway.
func runListenMode(httpPort, bootstrapPort, publicPort int, dataDir, pkiDir, secretsDir, passkeyRpID, passkeyRpName string, logLevel string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	logger.Info("g8e Operator - Listen Mode (operator)", "version", version, "build", buildID)

	cfg, err := config.LoadListen(config.ListenOptions{
		HTTPPort:          httpPort,
		BootstrapPort:     bootstrapPort,
		PublicPort:        publicPort,
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       passkeyRpID,
		PasskeyRpName:     passkeyRpName,
		MCPDownstreamURL:  "",
		A2ADownstreamURL:  "",
		AllowTestPortZero: false,
	})
	if err != nil {
		logger.Error("Failed to load listen configuration", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitConfigError)
	}
	cfg.Version = version

	svc, err := listen.NewListenService(cfg, logger)
	if err != nil {
		logger.Error("Failed to create listen service", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize In-Process Execution Gateway
	logger.Info("Initializing in-process execution Gateway...")
	execSvc := execution.NewExecutionService(cfg, logger)
	fileSvc := execution.NewFileEditService(cfg, logger)

	// Resolve Git for ledger
	gitPath := system.ResolveGitBinary(logger)
	cfg.GitPath = gitPath
	cfg.GitAvailable = gitPath != ""

	// Use the listen-mode database for everything
	govDeps := svc.GetGovernanceDeps()
	sm := svc.GetSecretManager()

	ActuatorPriv, ActuatorKeyID, err := sm.GetActuatorKey()
	if err != nil {
		logger.Error("Failed to load Actuator signing key - mutations will fail", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitConfigError)
	}

	// Export Actuator public key for receipt verification by evals harness
	ActuatorPub := ActuatorPriv.Public().(ed25519.PublicKey)
	if err := exportActuatorPublicKey(cfg.PKIDir, ActuatorPub, ActuatorKeyID, logger); err != nil {
		logger.Error("Failed to export Actuator public key", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitConfigError)
	}

	// Loopback Pub/Sub for in-process command dispatch
	loopbackClient := pubsub.NewInProcessPubSubClient(svc.GetHTTPHandler().GetPubSubBroker())

	// Resolve the MCP gateway up-front so the pubsub command service can
	// reach it for Actuator egress dispatch on verified MCP_CALL transactions.
	mcpSvc := svc.GetHTTPHandler().GetMCPGateway()

	psConfig := pubsub.CommandServiceConfig{
		Config:             cfg,
		Logger:             logger,
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       loopbackClient,
		ResultsService:     nil, // Results handled via direct loopback publish if needed
		LocalStore:         nil, // Not used in listen mode
		RawVault:           nil, // Not used in listen mode
		AuditVault:         nil, // Handled by Actuator direct audit
		Ledger:             nil, // P1: Ledger in listen mode
		HistoryHandler:     nil, // P1: History in listen mode
		Sentinel:           sentinel.NewSentinel(services.ProductionSentinelConfig(), logger),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		L3Verifier:         govDeps.L3Verifier,
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpSvc,
	}

	cmdSvc, err := pubsub.NewPubSubCommandService(psConfig)
	if err != nil {
		logger.Error("Failed to initialize in-process command service", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	// Wire the synchronous fail-closed mutation gate into the listen HTTP
	// surface. Once set, BYO clients can POST UAP envelopes to
	// /api/governance/envelope and receive a signed ActionReceipt.
	svc.SetEnvelopeProcessor(cmdSvc)

	// Wire MCP gateway -> Gateway processor. Egress dispatch back to the
	// downstream MCP server is invoked from the Actuator via cmdSvc, so the
	// gateway only needs the Gateway processor + signing identity here.
	if mcpSvc != nil {
		mcpSvc.SetDependencies(cmdSvc, govDeps.StateRootProvider, ActuatorPriv, ActuatorKeyID, cfg.Listen.MCPDownstreamURL)
	}

	go func() {
		if err := svc.Start(ctx); err != nil {
			logger.Error("Listen service failed", constants.ConnectionStateErrorStr, err)
			os.Exit(constants.ExitCodeFromError(err))
		}
	}()

	// Start the command service once the listen service is ready
	go func() {
		for !svc.IsReady() {
			time.Sleep(100 * time.Millisecond)
			if ctx.Err() != nil {
				return
			}
		}
		logger.Info("Listen service ready, starting in-process command service")
		if err := cmdSvc.Start(ctx); err != nil {
			logger.Error("In-process command service failed to start", constants.ConnectionStateErrorStr, err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if cmdSvc != nil {
		if err := cmdSvc.Stop(); err != nil {
			logger.Error("Command service stop error", constants.ConnectionStateErrorStr, err)
		}
	}

	if err := svc.Stop(shutdownCtx); err != nil {
		logger.Error("Listen shutdown error", constants.ConnectionStateErrorStr, err)
	}
	logger.Info("Listen mode stopped")
}

// handleVaultCommand processes vault management CLI commands
func handleVaultCommand(rekeyVault, verifyVault, resetVault bool, newAPIKey, oldAPIKey, logLevel, workDir string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	dataDir := filepath.Join(workDir, ".g8e", "data")
	if s := config.LoadSettings().DataDir; s != "" {
		dataDir = s
	}

	vault, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dataDir,
		Logger:  logger,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create vault: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}
	defer vault.Close()

	switch {
	case rekeyVault:
		handleRekeyVault(vault, oldAPIKey, newAPIKey, logger)
	case verifyVault:
		handleVerifyVault(vault, newAPIKey, logger)
	case resetVault:
		handleResetVault(vault, logger)
	}
}

// handleRekeyVault re-encrypts the vault DEK with a new API key
func handleRekeyVault(vault *vault.Vault, oldAPIKey, newAPIKey string, logger *slog.Logger) {
	if oldAPIKey == "" {
		fmt.Fprintf(os.Stderr, "Error: --old-key is required for --rekey-vault\n")
		fmt.Fprintf(os.Stderr, "Usage: g8e.operator --rekey-vault --old-key <old-key> -k <new-key>\n")
		os.Exit(constants.ExitConfigError)
	}

	if newAPIKey == "" {
		newAPIKey = config.LoadSettings().OperatorAPIKey
		if newAPIKey == "" {
			fmt.Fprintf(os.Stderr, "Error: New API key is required (-k or G8E_OPERATOR_API_KEY)\n")
			os.Exit(constants.ExitConfigError)
		}
	}

	if !vault.IsInitialized() {
		fmt.Fprintf(os.Stderr, "Error: No vault found. Nothing to rekey.\n")
		os.Exit(constants.ExitConfigError)
	}

	logger.Info("Re-keying vault")

	if err := vault.Rekey(oldAPIKey, newAPIKey); err != nil {
		logger.Error("Failed to rekey vault", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault successfully rekeyed")
	os.Exit(constants.ExitSuccess)
}

// handleVerifyVault checks vault integrity
func handleVerifyVault(vault *vault.Vault, apiKey string, logger *slog.Logger) {
	if apiKey == "" {
		apiKey = config.LoadSettings().OperatorAPIKey
		if apiKey == "" {
			fmt.Fprintf(os.Stderr, "Error: API key is required for vault verification\n")
			os.Exit(constants.ExitConfigError)
		}
	}

	if !vault.IsInitialized() {
		logger.Info("Vault not initialized")
		os.Exit(constants.ExitSuccess)
	}

	logger.Info("Verifying vault integrity")

	if err := vault.VerifyIntegrity(apiKey); err != nil {
		logger.Error("Vault verification failed", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault verification passed")
	os.Exit(constants.ExitSuccess)
}

// runOpenClawMode starts the Operator as an OpenClaw Node Host.
// The Operator connects to the OpenClaw Gateway via WebSocket, advertises
// system.run and system.which, and executes shell commands on demand.
// No g8e infrastructure (g8ee, client) is required.
func runOpenClawMode(gatewayURL, token, nodeID, displayName, pathEnv, logLevel string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	cfg, err := config.LoadOpenClaw(config.OpenClawOptions{
		GatewayURL:  gatewayURL,
		Token:       token,
		NodeID:      nodeID,
		DisplayName: displayName,
		PathEnv:     pathEnv,
		LogLevel:    logLevel,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "OpenClaw configuration error: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	logger.Info("g8e Operator - OpenClaw Node Host", "version", version, "build", buildID)

	svc, err := openclaw.NewOpenClawNodeService(
		cfg.GatewayURL,
		cfg.Token,
		cfg.NodeID,
		cfg.DisplayName,
		cfg.PathEnv,
		logger,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create OpenClaw node service: %v\n", err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := svc.Start(ctx); err != nil {
			logger.Error("OpenClaw node service failed", constants.ConnectionStateErrorStr, err)
			os.Exit(constants.ExitCodeFromError(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()
	svc.Stop()
	logger.Info("OpenClaw node host stopped")
}

// handleResetVault destroys the vault (requires confirmation)
func handleResetVault(vault *vault.Vault, logger *slog.Logger) {
	if !vault.IsInitialized() {
		logger.Info("No vault found, nothing to reset")
		os.Exit(constants.ExitSuccess)
	}

	fmt.Fprint(os.Stderr, "WARNING: This will PERMANENTLY DESTROY all encrypted vault data. Type 'DESTROY' to confirm: ")

	var confirmation string
	_, _ = fmt.Fscan(os.Stdin, &confirmation)

	if confirmation != "DESTROY" {
		logger.Info("Reset cancelled")
		os.Exit(constants.ExitSuccess)
	}

	if err := vault.Reset(true); err != nil {
		logger.Error("Failed to reset vault", constants.ConnectionStateErrorStr, err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault has been reset, all encrypted data has been destroyed")
	os.Exit(constants.ExitSuccess)
}

// exportActuatorPublicKey writes the Actuator's public key to both PEM and JSON formats
// in the PKI directory for receipt verification by the evals harness.
func exportActuatorPublicKey(pkiDir string, pubKey ed25519.PublicKey, keyID string, logger *slog.Logger) error {
	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		return fmt.Errorf("create PKI directory: %w", err)
	}

	// Write PEM format
	pemPath := filepath.Join(pkiDir, "Actuator_pub.pem")
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKey,
	})
	if err := os.WriteFile(pemPath, pemData, 0600); err != nil {
		return fmt.Errorf("write Actuator_pub.pem: %w", err)
	}
	if logger != nil {
		logger.Info("Actuator public key exported", "path", pemPath, "format", "PEM")
	}

	// Write JSON format
	jsonPath := filepath.Join(pkiDir, "Actuator_pub.json")
	jsonData := map[string]string{
		"key_id":     keyID,
		"public_key": hex.EncodeToString(pubKey),
		"algorithm":  "ed25519",
	}
	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal Actuator_pub.json: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0600); err != nil {
		return fmt.Errorf("write Actuator_pub.json: %w", err)
	}
	if logger != nil {
		logger.Info("Actuator public key exported", "path", jsonPath, "format", "JSON")
	}

	return nil
}

// runMCPServe runs the MCP stdio JSON-RPC proxy to the Operator's mTLS HTTP API.
func runMCPServe(endpointURL, pkiDir, logLevel string) {
	// 1. Resolve mTLS certificates
	cliCertFile := os.Getenv("G8E_CLI_CERT")
	if cliCertFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get user home directory: %v\n", err)
			os.Exit(1)
		}
		cliCertFile = filepath.Join(home, ".g8e", "cli.crt")
	}
	cliKeyFile := os.Getenv("G8E_CLI_KEY")
	if cliKeyFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get user home directory: %v\n", err)
			os.Exit(1)
		}
		cliKeyFile = filepath.Join(home, ".g8e", "cli.key")
	}

	// 2. Resolve trust bundle
	trustBundle := os.Getenv("G8E_TRUST_BUNDLE")
	if trustBundle == "" {
		if pkiDir != "" {
			trustBundle = filepath.Join(pkiDir, "trust", "hub-bundle.pem")
		} else {
			// fallback
			trustBundle = ".g8e/pki/trust/hub-bundle.pem"
		}
	}

	// Read certificates
	cert, err := tls.LoadX509KeyPair(cliCertFile, cliKeyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load client keypair (cert: %s, key: %s): %v\n", cliCertFile, cliKeyFile, err)
		os.Exit(1)
	}

	caCert, err := os.ReadFile(trustBundle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read trust bundle %s: %v\n", trustBundle, err)
		os.Exit(1)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	// Operator URL
	operatorURL := os.Getenv("G8E_OPERATOR_URL")
	if operatorURL == "" {
		if endpointURL != "" {
			if !strings.HasPrefix(endpointURL, "http") {
				operatorURL = "https://" + endpointURL
			} else {
				operatorURL = endpointURL
			}
		} else {
			operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps)
		}
	}

	sessionID := os.Getenv("G8E_OPERATOR_SESSION_ID")

	// Read JSON-RPC from stdin line-by-line
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      interface{}     `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			sendRPCError(nil, -32700, "Parse error: "+err.Error())
			continue
		}

		switch req.Method {
		case "initialize":
			sendRPCResponse(req.ID, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "g8e-gateway",
					"version": "1.0.0",
				},
			})
		case "notifications/initialized":
			// No response needed for notifications

		case "ping":
			sendRPCResponse(req.ID, map[string]interface{}{})

		case "tools/list":
			respBytes, err := forwardToOperator(client, operatorURL, "/api/mcp/v1/tools/list", line, sessionID)
			if err != nil {
				sendRPCError(req.ID, -32603, "Failed to call Operator: "+err.Error())
			} else {
				if _, err := os.Stdout.Write(respBytes); err != nil {
					fmt.Fprintf(os.Stderr, "failed to write response: %v\n", err)
				}
				if _, err := os.Stdout.WriteString("\n"); err != nil {
					fmt.Fprintf(os.Stderr, "failed to write newline: %v\n", err)
				}
			}

		case "tools/call":
			respBytes, err := forwardToOperator(client, operatorURL, "/api/mcp/v1/tools/call", line, sessionID)
			if err != nil {
				sendRPCError(req.ID, -32603, "Failed to call Operator: "+err.Error())
			} else {
				if _, err := os.Stdout.Write(respBytes); err != nil {
					fmt.Fprintf(os.Stderr, "failed to write response: %v\n", err)
				}
				if _, err := os.Stdout.WriteString("\n"); err != nil {
					fmt.Fprintf(os.Stderr, "failed to write newline: %v\n", err)
				}
			}

		default:
			if req.ID != nil {
				sendRPCError(req.ID, -32601, "Method not found: "+req.Method)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "mcp serve stdin read error: %v\n", err)
	}
}

func sendRPCResponse(id interface{}, result interface{}) {
	res := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	b, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal RPC response: %v\n", err)
		return
	}
	if _, err := os.Stdout.Write(b); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write response: %v\n", err)
	}
	if _, err := os.Stdout.WriteString("\n"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write newline: %v\n", err)
	}
}

func sendRPCError(id interface{}, code int, message string) {
	res := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	b, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal RPC error: %v\n", err)
		return
	}
	if _, err := os.Stdout.Write(b); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write response: %v\n", err)
	}
	if _, err := os.Stdout.WriteString("\n"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write newline: %v\n", err)
	}
}

func forwardToOperator(client *http.Client, operatorURL, path string, body []byte, sessionID string) ([]byte, error) {
	req, err := http.NewRequest("POST", operatorURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("X-G8E-Authorization", "Bearer "+sessionID)
		req.Header.Set("X-G8E-CLI-Session-ID", os.Getenv("G8E_CLI_SESSION_ID"))
		req.Header.Set("X-G8E-Source-Component", "client")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}
