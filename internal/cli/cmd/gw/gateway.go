// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/docker"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/cli/wizard"
	g8econfig "github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
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
	Posture             string
	HTTPPort            int
	HTTPSPort           int
	DataDir             string
	PKIDir              string
	SecretsDir          string
	VaultDir            string
	VaultKeyPath        string
	PasskeyRpID         string
	PasskeyRpName       string
	PasskeyRpOrigins    []string
	RateLimitRPS        float64
	RateLimitBurst      int
	LogLevel            string
	CertIdentityMode    string
	ConsensusID         string
	ConsensusURL        string
	ConsensusBootstrap  string
	MCPDownstreamURL    string
	MCPDownstreamCmd    string
	MCPDownstreamArgs   string
	A2ADownstreamURL    string
	EnsembleUpstreamURL string
	PublicBaseURL       string
	AllowedOrigins      []string
	DoctrineDir         string

	PublicSpectatorEnabled            bool
	PublicSpectatorPrivateAddr        string
	PublicSpectatorPublicAddr         string
	PublicSpectatorAllowContainerBind bool
	EvalExplorerAddr                  string
	EvalExplorerRoot                  string
	PublicSpectatorTrustedProxyCIDRs  []string
}

// addGatewayFlags registers all shared gateway flags on the given cobra command,
// binding them to the provided GatewayFlags struct.
func addGatewayFlags(cmd *cobra.Command, f *GatewayFlags) {
	cmd.Flags().StringVar(&f.Posture, "posture", "doctrine", "Gateway posture: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), ratify (L1/L3 enforced, L2 audited), notary (L1/L2/L3 strictly enforced)")
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
	cmd.Flags().StringVar(&f.ConsensusID, "consensus-id", "", "ID of the ConsensusPolicy for L2 consensus (required for --consensus)")
	cmd.Flags().StringVar(&f.ConsensusURL, "consensus-url", "", "URL of the Consensus service for L2 deliberation (e.g. https://localhost:8443/consensus/v1/deliberate)")
	cmd.Flags().StringVar(&f.ConsensusBootstrap, "consensus-bootstrap", "", "Path to a JSON file that seeds a ConsensusPolicy and trusted signers at startup (for deterministic demo deployments)")
	cmd.Flags().StringVar(&f.MCPDownstreamURL, "mcp-downstream-url", "", "URL of a downstream MCP server to proxy discovery and execution to (default: none)")
	cmd.Flags().StringVar(&f.MCPDownstreamCmd, "mcp-downstream-cmd", "", "Command of a downstream MCP subprocess to proxy discovery and execution to (default: none)")
	cmd.Flags().StringVar(&f.MCPDownstreamArgs, "mcp-downstream-args", "", "Comma-separated arguments for the downstream MCP subprocess (default: none)")
	cmd.Flags().StringVar(&f.A2ADownstreamURL, "a2a-downstream-url", "", "URL of a downstream A2A server to proxy execution to (default: none)")
	cmd.Flags().StringVar(&f.EnsembleUpstreamURL, "ensemble-upstream-url", "", "HTTP URL of the g8ee ensemble for browser proxy forwarding (default: http://127.0.0.1:8000)")
	cmd.Flags().StringVar(&f.PublicBaseURL, "public-base-url", "", "Public base URL for approval links and host validation (e.g., https://demo.g8e.ai)")
	cmd.Flags().StringArrayVar(&f.AllowedOrigins, "cors-origin", nil, "Allowed CORS origin for cross-origin browser access (repeatable, e.g. https://lovable.dev)")
	cmd.Flags().StringVar(&f.DoctrineDir, "doctrine-dir", "", "Directory containing doctrine JSON files for L1 threat detection (default: hardcoded MITRE patterns only)")
	cmd.Flags().BoolVar(&f.PublicSpectatorEnabled, "public-spectator", true, "Start the in-process public mirror and evaluation explorer listeners")
	cmd.Flags().StringVar(&f.PublicSpectatorPrivateAddr, "public-spectator-private-listen", "", fmt.Sprintf("Loopback address for authenticated public mirror ingest (default: 127.0.0.1:%d)", constants.PublicSpectatorPrivatePort))
	cmd.Flags().StringVar(&f.PublicSpectatorPublicAddr, "public-spectator-public-listen", "", fmt.Sprintf("Loopback address for anonymous public mirror reads (default: 127.0.0.1:%d)", constants.PublicSpectatorPublicPort))
	cmd.Flags().BoolVar(&f.PublicSpectatorAllowContainerBind, "public-spectator-allow-container-bind", false, "Allow the public mirror and explorer listeners to bind 0.0.0.0 so a container network can reach them (set only inside a container deployment)")
	cmd.Flags().StringVar(&f.EvalExplorerAddr, "eval-explorer-listen", "", fmt.Sprintf("Loopback address for the evaluation explorer SPA (default: 127.0.0.1:%d)", constants.EvalExplorerDefaultPort))
	cmd.Flags().StringVar(&f.EvalExplorerRoot, "eval-explorer-root", "", "Directory containing the built evaluation explorer dist assets")
	cmd.Flags().StringArrayVar(&f.PublicSpectatorTrustedProxyCIDRs, "public-spectator-trusted-proxy-cidr", nil, "Trusted proxy CIDR allowed to provide exactly one CF-Connecting-IP address (repeatable)")
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
	if f.DoctrineDir == "" {
		f.DoctrineDir = os.Getenv(string(constants.EnvVar.DoctrineDir))
	}
	return f
}

// gatewayFlagsToServeConfig converts GatewayFlags into serve.GatewayConfig.
// This is the single conversion point between CLI flags and the foreground
// gateway config struct.
func gatewayFlagsToServeConfig(f GatewayFlags) serve.GatewayConfig {
	return serve.GatewayConfig{
		Posture:                           g8econfig.GatewayPosture(f.Posture),
		HTTPPort:                          f.HTTPPort,
		HTTPSPort:                         f.HTTPSPort,
		DataDir:                           f.DataDir,
		PKIDir:                            f.PKIDir,
		SecretsDir:                        f.SecretsDir,
		VaultDir:                          f.VaultDir,
		VaultKeyPath:                      f.VaultKeyPath,
		PasskeyRpID:                       f.PasskeyRpID,
		PasskeyRpName:                     f.PasskeyRpName,
		PasskeyRpOrigins:                  f.PasskeyRpOrigins,
		RateLimitRPS:                      f.RateLimitRPS,
		RateLimitBurst:                    f.RateLimitBurst,
		LogLevel:                          f.LogLevel,
		CertIdentityMode:                  f.CertIdentityMode,
		ConsensusID:                       f.ConsensusID,
		ConsensusURL:                      f.ConsensusURL,
		ConsensusBootstrap:                f.ConsensusBootstrap,
		MCPDownstreamURL:                  f.MCPDownstreamURL,
		MCPDownstreamCmd:                  f.MCPDownstreamCmd,
		MCPDownstreamArgs:                 parseDownstreamArgs(f.MCPDownstreamArgs),
		A2ADownstreamURL:                  f.A2ADownstreamURL,
		EnsembleUpstreamURL:               f.EnsembleUpstreamURL,
		PublicBaseURL:                     f.PublicBaseURL,
		AllowedOrigins:                    f.AllowedOrigins,
		DoctrineDir:                       f.DoctrineDir,
		PublicSpectatorEnabled:            f.PublicSpectatorEnabled,
		PublicSpectatorPrivateAddr:        f.PublicSpectatorPrivateAddr,
		PublicSpectatorPublicAddr:         f.PublicSpectatorPublicAddr,
		PublicSpectatorAllowContainerBind: f.PublicSpectatorAllowContainerBind,
		EvalExplorerAddr:                  f.EvalExplorerAddr,
		EvalExplorerRoot:                  f.EvalExplorerRoot,
		PublicSpectatorTrustedProxyCIDRs:  f.PublicSpectatorTrustedProxyCIDRs,
	}
}

func parseDownstreamArgs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
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

func Cmd() *cobra.Command {
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
		gatewayConnectCmd(),
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
			cmd.Printf("  Consensus ID:       %s\n", resolved.ConsensusID)
			cmd.Printf("  Consensus URL:      %s\n", resolved.ConsensusURL)
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
	return gatewayStartCmdWithConfig(shared.LoadConfig, shared.NewFileSvc, defaultWizardRunner)
}

func gatewayStartCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	runWizard wizardRunner,
) *cobra.Command {
	var flags GatewayFlags
	var follow bool
	var interactive bool
	var quiet bool

	cmd := &cobra.Command{
		Use:   string(constants.ThinkingPhaseStart),
		Short: "Start the g8e Gateway",
		Long: `Start the g8e Gateway as a background process. The gateway runs in its own
session (setsid) so Ctrl+C in the terminal does not affect it.

When --follow (-f) is used, the gateway runs in the foreground instead of the
background. Ctrl+C will stop the gateway directly.

When --cert-mode full is selected, the CLI detects network identity once, writes
it to a temporary JSON file in the runtime directory, and passes that file to
the Gateway subprocess. --cert-mode localhost continues to use loopback-only
identities, including IPv6 localhost when available.

Posture Persistence: The complete launch configuration is persisted in
.g8e/pids/operator-launch-profile.json on every successful background start. When
using 'gateway restart', the full configuration (CORS, passkey, ports, posture,
downstream routes, rate limits, doctrine, consensus, vault, cert mode, public
base URL) is read from this profile and restored. If the profile is missing or
malformed, the restart fails closed rather than falling back to default settings.
Valid posture values are 'doctrine', 'consensus', 'ratify', and 'notary'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if quiet {
				previousOut := cmd.OutOrStdout()
				defer cmd.SetOut(previousOut)
				cmd.SetOut(io.Discard)
			}
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
					fileSvc, err := fileSvcFactory("", slog.Default())
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
				return serve.RunGateway(gatewayCfg, shared.VersionInfoFromCmd(cmd))
			}

			// Background mode: start gateway as a background process
			fileSvc, err := fileSvcFactory("", slog.Default())
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
			startOpts := platform.OperatorStartOptions{GatewayConfig: gatewayCfg}
			if err := pm.StartOperator(&startOpts); err != nil {
				return fmt.Errorf("%w: %v", constants.ErrProcessStartFailed, err)
			}
			gatewayCfg = startOpts.GatewayConfig

			_, pid, err = pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
			}

			// Persist the complete validated launch profile so `gw restart`
			// can reconstruct the full configuration rather than falling back
			// to posture-only defaults. Written only after StartOperator
			// succeeds so the profile always reflects a known-good launch.
			if err := serve.WriteLaunchProfile(fileSvc, gatewayCfg); err != nil {
				if stopErr := pm.StopOperator(); stopErr != nil {
					return fmt.Errorf("%w: persist launch profile: %w; stop untracked gateway: %w", constants.ErrInternal, err, stopErr)
				}
				return fmt.Errorf("%w: persist launch profile: %w", constants.ErrInternal, err)
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
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress startup guidance")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Run gateway in foreground (Ctrl+C stops gateway)")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Launch interactive onboarding wizard")

	return cmd
}

func gatewayStopCmd() *cobra.Command {
	return gatewayStopCmdWithConfig(shared.LoadConfig, shared.NewFileSvc)
}

func gatewayStopCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
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

			fileSvc, err := fileSvcFactory("", slog.Default())
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
	return gatewayStatusCmdWithConfig(shared.LoadConfig, authcmd.DefaultAPIClientFactory, shared.NewFileSvc)
}

func gatewayStatusCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var brief bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check Gateway health and status",
		Long: `Check whether the g8e Gateway is running by first attempting an HTTP health
check against the gateway API, then falling back to a process-manager check.
Reports connected operators when the gateway is running.
Also reports the status of the Docker Compose unified stack when containers are running.
Displays the process ID and endpoint URLs when the gateway is running.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			if brief {
				return printBriefStatus(cmd, cfg, clientFactory, fileSvcFactory)
			}

			cmd.Println("g8e Gateway Status")
			cmd.Println("========================")

			// Try HTTP check first (works for Docker/foreground/background modes)
			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			cmd.Println()
			cmd.Println("Localhost Gateway")
			cmd.Println("-----------------")
			httpOK := false
			var apiClient authcmd.APIClient
			client, err := clientFactory(fileSvc, cfg)
			if err == nil {
				apiClient = client
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
					httpOK = true
				}
			}

			// Fallback to ProcessManager check (for background/host mode) when
			// the HTTP health probe did not succeed. Fail-closed on internal
			// errors, matching the pre-Docker-section behavior.
			running := httpOK
			if !httpOK {
				pm, err := platform.NewProcessManager(fileSvc)
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrInternal, err)
				}
				pmRunning, pid, err := pm.OperatorStatus()
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
				}
				if pmRunning {
					running = true
					cmd.Printf("State: RUNNING (PID: %d)\n", pid)
					cmd.Printf("\nEndpoints:\n")
					cmd.Printf("  Operator Bootstrap: https://%s:%d\n", network.GetExternalInterfaceIP(), constants.Ports.OperatorHttps)
					cmd.Printf("  Public API:         %s (Public browser/BYO bootstrap)\n", network.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
					cmd.Printf("  Console UI:         %s/console/ (WebAuthn/passkey dashboard)\n", network.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
					cmd.Printf("  MCP HTTP:           %s (Plain HTTP for MCP calls)\n", network.LocalhostHTTPURL(constants.Ports.OperatorHttp))
				} else {
					cmd.Println("State: STOPPED")
				}
			}

			// Report connected operators when the gateway is running.
			if running {
				cmd.Println()
				printConnectedOperators(cmd.OutOrStdout(), apiClient, fileSvc, cfg)
			}

			// Report the Docker Compose unified stack status if at least one
			// container is actually running.
			var dockerBuf bytes.Buffer
			if err := docker.PrintDockerStackStatus(&dockerBuf, ""); err == nil && dockerBuf.Len() > 0 {
				cmd.Println()
				cmd.Print(dockerBuf.String())
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&brief, "brief", false, "Show a compact Gateway and connected operator summary")
	return cmd
}

func printConnectedOperators(w io.Writer, client authcmd.APIClient, fileSvc fs.RuntimeFileService, cfg *config.Config) {
	fmt.Fprintln(w, "Connected Operators")
	fmt.Fprintln(w, "-------------------")

	if client == nil {
		fmt.Fprintln(w, "No connected operators")
		return
	}

	creds, _ := auth.LoadCredentials(fileSvc, cfg)
	reqPath := constants.APIPaths.Operators
	if creds != nil && creds.UserID != "" {
		reqPath += "?user_id=" + creds.UserID
	}

	resp, err := client.Get(reqPath)
	if err != nil {
		fmt.Fprintln(w, "No connected operators")
		return
	}

	var slotResp models.OperatorSlotResponse
	if err := json.Unmarshal(resp, &slotResp); err != nil {
		fmt.Fprintln(w, "No connected operators")
		return
	}

	var connected []models.OperatorDocumentGo
	for _, op := range slotResp.Operators {
		if isOperatorConnected(op) {
			connected = append(connected, op)
		}
	}

	if len(connected) == 0 {
		fmt.Fprintln(w, "No connected operators")
		return
	}

	fmt.Fprintf(w, "  %-36s  %-12s  %-24s  %-36s  %-15s\n", "ID", "Role", "Hostname", "Session ID", "Status")
	for _, op := range connected {
		sessionID := op.OperatorSessionID
		if sessionID == "" {
			sessionID = "-"
		}
		fmt.Fprintf(w, "  %-36s  %-12s  %-24s  %-36s  %-15s\n",
			op.ID,
			operatorRoleDisplay(op),
			operatorHostnameDisplay(op),
			sessionID,
			op.Status,
		)
	}
}

func isOperatorConnected(op models.OperatorDocumentGo) bool {
	if op.IsSlot && !op.Claimed {
		return false
	}
	switch op.Status {
	case constants.OperatorStatusActive, constants.OperatorStatusBound, constants.OperatorStatusStale:
		return true
	default:
		return false
	}
}

func operatorRoleDisplay(op models.OperatorDocumentGo) string {
	if op.OperatorRole != "" {
		return string(op.OperatorRole)
	}
	role := operatorcapability.GetOperatorRole(op)
	if role != "" {
		return string(role)
	}
	if op.OperatorType != "" {
		return string(op.OperatorType)
	}
	return "-"
}

func operatorHostnameDisplay(op models.OperatorDocumentGo) string {
	if op.CurrentHostname != "" {
		return op.CurrentHostname
	}
	if op.Name != "" {
		return op.Name
	}
	return "-"
}

func gatewayRestartCmd() *cobra.Command {
	return gatewayRestartCmdWithConfig(shared.LoadConfig, shared.NewFileSvc)
}

func gatewayRestartCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the g8e Gateway",
		Long: `Restart the g8e Gateway by stopping the current process and starting a new
one. The complete launch configuration is read from the persisted launch profile
(.g8e/pids/operator-launch-profile.json) and restored across the restart. If the
profile is missing, malformed, or unsupported, the restart fails closed rather
than falling back to default settings. Network identity is re-detected on every
restart.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			pm, err := platform.NewProcessManager(fileSvc)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			// Read the complete launch profile before stopping the
			// gateway. Missing, malformed, or unsupported profiles fail
			// closed — restart no longer falls back to posture-only
			// defaults.
			profile, err := serve.ReadLaunchProfile(fileSvc)
			if err != nil {
				return fmt.Errorf("gateway: read launch profile: %w", err)
			}
			previousConfig := profile.Config

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
			cmd.Printf("[g8e] Restarting with posture: %s\n", profile.Config.Posture)

			// Re-run network identity detection with the persisted cert
			// mode so the subprocess gets fresh identity state. The
			// subprocess re-detects identity in foreground; the parent
			// detection is for display and to resolve the effective mode
			// (which may fall back to "localhost" on detection failure).
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
			identityResult := detectIdentity(context.Background(), logger, profile.Config.CertIdentityMode)

			if identityResult.ShouldFallback {
				cmd.Printf("Warning: Failed to detect network identity, falling back to localhost-only mode\n")
			} else if identityResult.Identity != nil {
				cmd.Println(identityResult.Identity.FormatForDisplay())
				cmd.Println()
			}

			// Apply the resolved cert mode (may differ from the persisted
			// value if detection failed and fell back to "localhost").
			profile.Config.CertIdentityMode = identityResult.CertMode

			startOpts := platform.OperatorStartOptions{GatewayConfig: profile.Config}
			if err := pm.StartOperator(&startOpts); err != nil {
				if running {
					rollbackOpts := platform.OperatorStartOptions{GatewayConfig: previousConfig}
					if rollbackErr := pm.StartOperator(&rollbackOpts); rollbackErr != nil {
						return fmt.Errorf("%w: restart: %w; rollback: %w", constants.ErrProcessStartFailed, err, rollbackErr)
					}
				}
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			profile.Config = startOpts.GatewayConfig

			// Re-persist the profile with the resolved cert mode so the
			// next restart uses the effective configuration.
			if err := serve.WriteLaunchProfile(fileSvc, profile.Config); err != nil {
				if stopErr := pm.StopOperator(); stopErr != nil {
					return fmt.Errorf("%w: persist launch profile: %w; stop untracked gateway: %w", constants.ErrInternal, err, stopErr)
				}
				if running {
					rollbackOpts := platform.OperatorStartOptions{GatewayConfig: previousConfig}
					if rollbackErr := pm.StartOperator(&rollbackOpts); rollbackErr != nil {
						return fmt.Errorf("%w: persist launch profile: %w; rollback: %w", constants.ErrInternal, err, rollbackErr)
					}
				}
				return fmt.Errorf("%w: persist launch profile: %w", constants.ErrInternal, err)
			}

			cmd.Println("g8e Gateway restarted successfully")
			postureObj, _ := governance.ParseGovernancePosture(string(profile.Config.Posture))
			cmd.Printf("Governance mode: %s\n", postureObj.Description())
			cmd.Printf("\nConsole UI: %s/console/ (WebAuthn/passkey dashboard)\n", network.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
			if postureObj.RequiresL3Proof() {
				cmd.Printf("\nNext step: Run '%s auth enroll user' to register a passkey (required for %s posture)\n", getBinaryName(), postureObj.Name())
			} else {
				cmd.Printf("\nNext step: Run '%s auth enroll user' to authenticate (passkey optional for %s posture)\n", getBinaryName(), postureObj.Name())
			}
			return nil
		},
	}

	return cmd
}

func gatewayLogsCmd() *cobra.Command {
	return gatewayLogsCmdWithConfig(shared.LoadConfig, shared.NewFileSvc)
}

func gatewayLogsCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
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

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			pm, err := platform.NewProcessManager(fileSvc)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			logSvc := pm.LogService()
			exists, err := logSvc.LogFileExists(context.Background())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}
			if !exists {
				cmd.Printf("No log file found at %s\n", logSvc.LogFilePath())
				return nil
			}
			handle, err := logSvc.OpenLogForRead(context.Background())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}
			defer handle.Close()
			return platform.TailLog(handle, follow)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output (like tail -f)")

	return cmd
}

func gatewaySettingsCmd() *cobra.Command {
	return gatewaySettingsCmdWithConfig(shared.LoadConfig, authcmd.DefaultAPIClientFactory, shared.NewFileSvc)
}

func gatewaySettingsCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory authcmd.APIClientFactory, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
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

			fileSvc, err := fileSvcFactory("", slog.Default())
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
	var skipBackup bool

	cmd := &cobra.Command{
		Use:   string(constants.HistoryEventTypeReset),
		Short: "Reset Gateway data and secrets, then restart (.g8e is renamed to .g8e-<MMDDHHMM>)",
		Long: `Reset the g8e Gateway by stopping all services, running the same full runtime
cleanup as 'gw clean', then starting a new gateway. The runtime directory is not
deleted: it is renamed to .g8e-<MMDDHHMM> so it can be recovered.

Before proceeding, reset asks whether to back up evaluation evidence first.
Use --skip-backup to opt out and --yes/--force to skip the prompts (the backup
still runs unless --skip-backup is also given).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			proceed, err := shared.ConfirmDestructive(cmd, shared.DestructiveOptions{
				Effects: []string{
					"Stop all running g8e services",
					"Rename the runtime directory (.g8e) aside to .g8e-<MMDDHHMM>; databases, secrets, logs and TLS/PKI are no longer used",
					"Remove g8e root CA anchors from the OS trust store; CLI credentials become invalid",
					"Start a fresh gateway with a new trust domain",
				},
				AssumeYes:  force,
				SkipBackup: skipBackup,
				Backup:     shared.EvalEvidenceBackup(shared.LoadConfig, shared.NewFileSvc),
			})
			if err != nil || !proceed {
				return err
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
			// Reset already confirmed and offered the backup above.
			cleanCmd.SetArgs([]string{"--force", "--" + shared.FlagSkipBackup})
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
	shared.AddSkipBackupFlag(cmd, &skipBackup)

	return cmd
}

func gatewayCleanCmd() *cobra.Command {
	return gatewayCleanCmdWithConfig(shared.LoadConfig, shared.NewFileSvc, defaultTrustInstallerFactory)
}

// systemTrustCleaner is the subset of platform.SystemTrustInstaller used by
// `gw clean` to remove g8e root CA anchors from the OS trust store before
// wiping the runtime directory. Defining it in the cmd package lets Tier 1
// tests inject a mock without touching the real OS trust store.
type systemTrustCleaner interface {
	ListStaleAnchors(ctx context.Context, currentFingerprint string) ([]platform.StaleAnchor, error)
	RemoveStaleAnchors(ctx context.Context, anchors []platform.StaleAnchor) error
}

// defaultTrustInstallerFactory returns the production SystemTrustInstaller.
// It is the default injected into gatewayCleanCmdWithConfig; tests pass a
// mock factory instead.
func defaultTrustInstallerFactory() (systemTrustCleaner, error) {
	return platform.NewSystemTrustInstaller(), nil
}

func gatewayCleanCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	trustInstallerFactory func() (systemTrustCleaner, error),
) *cobra.Command {
	var force bool
	var skipBackup bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Destructively remove all Gateway state (.g8e is renamed to .g8e-<MMDDHHMM>)",
		Long: `Remove all g8e Gateway state: stops all services and moves the entire runtime
directory (SQLite databases, bootstrap secrets, logs, TLS/PKI certificates/keys)
out of the way. The directory is not deleted: it is renamed to
.g8e-<MMDDHHMM> (for example .g8e-09301401) so it can be recovered by hand.
Trust routes and credentials in the new runtime are gone, and CLI credentials
become invalid after this operation.

Before proceeding, clean asks whether to back up evaluation evidence first.
Use --skip-backup to opt out and --yes/--force to skip the prompts (the backup
still runs unless --skip-backup is also given).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gateway: load config: %w", err)
			}

			proceed, err := shared.ConfirmDestructive(cmd, shared.DestructiveOptions{
				Effects: []string{
					"Stop all running g8e services",
					"Rename the runtime directory (.g8e) aside to .g8e-<MMDDHHMM>; nothing is deleted, but the gateway starts over with no databases, secrets, logs, or TLS/PKI",
					"Remove g8e root CA anchors from the OS trust store",
					"CLI credentials become invalid; run './g8e auth enroll user' again after restarting the gateway",
				},
				AssumeYes:  force,
				SkipBackup: skipBackup,
				Backup:     shared.EvalEvidenceBackup(configLoader, fileSvcFactory),
			})
			if err != nil || !proceed {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			pm, err := platform.NewProcessManager(fileSvc)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			// Remove g8e root CA anchors from the OS trust store BEFORE
			// wiping the runtime directory. An empty keep-fingerprint lists
			// every g8e anchor (after clean there is no "current" one). This
			// runs before the runtime wipe so that, if OS cleanup fails with
			// an elevation error, the user can retry while the runtime state
			// is still intact. Best-effort: on ErrSystemTrustUnsupported
			// (stub platform) or any trust-store error, proceed with the
			// runtime wipe (the runtime wipe is the destructive primary
			// action). Log the error for visibility.
			trustCleaner, terr := trustInstallerFactory()
			if terr != nil {
				cmd.Println(fmt.Sprintf("Warning: could not initialize OS trust cleaner (%v). Proceeding with runtime wipe.", terr))
			} else {
				anchors, lerr := trustCleaner.ListStaleAnchors(context.Background(), "")
				if lerr != nil {
					if !errors.Is(lerr, constants.ErrSystemTrustUnsupported) {
						cmd.Println(fmt.Sprintf("Warning: could not enumerate OS trust anchors (%v). Proceeding with runtime wipe.", lerr))
					}
				} else if len(anchors) > 0 {
					cmd.Println(fmt.Sprintf("Removing %d g8e root CA anchor(s) from the OS trust store...", len(anchors)))
					if rerr := trustCleaner.RemoveStaleAnchors(context.Background(), anchors); rerr != nil {
						cmd.Println(fmt.Sprintf("Warning: could not remove all OS trust anchors (%v). Proceeding with runtime wipe. You may need to remove them manually.", rerr))
					} else {
						cmd.Println("OS trust anchors removed.")
					}
				}
			}

			archived, err := pm.Clean()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			if archived == "" {
				cmd.Println("Clean complete. No runtime directory existed.")
			} else {
				cmd.Printf("Clean complete. Previous runtime state moved to %s (delete it manually once you no longer need it).\n", archived)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&force, "y", false, "Skip confirmation prompt (shorthand)")
	cmd.Flags().BoolVar(&force, "yes", false, "Skip confirmation prompt (shorthand)")
	shared.AddSkipBackupFlag(cmd, &skipBackup)

	return cmd
}
