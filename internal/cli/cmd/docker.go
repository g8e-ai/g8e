// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
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
// starts the operator/dashboard/ensemble workloads.
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
` + "`" + `bootstrapped` + "`" + ` profile and require owner enrollment before they can start. Pass
--profile bootstrapped (or --full) to bring up the workloads after enrolling.

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

// resolveDockerProfile returns the bootstrapped profile name when full is true,
// or the explicit profile override when set, otherwise the empty string
// (gateway-only startup).
func resolveDockerProfile(full bool, profile string) string {
	if profile != "" {
		return profile
	}
	if full {
		return constants.DockerBootstrappedProfile
	}
	return ""
}

func dockerStartCmd() *cobra.Command {
	return dockerStartCmdWithConfig(loadConfig, newFileSvc, defaultAPIClientFactory, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
}

// dockerStartDeps holds the injectable dependencies for the interactive docker
// start walkthrough. Production wires real factories; tests wire stubs. The
// cfg and fileSvc fields are pre-resolved by the command RunE before containers
// start so a factory failure aborts early.
type dockerStartDeps struct {
	clientFactory        apiClientFactory
	checkOperatorRunning func(*config.Config) error
	enrollerFactory      enrollerFactory
	waitGatewayHealthy   func(*cobra.Command) error
	cfg                  *config.Config
	fileSvc              fs.RuntimeFileService
}

func dockerStartCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	clientFactory apiClientFactory,
	checkOperatorRunning func(*config.Config) error,
	enrollerFactory enrollerFactory,
) *cobra.Command {
	var full bool
	var profile string
	var skipEnroll bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Docker Compose unified stack",
		Long: `Start the Docker Compose unified stack in the background.

By default only the gateway starts. Pass --full (or --profile bootstrapped) to
also start the operator, ensemble, and dashboard workloads, which require
owner enrollment before they become ready.

When --full is set, the command walks the owner through interactive enrollment:
  1. Enrolls the CLI user (the first owner) with the gateway.
  2. Prompts to approve the Ensemble platform enrollment request.
  3. Prompts to approve the Dashboard platform enrollment request.
  4. Prompts to approve the Operator platform enrollment request.

Each component prompt accepts y to approve or n (or any other input) to skip.
Use --skip-enroll to start the bootstrapped profile without the interactive
walkthrough (the workloads will block waiting for manual approval).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			resolved := resolveDockerProfile(full, profile)
			scope := "gateway"
			if resolved != "" {
				scope = fmt.Sprintf("full stack (profile %s)", resolved)
			}

			// When the interactive walkthrough is active, resolve config and
			// fileSvc BEFORE starting containers so a factory failure aborts
			// early without leaving half-started containers.
			var walkthroughDeps *dockerStartDeps
			if resolved != "" && !skipEnroll {
				cfg, err := configLoader("")
				if err != nil {
					return err
				}
				fileSvc, err := fileSvcFactory("", slog.Default())
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
				}
				walkthroughDeps = &dockerStartDeps{
					clientFactory:        clientFactory,
					checkOperatorRunning: checkOperatorRunning,
					enrollerFactory:      enrollerFactory,
					waitGatewayHealthy:   waitForDockerGatewayHealthy,
					cfg:                  cfg,
					fileSvc:              fileSvc,
				}
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
				return nil
			}

			if skipEnroll {
				cmd.Println()
				cmd.Println("--skip-enroll set: workloads are started but will block waiting")
				cmd.Println("for manual platform enrollment approval. Run:")
				cmd.Println("  g8e auth pending-platform-enrollments")
				cmd.Println("  g8e auth approve-platform-enrollment <request-id>")
				return nil
			}

			return runDockerStartWalkthrough(cmd, *walkthroughDeps)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Start the full stack (gateway + operator + ensemble + dashboard)")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to start (e.g. bootstrapped)")
	cmd.Flags().BoolVar(&skipEnroll, "skip-enroll", false, "Start the bootstrapped profile without the interactive enrollment walkthrough")
	return cmd
}

// runDockerStartWalkthrough drives the interactive enrollment walkthrough after
// the bootstrapped profile containers are up. It enrolls the CLI owner, waits for
// the gateway to be reachable, then prompts the owner to approve each
// component's platform enrollment request in order: ensemble, dashboard,
// operator. Each prompt is skippable (any answer other than y skips that
// component without aborting the walkthrough).
func runDockerStartWalkthrough(cmd *cobra.Command, deps dockerStartDeps) error {
	ctx := cmd.Context()

	cmd.Println()
	cmd.Println("=== Interactive enrollment walkthrough ===")
	cmd.Println()

	waitHealthy := deps.waitGatewayHealthy
	if waitHealthy == nil {
		waitHealthy = waitForDockerGatewayHealthy
	}
	if err := waitHealthy(cmd); err != nil {
		return err
	}

	cfg := deps.cfg
	fileSvc := deps.fileSvc

	cmd.Println("Step 1: Enroll the CLI owner with the gateway.")
	cmd.Println("  This creates the first user/session and registers a passkey.")
	if err := deps.checkOperatorRunning(cfg); err != nil {
		cmd.Printf("  Gateway not reachable for enrollment: %v\n", err)
		return fmt.Errorf("%w: %w", constants.ErrDockerStartEnrollmentFailed, err)
	}

	coordinator := deps.enrollerFactory(func(format string, a ...any) {
		cmd.Printf(format+"\n", a...)
	}, fileSvc, cfg)
	result, err := coordinator.Enroll(ctx, auth.EnrollmentOptions{})
	if err != nil {
		cmd.Printf("  Owner enrollment failed: %v\n", err)
		return fmt.Errorf("%w: %w", constants.ErrDockerStartEnrollmentFailed, err)
	}
	if result.Reused {
		cmd.Printf("  Reusing existing CLI identity (no new certificate issued).\n")
	} else {
		cmd.Printf("  CLI session %s complete\n", result.Source)
	}
	cmd.Printf("  User ID: %s\n", result.UserID)
	cmd.Printf("  CLI Session ID: %s\n", result.CLISessionID)
	cmd.Println()

	client, err := deps.clientFactory(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("%w: create API client: %w", constants.ErrDockerStartApprovalFailed, err)
	}

	components := []models.PlatformComponentKind{
		models.PlatformComponentEnsemble,
		models.PlatformComponentDashboard,
		models.PlatformComponentOperator,
	}
	for i, component := range components {
		step := i + 2
		cmd.Printf("Step %d: Approve the %s platform enrollment request.\n", step, component)
		if err := promptApproveComponent(cmd, ctx, client, component); err != nil {
			cmd.Printf("  %s enrollment step failed: %v\n", component, err)
		}
		cmd.Println()
	}

	cmd.Println("Interactive enrollment walkthrough complete.")
	cmd.Println("Run 'g8e docker status' to check service status.")
	cmd.Println("Run 'g8e docker logs' to follow logs.")
	return nil
}

// waitForDockerGatewayHealthy polls the gateway HTTP health endpoint until it
// responds 200 or the timeout elapses. The gateway container publishes 8080.
func waitForDockerGatewayHealthy(cmd *cobra.Command) error {
	cmd.Println("Waiting for the gateway container to become healthy...")
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", constants.Ports.OperatorHttp)
	plainClient := &http.Client{Timeout: 2 * time.Second} //nolint:gosec
	const (
		maxAttempts  = 60
		pollInterval = 500 * time.Millisecond
	)
	for i := 0; i < maxAttempts; i++ {
		resp, err := plainClient.Get(healthURL) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				cmd.Println("Gateway is healthy.")
				return nil
			}
		}
		if i == maxAttempts-1 {
			return fmt.Errorf("%w: gateway did not become healthy after %v",
				constants.ErrGatewayNotReady, time.Duration(maxAttempts)*pollInterval)
		}
		time.Sleep(pollInterval)
	}
	return nil
}

// promptApproveComponent fetches pending platform enrollment requests, finds
// the one matching the given component kind, displays it, and prompts the owner
// to approve (y) or skip (any other input). When approved it posts the
// decision. A missing pending request is reported but not fatal — the
// component may already be enrolled, or its container may not have submitted
// its request yet.
func promptApproveComponent(cmd *cobra.Command, ctx context.Context, client apiClient, component models.PlatformComponentKind) error {
	pendingBody, err := client.Get(constants.APIPaths.AuthPlatformEnrollmentPending)
	if err != nil {
		return fmt.Errorf("fetch pending list: %w", err)
	}

	var pendingResp models.PlatformEnrollmentPendingResponse
	if err := json.Unmarshal(pendingBody, &pendingResp); err != nil {
		return fmt.Errorf("parse pending list: %w", err)
	}

	req := findPendingRequestByComponent(pendingResp.Requests, component)
	if req == nil {
		cmd.Printf("  No pending %s enrollment request found (it may already be enrolled\n", component)
		cmd.Printf("  or its container has not submitted a request yet). Skipping.\n")
		return nil
	}

	printPlatformEnrollmentRequestDetails(cmd, req)

	if !confirmAction(cmd, fmt.Sprintf("Approve this %s enrollment request?", component)) {
		cmd.Printf("  Skipped %s enrollment. You can approve it later with:\n", component)
		cmd.Printf("  g8e auth approve-platform-enrollment %s\n", req.RequestID)
		return nil
	}

	decisionReq := models.PlatformEnrollmentDecisionRequest{
		RequestID: req.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	}
	if err := decisionReq.Validate(); err != nil {
		return fmt.Errorf("validate decision: %w", err)
	}

	respBody, err := client.Post(constants.APIPaths.AuthPlatformEnrollmentDecision, decisionReq)
	if err != nil {
		return fmt.Errorf("%w: post decision: %w", constants.ErrDockerStartApprovalFailed, err)
	}

	var resp models.PlatformEnrollmentDecisionResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("parse decision response: %w", err)
	}
	cmd.Printf("  %s enrollment request %s.\n", component, string(resp.State))
	return nil
}

// findPendingRequestByComponent returns the first pending request matching the
// given component kind, or nil if none match.
func findPendingRequestByComponent(requests []models.PlatformEnrollmentPendingRequest, component models.PlatformComponentKind) *models.PlatformEnrollmentPendingRequest {
	for i := range requests {
		if requests[i].ComponentKind == component {
			return &requests[i]
		}
	}
	return nil
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
			if err := runDockerCompose([]string{"down"}, resolveDockerProfile(true, profile)); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
			}
			cmd.Println("\nDocker Compose stack stopped successfully.")
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. bootstrapped)")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. bootstrapped)")
	return cmd
}

func resolveDockerBuildID(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("resolve Docker build ID: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func dockerBuildArgs(buildID string, noCache bool) []string {
	args := []string{"build", "--build-arg", "BUILD_ID=" + buildID}
	if noCache {
		args = append(args, "--no-cache")
	}
	return args
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
			buildID, err := resolveDockerBuildID(cmd.Context())
			if err != nil {
				return err
			}
			cmd.Println("Building Docker images...")
			if err := runDockerCompose(dockerBuildArgs(buildID, noCache), resolveDockerProfile(true, profile)); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			cmd.Println("\nDocker images built successfully.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Build without using the Docker cache")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. bootstrapped)")
	return cmd
}

func dockerCleanCmd() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove containers, volumes, and networks for the unified stack",
		Long: `Remove containers, volumes, and networks for the Docker Compose unified stack.

This is a destructive operation that removes all associated Docker volumes and
networks, including the gateway data volume. Use --yes=false to confirm first.

Clean always targets the bootstrapped profile so that operator, ensemble, and
dashboard containers are removed alongside the gateway, not just the
default-profile gateway container.`,
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
			// Always pass the bootstrapped profile so operator, ensemble, and
			// dashboard containers are removed together with the gateway.
			// Without it, down only touches default-profile services and the
			// bootstrapped-profile containers keep running, holding their volumes
			// and the shared network open.
			if err := runDockerCompose([]string{"down", "-v", "--remove-orphans", "-t", "0"}, constants.DockerBootstrappedProfile); err != nil {
				cmd.Printf("Warning: compose down had issues: %v\n", err)
			}
			forceRemoveLeftovers(cmd, constants.DockerProjectPrefix)
			cmd.Println("\nDocker Compose stack cleaned successfully.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipConfirm, "yes", true, "Skip interactive confirmation (default: true)")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to start (e.g. bootstrapped)")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to start (e.g. bootstrapped)")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. bootstrapped)")
	return cmd
}
