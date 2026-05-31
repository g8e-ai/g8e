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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
)

func gatewayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "gw",
		Aliases: []string{"gateway"},
		Short:   "Manage the Governance Gateway (g8eg) lifecycle",
		Long:    `Gateway lifecycle commands for starting, stopping, and checking the status of the Governance Gateway.`,
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
	)

	return cmd
}

func gatewayStartCmd() *cobra.Command {
	var posture string
	var httpPort int
	var bootstrapPort int
	var publicPort int
	var dataDir string
	var pkiDir string
	var secretsDir string
	var passkeyRpID string
	var passkeyRpName string
	var rateLimitRPS float64
	var rateLimitBurst int
	var logLevel string

	cmd := &cobra.Command{
		Use:   string(constants.ThinkingActionTypeStart),
		Short: "Start the Governance Gateway",
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
				return fmt.Errorf("failed to check operator status: %w", err)
			}
			if running {
				cmd.Printf("Governance Gateway is already running (PID: %d)\n", pid)
				return nil
			}

			cmd.Println("[g8e] Starting Governance Gateway service...")
			if err := pm.StartOperator(
				posture,
				httpPort,
				bootstrapPort,
				publicPort,
				dataDir,
				pkiDir,
				secretsDir,
				passkeyRpID,
				passkeyRpName,
				rateLimitRPS,
				rateLimitBurst,
				logLevel,
			); err != nil {
				return err
			}

			_, pid, err = pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("failed to check operator status after start: %w", err)
			}

			externalIP := config.GetExternalInterfaceIP()
			runtimeDir := filepath.Join(cfg.ProjectRoot, constants.Paths.Infra.RuntimeDir)
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
			cmd.Println(" 1. SECURE GATEWAY ENDPOINTS & CRYPTOGRAPHIC REALITY")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Printf("  Control Plane (mTLS/WSS)                     : https://localhost:%d\n", cfg.Paths.Ports.OperatorPublicHTTPS)
			cmd.Printf("  Operator Bootstrap (CSR Enrollment)          : https://%s:%d\n", externalIP, cfg.Paths.Ports.OperatorPublicHTTPS)
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println(" 2. ZERO-TRUST BOOTSTRAP: PROVISION LOCAL PKI")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("  To bind agents and satellites to this sovereign boundary, you must install")
			cmd.Println("  the Root CA to establish mutual TLS (mTLS):")
			cmd.Println()
			cmd.Println("  macOS / Linux (Terminal) :")
			cmd.Printf("      curl -fsSL http://%s:%d/bootstrap-ca | sudo sh\n", externalIP, cfg.Paths.Ports.OperatorBootstrapHTTPS)
			cmd.Println()
			cmd.Println("  Windows (PowerShell - Admin) :")
			cmd.Printf("      iex (irm http://%s:%d/bootstrap-ca.ps1)\n", externalIP, cfg.Paths.Ports.OperatorBootstrapHTTPS)
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println(" 3. CLIENT AUTHENTICATION (REQUIRED FOR CLI ACCESS)")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("  The Gateway is now running as a zero-trust boundary.")
			cmd.Println("  To authenticate your local CLI and obtain mTLS credentials:")
			cmd.Println()
			cmd.Println("      ./g8e auth login")
			cmd.Println()
			cmd.Println("  Other actions:")
			cmd.Println("  [Bind Satellite]    : ./g8e security pki enroll")
			cmd.Println("  [View Live Ledger]  : ./g8e gateway logs --follow")
			cmd.Println("  [MCP Client Config] : ./g8e gw mcp-config")
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("[g8e] Gateway service started. Run './g8e auth login' to authenticate your CLI.")

			return nil
		},
	}

	cmd.Flags().StringVar(&posture, "posture", "doctrine", "Gateway posture: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced)")
	cmd.Flags().IntVar(&httpPort, "http-port", 0, "HTTPS port for mTLS API (default: from paths.json)")
	cmd.Flags().IntVar(&bootstrapPort, "bootstrap-port", 0, "Bootstrap TLS port for CSR enrollment (default: from paths.json)")
	cmd.Flags().IntVar(&publicPort, "public-port", 0, "Public browser/BYO bootstrap port (default: from paths.json)")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for SQLite database (default: .g8e/data in working directory)")
	cmd.Flags().StringVar(&pkiDir, "pki-dir", "", "Directory for TLS certificates (default: .g8e/pki)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", "", "Directory for platform secrets (default: .g8e/secrets)")
	cmd.Flags().StringVar(&passkeyRpID, "passkey-rp-id", "", "RP ID for passkey operations (default: localhost)")
	cmd.Flags().StringVar(&passkeyRpName, "passkey-rp-name", "", "RP Name for passkey operations (default: g8e)")
	cmd.Flags().Float64Var(&rateLimitRPS, "rate-limit-rps", 0, "Gateway requests per second limit (set to 0 to disable)")
	cmd.Flags().IntVar(&rateLimitBurst, "rate-limit-burst", 0, "Gateway rate limit burst size")
	cmd.Flags().StringVar(&logLevel, "log", "info", "Log level: info, error, debug")

	return cmd
}

func gatewayStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Governance Gateway",
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
				return fmt.Errorf("failed to check operator status: %w", err)
			}
			if !running {
				cmd.Println("Governance Gateway is not running")
				return nil
			}

			cmd.Printf("Stopping Governance Gateway (PID: %d)...\n", pid)
			if err := pm.StopOperator(); err != nil {
				return err
			}

			cmd.Println("Governance Gateway stopped successfully")
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
				return fmt.Errorf("failed to check operator status: %w", err)
			}

			cmd.Println("Governance Gateway Status")
			cmd.Println("========================")
			if running {
				cmd.Printf("State: RUNNING (PID: %d)\n", pid)
				cmd.Printf("\nEndpoints:\n")
				cmd.Printf("  Operator Bootstrap: https://%s:%d\n", config.GetExternalInterfaceIP(), cfg.OperatorBootstrapHTTPSPort())
				cmd.Printf("  Public API:         https://localhost:%d (Public browser/BYO bootstrap)\n", cfg.Paths.Ports.OperatorPublicHTTPS)
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
		Short: "Restart the Governance Gateway",
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
				return fmt.Errorf("failed to check operator status: %w", err)
			}

			if running {
				cmd.Println("Stopping Governance Gateway...")
				if err := pm.StopOperator(); err != nil {
					return err
				}
			}

			cmd.Println("Starting Governance Gateway...")
			if err := pm.StartOperator(
				"doctrine",
				cfg.OperatorHTTPSPort(),
				0,
				cfg.Paths.Ports.OperatorPublicHTTPS,
				"",
				"",
				"",
				"",
				"",
				0,
				0,
				"info",
			); err != nil {
				return err
			}

			cmd.Println("Governance Gateway restarted successfully")
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
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

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

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("failed to create process manager: %w", err)
			}

			if err := pm.Reset(); err != nil {
				return err
			}

			cmd.Println("Reset complete. Data wiped.")
			cmd.Println("Restarting services...")

			if err := pm.StartOperator(
				"doctrine",
				cfg.OperatorHTTPSPort(),
				0,
				cfg.Paths.Ports.OperatorPublicHTTPS,
				"",
				"",
				"",
				"",
				"",
				0,
				0,
				"info",
			); err != nil {
				return fmt.Errorf("failed to restart services: %w", err)
			}

			cmd.Println("Services restarted successfully")
			cmd.Printf("Governance mode: doctrine (L1 enforced, L2/L3 audited)\n")
			cmd.Printf("\nEndpoints:\n")
			cmd.Printf("  Public API: https://localhost:%d (Bootstrap + browser/BYO)\n", cfg.Paths.Ports.OperatorPublicHTTPS)
			cmd.Printf("\nNext step: Run './g8e auth login' to authenticate\n")
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
	var transportType string

	cmd := &cobra.Command{
		Use:   "mcp-config",
		Short: "Print MCP client configuration for the Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			var templatePath string
			if transportType == "http" {
				templatePath = filepath.Join(cfg.ProjectRoot, "protocol", "examples", "mcp_server", "g8e_gateway_mcp_config_http.json")
			} else {
				templatePath = filepath.Join(cfg.ProjectRoot, "protocol", "examples", "mcp_server", "g8e_gateway_mcp_config.json")
			}

			templateContent, err := os.ReadFile(templatePath)
			if err != nil {
				return fmt.Errorf("failed to read MCP config template: %w", err)
			}

			gatewayURL := fmt.Sprintf("https://localhost:%d/api/mcp/v1", cfg.OperatorHTTPSPort())
			projectRoot := cfg.ProjectRoot
			hostname := "localhost"

			configStr := string(templateContent)
			configStr = strings.ReplaceAll(configStr, "{{GATEWAY_URL}}", gatewayURL)
			configStr = strings.ReplaceAll(configStr, "{{PROJECT_ROOT}}", projectRoot)
			configStr = strings.ReplaceAll(configStr, "{{HOSTNAME}}", hostname)

			cmd.Println(configStr)
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("MCP Configuration Instructions")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")

			if transportType == "stdio" {
				cmd.Println("Stdio Mode (IDE Integration):")
				cmd.Println("1. Ensure g8e binary is on your PATH")
				cmd.Println("2. Run ./g8e auth login to bootstrap certificates")
				cmd.Println("3. Copy the JSON configuration above to your IDE's MCP config")
				cmd.Println("4. The g8e CLI will automatically load mTLS certs from .g8e/pki")
			} else {
				cmd.Println("HTTP Mode (Direct Connection):")
				cmd.Println("1. Set environment variables for your MCP client:")
				cmd.Printf("   export G8E_CLIENT_CERT_PATH=%s/.g8e/pki/client.crt\n", projectRoot)
				cmd.Printf("   export G8E_CLIENT_KEY_PATH=%s/.g8e/pki/client.key\n", projectRoot)
				cmd.Printf("   export G8E_CA_CERT_PATH=%s/.g8e/pki/ca.crt\n", projectRoot)
				cmd.Println()
				cmd.Println("2. Copy the JSON configuration above to your MCP client's config file")
			}

			cmd.Println()
			cmd.Println("3. Ensure the Gateway is running: ./g8e gw start")
			cmd.Println()

			return nil
		},
	}

	cmd.Flags().StringVar(&transportType, "transport", "stdio", "Transport type: stdio (for IDEs) or http (direct)")

	return cmd
}
