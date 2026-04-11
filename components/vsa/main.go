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
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/certs"
	"github.com/g8e-ai/g8e/components/vsa/cmd"
	"github.com/g8e-ai/g8e/components/vsa/config"
	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/services"
	auth "github.com/g8e-ai/g8e/components/vsa/services/auth"
	listen "github.com/g8e-ai/g8e/components/vsa/services/listen"
	openclaw "github.com/g8e-ai/g8e/components/vsa/services/openclaw"
	vault "github.com/g8e-ai/g8e/components/vsa/services/vault"
)

// Version information (set via ldflags during build)
var (
	version  = constants.Status.VersionStability.Dev
	buildID  = "unknown"
	platform = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "stream" {
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
	var operatorSessionID string
	var deviceToken string
	var endpointURL string
	var caURL string
	var workingDir string
	var cloudMode bool
	var cloudProvider string
	var localStorage bool
	var logLevel string
	var showVersion bool

	var noGit bool

	var wssPort int
	var httpPort int

	var listenMode bool
	var listenWSSPort int
	var listenHTTPPort int
	var listenDataDir string
	var listenSSLDir string
	var listenBinaryDir string
	var listenTLSCert string
	var listenTLSKey string
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
	flag.StringVar(&apiKey, "k", "", "API key")
	flag.StringVar(&operatorSessionID, "S", "", "Pre-authorized operator session ID (from device link auth)")
	flag.StringVar(&deviceToken, "D", "", "Device link token for operator deployment")
	flag.StringVar(&endpointURL, "e", "", "Endpoint (hostname or IP)")
	flag.BoolVar(&cloudMode, "c", true, "Cloud mode")
	flag.StringVar(&cloudProvider, "p", "", "Cloud provider")
	flag.BoolVar(&localStorage, "s", true, "Enable local storage (stores data in current directory)")
	flag.StringVar(&logLevel, "l", "info", "Log level")
	flag.BoolVar(&noGit, "G", false, "Disable git (ledger)")
	flag.BoolVar(&showVersion, "v", false, "Version")
	flag.IntVar(&wssPort, "wss-port", 443, "WSS port to dial on VSODB (default: 443)")
	flag.IntVar(&httpPort, "http-port", 443, "HTTPS port for auth/bootstrap via VSODB proxy (default: 443)")
	flag.StringVar(&apiKey, "key", "", "API key")
	flag.StringVar(&operatorSessionID, "operator_session", "", "Pre-authorized operator session ID (from device link auth)")
	flag.StringVar(&deviceToken, "device-token", "", "Device link token for operator deployment")
	flag.StringVar(&endpointURL, "endpoint", "", "Endpoint (hostname or IP)")
	flag.StringVar(&caURL, "ca-url", "", "Override URL for hub CA certificate fetch (default: https://<endpoint>/ssl/ca.crt)")
	flag.StringVar(&workingDir, "working-dir", "", "Working directory (default: directory operator was launched from)")
	flag.BoolVar(&cloudMode, constants.Status.OperatorType.Cloud, true, "Cloud mode")
	flag.StringVar(&cloudProvider, "provider", "", "Cloud provider")
	flag.BoolVar(&localStorage, "local-storage", true, "Enable local storage (stores data in current directory)")
	flag.StringVar(&logLevel, "log", "info", "Log level")
	flag.BoolVar(&noGit, "no-git", false, "Disable git (ledger)")
	flag.BoolVar(&showVersion, "version", false, "Version")

	flag.BoolVar(&listenMode, "listen", false, "Listen mode: platform persistence + pub/sub broker")
	flag.IntVar(&listenWSSPort, "wss-listen-port", 443, "WSS/TLS port for operator pub/sub connections (default: 443)")
	flag.IntVar(&listenHTTPPort, "http-listen-port", 443, "HTTPS port for internal g8ee/VSOD service traffic (default: 443)")
	flag.StringVar(&listenDataDir, "data-dir", "", "Data directory for SQLite database (default: .g8e/data in working directory)")
	flag.StringVar(&listenSSLDir, "ssl-dir", "", "Directory for TLS certificates (default: data-dir/ssl)")
	flag.StringVar(&listenBinaryDir, "binary-dir", "", "Directory containing operator binaries to serve (default: .g8e/bin in working directory)")
	flag.StringVar(&listenTLSCert, "tls-cert", "", "Path to TLS certificate file (optional; auto-generated when empty)")
	flag.StringVar(&listenTLSKey, "tls-key", "", "Path to TLS private key file (optional; auto-generated when empty)")
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
		fmt.Fprintf(os.Stderr, "  -S, --session <id>      Pre-authorized operator session ID (from device link auth)\n")
		fmt.Fprintf(os.Stderr, "  -D, --device-token <tok> Device link token for operator deployment\n")
		fmt.Fprintf(os.Stderr, "  -e, --endpoint <host>     Operator endpoint: IP address of the Docker host running VSODB\n")
		fmt.Fprintf(os.Stderr, "      --ca-url <url>        Override URL for hub CA certificate fetch (default: https://<endpoint>/ssl/ca.crt)\n")
		fmt.Fprintf(os.Stderr, "      --working-dir <dir>   Working directory (default: directory operator was launched from)\n")
		fmt.Fprintf(os.Stderr, "                            All commands and data storage are anchored to this directory\n")
		fmt.Fprintf(os.Stderr, "      --wss-port <port>     WSS port to dial on VSODB for pub/sub (default: 443)\n")
		fmt.Fprintf(os.Stderr, "      --http-port <port>    HTTPS port to dial for auth/bootstrap (default: 443)\n")
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
		fmt.Fprintf(os.Stderr, "  --wss-listen-port <port>    WSS/TLS port for operator pub/sub connections (default: 443)\n")
		fmt.Fprintf(os.Stderr, "  --http-listen-port <port>   HTTPS port for internal g8ee/VSOD traffic (default: 443)\n")
		fmt.Fprintf(os.Stderr, "  --data-dir <dir>            Data directory for SQLite (default: .g8e/data in working directory)\n")
		fmt.Fprintf(os.Stderr, "  --ssl-dir <dir>             Directory for TLS certificates (default: data-dir/ssl)\n")
		fmt.Fprintf(os.Stderr, "  --binary-dir <dir>          Directory containing operator binaries to serve (default: .g8e/bin in working directory)\n")
		fmt.Fprintf(os.Stderr, "  --tls-cert <path>           Path to TLS certificate (optional; auto-generated when empty)\n")
		fmt.Fprintf(os.Stderr, "  --tls-key <path>            Path to TLS private key (optional; auto-generated when empty)\n")
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
		runListenMode(listenWSSPort, listenHTTPPort, listenDataDir, listenSSLDir, listenBinaryDir, listenTLSCert, listenTLSKey, logLevel)
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

	// Try to read CA from local SSL volume first (when running in container)
	if caURL == "" {
		// Check common SSL volume mount points
		sslPaths := []string{"/ssl/ca.crt", "/vsodb/ca.crt", "/vsodb/ssl/ca.crt", "/data/ssl/ca.crt"}
		for _, path := range sslPaths {
			if pemData, err := os.ReadFile(path); err == nil {
				logger.Info("Loading CA certificate from local SSL volume", "path", path)
				// Validate it's a valid PEM certificate
				pool := x509.NewCertPool()
				if pool.AppendCertsFromPEM(pemData) {
					certs.SetCA(pemData)
					logger.Info("CA certificate loaded from local file")
					goto caLoaded
				} else {
					logger.Warn("CA file exists but contains invalid certificate", "path", path)
				}
			}
		}
		// Fallback to HTTPS fetch if no local file found
		caURL = fmt.Sprintf("https://%s/ssl/ca.crt", operatorEndpoint)
	}

	logger.Info("Fetching hub CA certificate", "url", caURL)
	if err := certs.FetchAndSetCA(context.Background(), caURL); err != nil {
		logger.Error("Failed to fetch hub CA certificate", "url", caURL, "error", err)
		fmt.Fprintf(os.Stderr, "Failed to fetch CA certificate from hub: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Ensure the platform is running: ./g8e platform start\n")
		os.Exit(constants.ExitConfigError)
	}

caLoaded:
	logger.Info("Hub CA certificate loaded")

	if operatorSessionID == "" {
		if deviceToken == "" {
			deviceToken = settings.DeviceToken
		}
		if deviceToken != "" {
			logger.Info("Device link token provided, authenticating...")
			deviceResult, err := auth.AuthenticateWithDeviceToken(deviceToken, operatorEndpoint, logger, settings.User)
			if err != nil {
				logger.Error("Device link authentication failed", "error", err)
				fmt.Fprintf(os.Stderr, "Device authentication failed: %v\n", err)
				os.Exit(constants.ExitAuthFailure)
			}
			operatorSessionID = deviceResult.OperatorSessionID
			logger.Info("Device authentication successful", "operator_id", deviceResult.OperatorID)
		}
	}

	if operatorSessionID == "" {
		operatorSessionID = settings.OperatorSessionID
	}

	if logLevel == "info" {
		if settings.LogLevel != "" {
			logLevel = settings.LogLevel
		}
	}

	var authMode string
	if operatorSessionID != "" {
		authMode = constants.Status.AuthMode.OperatorSession
	} else {
		authMode = constants.Status.AuthMode.APIKey
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
	}

	// Resolve the effective working directory: flag overrides launch dir.
	effectiveWorkDir := launchDir
	if workingDir != "" {
		effectiveWorkDir = workingDir
	}

	cfg, err := config.Load(config.LoadOptions{
		APIKey:              apiKey,
		OperatorEndpoint:    operatorEndpoint,
		WSSPort:             wssPort,
		HTTPPort:            httpPort,
		AuthMode:            authMode,
		OperatorSessionID:   operatorSessionID,
		CloudMode:           cloudMode,
		CloudProvider:       cloudProvider,
		LocalStorageEnabled: localStorage,
		NoGit:               noGit,
		LogLevel:            logLevel,
		WorkDir:             effectiveWorkDir,
		HeartbeatInterval:   heartbeatInterval,
		Shell:               settings.Shell,
		Lang:                settings.Lang,
		Term:                settings.Term,
		TZ:                  settings.TZ,
		IPService:           settings.IPService,
		IPResolver:          settings.IPResolver,
	})
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(constants.ExitConfigError)
	}

	cfg.Version = version

	if cfg.CloudMode {
		logger.Info("Cloud Operator mode enabled", "provider", cfg.CloudProvider)
	}

	if cfg.LocalStoreEnabled {
		logger.Info("Local storage enabled - data stays in working directory", "db_path", cfg.LocalStoreDBPath, "working_dir", cfg.WorkDir)
	} else {
		logger.Info("Local storage disabled (command output sent to cloud)")
	}

	vsaService, err := services.NewVSAService(cfg, logger)
	if err != nil {
		logger.Error("Failed to create Operator service", "error", err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := vsaService.Start(ctx); err != nil {
			logger.Error("Failed to start g8e Operator", "error", err)
			os.Exit(constants.ExitCodeFromError(err))
		}
	}()

	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := vsaService.Stop(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", "error", err)
	}

	os.Exit(constants.ExitSuccess)
}

func printVersion() {
	fmt.Printf("g8e Operator\n  Version:  %s\n  Build ID: %s\n  Platform: %s\n", version, buildID, platform)
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
	case "error":
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

// runListenMode starts the Operator in listen mode — the platform's central
// persistence (VSODB) and pub/sub broker. In this mode, the Operator does
// NOT execute commands, initiate outbound connections, or perform
// authentication against a remote hub. It is strictly an inbound service
// for g8ee, VSOD, and Outbound Operators.
func runListenMode(wssPort, httpPort int, dataDir, sslDir, binaryDir, tlsCertPath, tlsKeyPath string, logLevel string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	logger.Info("g8e Operator — Listen Mode (VSODB)", "version", version, "build", buildID)

	cfg, err := config.LoadListen(wssPort, httpPort, dataDir, sslDir, binaryDir, tlsCertPath, tlsKeyPath)
	if err != nil {
		logger.Error("Failed to load listen configuration", "error", err)
		os.Exit(constants.ExitConfigError)
	}
	cfg.Version = version

	svc, err := listen.NewListenService(cfg, logger)
	if err != nil {
		logger.Error("Failed to create listen service", "error", err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := svc.Start(ctx); err != nil {
			logger.Error("Listen service failed", "error", err)
			os.Exit(constants.ExitCodeFromError(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := svc.Stop(shutdownCtx); err != nil {
		logger.Error("Listen shutdown error", "error", err)
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
		logger.Error("Failed to rekey vault", "error", err)
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
		logger.Error("Vault verification failed", "error", err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault verification passed")
	os.Exit(constants.ExitSuccess)
}

// runOpenClawMode starts the Operator as an OpenClaw Node Host.
// The Operator connects to the OpenClaw Gateway via WebSocket, advertises
// system.run and system.which, and executes shell commands on demand.
// No g8e infrastructure (g8ee, VSOD) is required.
func runOpenClawMode(gatewayURL, token, nodeID, displayName, pathEnv, logLevel string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	cfg, err := config.LoadOpenClaw(gatewayURL, token, nodeID, displayName, pathEnv, logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OpenClaw configuration error: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	logger.Info("g8e Operator — OpenClaw Node Host", "version", version, "build", buildID)

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
			logger.Error("OpenClaw node service failed", "error", err)
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
	fmt.Fscan(os.Stdin, &confirmation)

	if confirmation != "DESTROY" {
		logger.Info("Reset cancelled")
		os.Exit(constants.ExitSuccess)
	}

	if err := vault.Reset(true); err != nil {
		logger.Error("Failed to reset vault", "error", err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault has been reset, all encrypted data has been destroyed")
	os.Exit(constants.ExitSuccess)
}
