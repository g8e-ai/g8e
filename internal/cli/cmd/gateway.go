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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/cli/serve"
	"github.com/g8e-ai/g8e/internal/cli/wizard"
	g8econfig "github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/network"
)

func getBinaryName() string {
	if runtime.GOOS == "windows" {
		return constants.LocalBinaryNameWindows
	}
	return constants.LocalBinaryName
}

// GatewayFlags holds all gateway CLI flag values shared by gatewayStartCmd.
// It is populated by addGatewayFlags and converted to serve.GatewayConfig
// via gatewayFlagsToServeConfig.
type GatewayFlags struct {
	Posture            string
	HTTPPort           int
	HTTPSPort          int
	DataDir            string
	PKIDir             string
	SecretsDir         string
	VaultDir           string
	VaultKeyPath       string
	PasskeyRpID        string
	PasskeyRpName      string
	PasskeyRpOrigins   []string
	RateLimitRPS       float64
	RateLimitBurst     int
	LogLevel           string
	CertIdentityMode   string
	ConsensusID        string
	ConsensusURL       string
	ConsensusBootstrap string
	MCPDownstreamURL   string
	A2ADownstreamURL   string
	PublicBaseURL      string
	AllowedOrigins     []string
}

// addGatewayFlags registers all shared gateway flags on the given cobra command,
// binding them to the provided GatewayFlags struct.
func addGatewayFlags(cmd *cobra.Command, f *GatewayFlags) {
	cmd.Flags().StringVar(&f.Posture, "posture", "doctrine", "Gateway posture: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced)")
	cmd.Flags().IntVar(&f.HTTPPort, "http-port", 0, "HTTP port for bootstrap and MCP (default: from constants.Ports.OperatorHttp)")
	cmd.Flags().IntVar(&f.HTTPSPort, "https-port", 0, "HTTPS port for mTLS API (default: from constants.Ports.OperatorHttps)")
	cmd.Flags().StringVar(&f.DataDir, "data-dir", "", fmt.Sprintf("Data directory for SQLite database (default: %s in working directory)", constants.DefaultDataDir))
	cmd.Flags().StringVar(&f.PKIDir, "pki-dir", "", fmt.Sprintf("Directory for TLS certificates (default: %s)", constants.DefaultPKIDir))
	cmd.Flags().StringVar(&f.SecretsDir, "secrets-dir", "", fmt.Sprintf("Directory for platform secrets (default: %s)", constants.DefaultSecretsDir))
	cmd.Flags().StringVar(&f.VaultDir, "vault-dir", "", fmt.Sprintf("Directory for vault data (default: %s)", constants.DefaultVaultDirDesc))
	cmd.Flags().StringVar(&f.VaultKeyPath, "vault-key", "", fmt.Sprintf("Path to vault private key (default: %s)", constants.DefaultVaultKeyDesc))
	cmd.Flags().StringVar(&f.PasskeyRpID, "passkey-rp-id", "", "RP ID for passkey operations (default: localhost)")
	cmd.Flags().StringVar(&f.PasskeyRpName, "passkey-rp-name", "", "RP Name for passkey operations (default: g8e)")
	cmd.Flags().StringArrayVar(&f.PasskeyRpOrigins, "passkey-rp-origin", nil, "Additional RP origin for passkey operations (repeatable, e.g. http://localhost:8087)")
	cmd.Flags().Float64Var(&f.RateLimitRPS, "rate-limit-rps", 0, "Gateway requests per second limit (set to 0 to disable)")
	cmd.Flags().IntVar(&f.RateLimitBurst, "rate-limit-burst", 0, "Gateway rate limit burst size")
	cmd.Flags().StringVar(&f.LogLevel, "log", "info", "Log level: info, error, debug")
	cmd.Flags().StringVar(&f.CertIdentityMode, "cert-mode", "", "Certificate mode: full (all hostnames/IPs), localhost (only localhost)")
	cmd.Flags().StringVar(&f.ConsensusID, "consensus-id", "", "ID of the TribunalPolicy for L2 consensus (required for --consensus)")
	cmd.Flags().StringVar(&f.ConsensusURL, "consensus-url", "", "URL of the Tribunal service for L2 deliberation (e.g. https://localhost:8443/consensus/v1/deliberate)")
	cmd.Flags().StringVar(&f.ConsensusBootstrap, "consensus-bootstrap", "", "Path to a JSON file that seeds a TribunalPolicy and trusted signers at startup (for deterministic demo deployments)")
	cmd.Flags().StringVar(&f.MCPDownstreamURL, "mcp-downstream-url", "", "URL of a downstream MCP server to proxy discovery and execution to (default: none)")
	cmd.Flags().StringVar(&f.A2ADownstreamURL, "a2a-downstream-url", "", "URL of a downstream A2A server to proxy execution to (default: none)")
	cmd.Flags().StringVar(&f.PublicBaseURL, "public-base-url", "", "Public base URL for approval links and host validation (e.g., https://demo.g8e.ai)")
	cmd.Flags().StringArrayVar(&f.AllowedOrigins, "cors-origin", nil, "Allowed CORS origin for cross-origin browser access (repeatable, e.g. https://lovable.dev)")
}

// resolveGatewayFlags applies environment variable overrides for vault and
// consensus settings when the corresponding CLI flags are not set.
func resolveGatewayFlags(f GatewayFlags) GatewayFlags {
	if f.VaultDir == "" {
		f.VaultDir = os.Getenv(string(constants.EnvVar.VaultDir))
	}
	if f.VaultKeyPath == "" {
		f.VaultKeyPath = os.Getenv(string(constants.EnvVar.VaultKey))
	}
	if f.ConsensusID == "" {
		f.ConsensusID = os.Getenv(string(constants.EnvVar.ConsensusID))
	}
	if f.ConsensusURL == "" {
		f.ConsensusURL = os.Getenv(string(constants.EnvVar.ConsensusURL))
	}
	if f.ConsensusBootstrap == "" {
		f.ConsensusBootstrap = os.Getenv(string(constants.EnvVar.ConsensusBootstrap))
	}
	if f.PublicBaseURL == "" {
		f.PublicBaseURL = os.Getenv(string(constants.EnvVar.PublicBaseURL))
	}
	if f.PasskeyRpID == "" {
		f.PasskeyRpID = os.Getenv(string(constants.EnvVar.PasskeyRpID))
	}
	if f.PasskeyRpName == "" {
		f.PasskeyRpName = os.Getenv(string(constants.EnvVar.PasskeyRpName))
	}
	if len(f.PasskeyRpOrigins) == 0 {
		if v := os.Getenv(string(constants.EnvVar.PasskeyRpOrigins)); v != "" {
			f.PasskeyRpOrigins = strings.Split(v, ",")
		}
	}
	if len(f.AllowedOrigins) == 0 {
		if v := os.Getenv(string(constants.EnvVar.AllowedOrigins)); v != "" {
			f.AllowedOrigins = strings.Split(v, ",")
		}
	}
	return f
}

// gatewayFlagsToServeConfig converts GatewayFlags into serve.GatewayConfig.
// This is the single conversion point between CLI flags and the foreground
// gateway config struct.
func gatewayFlagsToServeConfig(f GatewayFlags) serve.GatewayConfig {
	return serve.GatewayConfig{
		Posture:            g8econfig.GatewayPosture(f.Posture),
		HTTPPort:           f.HTTPPort,
		HTTPSPort:          f.HTTPSPort,
		DataDir:            f.DataDir,
		PKIDir:             f.PKIDir,
		SecretsDir:         f.SecretsDir,
		VaultDir:           f.VaultDir,
		VaultKeyPath:       f.VaultKeyPath,
		PasskeyRpID:        f.PasskeyRpID,
		PasskeyRpName:      f.PasskeyRpName,
		PasskeyRpOrigins:   f.PasskeyRpOrigins,
		RateLimitRPS:       f.RateLimitRPS,
		RateLimitBurst:     f.RateLimitBurst,
		LogLevel:           f.LogLevel,
		CertIdentityMode:   f.CertIdentityMode,
		ConsensusID:        f.ConsensusID,
		ConsensusURL:       f.ConsensusURL,
		ConsensusBootstrap: f.ConsensusBootstrap,
		MCPDownstreamURL:   f.MCPDownstreamURL,
		A2ADownstreamURL:   f.A2ADownstreamURL,
		PublicBaseURL:      f.PublicBaseURL,
		AllowedOrigins:     f.AllowedOrigins,
	}
}

// wizardRunner is the function signature for launching the interactive wizard.
// Tests inject a fake runner to avoid starting a real Bubble Tea program.
type wizardRunner func(wizard.Options) (wizard.Result, error)

// defaultWizardRunner calls wizard.Run with the given options.
func defaultWizardRunner(opts wizard.Options) (wizard.Result, error) {
	return wizard.Run(opts)
}

// wizardConfigFromFlags maps resolved GatewayFlags into the focused wizard.Config.
// Only wizard-owned fields are included — the wizard never sees flags it cannot edit.
func wizardConfigFromFlags(f GatewayFlags) wizard.Config {
	return wizard.Config{
		PublicBaseURL:      f.PublicBaseURL,
		CertIdentityMode:   f.CertIdentityMode,
		AllowedOrigins:     f.AllowedOrigins,
		Posture:            f.Posture,
		ConsensusID:        f.ConsensusID,
		ConsensusURL:       f.ConsensusURL,
		ConsensusBootstrap: f.ConsensusBootstrap,
		PasskeyRpID:        f.PasskeyRpID,
		PasskeyRpName:      f.PasskeyRpName,
		PasskeyRpOrigins:   f.PasskeyRpOrigins,
		MCPDownstreamURL:   f.MCPDownstreamURL,
		A2ADownstreamURL:   f.A2ADownstreamURL,
	}
}

// applyWizardConfig merges wizard-owned fields from the wizard result back into
// resolved GatewayFlags. Only fields the wizard edits are overwritten; all other
// flags (ports, directories, log level, rate limits, etc.) are preserved.
func applyWizardConfig(f GatewayFlags, wc wizard.Config) GatewayFlags {
	f.PublicBaseURL = wc.PublicBaseURL
	f.CertIdentityMode = wc.CertIdentityMode
	f.AllowedOrigins = wc.AllowedOrigins
	f.Posture = wc.Posture
	f.ConsensusID = wc.ConsensusID
	f.ConsensusURL = wc.ConsensusURL
	f.ConsensusBootstrap = wc.ConsensusBootstrap
	f.PasskeyRpID = wc.PasskeyRpID
	f.PasskeyRpName = wc.PasskeyRpName
	f.PasskeyRpOrigins = wc.PasskeyRpOrigins
	f.MCPDownstreamURL = wc.MCPDownstreamURL
	f.A2ADownstreamURL = wc.A2ADownstreamURL
	return f
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
		gatewayStopCmd(),
		gatewayStatusCmd(),
		gatewayRestartCmd(),
		gatewayLogsCmd(),
		gatewaySettingsCmd(),
		gatewayResetCmd(),
		gatewayCleanCmd(),
		gatewaySetupCmd(),
		dataCmd(),
		securityCmd(),
		tunnelCmd(),
	)

	return cmd
}

func gatewaySetupCmd() *cobra.Command {
	return gatewaySetupCmdWithConfig(defaultWizardRunner)
}

func gatewaySetupCmdWithConfig(runWizard wizardRunner) *cobra.Command {
	var flags GatewayFlags

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Run the interactive setup wizard",
		Long: `Launch the interactive onboarding wizard to configure gateway settings
such as posture, consensus, passkey, CORS, and certificate options.
The wizard guides you through each setting and produces a resolved configuration.

Any flags provided on the command line are used as initial values in the wizard.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := resolveGatewayFlags(flags)

			result, err := runWizard(wizard.Options{
				InitialConfig: wizardConfigFromFlags(resolved),
				ProgramOptions: []tea.ProgramOption{
					tea.WithInput(cmd.InOrStdin()),
					tea.WithOutput(cmd.OutOrStdout()),
				},
			})
			if err != nil {
				return fmt.Errorf("gateway: wizard: %w", err)
			}
			if result.Cancel {
				cmd.Println("Setup cancelled.")
				return nil
			}

			resolved = applyWizardConfig(resolved, result.Config)

			cmd.Println("Setup complete. Configuration:")
			cmd.Printf("  Posture:            %s\n", resolved.Posture)
			cmd.Printf("  Public Base URL:    %s\n", resolved.PublicBaseURL)
			cmd.Printf("  Cert Identity Mode: %s\n", resolved.CertIdentityMode)
			cmd.Printf("  Tribunal ID:        %s\n", resolved.ConsensusID)
			cmd.Printf("  Tribunal URL:       %s\n", resolved.ConsensusURL)
			cmd.Printf("  MCP Downstream URL: %s\n", resolved.MCPDownstreamURL)
			cmd.Printf("  A2A Downstream URL: %s\n", resolved.A2ADownstreamURL)
			cmd.Printf("  Passkey RP ID:      %s\n", resolved.PasskeyRpID)
			cmd.Printf("  Passkey RP Name:    %s\n", resolved.PasskeyRpName)
			if len(resolved.AllowedOrigins) > 0 {
				cmd.Printf("  Allowed Origins:    %s\n", strings.Join(resolved.AllowedOrigins, ", "))
			}
			if len(resolved.PasskeyRpOrigins) > 0 {
				cmd.Printf("  Passkey RP Origins: %s\n", strings.Join(resolved.PasskeyRpOrigins, ", "))
			}
			cmd.Println()
			cmd.Println("Run 'g8e gw start' to launch the gateway with these settings.")

			return nil
		},
	}

	addGatewayFlags(cmd, &flags)

	return cmd
}

func gatewayStartCmd() *cobra.Command {
	return gatewayStartCmdWithConfig(loadConfig, newFileSvc, defaultWizardRunner)
}

func gatewayStartCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func() (fs.RuntimeFileService, error),
	runWizard wizardRunner,
) *cobra.Command {
	var flags GatewayFlags
	var follow bool
	var interactive bool

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
			_, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			resolved := resolveGatewayFlags(flags)

			if interactive {
				result, err := runWizard(wizard.Options{
					InitialConfig: wizardConfigFromFlags(resolved),
					ProgramOptions: []tea.ProgramOption{
						tea.WithInput(cmd.InOrStdin()),
						tea.WithOutput(cmd.OutOrStdout()),
					},
				})
				if err != nil {
					return fmt.Errorf("gateway: wizard: %w", err)
				}
				if result.Cancel {
					cmd.Println("Onboarding cancelled.")
					return nil
				}
				resolved = applyWizardConfig(resolved, result.Config)
			}

			// Validate posture at CLI edge for clean error messages (before
			// network detection to fail fast on invalid input)
			postureObj, err := governance.ParseGovernancePosture(resolved.Posture)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidPosture, err)
			}

			// Detect and display network identity before prompting
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
			identityResult := detectIdentity(context.Background(), logger, resolved.CertIdentityMode)

			if identityResult.ShouldFallback {
				cmd.Printf("Warning: Failed to detect network identity\n")
				cmd.Println("Falling back to localhost-only mode")
			} else {
				cmd.Println(identityResult.Identity.FormatForDisplay())
				cmd.Println()
			}

			// Foreground mode: run gateway directly in the current process
			if follow {
				cmd.Println("[g8e] Starting g8e Gateway in foreground...")
				cmd.Printf("[g8e] Gateway posture: %s\n", postureObj.Description())

				// Write network identity to file if needed
				var networkIdentityFile string
				if len(identityResult.IdentityData) > 0 {
					fileSvc, err := fileSvcFactory()
					if err != nil {
						return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
					}
					pm, err := platform.NewProcessManager(fileSvc)
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
				gatewayCfg := gatewayFlagsToServeConfig(resolved)
				gatewayCfg.CertIdentityMode = identityResult.CertMode
				gatewayCfg.NetworkIdentityFile = networkIdentityFile

				// Run gateway (this blocks until shutdown)
				return serve.RunGateway(gatewayCfg, versionInfoFromCmd(cmd))
			}

			// Background mode: start gateway as a background process
			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			pm, err := platform.NewProcessManager(fileSvc)
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

			gatewayCfg := gatewayFlagsToServeConfig(resolved)
			gatewayCfg.CertIdentityMode = identityResult.CertMode
			if err := pm.StartOperator(platform.OperatorStartOptions{
				GatewayConfig: gatewayCfg,
			}); err != nil {
				return fmt.Errorf("%w: %v", constants.ErrProcessStartFailed, err)
			}

			_, pid, err = pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
			}

			externalIP := network.GetExternalInterfaceIP()
			hostname := pickHostname(identityResult.Identity)

			cmd.Printf("[g8e] Gateway started (PID: %d)\n", pid)
			cmd.Println()
			printNextSteps(cmd, postureObj, externalIP, hostname)

			return nil
		},
	}

	addGatewayFlags(cmd, &flags)
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Run gateway in foreground (Ctrl+C stops gateway)")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Launch interactive onboarding wizard")

	return cmd
}

func gatewayStopCmd() *cobra.Command {
	return gatewayStopCmdWithConfig(loadConfig, newFileSvc)
}

func gatewayStopCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func() (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the g8e Gateway",
		Long: `Stop the running g8e Gateway process by sending a termination signal to the
managed process. If the gateway is not running, this command is a no-op.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			pm, err := platform.NewProcessManager(fileSvc)
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
	return gatewayStatusCmdWithConfig(loadConfig, newFileSvc)
}

func gatewayStatusCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func() (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check Gateway health and status",
		Long: `Check whether the g8e Gateway is running by first attempting an HTTP health
check against the gateway API, then falling back to a process-manager check.
Displays the process ID and endpoint URLs when the gateway is running.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			cmd.Println("g8e Gateway Status")
			cmd.Println("========================")

			// Try HTTP check first (works for Docker/foreground/background modes)
			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			client, err := api.NewClient(fileSvc, cfg)
			if err == nil {
				respBody, err := client.Get("/api/v1/health")
				if err == nil {
					var health models.HealthResponse
					_ = json.Unmarshal(respBody, &health)
					if health.PID > 0 {
						cmd.Printf("State: RUNNING (PID: %d)\n", health.PID)
					} else {
						cmd.Println("State: RUNNING (HTTP check)")
					}
					cmd.Printf("\nEndpoints:\n")
					cmd.Printf("  Operator Bootstrap: https://%s:%d\n", network.GetExternalInterfaceIP(), constants.Ports.OperatorHttps)
					cmd.Printf("  Public API:         %s (Public browser/BYO bootstrap)\n", network.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
					cmd.Printf("  Console UI:         %s/console/ (WebAuthn/passkey dashboard)\n", network.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
					cmd.Printf("  MCP HTTP:           %s (Plain HTTP for MCP calls)\n", network.LocalhostHTTPURL(constants.Ports.OperatorHttp))
					return nil
				}
			}

			// Fallback to ProcessManager check (for background/host mode)
			pm, err := platform.NewProcessManager(fileSvc)
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
				cmd.Printf("  Public API:         %s (Public browser/BYO bootstrap)\n", network.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
				cmd.Printf("  Console UI:         %s/console/ (WebAuthn/passkey dashboard)\n", network.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
				cmd.Printf("  MCP HTTP:           %s (Plain HTTP for MCP calls)\n", network.LocalhostHTTPURL(constants.Ports.OperatorHttp))
			} else {
				cmd.Println("State: STOPPED")
			}

			return nil
		},
	}

	return cmd
}

func gatewayRestartCmd() *cobra.Command {
	return gatewayRestartCmdWithConfig(loadConfig, newFileSvc)
}

func gatewayRestartCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func() (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the g8e Gateway",
		Long: `Restart the g8e Gateway by stopping the current process and starting a new
one. The current posture is read from the persisted posture file
(.g8e/pids/operator.posture) and preserved across the restart. If the file is
missing, the gateway defaults to 'doctrine' posture.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			pm, err := platform.NewProcessManager(fileSvc)
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
				GatewayConfig: serve.GatewayConfig{
					Posture:  g8econfig.GatewayPosture(currentPosture),
					LogLevel: "info",
				},
			}); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}

			cmd.Println("g8e Gateway restarted successfully")
			postureObj, _ := governance.ParseGovernancePosture(currentPosture)
			cmd.Printf("Governance mode: %s\n", postureObj.Description())
			cmd.Printf("\nConsole UI: %s/console/ (WebAuthn/passkey dashboard)\n", network.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
			cmd.Printf("\nNext step: Run '%s auth enroll' to authenticate\n", getBinaryName())
			return nil
		},
	}

	return cmd
}

func gatewayLogsCmd() *cobra.Command {
	return gatewayLogsCmdWithConfig(loadConfig, newFileSvc)
}

func gatewayLogsCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func() (fs.RuntimeFileService, error),
) *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View Gateway logs",
		Long: `View the g8e Gateway log file. Use --follow to continuously tail the log
output (like tail -f).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			pm, err := platform.NewProcessManager(fileSvc)
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
	return gatewaySettingsCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func gatewaySettingsCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory, fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Manage Gateway settings",
		Long: `Fetch and display the current gateway platform settings from the running
Gateway over mTLS.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			client, err := clientFactory(fileSvc, cfg)
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
				reader := bufio.NewReader(cmd.InOrStdin())
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(response)
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
	return gatewayCleanCmdWithConfig(loadConfig, newFileSvc)
}

func gatewayCleanCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func() (fs.RuntimeFileService, error),
) *cobra.Command {
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
			_, err := configLoader("")
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
				reader := bufio.NewReader(cmd.InOrStdin())
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(response)
				if response != "y" && response != "Y" {
					cmd.Println("Aborted")
					return nil
				}
			}

			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			pm, err := platform.NewProcessManager(fileSvc)
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
