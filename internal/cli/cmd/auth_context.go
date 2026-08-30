// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func authContextCmd() *cobra.Command {
	return authContextCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func authContextCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory apiClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var projectRoot string
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Print the canonical local CLI authentication context as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader(projectRoot)
			if err != nil {
				return err
			}
			fileSvc, err := fileSvcFactory(projectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			context, err := auth.LoadClientAuthContext(fileSvc, cfg)
			if err != nil {
				return err
			}
			if context.OperatorSessionID == "" {
				client, err := clientFactory(fileSvc, cfg)
				if err != nil {
					return err
				}
				if err := resolveClientOperatorContext(client, context); err != nil {
					return err
				}
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(context); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root containing the "+constants.RuntimeDirname+" runtime (default: current working directory)")
	return cmd
}

func resolveClientOperatorContext(client apiClient, context *auth.ClientAuthContext) error {
	query := url.Values{"user_id": {context.UserID}}
	response, err := client.Get(constants.APIPaths.Operators + "?" + query.Encode())
	if err != nil {
		return fmt.Errorf("auth context: resolve operator: %w", err)
	}
	var slots models.OperatorSlotResponse
	if err := json.Unmarshal(response, &slots); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	matches := make([]models.OperatorDocumentGo, 0, len(slots.Operators))
	for _, operator := range slots.Operators {
		if operator.OperatorSessionID == "" {
			continue
		}
		if operator.Status == constants.OperatorStatusTerminated {
			continue
		}
		if context.OperatorID == "" || operator.ID == context.OperatorID {
			matches = append(matches, operator)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf("%w: expected one active operator binding, found %d", constants.ErrNotAuthenticated, len(matches))
	}
	context.OperatorID = matches[0].ID
	context.OperatorSessionID = matches[0].OperatorSessionID
	return nil
}
