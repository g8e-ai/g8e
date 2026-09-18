// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

type operatorBindClient interface {
	Bind(ctx context.Context, fileSvc fs.RuntimeFileService, operatorSessionID string) (auth.CLISessionBind, error)
}

type operatorBindClientFactory func(cfg *config.Config) operatorBindClient

func defaultOperatorBindClientFactory(cfg *config.Config) operatorBindClient {
	return auth.NewEnrollmentClient(cfg, nil)
}

func operatorBindCmd() *cobra.Command {
	return operatorBindCmdWithConfig(loadConfig, defaultAPIClientFactory, defaultOperatorBindClientFactory, newFileSvc)
}

func operatorBindCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory apiClientFactory,
	bindClientFactory operatorBindClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "bind <operator-session-id>",
		Short: "Bind the CLI session to a specific operator session",
		Long: `Bind the authenticated CLI session to the specified operator session.

The gateway validates that the operator session belongs to the authenticated
user and is active, then persists the binding server-side. When the binding
changes, a replacement CLI session is issued and saved to local credentials.

Use './g8e operator list' to discover operator session IDs. Confirmation can be
skipped with --yes for non-interactive automation.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			operatorSessionID := strings.TrimSpace(args[0])
			if operatorSessionID == "" {
				return fmt.Errorf("%w: operator session id is required", constants.ErrGatewayOperatorSessionIDRequired)
			}

			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			creds, err := auth.LoadCredentials(fileSvc, cfg)
			if err != nil || creds == nil {
				return fmt.Errorf("%w: Please run './g8e auth enroll user' first", constants.ErrNotAuthenticated)
			}

			client, err := clientFactory(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("operator bind: create API client: %w", err)
			}

			resp, err := client.Get(constants.APIPaths.Operators + "?user_id=" + creds.UserID)
			if err != nil {
				return fmt.Errorf("operator bind: list operators: %w", err)
			}
			var slotResp models.OperatorSlotResponse
			if err := json.Unmarshal(resp, &slotResp); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}

			target := findOperatorBySessionID(slotResp.Operators, operatorSessionID)
			if target == nil {
				return fmt.Errorf("operator bind: no operator found with session id %s for user %s", operatorSessionID, creds.UserID)
			}

			if !output.JSONEnabled(cmd) {
				cmd.Printf("Operator bind target\n")
				cmd.Println(strings.Repeat("=", 72))
				cmd.Printf("  Operator ID:         %s\n", target.ID)
				cmd.Printf("  Operator session ID: %s\n", target.OperatorSessionID)
				cmd.Printf("  Type:                %s\n", target.OperatorType)
				cmd.Printf("  Status:              %s\n", target.Status)
				if target.Name != "" {
					cmd.Printf("  Name:                %s\n", target.Name)
				}
				cmd.Printf("  Current CLI session: %s\n", creds.CLISessionID)
				if creds.OperatorSessionID == operatorSessionID {
					cmd.Printf("  Current binding:     already bound to this operator session\n")
				} else if creds.OperatorSessionID != "" {
					cmd.Printf("  Current binding:     %s\n", creds.OperatorSessionID)
				}
			}

			if !yes && !output.JSONEnabled(cmd) {
				reader := bufio.NewReader(os.Stdin)
				fmt.Printf("\nBind CLI session to operator session %s? (y/N): ", operatorSessionID)
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(strings.ToLower(response))
				if response != "y" && response != "yes" {
					cmd.Println("Aborted.")
					return nil
				}
			}

			bind, err := bindClientFactory(cfg).Bind(cmd.Context(), fileSvc, operatorSessionID)
			if err != nil {
				return fmt.Errorf("operator bind: %w", err)
			}

			creds.CLISessionID = bind.CLISessionID
			creds.OperatorSessionID = bind.OperatorSessionID
			creds.OperatorID = bind.OperatorID
			if err := auth.SaveCredentials(fileSvc, cfg, creds); err != nil {
				return fmt.Errorf("operator bind: save credentials: %w", err)
			}

			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), map[string]any{
					"success":              true,
					"cli_session_id":       bind.CLISessionID,
					"user_id":              bind.UserID,
					"operator_id":          bind.OperatorID,
					"operator_session_id":  bind.OperatorSessionID,
					"already_bound":        bind.AlreadyBound,
				})
			}

			if bind.AlreadyBound {
				cmd.Printf("CLI session already bound to operator session %s\n", bind.OperatorSessionID)
			} else {
				cmd.Printf("CLI session bound to operator session %s\n", bind.OperatorSessionID)
				cmd.Printf("New CLI session ID: %s\n", bind.CLISessionID)
			}
			cmd.Printf("Operator ID: %s\n", bind.OperatorID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive confirmation prompt (non-interactive automation).")
	return cmd
}

func findOperatorBySessionID(operators []models.OperatorDocumentGo, operatorSessionID string) *models.OperatorDocumentGo {
	for i := range operators {
		if operators[i].OperatorSessionID == operatorSessionID {
			return &operators[i]
		}
	}
	return nil
}
