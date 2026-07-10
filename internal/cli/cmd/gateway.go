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

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/cli/serve"
	g8econfig "github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/netutil"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/network"
)

func getBinaryName() string {
	if runtime.GOOS == "windows" {
		return constants.LocalBinaryNameWindows
	}
	return constants.LocalBinaryName
}

// startConfig holds resolved configuration for starting the gateway.
type startConfig struct {
	VaultDir           string
	VaultKeyPath       string
	VaultRequireUnlock bool
	Posture            string
	HTTPPort           int
	HTTPSPort          int
	DataDir            string
	PKIDir             string
	SecretsDir         string
	PasskeyRpID        string
	PasskeyRpName      string
	PasskeyRpOrigins   []string
	RateLimitRPS       float64
	RateLimitBurst     int
	LogLevel           string
	CertIdentityMode   string
	IdentityData       []byte
	TribunalID         string
	TribunalURL        string
	TribunalBootstrap  string
	MCPDownstreamURL   string
	A2ADownstreamURL   string
	PublicBaseURL      string
}

// resolveStartConfig resolves environment variable overrides and defaults for gateway start.
func resolveStartConfig(cfg startConfig) startConfig {
	// Environment variables override CLI flags
	if cfg.VaultDir == "" {
		cfg.VaultDir = os.Getenv(string(constants.EnvVar.VaultDir))
	}
	if cfg.VaultKeyPath == "" {
		cfg.VaultKeyPath = os.Getenv(string(constants.EnvVar.VaultKey))
	}
	if !cfg.VaultRequireUnlock {
		cfg.VaultRequireUnlock = os.Getenv(string(constants.EnvVar.VaultRequireUnlock)) == "true"
	}

	if cfg.TribunalID == "" {
		cfg.TribunalID = os.Getenv(string(constants.EnvVar.TribunalID))
	}
	if cfg.TribunalURL == "" {
		cfg.TribunalURL = os.Getenv(string(constants.EnvVar.TribunalURL))
	}
	if cfg.TribunalBootstrap == "" {
		cfg.TribunalBootstrap = os.Getenv(string(constants.EnvVar.TribunalBootstrap))
	}

	return cfg
}

// detectIdentityResult holds the result of network identity detection.
type detectIdentityResult struct {
	Identity       *network.NetworkIdentity
	CertMode       string
	IdentityData   []byte
	ShouldFallback bool
}

// detectIdentity performs network identity detection and returns the result.
func detectIdentity(ctx context.Context, logger *slog.Logger, certIdentityMode string) detectIdentityResult {
	netDetector := network.NewDetector(logger)
	netIdentity, err := netDetector.DetectAll(ctx)
	if err != nil {
		return detectIdentityResult{
			CertMode:       "localhost",
			ShouldFallback: true,
		}
	}

	// Default to full identity mode if not specified via flag
	if certIdentityMode == "" {
		certIdentityMode = "full"
	}

	// Serialize network identity to pass to subprocess
	var identityData []byte
	if certIdentityMode == "full" && netIdentity != nil {
		identityData, err = json.Marshal(netIdentity)
		if err != nil {
			return detectIdentityResult{
				CertMode:       "localhost",
				ShouldFallback: true,
			}
		}
	}

	return detectIdentityResult{
		Identity:     netIdentity,
		CertMode:     certIdentityMode,
		IdentityData: identityData,
	}
}

func gatewayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "gw",
		Aliases: []string{"gateway"},
		Short:   "Manage the g8e Gateway (g8eg) lifecycle",
		Long:    `Gateway lifecycle commands for starting, stopping, and checking the status of the g8e Gateway.`,
	}

	cmd.AddCommand(
		gatewayStartCmd(),
		gatewayServeCmd(),
		gatewayStopCmd(),
		gatewayStatusCmd(),
		gatewayRestartCmd(),
		gatewayLogsCmd(),
		gatewaySettingsCmd(),
		gatewayResetCmd(),
		gatewayCleanCmd(),
		dataCmd(),
		securityCmd(),
	)

	return cmd
}

func gatewayStartCmd() *cobra.Command {
	var posture string
	var httpPort int
	var httpsPort int
	var dataDir string
	var pkiDir string
	var secretsDir string
	var vaultDir string
	var vaultKeyPath string
	var vaultRequireUnlock bool
	var passkeyRpID string
	var passkeyRpName string
	var passkeyRpOrigins []string
	var rateLimitRPS float64
	var rateLimitBurst int
	var logLevel string
	var certIdentityMode string
	var tribunalID string
	var tribunalURL string
	var tribunalBootstrap string
	var mcpDownstreamURL string
	var a2aDownstreamURL string
	var publicBaseURL string
	var follow bool

	cmd := &cobra.Command{
		Use:   string(constants.ThinkingActionTypeStart),
		Short: "Start the g8e Gateway",
		Long: `Start the g8e Gateway as a background process. The gateway runs in its own
session (setsid) so Ctrl+C in the terminal does not affect it.

When --follow (-f) is used, the gateway runs in the foreground instead of the
background. Ctrl+C will stop the gateway directly.

When --cert-mode full is selected, the CLI detects network identity once, writes
it to a temporary JSON file in the runtime directory, and passes that file to
the Gateway subprocess. --cert-mode localhost continues to use loopback-only
identities, including IPv6 localhost when available.

Posture Persistence: The gateway posture is persisted in
.g8e/pids/operator.posture on startup. When using 'gateway restart', the
current posture is read from this file and preserved. If the file is missing or
corrupted, the gateway defaults to 'doctrine' posture. Valid posture values are
'doctrine', 'consensus', and 'notary'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			// Resolve configuration from flags and environment variables
			startCfg := resolveStartConfig(startConfig{
				VaultDir:           vaultDir,
				VaultKeyPath:       vaultKeyPath,
				VaultRequireUnlock: vaultRequireUnlock,
				Posture:            posture,
				HTTPPort:           httpPort,
				HTTPSPort:          httpsPort,
				DataDir:            dataDir,
				PKIDir:             pkiDir,
				SecretsDir:         secretsDir,
				PasskeyRpID:        passkeyRpID,
				PasskeyRpName:      passkeyRpName,
				PasskeyRpOrigins:   passkeyRpOrigins,
				RateLimitRPS:       rateLimitRPS,
				RateLimitBurst:     rateLimitBurst,
				LogLevel:           logLevel,
				CertIdentityMode:   certIdentityMode,
				TribunalID:         tribunalID,
				TribunalURL:        tribunalURL,
				TribunalBootstrap:  tribunalBootstrap,
				MCPDownstreamURL:   mcpDownstreamURL,
				A2ADownstreamURL:   a2aDownstreamURL,
				PublicBaseURL:      publicBaseURL,
			})

			// Detect and display network identity before prompting
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
			identityResult := detectIdentity(context.Background(), logger, startCfg.CertIdentityMode)

			if identityResult.ShouldFallback {
				cmd.Printf("Warning: Failed to detect network identity\n")
				cmd.Println("Falling back to localhost-only mode")
			} else {
				cmd.Println(identityResult.Identity.FormatForDisplay())
				cmd.Println()
			}

			// Validate posture at CLI edge for clean error messages
			postureObj, err := governance.ParseGovernancePosture(startCfg.Posture)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidPosture, err)
			}

			// Foreground mode: run gateway directly in the current process
			if follow {
				cmd.Println("[g8e] Starting g8e Gateway in foreground...")
				cmd.Printf("[g8e] Gateway posture: %s\n", postureObj.Description())

				// Write network identity to file if needed
				var networkIdentityFile string
				if len(identityResult.IdentityData) > 0 {
					pm, err := platform.NewProcessManager(cfg.ProjectRoot)
					if err != nil {
						return fmt.Errorf("%w: %w", constants.ErrInternal, err)
					}
					if err := pm.CreateDirectories(); err != nil {
						return fmt.Errorf("%w: %w", constants.ErrInternal, err)
					}
					networkIdentityFile, err = pm.WriteNetworkIdentityFile(identityResult.IdentityData)
					if err != nil {
						return fmt.Errorf("%w: %w", constants.ErrInternal, err)
					}
				}

				// Build gateway config for foreground execution
				gatewayCfg := serve.GatewayConfig{
					Posture:             g8econfig.GatewayPosture(startCfg.Posture),
					HTTPPort:            startCfg.HTTPPort,
					HTTPSPort:           startCfg.HTTPSPort,
					DataDir:             startCfg.DataDir,
					PKIDir:              startCfg.PKIDir,
					SecretsDir:          startCfg.SecretsDir,
					VaultDir:            startCfg.VaultDir,
					VaultKeyPath:        startCfg.VaultKeyPath,
					VaultRequireUnlock:  startCfg.VaultRequireUnlock,
					PasskeyRpID:         startCfg.PasskeyRpID,
					PasskeyRpName:       startCfg.PasskeyRpName,
					PasskeyRpOrigins:    startCfg.PasskeyRpOrigins,
					RateLimitRPS:        startCfg.RateLimitRPS,
					RateLimitBurst:      startCfg.RateLimitBurst,
					LogLevel:            startCfg.LogLevel,
					CertIdentityMode:    identityResult.CertMode,
					NetworkIdentityFile: networkIdentityFile,
					TribunalID:          startCfg.TribunalID,
					TribunalURL:         startCfg.TribunalURL,
					TribunalBootstrap:   startCfg.TribunalBootstrap,
					MCPDownstreamURL:    startCfg.MCPDownstreamURL,
					A2ADownstreamURL:    startCfg.A2ADownstreamURL,
					PublicBaseURL:       startCfg.PublicBaseURL,
				}

				// Run gateway (this blocks until shutdown)
				return serve.RunGateway(gatewayCfg, versionInfoFromCmd(cmd))
			}

			// Background mode: start gateway as a background process
			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			running, pid, err := pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
			}
			if running {
				cmd.Printf("g8e Gateway is already running (PID: %d)\n", pid)
				return nil
			}

			cmd.Println("[g8e] Starting g8e Gateway service...")
			cmd.Printf("[g8e] Gateway posture: %s\n", postureObj.Description())
			if err := pm.StartOperator(platform.OperatorStartOptions{
				Posture:            startCfg.Posture,
				HTTPPort:           startCfg.HTTPPort,
				HTTPSPort:          startCfg.HTTPSPort,
				DataDir:            startCfg.DataDir,
				PKIDir:             startCfg.PKIDir,
				SecretsDir:         startCfg.SecretsDir,
				VaultDir:           startCfg.VaultDir,
				VaultKeyPath:       startCfg.VaultKeyPath,
				VaultRequireUnlock: startCfg.VaultRequireUnlock,
				PasskeyRpID:        startCfg.PasskeyRpID,
				PasskeyRpName:      startCfg.PasskeyRpName,
				PasskeyRpOrigins:   startCfg.PasskeyRpOrigins,
				RateLimitRPS:       startCfg.RateLimitRPS,
				RateLimitBurst:     startCfg.RateLimitBurst,
				LogLevel:           startCfg.LogLevel,
				CertIdentityMode:   identityResult.CertMode,
				IdentityData:       identityResult.IdentityData,
				TribunalID:         startCfg.TribunalID,
				TribunalURL:        startCfg.TribunalURL,
				TribunalBootstrap:  startCfg.TribunalBootstrap,
				MCPDownstreamURL:   startCfg.MCPDownstreamURL,
				A2ADownstreamURL:   startCfg.A2ADownstreamURL,
				PublicBaseURL:      startCfg.PublicBaseURL,
			}); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}

			_, pid, err = pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
			}

			externalIP := network.GetExternalInterfaceIP()

			cmd.Printf("[g8e] Gateway started (PID: %d)\n", pid)
			cmd.Println()
			printNextSteps(cmd, postureObj, externalIP)

			return nil
		},
	}

	cmd.Flags().StringVar(&posture, "posture", "doctrine", "Gateway posture: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced)")
	cmd.Flags().IntVar(&httpPort, "http-port", 0, "HTTP port for bootstrap and MCP (default: from constants.Ports.OperatorHttp)")
	cmd.Flags().IntVar(&httpsPort, "https-port", 0, "HTTPS port for mTLS API (default: from constants.Ports.OperatorHttps)")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", fmt.Sprintf("Data directory for SQLite database (default: %s in working directory)", constants.DefaultDataDir))
	cmd.Flags().StringVar(&pkiDir, "pki-dir", "", fmt.Sprintf("Directory for TLS certificates (default: %s)", constants.DefaultPKIDir))
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", "", fmt.Sprintf("Directory for platform secrets (default: %s)", constants.DefaultSecretsDir))
	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", fmt.Sprintf("Directory for vault data (default: %s)", constants.DefaultVaultDirDesc))
	cmd.Flags().StringVar(&vaultKeyPath, "vault-key", "", fmt.Sprintf("Path to vault private key (default: %s)", constants.DefaultVaultKeyDesc))
	cmd.Flags().BoolVar(&vaultRequireUnlock, "vault-require-unlock", false, "Require vault to be unlocked at startup (fail if vault cannot be unlocked)")
	cmd.Flags().StringVar(&passkeyRpID, "passkey-rp-id", "", "RP ID for passkey operations (default: localhost)")
	cmd.Flags().StringVar(&passkeyRpName, "passkey-rp-name", "", "RP Name for passkey operations (default: g8e)")
	cmd.Flags().StringArrayVar(&passkeyRpOrigins, "passkey-rp-origin", nil, "Additional RP origin for passkey operations (repeatable, e.g. http://localhost:8087)")
	cmd.Flags().Float64Var(&rateLimitRPS, "rate-limit-rps", 0, "Gateway requests per second limit (set to 0 to disable)")
	cmd.Flags().IntVar(&rateLimitBurst, "rate-limit-burst", 0, "Gateway rate limit burst size")
	cmd.Flags().StringVar(&logLevel, "log", "info", "Log level: info, error, debug")
	cmd.Flags().StringVar(&certIdentityMode, "cert-mode", "", "Certificate mode: full (all hostnames/IPs), localhost (only localhost)")
	cmd.Flags().StringVar(&tribunalID, "tribunal-id", "", "ID of the TribunalPolicy for L2 consensus (required for --consensus)")
	cmd.Flags().StringVar(&tribunalURL, "tribunal-url", "", "URL of the Tribunal service for L2 deliberation (e.g. https://localhost:8443/tribunal/v1/deliberate)")
	cmd.Flags().StringVar(&tribunalBootstrap, "tribunal-bootstrap", "", "Path to a JSON file that seeds a TribunalPolicy and trusted signers at startup (for deterministic demo deployments)")
	cmd.Flags().StringVar(&mcpDownstreamURL, "mcp-downstream-url", "", "URL of a downstream MCP server to proxy discovery and execution to (default: none)")
	cmd.Flags().StringVar(&a2aDownstreamURL, "a2a-downstream-url", "", "URL of a downstream A2A server to proxy execution to (default: none)")
	cmd.Flags().StringVar(&publicBaseURL, "public-base-url", "", "Public base URL for approval links and host validation (e.g., https://demo.g8e.ai)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Run gateway in foreground (Ctrl+C stops gateway)")

	return cmd
}

func gatewayServeCmd() *cobra.Command {
	var posture string
	var httpPort int
	var httpsPort int
	var dataDir string
	var pkiDir string
	var secretsDir string
	var vaultDir string
	var vaultKeyPath string
	var vaultRequireUnlock bool
	var passkeyRpID string
	var passkeyRpName string
	var passkeyRpOrigins []string
	var rateLimitRPS float64
	var rateLimitBurst int
	var logLevel string
	var certIdentityMode string
	var networkIdentityFile string
	var tribunalID string
	var tribunalURL string
	var tribunalBootstrap string
	var mcpDownstreamURL string
	var a2aDownstreamURL string
	var publicBaseURL string

	cmd := &cobra.Command{
		Use:    "serve",
		Short:  "Run the g8e Gateway in foreground (worker mode)",
		Long:   `Run the g8e Gateway in foreground as a worker. This is the re-exec target for 'gw start' and can also be run directly for debugging or Docker environments.`,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate posture
			if _, err := governance.ParseGovernancePosture(posture); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidPosture, err)
			}

			// Build gateway config
			cfg := serve.GatewayConfig{
				Posture:             g8econfig.GatewayPosture(posture),
				HTTPPort:            httpPort,
				HTTPSPort:           httpsPort,
				DataDir:             dataDir,
				PKIDir:              pkiDir,
				SecretsDir:          secretsDir,
				VaultDir:            vaultDir,
				VaultKeyPath:        vaultKeyPath,
				VaultRequireUnlock:  vaultRequireUnlock,
				PasskeyRpID:         passkeyRpID,
				PasskeyRpName:       passkeyRpName,
				PasskeyRpOrigins:    passkeyRpOrigins,
				RateLimitRPS:        rateLimitRPS,
				RateLimitBurst:      rateLimitBurst,
				LogLevel:            logLevel,
				CertIdentityMode:    certIdentityMode,
				NetworkIdentityFile: networkIdentityFile,
				TribunalID:          tribunalID,
				TribunalURL:         tribunalURL,
				TribunalBootstrap:   tribunalBootstrap,
				MCPDownstreamURL:    mcpDownstreamURL,
				A2ADownstreamURL:    a2aDownstreamURL,
				PublicBaseURL:       publicBaseURL,
			}

			// Run gateway (this blocks until shutdown)
			return serve.RunGateway(cfg, versionInfoFromCmd(cmd))
		},
	}

	cmd.Flags().StringVar(&posture, "posture", "doctrine", "Gateway posture: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced)")
	cmd.Flags().IntVar(&httpPort, "http-port", 0, "HTTP port for bootstrap and MCP (default: from constants.Ports.OperatorHttp)")
	cmd.Flags().IntVar(&httpsPort, "https-port", 0, "HTTPS port for mTLS API (default: from constants.Ports.OperatorHttps)")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", fmt.Sprintf("Data directory for SQLite database (default: %s in working directory)", constants.DefaultDataDir))
	cmd.Flags().StringVar(&pkiDir, "pki-dir", "", fmt.Sprintf("Directory for TLS certificates (default: %s)", constants.DefaultPKIDir))
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", "", fmt.Sprintf("Directory for platform secrets (default: %s)", constants.DefaultSecretsDir))
	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", fmt.Sprintf("Directory for vault data (default: %s)", constants.DefaultVaultDirDesc))
	cmd.Flags().StringVar(&vaultKeyPath, "vault-key", "", fmt.Sprintf("Path to vault private key (default: %s)", constants.DefaultVaultKeyDesc))
	cmd.Flags().BoolVar(&vaultRequireUnlock, "vault-require-unlock", false, "Require vault to be unlocked at startup (fail if vault cannot be unlocked)")
	cmd.Flags().StringVar(&passkeyRpID, "passkey-rp-id", "", "RP ID for passkey operations (default: localhost)")
	cmd.Flags().StringVar(&passkeyRpName, "passkey-rp-name", "", "RP Name for passkey operations (default: g8e)")
	cmd.Flags().StringArrayVar(&passkeyRpOrigins, "passkey-rp-origin", nil, "Additional RP origin for passkey operations (repeatable, e.g. http://localhost:8087)")
	cmd.Flags().Float64Var(&rateLimitRPS, "rate-limit-rps", 0, "Gateway requests per second limit (set to 0 to disable)")
	cmd.Flags().IntVar(&rateLimitBurst, "rate-limit-burst", 0, "Gateway rate limit burst size")
	cmd.Flags().StringVar(&logLevel, "log", "info", "Log level: info, error, debug")
	cmd.Flags().StringVar(&certIdentityMode, "cert-mode", "", "Certificate mode: full (all hostnames/IPs), localhost (only localhost)")
	cmd.Flags().StringVar(&networkIdentityFile, "network-identity-file", "", "Path to network identity JSON file (for cert mode)")
	cmd.Flags().StringVar(&tribunalID, "tribunal-id", "", "ID of the TribunalPolicy for L2 consensus (required for --consensus)")
	cmd.Flags().StringVar(&tribunalURL, "tribunal-url", "", "URL of the Tribunal service for L2 deliberation (e.g. https://localhost:8443/tribunal/v1/deliberate)")
	cmd.Flags().StringVar(&tribunalBootstrap, "tribunal-bootstrap", "", "Path to a JSON file that seeds a TribunalPolicy and trusted signers at startup (for deterministic demo deployments)")
	cmd.Flags().StringVar(&mcpDownstreamURL, "mcp-downstream-url", "", "URL of a downstream MCP server to proxy discovery and execution to (default: none)")
	cmd.Flags().StringVar(&a2aDownstreamURL, "a2a-downstream-url", "", "URL of a downstream A2A server to proxy execution to (default: none)")
	cmd.Flags().StringVar(&publicBaseURL, "public-base-url", "", "Public base URL for approval links and host validation (e.g., https://demo.g8e.ai)")

	return cmd
}

func gatewayStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the g8e Gateway",
		Long: `Stop the running g8e Gateway process by sending a termination signal to the
managed process. If the gateway is not running, this command is a no-op.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			running, pid, err := pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
			}
			if !running {
				cmd.Println("g8e Gateway is not running")
				return nil
			}

			cmd.Printf("Stopping g8e Gateway (PID: %d)...\n", pid)
			if err := pm.StopOperator(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
			}

			cmd.Println("g8e Gateway stopped successfully")
			return nil
		},
	}
	return cmd
}

func gatewayStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check Gateway health and status",
		Long: `Check whether the g8e Gateway is running by first attempting an HTTP health
check against the gateway API, then falling back to a process-manager check.
Displays the process ID and endpoint URLs when the gateway is running.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			cmd.Println("g8e Gateway Status")
			cmd.Println("========================")

			// Try HTTP check first (works for Docker/foreground/background modes)
			client, err := api.NewClient(cfg)
			if err == nil {
				_, err = client.Get("/api/v1/health")
				if err == nil {
					cmd.Println("State: RUNNING (HTTP check)")
					cmd.Printf("\nEndpoints:\n")
					cmd.Printf("  Operator Bootstrap: https://%s:%d\n", network.GetExternalInterfaceIP(), constants.Ports.OperatorHttps)
					cmd.Printf("  Public API:         %s (Public browser/BYO bootstrap)\n", netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
					cmd.Printf("  Console UI:         %s/console/ (WebAuthn/passkey dashboard)\n", netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
					cmd.Printf("  MCP HTTP:           %s (Plain HTTP for MCP calls)\n", netutil.LocalhostHTTPURL(constants.Ports.OperatorHttp))
					return nil
				}
			}

			// Fallback to ProcessManager check (for background/host mode)
			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			running, pid, err := pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
			}

			if running {
				cmd.Printf("State: RUNNING (PID: %d)\n", pid)
				cmd.Printf("\nEndpoints:\n")
				cmd.Printf("  Operator Bootstrap: https://%s:%d\n", network.GetExternalInterfaceIP(), constants.Ports.OperatorHttps)
				cmd.Printf("  Public API:         %s (Public browser/BYO bootstrap)\n", netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
				cmd.Printf("  Console UI:         %s/console/ (WebAuthn/passkey dashboard)\n", netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
				cmd.Printf("  MCP HTTP:           %s (Plain HTTP for MCP calls)\n", netutil.LocalhostHTTPURL(constants.Ports.OperatorHttp))
			} else {
				cmd.Println("State: STOPPED")
			}

			return nil
		},
	}

	return cmd
}

func gatewayRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the g8e Gateway",
		Long: `Restart the g8e Gateway by stopping the current process and starting a new
one. The current posture is read from the persisted posture file
(.g8e/pids/operator.posture) and preserved across the restart. If the file is
missing, the gateway defaults to 'doctrine' posture.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			running, _, err := pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
			}

			if running {
				cmd.Println("Stopping g8e Gateway...")
				if err := pm.StopOperator(); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
				}
			}

			cmd.Println("Starting g8e Gateway...")
			currentPosture, err := pm.ReadPosture()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPostureReadFailed, err)
			}
			if currentPosture == "" {
				currentPosture = "doctrine"
				cmd.Println("[g8e] Warning: No posture file found, restarting with default 'doctrine' posture.")
			} else {
				cmd.Printf("[g8e] Restarting with current posture: %s\n", currentPosture)
			}
			if err := pm.StartOperator(platform.OperatorStartOptions{
				Posture:            currentPosture,
				HTTPPort:           cfg.OperatorHTTPSPort(),
				HTTPSPort:          constants.Ports.OperatorHttps,
				DataDir:            "",
				PKIDir:             "",
				SecretsDir:         "",
				VaultDir:           "",
				VaultKeyPath:       "",
				VaultRequireUnlock: false,
				PasskeyRpID:        "",
				PasskeyRpName:      "",
				RateLimitRPS:       0,
				RateLimitBurst:     0,
				LogLevel:           "info",
				CertIdentityMode:   "",
				IdentityData:       nil,
			}); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}

			cmd.Println("g8e Gateway restarted successfully")
			postureObj, _ := governance.ParseGovernancePosture(currentPosture)
			cmd.Printf("Governance mode: %s\n", postureObj.Description())
			cmd.Printf("\nConsole UI: %s/console/ (WebAuthn/passkey dashboard)\n", netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
			cmd.Printf("\nNext step: Run '%s auth enroll' to authenticate\n", getBinaryName())
			return nil
		},
	}

	return cmd
}

func gatewayLogsCmd() *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View Gateway logs",
		Long: `View the g8e Gateway log file. Use --follow to continuously tail the log
output (like tail -f).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			logPath := pm.GetLogPath()
			if _, err := os.Stat(logPath); os.IsNotExist(err) {
				cmd.Printf("No log file found at %s\n", logPath)
				return nil
			}

			return platform.TailLog(logPath, follow)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output (like tail -f)")

	return cmd
}

func gatewaySettingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Manage Gateway settings",
		Long: `Fetch and display the current gateway platform settings from the running
Gateway over mTLS.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			resp, err := client.Get("/api/settings")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
			}

			cmd.Println(string(resp))
			return nil
		},
	}
	return cmd
}

func gatewayResetCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   string(constants.HistoryEventTypeReset),
		Short: "Reset Gateway data and secrets (preserves CA)",
		Long: `Reset the g8e Gateway by stopping all services, wiping SQLite databases and
bootstrap secrets, then restarting with a fresh database. Existing TLS/PKI
certificates and keys are preserved. Use --force to skip the confirmation prompt.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				cmd.Println("This command will:")
				cmd.Println("  1. Stop all running g8e services")
				cmd.Println("  2. Wipe the SQLite databases and bootstrap secrets")
				cmd.Println("  3. Preserve your existing TLS/PKI certificates and keys")
				cmd.Println("  4. Restart the services with a fresh database")
				cmd.Print("\nContinue? [y/N]: ")
				var response string
				_, _ = fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					cmd.Println("Aborted")
					return nil
				}
			}

			stopCmd := gatewayStopCmd()
			stopCmd.SetArgs([]string{})
			stopCmd.SetOut(cmd.OutOrStdout())
			stopCmd.SetErr(cmd.ErrOrStderr())
			stopCmd.SetIn(cmd.InOrStdin())
			if err := stopCmd.Execute(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
			}

			cleanCmd := gatewayCleanCmd()
			cleanCmd.SetArgs([]string{"--force"})
			cleanCmd.SetOut(cmd.OutOrStdout())
			cleanCmd.SetErr(cmd.ErrOrStderr())
			cleanCmd.SetIn(cmd.InOrStdin())
			if err := cleanCmd.Execute(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			startCmd := gatewayStartCmd()
			startCmd.SetArgs([]string{})
			startCmd.SetOut(cmd.OutOrStdout())
			startCmd.SetErr(cmd.ErrOrStderr())
			startCmd.SetIn(cmd.InOrStdin())
			if err := startCmd.Execute(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&force, "y", false, "Skip confirmation prompt (shorthand)")
	cmd.Flags().BoolVar(&force, "yes", false, "Skip confirmation prompt (shorthand)")

	return cmd
}

func gatewayCleanCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Destructively remove all Gateway state",
		Long: `Destructively remove all g8e Gateway state: stops all services, completely
deletes the entire runtime directory including all SQLite databases, bootstrap
secrets, logs, and TLS/PKI certificates/keys. All trust routes and credentials
are permanently destroyed. CLI credentials become invalid after this operation.
Use --force to skip the confirmation prompt.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			if !force {
				cmd.Println("WARNING: This command will:")
				cmd.Println("  1. Stop all running g8e services")
				cmd.Println("  2. Completely delete the entire runtime directory")
				cmd.Println("  3. Delete all SQLite databases, bootstrap secrets, logs, AND TLS/PKI certificates/keys")
				cmd.Println("  4. All trust routes and credentials will be permanently destroyed")
				cmd.Println()
				cmd.Println("IMPORTANT: Your CLI credentials will become invalid after this operation.")
				cmd.Println("You will need to run './g8e auth enroll' again after restarting the gateway.")
				cmd.Print("\nContinue? [y/N]: ")
				var response string
				_, _ = fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					cmd.Println("Aborted")
					return nil
				}
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			if err := pm.Clean(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			cmd.Println("Clean complete. All runtime state and credentials destroyed.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&force, "y", false, "Skip confirmation prompt (shorthand)")
	cmd.Flags().BoolVar(&force, "yes", false, "Skip confirmation prompt (shorthand)")

	return cmd
}
