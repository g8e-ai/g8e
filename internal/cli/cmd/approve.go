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
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

type apiClient interface {
	Get(path string) ([]byte, error)
	Post(path string, body interface{}) ([]byte, error)
	Put(path string, body interface{}) ([]byte, error)
	Delete(path string) ([]byte, error)
}

type apiClientFactory func(fs.RuntimeFileService, *config.Config) (apiClient, error)

func defaultAPIClientFactory(fileSvc fs.RuntimeFileService, cfg *config.Config) (apiClient, error) {
	return api.NewClient(fileSvc, cfg)
}

func approveCmd() *cobra.Command {
	return approveCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func approveCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory apiClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <transaction_hash>",
		Short: "Approve a suspended L3 transaction via browser WebAuthn",
		Long: `Approve a suspended transaction by opening the gateway's browser-based approval page.
The browser handles the WebAuthn/passkey ceremony; the CLI subscribes to the
gateway's SSE stream and waits for the approval.completed event. CLI credentials
(mTLS) are required for L3 approval flows.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			txHash := args[0]
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			client, err := clientFactory(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("approve: create API client: %w", err)
			}

			// Build the browser approval URL (public endpoint, redirects to console SPA)
			approvalURL := cfg.OperatorPublicURL() + constants.APIPaths.ApprovePagePrefix + txHash

			cmd.Printf("Opening browser for WebAuthn approval...\n")
			cmd.Printf("  Transaction: %s\n", txHash)
			cmd.Printf("  URL: %s\n", approvalURL)

			if err := platform.OpenBrowser(approvalURL); err != nil {
				cmd.Printf("Failed to auto-open browser: %v\n", err)
				fmt.Fprintf(os.Stderr, "\n[g8e] Please visit: %s\n", approvalURL)
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			return waitForApprovalAndVerify(ctx, cmd, fileSvc, cfg, client, txHash)
		},
	}

	return cmd
}

// waitForApprovalAndVerify loads CLI credentials, builds an mTLS SSE client,
// waits for the approval.completed SSE event, then verifies the approval
// status via the mTLS status endpoint. CLI credentials are required — there
// is no polling fallback.
func waitForApprovalAndVerify(ctx context.Context, cmd *cobra.Command, fileSvc fs.RuntimeFileService, cfg *config.Config, client apiClient, txHash string) error {
	creds, err := auth.LoadCredentials(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("approve: load credentials: %w", err)
	}
	if creds == nil || creds.UserID == "" {
		return fmt.Errorf("approve: %w", constants.ErrNotAuthenticated)
	}

	sseClient, err := auth.BuildMTLSClient(fileSvc, cfg, 0)
	if err != nil {
		return fmt.Errorf("approve: build mTLS client: %w", err)
	}

	cmd.Printf("\nWaiting for browser approval (SSE)...\n")
	if err := auth.WaitForApprovalSSE(ctx, sseClient, cfg.OperatorPublicURL(), creds.UserID, txHash); err != nil {
		return fmt.Errorf("approve: %w", err)
	}

	statusPath := constants.APIPaths.ApprovalsCLIStatus + txHash
	resp, err := client.Get(statusPath)
	if err != nil {
		return fmt.Errorf("approve: verify status: %w", err)
	}

	var status models.ApprovalStatusResponse
	if err := json.Unmarshal(resp, &status); err != nil {
		return fmt.Errorf("approve: parse status response: %w", err)
	}

	switch status.Status {
	case string(constants.SuspendedTxStatusApproved):
		cmd.Printf("\n✓ Transaction %s approved successfully\n", txHash)
		if status.ToolName != "" {
			cmd.Printf("  Tool: %s\n", status.ToolName)
		}
		return nil
	case string(constants.SuspendedTxStatusExpiredOrNotFound):
		return fmt.Errorf("approve: transaction %s expired or not found", txHash)
	default:
		return fmt.Errorf("approve: unexpected status %q for transaction %s", status.Status, txHash)
	}
}
