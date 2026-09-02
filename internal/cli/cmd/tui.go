// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/tui"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// tuiDeps holds the injectable dependencies for the TUI command,
// enabling Tier 1 unit tests to stub external calls.
type tuiDeps struct {
	configLoader         func(string) (*config.Config, error)
	fileSvcFactory       func(string, *slog.Logger) (fs.RuntimeFileService, error)
	checkOperatorRunning func(*config.Config) error
	inspectDockerGateway func(context.Context) (dockerContainerState, error)
	loadCredentials      func(fs.RuntimeFileService, *config.Config) (*auth.Credentials, error)
	buildMTLSClient      func(fs.RuntimeFileService, *config.Config, time.Duration) (*http.Client, error)
	tuiRun               func(context.Context, tui.Options) error
}

type dockerContainerStatus string

type dockerContainerHealth string

type dockerContainerState struct {
	Status dockerContainerStatus
	Health dockerContainerHealth
}

const (
	dockerContainerStatusRunning dockerContainerStatus = "running"
	dockerContainerStatusExited  dockerContainerStatus = "exited"

	dockerContainerHealthHealthy   dockerContainerHealth = "healthy"
	dockerContainerHealthStarting  dockerContainerHealth = "starting"
	dockerContainerHealthUnhealthy dockerContainerHealth = "unhealthy"
	dockerContainerHealthNone      dockerContainerHealth = "none"

	dockerGatewayInspectionTimeout = 2 * time.Second
)

func defaultTUIDeps() tuiDeps {
	return tuiDeps{
		configLoader:         loadConfig,
		fileSvcFactory:       newFileSvc,
		checkOperatorRunning: auth.CheckOperatorRunning,
		inspectDockerGateway: inspectDockerGateway,
		loadCredentials:      auth.LoadCredentials,
		buildMTLSClient:      auth.BuildMTLSClient,
		tuiRun:               tui.Run,
	}
}

func inspectDockerGateway(ctx context.Context) (dockerContainerState, error) {
	ctx, cancel := context.WithTimeout(ctx, dockerGatewayInspectionTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}", constants.DockerGatewayContainer).Output()
	if err != nil {
		return dockerContainerState{}, fmt.Errorf("inspect Docker gateway: %w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 {
		return dockerContainerState{}, fmt.Errorf("%w: inspect Docker gateway state output %q", constants.ErrInternal, strings.TrimSpace(string(output)))
	}
	return dockerContainerState{Status: dockerContainerStatus(fields[0]), Health: dockerContainerHealth(fields[1])}, nil
}

func gatewayUnavailableDetail(cfg *config.Config, state dockerContainerState, inspectErr error) string {
	if inspectErr != nil {
		return "no running gateway was detected; start the Docker stack with 'g8e docker start' or start a local gateway with 'g8e gw start'"
	}
	if state.Status != dockerContainerStatusRunning {
		return fmt.Sprintf("Docker gateway is %s (health: %s); inspect it with 'g8e docker logs' and restart it with 'g8e docker start'", state.Status, state.Health)
	}
	switch state.Health {
	case dockerContainerHealthStarting:
		return "Docker gateway is running and its healthcheck is starting; wait and check 'g8e docker status'"
	case dockerContainerHealthUnhealthy:
		return "Docker gateway is running but unhealthy; inspect it with 'g8e docker logs'"
	case dockerContainerHealthHealthy:
		return fmt.Sprintf("Docker gateway is healthy, but the configured endpoint %s is unreachable; verify the endpoint and published ports with 'g8e docker status'", cfg.OperatorDiscoveryURL())
	case dockerContainerHealthNone:
		return "Docker gateway is running without a healthcheck, but its configured endpoint is unreachable; inspect it with 'g8e docker status' and 'g8e docker logs'"
	default:
		return fmt.Sprintf("Docker gateway is running with health state %s, but its configured endpoint is unreachable; inspect it with 'g8e docker status' and 'g8e docker logs'", state.Health)
	}
}

func tuiCmd() *cobra.Command {
	return tuiCmdWithDeps(defaultTUIDeps())
}

func tuiCmdWithDeps(deps tuiDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the Tactical Governance Console (TUI)",
		Long: `Launch the Tactical Governance Console — a real-time terminal UI that
connects to a running g8e Gateway via SSE and visualizes the execution
pipeline (L1-L5), Sovereign Audit Ledger, and L2 Consensus.

The Gateway must be running and the CLI must be enrolled (g8e auth enroll user)
before launching the TUI.

Controls:
  q / Ctrl+C   Quit
  j / ↓        Scroll ledger down (newer)
  k / ↑        Scroll ledger up (older)
  G            Jump to ledger bottom (newest)
  g            Jump to ledger top (oldest)`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cmd, args, deps)
		},
	}

	return cmd
}

func runTUI(cmd *cobra.Command, args []string, deps tuiDeps) error {
	cfg, err := deps.configLoader("")
	if err != nil {
		return fmt.Errorf("tui: load config: %w", err)
	}

	// Verify the gateway is reachable.
	if err := deps.checkOperatorRunning(cfg); err != nil {
		state, inspectErr := deps.inspectDockerGateway(cmd.Context())
		return fmt.Errorf("%w — %s: %w", constants.ErrGatewayNotReachable, gatewayUnavailableDetail(cfg, state, inspectErr), err)
	}

	fileSvc, err := deps.fileSvcFactory("", slog.Default())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	// Load CLI credentials (must be enrolled).
	creds, err := deps.loadCredentials(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}
	if creds == nil {
		return fmt.Errorf("%w — run 'g8e auth enroll user' first", constants.ErrNotEnrolled)
	}

	// Build mTLS HTTP client for SSE streaming (no timeout — context-controlled).
	httpClient, err := deps.buildMTLSClient(fileSvc, cfg, 0)
	if err != nil {
		return fmt.Errorf("tui: build mTLS client: %w", err)
	}

	// Construct the SSE stream URL. The CLI session ID is sent via the
	// X-G8E-CLI-Session-ID header (set by the TUI adapter from opts.CLISessionID),
	// not in the URL query string. The mTLS cert binds user_id at the gateway.
	sseURL := cfg.OperatorHTTPURL() + constants.APIPaths.SSEStream

	version := cmd.Root().Version
	if version == "" {
		version = string(constants.VersionStabilityDev)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return deps.tuiRun(ctx, tui.Options{
		Version:      version,
		NodeName:     creds.OperatorID,
		NetLabel:     "mTLS",
		SSEURL:       sseURL,
		CLISessionID: creds.CLISessionID,
		HTTPClient:   httpClient,
	})
}
