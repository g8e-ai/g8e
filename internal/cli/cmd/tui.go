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
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/tui"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// tuiDeps holds the injectable dependencies for the TUI command,
// enabling Tier 1 unit tests to stub external calls.
type tuiDeps struct {
	configLoader         func(string) (*config.Config, error)
	fileSvcFactory       func() (fs.RuntimeFileService, error)
	checkOperatorRunning func(*config.Config) error
	loadCredentials      func(fs.RuntimeFileService, *config.Config) (*auth.Credentials, error)
	buildMTLSClient      func(fs.RuntimeFileService, *config.Config, time.Duration) (*http.Client, error)
	tuiRun               func(context.Context, tui.Options) error
}

func defaultTUIDeps() tuiDeps {
	return tuiDeps{
		configLoader:         loadConfig,
		fileSvcFactory:       newFileSvc,
		checkOperatorRunning: auth.CheckOperatorRunning,
		loadCredentials:      auth.LoadCredentials,
		buildMTLSClient:      auth.BuildMTLSClient,
		tuiRun:               tui.Run,
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
pipeline (L1-L5), Sovereign Audit Ledger, and L2 Tribunal Consensus.

The Gateway must be running and the CLI must be enrolled (g8e auth enroll)
before launching the TUI.

Controls:
  q / Ctrl+C   Quit
  j / ↓        Scroll ledger down (newer)
  k / ↑        Scroll ledger up (older)
  G            Jump to ledger bottom (newest)
  g            Jump to ledger top (oldest)`,
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
		return fmt.Errorf("%w — start it with 'g8e gw start': %w", constants.ErrGatewayNotReachable, err)
	}

	fileSvc, err := deps.fileSvcFactory()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	// Load CLI credentials (must be enrolled).
	creds, err := deps.loadCredentials(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}
	if creds == nil {
		return fmt.Errorf("%w — run 'g8e auth enroll' first", constants.ErrNotEnrolled)
	}

	// Build mTLS HTTP client for SSE streaming (no timeout — context-controlled).
	httpClient, err := deps.buildMTLSClient(fileSvc, cfg, 0)
	if err != nil {
		return fmt.Errorf("tui: build mTLS client: %w", err)
	}

	// Construct the SSE stream URL.
	baseURL := cfg.OperatorHTTPURL()
	sseURL := baseURL + constants.APIPaths.SSEStream + "?cli_session_id=" + creds.CLISessionID

	version := cmd.Root().Version
	if version == "" {
		version = "dev"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return deps.tuiRun(ctx, tui.Options{
		Version:    version,
		NodeName:   creds.OperatorID,
		NetLabel:   "mTLS",
		SSEURL:     sseURL,
		HTTPClient: httpClient,
	})
}
