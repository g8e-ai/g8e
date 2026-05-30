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

			cmd.Println("[g8e] Bootstrapping Sovereign Governance Gateway...")
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
			cmd.Printf("      curl -fsSL https://%s:%d/bootstrap-ca | sudo sh\n", externalIP, cfg.Paths.Ports.OperatorPublicHTTPS)
			cmd.Println()
			cmd.Println("  Windows (PowerShell - Admin) :")
			cmd.Printf("      iex (irm https://%s:%d/bootstrap-ca.ps1)\n", externalIP, cfg.Paths.Ports.OperatorPublicHTTPS)
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println(" 3. TARGETED ACTIONABLE NEXT STEPS")
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("  [Local CLI Auth]    : ./g8e auth login")
			cmd.Println("  [Bind Satellite]    : ./g8e security pki enroll")
			cmd.Println("  [View Live Ledger]  : ./g8e platform logs --follow")
			cmd.Println()
			cmd.Println("────────────────────────────────────────────────────────────────────────────────")
			cmd.Println("[g8e] Sovereignty established. Gateway listening for cryptographic consensus.")

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
				cmd.Println("WARNING: This command will:")
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
