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

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/tui"
	"github.com/g8e-ai/g8e/internal/constants"
)

func tuiCmd() *cobra.Command {
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
		RunE: runTUI,
	}

	return cmd
}

func runTUI(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig("")
	if err != nil {
		return err
	}

	// Verify the gateway is reachable.
	if err := auth.CheckOperatorRunning(cfg); err != nil {
		return fmt.Errorf("gateway not reachable — start it with 'g8e gw start': %w", err)
	}

	// Load CLI credentials (must be enrolled).
	creds, err := auth.LoadCredentials(cfg)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}
	if creds == nil {
		return fmt.Errorf("not enrolled — run 'g8e auth enroll' first")
	}

	// Build mTLS HTTP client for SSE streaming (no timeout — context-controlled).
	httpClient, err := auth.BuildMTLSClient(cfg, 0)
	if err != nil {
		return err
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

	return tui.Run(ctx, tui.Options{
		Version:    version,
		NodeName:   creds.OperatorID,
		NetLabel:   "mTLS",
		SSEURL:     sseURL,
		HTTPClient: httpClient,
	})
}
