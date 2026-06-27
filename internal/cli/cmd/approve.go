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
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
)

type apiClient interface {
	Get(path string) ([]byte, error)
	Post(path string, body interface{}) ([]byte, error)
	Put(path string, body interface{}) ([]byte, error)
	Delete(path string) ([]byte, error)
}

type apiClientFactory func(*config.Config) (apiClient, error)

func defaultAPIClientFactory(cfg *config.Config) (apiClient, error) {
	return api.NewClient(cfg)
}

var (
	approvePollInterval  = 3 * time.Second
	approveMaxIterations = 100 // 5 minutes at 3s intervals
)

func approveCmd() *cobra.Command {
	return approveCmdWithConfig(loadConfig, defaultAPIClientFactory)
}

func approveCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <transaction_hash>",
		Short: "Approve a suspended L3 transaction via browser WebAuthn",
		Long: `Approve a suspended transaction by opening the gateway's browser-based approval page.
The browser handles the WebAuthn/passkey ceremony; the CLI polls the gateway's mTLS
status endpoint until the transaction is approved or times out.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			txHash := args[0]
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			client, err := clientFactory(cfg)
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

			cmd.Printf("\nWaiting for browser approval (polling gateway via mTLS)...\n")

			statusPath := constants.APIPaths.ApprovalsCLIStatus + txHash
			ticker := time.NewTicker(approvePollInterval)
			defer ticker.Stop()

			type statusResponse struct {
				Status   string `json:"status"`
				TxHash   string `json:"tx_hash,omitempty"`
				ToolName string `json:"tool_name,omitempty"`
			}

			for i := 0; i < approveMaxIterations; i++ {
				<-ticker.C
				resp, err := client.Get(statusPath)
				if err != nil {
					continue
				}

				var status statusResponse
				if err := json.Unmarshal(resp, &status); err != nil {
					continue
				}

				switch status.Status {
				case "approved":
					cmd.Printf("\n✓ Transaction %s approved successfully\n", txHash)
					if status.ToolName != "" {
						cmd.Printf("  Tool: %s\n", status.ToolName)
					}
					return nil
				case "expired_or_not_found":
					return fmt.Errorf("approve: transaction %s expired or not found", txHash)
				case "pending":
					if i%10 == 0 && i > 0 {
						cmd.Printf("  Still waiting... (%ds elapsed)\n", i*3)
					}
					continue
				}
			}

			return fmt.Errorf("approve: timed out waiting for browser approval after %d seconds", approveMaxIterations*3)
		},
	}

	return cmd
}
