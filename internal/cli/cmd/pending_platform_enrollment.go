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

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func pendingPlatformEnrollmentCmd() *cobra.Command {
	return pendingPlatformEnrollmentCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func pendingPlatformEnrollmentCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory apiClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pending-platform-enrollments",
		Short: "List pending platform workload enrollment requests via mTLS",
		Long: `List pending platform workload enrollment requests (dashboard, ensemble, or
operator) awaiting an owner decision.

The command uses the local enrolled CLI identity (mTLS) to fetch the
authenticated pending list from the gateway. The output includes request IDs,
component kind, instance ID, hostname, state, creation time, and expiry — never
requester tokens, token hashes, CSR PEM, or certificates.

Use the request ID with 'g8e auth approve-platform-enrollment <request-id>' to
approve or deny a specific request.`,
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
				return fmt.Errorf("pending-platform-enrollments: create API client: %w", err)
			}

			respBody, err := client.Get(constants.APIPaths.AuthPlatformEnrollmentPending)
			if err != nil {
				return fmt.Errorf("pending-platform-enrollments: fetch pending list: %w", err)
			}

			var resp models.PlatformEnrollmentPendingResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				return fmt.Errorf("pending-platform-enrollments: parse response: %w", err)
			}

			if len(resp.Requests) == 0 {
				cmd.Printf("No pending platform enrollment requests.\n")
				return nil
			}

			cmd.Printf("Pending Platform Enrollment Requests (%d):\n\n", len(resp.Requests))
			for i, req := range resp.Requests {
				cmd.Printf("[%d] Request ID:    %s\n", i+1, req.RequestID)
				cmd.Printf("    Component:     %s (%s)\n", string(req.ComponentKind), req.ComponentName)
				cmd.Printf("    Instance ID:   %s\n", req.InstanceID)
				cmd.Printf("    Hostname:      %s\n", req.Hostname)
				if req.SystemFingerprint != "" {
					cmd.Printf("    System FP:     %s\n", req.SystemFingerprint)
				}
				cmd.Printf("    State:         %s\n", string(req.State))
				cmd.Printf("    Created:       %s\n", req.CreatedAt.Format("2006-01-02 15:04:05 MST"))
				cmd.Printf("    Expires:       %s\n", req.ExpiresAt.Format("2006-01-02 15:04:05 MST"))
				fingerprints := nonEmptyFingerprints(req.Fingerprints)
				if len(fingerprints) > 0 {
					cmd.Printf("    Key fingerprints:\n")
					for _, fp := range fingerprints {
						cmd.Printf("      %s: %s\n", fp.label, fp.value)
					}
				}
				cmd.Printf("\n")
			}
			return nil
		},
	}
	return cmd
}
