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

	"github.com/g8e-ai/g8e/services/g8eo/internal/cli/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/cli/platform"
	"github.com/spf13/cobra"
)

func appsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apps",
		Short: "Manage optional application-layer adapters",
		Long:  `Manage optional application-layer adapters like g8ee (Engine).`,
	}

	cmd.AddCommand(
		appsStartCmd(),
		appsStopCmd(),
	)

	return cmd
}

func appsStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <app-name>",
		Short: "Start an optional application adapter",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName := args[0]
			if appName != "g8ee" {
				return fmt.Errorf("unsupported app: %s (only g8ee is supported)", appName)
			}

			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("failed to create process manager: %w", err)
			}

			running, pid, err := pm.G8eeStatus()
			if err != nil {
				return fmt.Errorf("failed to check g8ee status: %w", err)
			}
			if running {
				cmd.Printf("g8ee is already running (PID: %d)\n", pid)
				return nil
			}

			cmd.Println("Starting g8ee...")
			if err := pm.StartG8ee(); err != nil {
				return err
			}

			cmd.Println("g8ee started successfully")
			return nil
		},
	}

	return cmd
}

func appsStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <app-name>",
		Short: "Stop an optional application adapter",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName := args[0]
			if appName != "g8ee" {
				return fmt.Errorf("unsupported app: %s (only g8ee is supported)", appName)
			}

			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("failed to create process manager: %w", err)
			}

			running, pid, err := pm.G8eeStatus()
			if err != nil {
				return fmt.Errorf("failed to check g8ee status: %w", err)
			}
			if !running {
				cmd.Println("g8ee is not running")
				return nil
			}

			cmd.Printf("Stopping g8ee (PID: %d)...\n", pid)
			if err := pm.StopG8ee(); err != nil {
				return err
			}

			cmd.Println("g8ee stopped successfully")
			return nil
		},
	}

	return cmd
}
