// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func approvePlatformEnrollmentCmd() *cobra.Command {
	return approvePlatformEnrollmentCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func approvePlatformEnrollmentCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory apiClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var (
		deny   bool
		reason string
		yes    bool
	)
	cmd := &cobra.Command{
		Use:   "approve-platform-enrollment <request-id>",
		Short: "Approve or deny a pending platform workload enrollment request via mTLS",
		Long: `Approve or deny a pending platform workload enrollment request (dashboard,
ensemble, or operator) from the authenticated CLI identity (mTLS).

The command fetches the pending list to display the component kind, hostname,
instance ID, CSR fingerprints, creation time, and expiry before posting the
decision, unless --yes is supplied for non-interactive automation. The approver
must hold a valid, non-revoked CLI certificate bound to the active first user
(the persistent owner); the gateway enforces this server-side.

Use --deny to reject the request instead of approving it. Use --reason to attach
an optional bounded denial or approval note (max ` + fmt.Sprintf("%d", constants.PlatformEnrollmentMaxReasonBytes) + ` bytes).

The request body carries only the request ID, typed decision, and optional
reason — never a user ID or requester token. The requester token is held only by
the requesting workload and is never exposed through this command.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			requestID := args[0]
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
				return fmt.Errorf("approve-platform-enrollment: create API client: %w", err)
			}

			pendingBody, err := client.Get(constants.APIPaths.AuthPlatformEnrollmentPending)
			if err != nil {
				return fmt.Errorf("approve-platform-enrollment: fetch pending list: %w", err)
			}

			var pendingResp models.PlatformEnrollmentPendingResponse
			if err := json.Unmarshal(pendingBody, &pendingResp); err != nil {
				return fmt.Errorf("approve-platform-enrollment: parse pending list: %w", err)
			}

			req := findPendingRequest(pendingResp.Requests, requestID)
			if req == nil {
				return fmt.Errorf("approve-platform-enrollment: %w: %s", constants.ErrPlatformEnrollmentRequestNotFound, requestID)
			}

			printPlatformEnrollmentRequestDetails(cmd, req)

			decision := models.PlatformEnrollmentDecisionApprove
			actionWord := "Approve"
			if deny {
				decision = models.PlatformEnrollmentDecisionDeny
				actionWord = "Deny"
			}

			if !yes {
				reader := bufio.NewReader(os.Stdin)
				fmt.Printf("\n%s this platform enrollment request? (y/N): ", actionWord)
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(strings.ToLower(response))
				if response != "y" && response != "yes" {
					cmd.Printf("Aborted.\n")
					return nil
				}
			}

			decisionReq := models.PlatformEnrollmentDecisionRequest{
				RequestID: requestID,
				Decision:  decision,
				Reason:    reason,
			}
			if err := decisionReq.Validate(); err != nil {
				return fmt.Errorf("approve-platform-enrollment: %w", err)
			}

			respBody, err := client.Post(
				constants.APIPaths.AuthPlatformEnrollmentDecision,
				decisionReq,
			)
			if err != nil {
				return fmt.Errorf("approve-platform-enrollment: post decision: %w", err)
			}

			var resp models.PlatformEnrollmentDecisionResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				return fmt.Errorf("approve-platform-enrollment: parse response: %w", err)
			}

			cmd.Printf("Platform enrollment request %s.\n", string(resp.State))
			return nil
		},
	}

	cmd.Flags().BoolVar(&deny, "deny", false,
		"Deny the platform enrollment request instead of approving it.")
	cmd.Flags().StringVar(&reason, "reason", "",
		"Optional bounded note attached to the decision (max "+fmt.Sprintf("%d", constants.PlatformEnrollmentMaxReasonBytes)+" bytes).")
	cmd.Flags().BoolVar(&yes, "yes", false,
		"Skip the interactive confirmation prompt (non-interactive automation).")
	return cmd
}

// findPendingRequest looks up a request ID in the pending list. Returns nil if
// the request is not found (it may have been decided, expired, or completed
// since the pending list was last fetched).
func findPendingRequest(requests []models.PlatformEnrollmentPendingRequest, requestID string) *models.PlatformEnrollmentPendingRequest {
	for i := range requests {
		if requests[i].RequestID == requestID {
			return &requests[i]
		}
	}
	return nil
}

// printPlatformEnrollmentRequestDetails displays the owner-visible metadata for
// a pending platform enrollment request: component kind, instance ID, hostname,
// system fingerprint (if present), state, creation time, expiry, and CSR
// fingerprints. It never prints requester tokens, CSR PEM, or certificates.
func printPlatformEnrollmentRequestDetails(cmd *cobra.Command, req *models.PlatformEnrollmentPendingRequest) {
	cmd.Printf("Platform Enrollment Request\n")
	cmd.Printf("  Request ID:    %s\n", req.RequestID)
	cmd.Printf("  Component:     %s (%s)\n", string(req.ComponentKind), req.ComponentName)
	cmd.Printf("  Instance ID:   %s\n", req.InstanceID)
	cmd.Printf("  Hostname:      %s\n", req.Hostname)
	if req.SystemFingerprint != "" {
		cmd.Printf("  System FP:     %s\n", req.SystemFingerprint)
	}
	cmd.Printf("  State:         %s\n", string(req.State))
	cmd.Printf("  Created:       %s\n", req.CreatedAt.Format("2006-01-02 15:04:05 MST"))
	cmd.Printf("  Expires:       %s\n", req.ExpiresAt.Format("2006-01-02 15:04:05 MST"))

	fingerprints := nonEmptyFingerprints(req.Fingerprints)
	if len(fingerprints) > 0 {
		cmd.Printf("  Key fingerprints (compare with the workload output):\n")
		for _, fp := range fingerprints {
			cmd.Printf("    %s: %s\n", fp.label, fp.value)
		}
	}
}

type fingerprintDisplay struct {
	label string
	value string
}

// nonEmptyFingerprints returns the non-empty CSR fingerprints from a pending
// request as label/value pairs for display.
func nonEmptyFingerprints(fps models.PlatformEnrollmentCSRFingerprints) []fingerprintDisplay {
	var out []fingerprintDisplay
	if fps.App != "" {
		out = append(out, fingerprintDisplay{label: "App key", value: fps.App})
	}
	if fps.Operator != "" {
		out = append(out, fingerprintDisplay{label: "Operator key", value: fps.Operator})
	}
	if fps.CLI != "" {
		out = append(out, fingerprintDisplay{label: "CLI key", value: fps.CLI})
	}
	return out
}
