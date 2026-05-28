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

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
)

func platformCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "platform",
		Short: "Manage the Governance Gateway (g8eg) lifecycle",
		Long:  `Platform lifecycle commands for starting, stopping, and checking the status of the Governance Gateway.`,
	}

	cmd.AddCommand(
		platformStartCmd(),
		platformStopCmd(),
		platformStatusCmd(),
		platformRestartCmd(),
		platformLogsCmd(),
		platformSettingsCmd(),
		platformResetCmd(),
		platformCleanCmd(),
	)

	return cmd
}

func platformStartCmd() *cobra.Command {
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

			cmd.Println("[g8e] Initializing Governance Gateway...")
			if err := pm.StartOperator(
				cfg.OperatorHTTPSPort(),
				cfg.Paths.Ports.OperatorPublicHTTPS,
			); err != nil {
				return err
			}

			_, pid, err = pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("failed to check operator status after start: %w", err)
			}

			externalIP := config.GetExternalInterfaceIP()
			runtimeDir := filepath.Join(cfg.ProjectRoot, ".g8e")

			cmd.Println()
			cmd.Println(" ┌── Services Lifecycle ────────────────────────────────────────────────────────┐")
			cmd.Printf(" │  ✔ Core Operator Gateway (g8eo) : running (PID: %d)\n", pid)
			cmd.Println(" │  ✔ Local-First Audit Vault     : initialized & verified")
			cmd.Println(" └──────────────────────────────────────────────────────────────────────────────┘")
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println(" 1. SECURE GATEWAY ENDPOINTS & CRYPTOGRAPHIC REALITY")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Printf("  Platform Hub (Inbound mTLS & WSS Control)    : https://localhost:%d\n", cfg.Paths.Ports.G8eeHTTPS)
			cmd.Printf("  Local Runtime Dir (Local-First LFAA Vaults)  : %s\n", runtimeDir)
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println(" 2. SECURITY BOOTSTRAP: PROVISION LOCAL PKI PORTAL")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("  The Platform serves an automated bootstrap script to install the")
			cmd.Println("  Platform Root CA and provision local workload mTLS certificates.")
			cmd.Println("  Note: First connection bypasses cert verification to install the CA.")
			cmd.Println()
			cmd.Printf("  Run on macOS / Linux (Terminal):\n")
			cmd.Printf("     curl -fsSLk https://%s:%d/bootstrap-ca | sudo sh\n", externalIP, cfg.Paths.Ports.G8eeHTTPS)
			cmd.Println()
			cmd.Printf("  Run on Windows (PowerShell - Administrator):\n")
			cmd.Printf("     iex (irm https://%s:%d/bootstrap-ca.ps1 -SkipCertificateCheck)\n", externalIP, cfg.Paths.Ports.G8eeHTTPS)
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println(" 3. TARGETED ACTIONABLE NEXT STEPS [CHOOSE ONE]")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("  To start executing governed agentic tool calls, authorize your environment:")
			cmd.Println()
			cmd.Println("  A) AUTHENTICATE YOUR LOCAL CLI PROCESS (mTLS)")
			cmd.Println("     $ ./g8e auth login")
			cmd.Println()
			cmd.Println("  B) PROVISION A NEW OUTBOUND REMOTE OPERATOR SATELLITE")
			cmd.Println("     $ ./g8e data device-links create")
			cmd.Println()
			cmd.Println("  C) INTERACT VIA BROWSER / BYO CLIENT SURFACE")
			cmd.Printf("     URL: https://localhost:%d [CA Certificate Required]\n", cfg.Paths.Ports.G8eeHTTPS)
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("[g8e] System ready. Control plane is listening for outbound satellite links.")

			return nil
		},
	}

	return cmd
}

func platformStopCmd() *cobra.Command {
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

func platformStatusCmd() *cobra.Command {
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

func platformRestartCmd() *cobra.Command {
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
				cfg.OperatorHTTPSPort(),
				cfg.Paths.Ports.OperatorPublicHTTPS,
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

func platformLogsCmd() *cobra.Command {
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

func platformSettingsCmd() *cobra.Command {
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

func platformResetCmd() *cobra.Command {
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
				cfg.OperatorHTTPSPort(),
				cfg.Paths.Ports.OperatorPublicHTTPS,
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

func platformCleanCmd() *cobra.Command {
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
				cmd.Println("This command will:")
				cmd.Println("  1. Stop all running g8e services")
				cmd.Println("  2. Completely delete the entire runtime directory")
				cmd.Println("  3. Delete all SQLite databases, bootstrap secrets, logs, AND TLS/PKI certificates/keys")
				cmd.Println("  4. All trust routes and credentials will be permanently destroyed")
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
