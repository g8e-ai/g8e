// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/constants"
)

// dockerComposePath resolves the root unified-stack compose file relative to
// the current working directory. The root compose file deploys the full
// platform stack (gateway, operator, ensemble, dashboard).
func dockerComposePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	return filepath.Join(cwd, constants.DockerComposeFile), nil
}

// checkDockerComposeFileExists verifies the root compose file is present in
// the current working directory.
func checkDockerComposeFileExists() error {
	composePath, err := dockerComposePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(composePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s (run this command from the repository root)", constants.ErrNotFound, constants.DockerComposeFile)
		}
		return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
	}
	return nil
}

// runDockerCompose builds and runs a `docker compose` command against the root
// compose file, streaming stdout/stderr to the console. The optional profile
// activates the operator/dashboard/ensemble workloads.
func runDockerCompose(args []string, profile string) error {
	composePath, err := dockerComposePath()
	if err != nil {
		return err
	}
	if err := checkDockerAvailable(); err != nil {
		return err
	}
	fullArgs := []string{"compose", "-f", toDockerPath(composePath)}
	if profile != "" {
		fullArgs = append(fullArgs, "--profile", profile)
	}
	fullArgs = append(fullArgs, args...)

	c := exec.Command("docker", fullArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func dockerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docker",
		Short: "Manage the Docker Compose unified stack",
		Long: `Manage the root Docker Compose unified stack (gateway, operator, ensemble, dashboard).

The root ` + "`" + `docker-compose.yml` + "`" + ` deploys the full platform. Only the gateway starts
by default; the operator, ensemble, and dashboard are gated behind the
` + "`" + `activated` + "`" + ` profile and require owner enrollment before they can start. Pass
--profile activated (or --full) to bring up the workloads after enrolling.

Run these commands from the repository root where ` + "`" + `docker-compose.yml` + "`" + ` lives.`,
	}
	cmd.AddCommand(
		dockerStartCmd(),
		dockerStopCmd(),
		dockerStatusCmd(),
		dockerBuildCmd(),
		dockerCleanCmd(),
		dockerResetCmd(),
		dockerRebuildCmd(),
		dockerLogsCmd(),
	)
	return cmd
}

// resolveDockerProfile returns the activated profile name when full is true,
// or the explicit profile override when set, otherwise the empty string
// (gateway-only startup).
func resolveDockerProfile(full bool, profile string) string {
	if profile != "" {
		return profile
	}
	if full {
		return constants.DockerActivatedProfile
	}
	return ""
}

func dockerStartCmd() *cobra.Command {
	var full bool
	var profile string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Docker Compose unified stack",
		Long: `Start the Docker Compose unified stack in the background.

By default only the gateway starts. Pass --full (or --profile activated) to
also start the operator, ensemble, and dashboard workloads, which require
owner enrollment before they become ready.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			resolved := resolveDockerProfile(full, profile)
			scope := "gateway"
			if resolved != "" {
				scope = fmt.Sprintf("full stack (profile %s)", resolved)
			}
			cmd.Printf("Starting Docker Compose %s...\n", scope)
			if err := runDockerCompose([]string{"up", "-d"}, resolved); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			cmd.Printf("\nDocker Compose %s started successfully.\n", scope)
			cmd.Println("Run 'g8e docker status' to check service status.")
			cmd.Println("Run 'g8e docker logs' to follow logs.")
			if resolved == "" {
				cmd.Println()
				cmd.Println("To bring up the operator, ensemble, and dashboard after enrolling")
				cmd.Println("the first owner, run 'g8e docker start --full'.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Start the full stack (gateway + operator + ensemble + dashboard)")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to activate (e.g. activated)")
	return cmd
}

func dockerStopCmd() *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Docker Compose unified stack",
		Long:  `Stop and remove containers for the Docker Compose unified stack, preserving volumes and networks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			cmd.Println("Stopping Docker Compose stack...")
			if err := runDockerCompose([]string{"down"}, profile); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
			}
			cmd.Println("\nDocker Compose stack stopped successfully.")
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. activated)")
	return cmd
}

func dockerStatusCmd() *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of the Docker Compose unified stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			if err := runDockerCompose([]string{"ps"}, profile); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. activated)")
	return cmd
}

func dockerBuildCmd() *cobra.Command {
	var noCache bool
	var profile string

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build Docker images for the unified stack",
		Long:  `Build all Docker images defined in the root docker-compose.yml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			buildArgs := []string{"build"}
			if noCache {
				buildArgs = append(buildArgs, "--no-cache")
			}
			cmd.Println("Building Docker images...")
			if err := runDockerCompose(buildArgs, profile); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			cmd.Println("\nDocker images built successfully.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Build without using the Docker cache")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. activated)")
	return cmd
}

func dockerCleanCmd() *cobra.Command {
	var skipConfirm bool
	var profile string

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove containers, volumes, and networks for the unified stack",
		Long: `Remove containers, volumes, and networks for the Docker Compose unified stack.

This is a destructive operation that removes all associated Docker volumes and
networks, including the gateway data volume. Use --yes=false to confirm first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			if !skipConfirm {
				cmd.Println("WARNING: This will remove ALL containers, volumes, and networks for the unified stack.")
				if !confirmAction(cmd, "Proceed with clean?") {
					cmd.Println("Clean cancelled.")
					return nil
				}
			}
			if err := checkDockerAvailable(); err != nil {
				cmd.Println("Docker not available — nothing to clean.")
				return nil
			}
			cmd.Println("Cleaning Docker Compose stack...")
			if err := runDockerCompose([]string{"down", "-v", "--remove-orphans", "-t", "0"}, profile); err != nil {
				cmd.Printf("Warning: compose down had issues: %v\n", err)
			}
			forceRemoveLeftovers(cmd, constants.DockerProjectPrefix)
			cmd.Println("\nDocker Compose stack cleaned successfully.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipConfirm, "yes", true, "Skip interactive confirmation (default: true)")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. activated)")
	return cmd
}

func dockerResetCmd() *cobra.Command {
	var full bool
	var profile string

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Clean and restart the Docker Compose unified stack",
		Long:  `Clean (remove containers, volumes, networks) and restart the Docker Compose unified stack.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			cmd.Println("Cleaning Docker Compose stack...")
			if err := runDockerCompose([]string{"down", "-v", "--remove-orphans", "-t", "0"}, profile); err != nil {
				cmd.Printf("Warning: compose down had issues: %v\n", err)
			}
			forceRemoveLeftovers(cmd, constants.DockerProjectPrefix)
			resolved := resolveDockerProfile(full, profile)
			scope := "gateway"
			if resolved != "" {
				scope = fmt.Sprintf("full stack (profile %s)", resolved)
			}
			cmd.Printf("\nStarting Docker Compose %s...\n", scope)
			if err := runDockerCompose([]string{"up", "-d"}, resolved); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			cmd.Printf("\nDocker Compose %s reset successfully.\n", scope)
			return nil
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Start the full stack (gateway + operator + ensemble + dashboard)")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to activate (e.g. activated)")
	return cmd
}

func dockerRebuildCmd() *cobra.Command {
	var noCache bool
	var full bool
	var profile string

	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild images and restart the Docker Compose unified stack",
		Long: `Stop the unified stack, rebuild all Docker images, and start it again.

Use --no-cache=false to reuse the Docker build cache.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			cmd.Println("Stopping Docker Compose stack...")
			if err := runDockerCompose([]string{"down"}, profile); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
			}
			buildArgs := []string{"build"}
			if noCache {
				buildArgs = append(buildArgs, "--no-cache")
			}
			cmd.Println("\nRebuilding Docker images...")
			if err := runDockerCompose(buildArgs, profile); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			resolved := resolveDockerProfile(full, profile)
			scope := "gateway"
			if resolved != "" {
				scope = fmt.Sprintf("full stack (profile %s)", resolved)
			}
			cmd.Printf("\nStarting Docker Compose %s...\n", scope)
			if err := runDockerCompose([]string{"up", "-d"}, resolved); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			cmd.Printf("\nDocker Compose %s rebuilt and started successfully.\n", scope)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noCache, "no-cache", true, "Rebuild without using the Docker cache")
	cmd.Flags().BoolVar(&full, "full", false, "Start the full stack (gateway + operator + ensemble + dashboard)")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to activate (e.g. activated)")
	return cmd
}

func dockerLogsCmd() *cobra.Command {
	var follow bool
	var profile string

	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Show logs for the Docker Compose unified stack",
		Long:  `Show logs for the Docker Compose unified stack. Optionally pass a service name to filter, and --follow (-f) to stream.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			logArgs := []string{"logs"}
			if follow {
				logArgs = append(logArgs, "-f")
			}
			if len(args) == 1 {
				logArgs = append(logArgs, args[0])
			}
			if err := runDockerCompose(logArgs, profile); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. activated)")
	return cmd
}

// dockerStackRunning reports whether any containers for the root compose
// project are currently running.
func dockerStackRunning() bool {
	composePath, err := dockerComposePath()
	if err != nil {
		return false
	}
	c := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "ps", "-q")
	out, err := c.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}
