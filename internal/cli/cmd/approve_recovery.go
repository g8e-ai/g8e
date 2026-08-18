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
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

func approveRecoveryCmd() *cobra.Command {
	return approveRecoveryCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func approveRecoveryCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory apiClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var deny bool
	cmd := &cobra.Command{
		Use:   "approve-recovery <token>",
		Short: "Approve or deny a pending CLI recovery request via mTLS",
		Long: `Approve or deny a pending CLI recovery request from another CLI's 'auth enroll --headless' run.

This command uses the local enrolled CLI identity (mTLS) to approve or deny the
request. It is the CLI-side counterpart to the browser Console SPA approve
action. The approver must hold a valid, non-revoked CLI certificate bound to an
active user.

Use --deny to reject the request instead of approving it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token := args[0]
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
				return fmt.Errorf("approve-recovery: create API client: %w", err)
			}

			respBody, err := client.Post(
				constants.APIPaths.AuthCLIRecoveryApproveCLI,
				models.CLIRecoveryApproveRequest{Token: token, Approve: !deny},
			)
			if err != nil {
				return fmt.Errorf("approve-recovery: post approve: %w", err)
			}

			var resp models.CLIRecoveryApproveResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				return fmt.Errorf("approve-recovery: parse response: %w", err)
			}

			switch resp.State {
			case models.CLIRecoveryStateApproved:
				cmd.Printf("Recovery request approved.\n")
				return nil
			case models.CLIRecoveryStateDenied:
				cmd.Printf("Recovery request denied.\n")
				return nil
			default:
				return fmt.Errorf("approve-recovery: %w: state %q", constants.ErrCLIRecoveryRequestFailed, resp.State)
			}
		},
	}

	cmd.Flags().BoolVar(&deny, "deny", false,
		"Deny the recovery request instead of approving it.")
	return cmd
}
