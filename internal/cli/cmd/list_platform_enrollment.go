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
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func listPlatformEnrollmentCmd() *cobra.Command {
	return listPlatformEnrollmentCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func listPlatformEnrollmentCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory apiClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List completed platform workload enrollments via mTLS",
		Long: `List completed and revoked platform workload enrollments (dashboard,
ensemble, or operator).

The command uses the local enrolled CLI identity (mTLS) to fetch the
authenticated enrolled list from the gateway. The output includes enrollment
request IDs, component kind, instance ID, hostname, state, and issued
identity metadata — never requester tokens, token hashes, CSR PEM, or
certificates.

Use the request ID with 'g8e auth enroll revoke <request-id>' to revoke a
completed enrollment.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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
				return fmt.Errorf("list: create API client: %w", err)
			}

			respBody, err := client.Get(constants.APIPaths.AuthPlatformEnrollmentEnrolled)
			if err != nil {
				return fmt.Errorf("list: fetch enrolled list: %w", err)
			}

			var resp models.PlatformEnrollmentEnrolledResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				return fmt.Errorf("list: parse response: %w", err)
			}

			if len(resp.Enrollments) == 0 {
				if output.JSONEnabled(cmd) {
					return output.WriteJSON(cmd.OutOrStdout(), resp)
				}
				cmd.Printf("No completed platform enrollment requests.\n")
				return nil
			}

			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), resp)
			}

			cmd.Printf("Platform Enrollments (%d total)\n", len(resp.Enrollments))
			cmd.Println(strings.Repeat("=", 160))
			cmd.Printf("  %-36s  %-10s  %-24s  %-24s  %-10s  %-36s\n",
				"Request ID", "Component", "Instance ID", "Hostname", "State", "Issued Identity")
			cmd.Println(strings.Repeat("-", 160))
			for _, enrollment := range resp.Enrollments {
				cmd.Printf("  %-36s  %-10s  %-24s  %-24s  %-10s  %-36s\n",
					enrollment.RequestID,
					string(enrollment.ComponentKind),
					enrollment.InstanceID,
					enrollment.Hostname,
					string(enrollment.State),
					platformEnrollmentIssuedIdentity(enrollment),
				)
			}
			return nil
		},
	}
	return cmd
}

func platformEnrollmentIssuedIdentity(enrollment models.PlatformEnrollmentEnrolledRequest) string {
	switch enrollment.ComponentKind {
	case models.PlatformComponentOperator:
		if enrollment.OperatorID != "" {
			return enrollment.OperatorID
		}
		return enrollment.OperatorSessionID
	default:
		return enrollment.PolicyID
	}
}
