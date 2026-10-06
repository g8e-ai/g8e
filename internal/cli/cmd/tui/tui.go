// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tuicmd

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/api"
	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
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
	loadAuthContext      func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error)
	newSession           func(fs.RuntimeFileService, *config.Config) (tui.Session, error)
	openBrowser          func(string) error
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
		configLoader:         shared.LoadConfig,
		fileSvcFactory:       shared.NewFileSvc,
		checkOperatorRunning: auth.CheckOperatorRunning,
		inspectDockerGateway: inspectDockerGateway,
		loadAuthContext:      auth.LoadClientAuthContext,
		newSession:           newAPISession,
		openBrowser:          platform.OpenBrowser,
		tuiRun:               tui.Run,
	}
}

// newAPISession opens the same mTLS API client every other CLI command uses.
func newAPISession(fileSvc fs.RuntimeFileService, cfg *config.Config) (tui.Session, error) {
	client, err := api.NewClient(fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	return client, nil
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

func Cmd() *cobra.Command {
	return tuiCmdWithDeps(defaultTUIDeps())
}

func tuiCmdWithDeps(deps tuiDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the Tactical Governance Console (TUI)",
		Long: `Launch the Tactical Governance Console — a real-time terminal UI that
connects to a running g8e Gateway over the enrolled CLI session (mTLS + SSE).
It shows your CLI identity and the Gateway's governance posture, the execution
pipeline (L1-L5), the Sovereign Audit Ledger, the pending L3 approval queue,
and your connected Operators. Pending approvals are listed on connect and
whenever the Gateway reports a change; approving one opens the same browser
WebAuthn page as 'g8e auth approve <tx_hash>' and verifies the result.

The Gateway must be running and the CLI must be enrolled (g8e auth enroll user)
before launching the TUI. If the CLI session expires, run 'g8e auth refresh'.

Controls:
  q / Ctrl+C       Quit
  Tab / Shift+Tab  Focus the next / previous pane (ledger, approvals, operators)
  j / ↓, k / ↑     Move in the focused pane (ledger: newer / older)
  G / g            Jump to ledger bottom (newest) / top (oldest)
  a / Enter        Approve the selected pending transaction (browser WebAuthn)
  r                Refresh approvals, operators, and posture`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd, deps)
		},
	}

	return cmd
}

func runTUI(cmd *cobra.Command, deps tuiDeps) error {
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

	// Load the enrolled CLI identity with the same validation and errors as
	// the rest of the CLI.
	authCtx, err := deps.loadAuthContext(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	// Open the CLI session: the same mTLS identity, CLI session header, and
	// SSE stream every other CLI command uses.
	session, err := deps.newSession(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("tui: open CLI session: %w", err)
	}

	version := cmd.Root().Version
	if version == "" {
		version = string(constants.VersionStabilityDev)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	return deps.tuiRun(ctx, tui.Options{
		Version: version,
		Identity: tui.Identity{
			UserID:       authCtx.UserID,
			CLISessionID: authCtx.CLISessionID,
			OperatorID:   authCtx.OperatorID,
		},
		Session:     session,
		ApprovalURL: func(txHash string) string { return auth.ApprovalPageURL(cfg, txHash) },
		OpenBrowser: deps.openBrowser,
	})
}
