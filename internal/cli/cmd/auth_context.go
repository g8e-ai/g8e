// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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
				if err := persistClientOperatorContext(fileSvc, cfg, context); err != nil {
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

// resolveClientOperatorContext resolves the authoritative operator binding
// from the gateway's persisted CLI session record (GET
// /api/v1/auth/cli/session). The persisted binding is authoritative — it is
// stamped at session issuance and enforced by the auth middleware — so this
// endpoint, not an operator listing query, is the source of truth.
func resolveClientOperatorContext(client apiClient, context *auth.ClientAuthContext) error {
	response, err := client.Get(constants.APIPaths.AuthCLISession)
	if err != nil {
		// A stale local operator binding is rejected with 403 before the
		// session record can be returned; direct the user to the refresh
		// path that resyncs the binding.
		if errors.Is(err, constants.ErrHTTPStatusError) && strings.Contains(err.Error(), constants.ErrOperatorBindingMismatch.Error()) {
			return fmt.Errorf("%w: %s", constants.ErrNotAuthenticated, constants.ErrOperatorBindingMismatch.Error())
		}
		return fmt.Errorf("auth context: resolve CLI session: %w", err)
	}
	var info models.CLISessionInfoResponse
	if err := json.Unmarshal(response, &info); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	if info.OperatorSessionID == "" || info.OperatorID == "" {
		return fmt.Errorf("%w: CLI session has no operator binding; run './g8e auth refresh' or re-enroll with './g8e auth enroll user'", constants.ErrNotAuthenticated)
	}
	context.OperatorID = info.OperatorID
	context.OperatorSessionID = info.OperatorSessionID
	return nil
}

// persistClientOperatorContext writes the authoritative operator binding
// resolved by resolveClientOperatorContext back to the local credentials
// file so subsequent requests send matching operator headers.
func persistClientOperatorContext(fileSvc fs.RuntimeFileService, cfg *config.Config, context *auth.ClientAuthContext) error {
	creds, err := auth.LoadCredentials(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("auth context: %w", err)
	}
	if creds == nil {
		return fmt.Errorf("%w: local CLI credentials are absent; run './g8e auth enroll user'", constants.ErrNotAuthenticated)
	}
	creds.OperatorID = context.OperatorID
	creds.OperatorSessionID = context.OperatorSessionID
	if err := auth.SaveCredentials(fileSvc, cfg, creds); err != nil {
		return fmt.Errorf("auth context: save credentials: %w", err)
	}
	return nil
}
