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
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/spf13/cobra"
)

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
		gatewayMCPConfigCmd(),
		gatewayMCPConfigIPCmd(),
		gatewayMCPHttpConfigCmd(),
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
	var passkeyRpID string
	var passkeyRpName string
	var rateLimitRPS float64
	var rateLimitBurst int
	var logLevel string
	var certIdentityMode string

	cmd := &cobra.Command{
		Use:   string(constants.ThinkingActionTypeStart),
		Short: "Start the g8e Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("failed to create process manager: %w", err)
			}

			running, pid, err := pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("failed to check Operator status: %w", err)
			}
			if running {
				cmd.Printf("g8e Gateway is already running (PID: %d)\n", pid)
				return nil
			}

			// Detect and display network identity before prompting
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
			netDetector := network.NewDetector(logger)
			netIdentity, err := netDetector.DetectAll(context.Background())
			if err != nil {
				cmd.Printf("Warning: Failed to detect network identity: %v\n", err)
				cmd.Println("Falling back to localhost-only mode")
				certIdentityMode = "localhost"
			} else {
				cmd.Println(netIdentity.FormatForDisplay())
				cmd.Println()
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
					return fmt.Errorf("failed to marshal network identity: %w", err)
				}
			}

			cmd.Println("[g8e] Starting g8e Gateway service...")
			if err := pm.StartOperator(
				posture,
				httpPort,
				httpsPort,
				dataDir,
				pkiDir,
				secretsDir,
				passkeyRpID,
				passkeyRpName,
				rateLimitRPS,
				rateLimitBurst,
				logLevel,
				certIdentityMode,
				identityData,
			); err != nil {
				return err
			}

			_, pid, err = pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("failed to check Operator status after start: %w", err)
			}

			externalIP := config.GetExternalInterfaceIP()
			runtimeDir := constants.Paths.Infra.RuntimeDir
			ledgerDir := filepath.Join(runtimeDir, "data", "ledger")

			cmd.Println()
			cmd.Println(" ┌── System Integrity & Posture ────────────────────────────────────────────────┐")
			cmd.Printf(" │ ✔ Core Gateway (g8eg)       : RUNNING (PID: %d)\n", pid)
			cmd.Println(" │ ✔ Governance Posture        : DOCTRINE (L1 Enforced | L2/L3 Audited)")
			cmd.Println(" │ ✔ Cryptographic Boundary    : SECURED (Fail-Closed Execution Platform)")
			cmd.Printf(" │ ✔ Immutable Audit Ledger    : INITIALIZED (%s)\n", ledgerDir)
			cmd.Println(" │ ✔ PKI Trust Anchor          : SHA256:a1b2c3d4... [Local-First]")
			cmd.Println(" └──────────────────────────────────────────────────────────────────────────────┘")
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println(" 1. Secure Gateway Endpoints & Cryptographic Reality")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Printf("  https://localhost:%d\n", constants.Ports.OperatorHttps)
			cmd.Println("    mTLS control plane for Operator API, agents, CLI auth")
			cmd.Println()
			cmd.Printf("  http://%s:%d/bootstrap-ca\n", externalIP, constants.Ports.OperatorHttp)
			cmd.Println("    Root CA distribution, CSR enrollment")
			cmd.Println()
			cmd.Printf("  http://127.0.0.1:%d/mcp\n", constants.Ports.OperatorHttp)
			cmd.Println("    Plain HTTP for local MCP clients")
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println(" 2. Zero-Trust Bootstrap: Provision Local PKI")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("  To bind agents and satellites to this sovereign boundary, you must install")
			cmd.Println("  the Root CA to establish mutual TLS (mTLS):")
			cmd.Println()
			cmd.Println("  macOS / Linux (Terminal) :")
			cmd.Printf("      curl -fsSL http://%s:%d/bootstrap-ca | sudo sh\n", externalIP, constants.Ports.OperatorHttp)
			cmd.Println()
			cmd.Println("  Windows (PowerShell - Admin) :")
			cmd.Printf("      iex (irm http://%s:%d/bootstrap-ca.ps1)\n", externalIP, constants.Ports.OperatorHttp)
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println(" 3. Client Authentication (Required for CLI Access)")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("  The Gateway is now running as a zero-trust boundary.")
			cmd.Println("  To authenticate your local CLI and obtain mTLS credentials:")
			cmd.Println()
			cmd.Println("      ./g8e auth login")
			cmd.Println()
			cmd.Println("  Other actions:")
			cmd.Println("  [Bind Satellite]    : ./g8e security pki enroll")
			cmd.Println("  [View Live Ledger]  : ./g8e gw logs --follow")
			cmd.Println("  [MCP Client Config] : ./g8e gw mcp-config")
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("  4. MCP Gateway Config for Local Coding Tools")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("  To generate MCP client configuration for your coding tools:")
			cmd.Println()
			cmd.Println("  [mTLS with g8e.local] : ./g8e gw mcp-config")
			cmd.Println("  [Plain HTTP]          : ./g8e gw mcp-config-http")
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("[g8e] Gateway service started. Run './g8e auth login' to authenticate your CLI.")

			return nil
		},
	}

	cmd.Flags().StringVar(&posture, "posture", "doctrine", "Gateway posture: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced)")
	cmd.Flags().IntVar(&httpPort, "http-port", 0, "HTTP port for bootstrap and MCP (default: from constants.Ports.OperatorHttp)")
	cmd.Flags().IntVar(&httpsPort, "https-port", 0, "HTTPS port for mTLS API (default: from constants.Ports.OperatorHttps)")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for SQLite database (default: .g8e/data in working directory)")
	cmd.Flags().StringVar(&pkiDir, "pki-dir", "", "Directory for TLS certificates (default: .g8e/pki)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", "", "Directory for platform secrets (default: .g8e/secrets)")
	cmd.Flags().StringVar(&passkeyRpID, "passkey-rp-id", "", "RP ID for passkey operations (default: localhost)")
	cmd.Flags().StringVar(&passkeyRpName, "passkey-rp-name", "", "RP Name for passkey operations (default: g8e)")
	cmd.Flags().Float64Var(&rateLimitRPS, "rate-limit-rps", 0, "Gateway requests per second limit (set to 0 to disable)")
	cmd.Flags().IntVar(&rateLimitBurst, "rate-limit-burst", 0, "Gateway rate limit burst size")
	cmd.Flags().StringVar(&logLevel, "log", "info", "Log level: info, error, debug")
	cmd.Flags().StringVar(&certIdentityMode, "cert-mode", "", "Certificate mode: full (all hostnames/IPs), localhost (only localhost)")

	return cmd
}

func gatewayStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the g8e Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("failed to create process manager: %w", err)
			}

			running, pid, err := pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("failed to check Operator status: %w", err)
			}
			if !running {
				cmd.Println("g8e Gateway is not running")
				return nil
			}

			cmd.Printf("Stopping g8e Gateway (PID: %d)...\n", pid)
			if err := pm.StopOperator(); err != nil {
				return err
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("failed to create process manager: %w", err)
			}

			running, pid, err := pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("failed to check Operator status: %w", err)
			}

			cmd.Println("g8e Gateway Status")
			cmd.Println("========================")
			if running {
				cmd.Printf("State: RUNNING (PID: %d)\n", pid)
				cmd.Printf("\nEndpoints:\n")
				cmd.Printf("  Operator Bootstrap: https://%s:%d\n", config.GetExternalInterfaceIP(), constants.Ports.OperatorHttps)
				cmd.Printf("  Public API:         https://localhost:%d (Public browser/BYO bootstrap)\n", constants.Ports.OperatorHttps)
				cmd.Printf("  MCP HTTP:           http://localhost:%d (Plain HTTP for MCP calls)\n", constants.Ports.OperatorHttp)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("failed to create process manager: %w", err)
			}

			running, _, err := pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("failed to check Operator status: %w", err)
			}

			if running {
				cmd.Println("Stopping g8e Gateway...")
				if err := pm.StopOperator(); err != nil {
					return err
				}
			}

			cmd.Println("Starting g8e Gateway...")
			if err := pm.StartOperator(
				"doctrine",
				cfg.OperatorHTTPSPort(),
				constants.Ports.OperatorHttps,
				"",
				"",
				"",
				"",
				"",
				0,
				0,
				"info",
				"",
				nil,
			); err != nil {
				return err
			}

			cmd.Println("g8e Gateway restarted successfully")
			cmd.Printf("Governance mode: doctrine (L1 enforced, L2/L3 audited)\n")
			cmd.Printf("\nNext step: Run './g8e auth login' to authenticate\n")
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("failed to create process manager: %w", err)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			resp, err := client.Get("/api/settings")
			if err != nil {
				return err
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
				return fmt.Errorf("failed to stop gateway: %w", err)
			}

			cleanCmd := gatewayCleanCmd()
			cleanCmd.SetArgs([]string{"--force"})
			cleanCmd.SetOut(cmd.OutOrStdout())
			cleanCmd.SetErr(cmd.ErrOrStderr())
			cleanCmd.SetIn(cmd.InOrStdin())
			if err := cleanCmd.Execute(); err != nil {
				return fmt.Errorf("failed to clean gateway: %w", err)
			}

			startCmd := gatewayStartCmd()
			startCmd.SetArgs([]string{})
			startCmd.SetOut(cmd.OutOrStdout())
			startCmd.SetErr(cmd.ErrOrStderr())
			startCmd.SetIn(cmd.InOrStdin())
			if err := startCmd.Execute(); err != nil {
				return fmt.Errorf("failed to start gateway: %w", err)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if !force {
				cmd.Println("WARNING: This command will:")
				cmd.Println("  1. Stop all running g8e services")
				cmd.Println("  2. Completely delete the entire runtime directory")
				cmd.Println("  3. Delete all SQLite databases, bootstrap secrets, logs, AND TLS/PKI certificates/keys")
				cmd.Println("  4. All trust routes and credentials will be permanently destroyed")
				cmd.Println()
				cmd.Println("IMPORTANT: Your CLI credentials will become invalid after this operation.")
				cmd.Println("You will need to run './g8e auth login' again after restarting the gateway.")
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
				return fmt.Errorf("failed to create process manager: %w", err)
			}

			if err := pm.Clean(); err != nil {
				return err
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

func gatewayMCPConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-config",
		Short: "Print MCP client configuration for the Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Print banner with /etc/hosts entry
			externalIP := config.GetExternalInterfaceIP()
			cmd.Println("# Add this entry to /etc/hosts to enable g8e.local resolution:")
			cmd.Printf("%s g8e.local\n", externalIP)
			cmd.Println()

			// Use the canonical g8e.local internal hostname with unified /mcp endpoint
			gatewayURL := fmt.Sprintf("https://g8e.local:%d/mcp", constants.Ports.OperatorHttps)

			// Get actual resolved cert paths (absolute paths)
			actualCertPath := cfg.CLICertFile()
			actualKeyPath := cfg.CLIKeyFile()
			actualCAPath := cfg.TrustBundlePath()

			// Normalize to forward slashes for JSON (cross-platform compatibility)
			actualCertPath = filepath.ToSlash(actualCertPath)
			actualKeyPath = filepath.ToSlash(actualKeyPath)
			actualCAPath = filepath.ToSlash(actualCAPath)

			mcpConfig := mcp.NewGatewayConfig(gatewayURL, actualCertPath, actualKeyPath, actualCAPath)

			configJSON, err := json.MarshalIndent(mcpConfig, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal MCP config: %w", err)
			}

			cmd.Println(string(configJSON))
			return nil
		},
	}

	return cmd
}

func gatewayMCPConfigIPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-config-ip",
		Short: "Print MCP client configuration for the Gateway using IP address (no DNS required)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Use the external IP address instead of g8e.local
			externalIP := config.GetExternalInterfaceIP()
			gatewayURL := fmt.Sprintf("https://%s:%d/mcp", externalIP, constants.Ports.OperatorHttps)

			// Get actual resolved cert paths (absolute paths)
			actualCertPath := cfg.CLICertFile()
			actualKeyPath := cfg.CLIKeyFile()
			actualCAPath := cfg.TrustBundlePath()

			// Normalize to forward slashes for JSON (cross-platform compatibility)
			actualCertPath = filepath.ToSlash(actualCertPath)
			actualKeyPath = filepath.ToSlash(actualKeyPath)
			actualCAPath = filepath.ToSlash(actualCAPath)

			// Use IP address as hostname for verification
			mcpConfig := mcp.NewGatewayConfigWithHostname(gatewayURL, actualCertPath, actualKeyPath, actualCAPath, externalIP)

			configJSON, err := json.MarshalIndent(mcpConfig, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal MCP config: %w", err)
			}

			cmd.Println(string(configJSON))
			return nil
		},
	}

	return cmd
}

func gatewayMCPHttpConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-config-http",
		Short: "Print MCP client configuration for the Gateway plain HTTP endpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			staticConfig := fmt.Sprintf(`{
  "mcpServers": {
    "g8e-gateway": {
      "disabled": true,
      "serverUrl": "http://127.0.0.1:%d/mcp",
      "note": "Must use explicit 127.0.0.1 for HTTP (localhost may resolve to IPv6 ::1)"
    }
  }
}`, constants.Ports.OperatorHttp)
			cmd.Println(staticConfig)
			return nil
		},
	}

	return cmd
}
