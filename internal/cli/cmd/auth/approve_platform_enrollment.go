// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package authcmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

type platformEnrollmentDecisionSpec struct {
	verb       string
	promptVerb string
	decision   models.PlatformEnrollmentDecision
	short      string
	long       string
}

func approvePlatformEnrollmentCmd() *cobra.Command {
	return platformEnrollmentDecisionCmdWithConfig(
		platformEnrollmentDecisionSpec{
			verb:       "approve",
			promptVerb: "Approve",
			decision:   models.PlatformEnrollmentDecisionApprove,
			short:      "Approve pending platform workload enrollment requests via mTLS",
			long: `Approve one or more pending platform workload enrollment requests (dashboard,
ensemble, or operator) from the authenticated CLI identity (mTLS).

Name requests with any mix of space-separated selectors, each matched against
the pending list by request ID, instance ID, or hostname (case-insensitive). A
selector that matches several requests (for example a hostname running both an
operator and an ensemble) approves all of them. Use --all to approve every
pending request. Every selector must match a pending request or nothing is
approved.

The command fetches the pending list to display the component kind, hostname,
instance ID, CSR fingerprints, creation time, and expiry of every matched
request, then asks for a single confirmation before posting one atomic batch decision,
unless --yes is supplied for non-interactive automation. The approver must hold
a valid, non-revoked CLI certificate bound to the active first user (the
persistent owner); the gateway enforces this server-side.

Examples:
  g8e auth enroll approve 4d669cdb-34b6-4977-b7f5-175ba2c762b7 29ed21d0-d495-471b-8126-000b5bd76146
  g8e auth enroll approve web-01.example.com
  g8e auth enroll approve i-0abc123def456 --yes
  g8e auth enroll approve --all --yes

Use --reason to attach an optional bounded approval note (max ` + fmt.Sprintf("%d", constants.PlatformEnrollmentMaxReasonBytes) + ` bytes).

The request body binds the selected request IDs, displayed fingerprints, typed
decision, and optional reason — never a user ID or requester token. The requester token is held only by
the requesting workload and is never exposed through this command.`,
		},
		shared.LoadConfig, DefaultAPIClientFactory, shared.NewFileSvc,
	)
}

func denyPlatformEnrollmentCmd() *cobra.Command {
	return platformEnrollmentDecisionCmdWithConfig(
		platformEnrollmentDecisionSpec{
			verb:       "deny",
			promptVerb: "Deny",
			decision:   models.PlatformEnrollmentDecisionDeny,
			short:      "Deny pending platform workload enrollment requests via mTLS",
			long: `Deny one or more pending platform workload enrollment requests (dashboard,
ensemble, or operator) from the authenticated CLI identity (mTLS).

Selectors work as in 'g8e auth enroll approve': space-separated request IDs,
instance IDs, or hostnames, or --all for every pending request. Every selector
must match a pending request or nothing is denied.

The command fetches the pending list to display the component kind, hostname,
instance ID, CSR fingerprints, creation time, and expiry of every matched
request, then asks for a single confirmation before posting one atomic batch decision,
unless --yes is supplied for non-interactive automation. The approver must hold
a valid, non-revoked CLI certificate bound to the active first user (the
persistent owner); the gateway enforces this server-side.

Use --reason to attach an optional bounded denial note (max ` + fmt.Sprintf("%d", constants.PlatformEnrollmentMaxReasonBytes) + ` bytes).

The request body binds the selected request IDs, displayed fingerprints, typed
decision, and optional reason — never a user ID or requester token. The requester token is held only by
the requesting workload and is never exposed through this command.`,
		},
		shared.LoadConfig, DefaultAPIClientFactory, shared.NewFileSvc,
	)
}

func revokePlatformEnrollmentCmd() *cobra.Command {
	return revokePlatformEnrollmentCmdWithConfig(shared.LoadConfig, DefaultAPIClientFactory, shared.NewFileSvc)
}

func revokePlatformEnrollmentCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var reason string
	var yes bool
	cmd := &cobra.Command{
		Use:   "revoke <request-id>",
		Short: "Revoke a completed platform workload enrollment via mTLS",
		Long:  "Revoke a completed dashboard, ensemble, or operator enrollment by its enrollment request ID. The gateway revokes issued certificates and disables the corresponding policy and sessions immediately.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			request := models.PlatformEnrollmentRevokeRequest{RequestID: strings.TrimSpace(args[0]), Reason: strings.TrimSpace(reason)}
			if err := request.Validate(); err != nil {
				return fmt.Errorf("enroll revoke: %w", err)
			}
			if !yes {
				cmd.Printf("Revoke platform enrollment %s? (y/N): ", request.RequestID)
				response, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				response = strings.TrimSpace(strings.ToLower(response))
				if response != "y" && response != "yes" {
					cmd.Println("Aborted.")
					return nil
				}
			}
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
				return fmt.Errorf("enroll revoke: create API client: %w", err)
			}
			body, err := client.Post(constants.APIPaths.AuthPlatformEnrollmentRevoke, request)
			if err != nil {
				return fmt.Errorf("enroll revoke: post revocation: %w", err)
			}
			response, err := auth.DecodePlatformEnrollmentRevoke(body)
			if err != nil {
				return fmt.Errorf("enroll revoke: %w", err)
			}
			cmd.Printf("Platform enrollment %s revoked (%s).\n", response.RequestID, response.ComponentKind)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Optional bounded revocation reason")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive confirmation prompt")
	return cmd
}

func approvePlatformEnrollmentCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	return platformEnrollmentDecisionCmdWithConfig(
		platformEnrollmentDecisionSpec{
			verb:       "approve",
			promptVerb: "Approve",
			decision:   models.PlatformEnrollmentDecisionApprove,
			short:      "Approve a pending platform workload enrollment request via mTLS",
			long:       "Approve a pending platform workload enrollment request via mTLS.",
		},
		configLoader, clientFactory, fileSvcFactory,
	)
}

func denyPlatformEnrollmentCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	return platformEnrollmentDecisionCmdWithConfig(
		platformEnrollmentDecisionSpec{
			verb:       "deny",
			promptVerb: "Deny",
			decision:   models.PlatformEnrollmentDecisionDeny,
			short:      "Deny a pending platform workload enrollment request via mTLS",
			long:       "Deny a pending platform workload enrollment request via mTLS.",
		},
		configLoader, clientFactory, fileSvcFactory,
	)
}

func platformEnrollmentDecisionCmdWithConfig(
	spec platformEnrollmentDecisionSpec,
	configLoader func(string) (*config.Config, error),
	clientFactory APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var (
		reason string
		yes    bool
		all    bool
	)
	cmd := &cobra.Command{
		Use:   spec.verb + " <request-id|instance-id|hostname>... | --all",
		Short: spec.short,
		Long:  spec.long,
		Args: func(cmd *cobra.Command, args []string) error {
			if all && len(args) > 0 {
				return fmt.Errorf("enroll %s: --all cannot be combined with explicit selectors", spec.verb)
			}
			if !all && len(args) == 0 {
				return fmt.Errorf("enroll %s: specify one or more request IDs, instance IDs, or hostnames, or use --all", spec.verb)
			}
			return nil
		},
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
				return fmt.Errorf("enroll %s: create API client: %w", spec.verb, err)
			}

			pendingBody, err := client.Get(constants.APIPaths.AuthPlatformEnrollmentPending)
			if err != nil {
				return fmt.Errorf("enroll %s: fetch pending list: %w", spec.verb, err)
			}

			var pendingResp models.PlatformEnrollmentPendingResponse
			if err := json.Unmarshal(pendingBody, &pendingResp); err != nil {
				return fmt.Errorf("enroll %s: parse pending list: %w", spec.verb, err)
			}

			var targets []models.PlatformEnrollmentPendingRequest
			if all {
				targets = pendingResp.Requests
				if len(targets) == 0 {
					cmd.Printf("No pending platform enrollment requests.\n")
					return nil
				}
			} else {
				var unmatched []string
				targets, unmatched = selectPendingRequests(pendingResp.Requests, args)
				if len(unmatched) > 0 {
					return fmt.Errorf("enroll %s: %w: %s", spec.verb, constants.ErrPlatformEnrollmentRequestNotFound, strings.Join(unmatched, ", "))
				}
			}

			decision := models.PlatformEnrollmentBatchDecisionRequest{Decision: spec.decision, Reason: reason}
			for _, target := range targets {
				decision.Requests = append(decision.Requests, models.PlatformEnrollmentDecisionTarget{RequestID: target.RequestID, Fingerprints: target.Fingerprints})
			}
			if err := decision.Validate(); err != nil {
				return fmt.Errorf("enroll %s: %w", spec.verb, err)
			}

			for i := range targets {
				PrintPlatformEnrollmentRequestDetails(cmd, &targets[i])
				cmd.Printf("\n")
			}

			if !yes {
				reader := bufio.NewReader(cmd.InOrStdin())
				if len(targets) == 1 {
					cmd.Printf("%s this platform enrollment request? (y/N): ", spec.promptVerb)
				} else {
					cmd.Printf("%s these %d platform enrollment requests? (y/N): ", spec.promptVerb, len(targets))
				}
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(strings.ToLower(response))
				if response != "y" && response != "yes" {
					cmd.Printf("Aborted.\n")
					return nil
				}
			}

			resp, err := PostPlatformEnrollmentBatchDecision(client, decision)
			if err != nil {
				return fmt.Errorf("enroll %s: %w", spec.verb, err)
			}
			for _, result := range resp.Requests {
				cmd.Printf("Platform enrollment request %s %s.\n", result.RequestID, result.State)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "",
		"Optional bounded note attached to the decision (max "+fmt.Sprintf("%d", constants.PlatformEnrollmentMaxReasonBytes)+" bytes).")
	cmd.Flags().BoolVar(&yes, "yes", false,
		"Skip the interactive confirmation prompt (non-interactive automation).")
	cmd.Flags().BoolVar(&all, "all", false,
		"Apply the decision to every pending request instead of naming selectors.")
	return cmd
}

// PostPlatformEnrollmentDecision posts an owner decision for a pending platform
// enrollment request and returns the gateway response.
func PostPlatformEnrollmentDecision(client APIClient, decisionReq models.PlatformEnrollmentDecisionRequest) (*models.PlatformEnrollmentDecisionResponse, error) {
	if err := decisionReq.Validate(); err != nil {
		return nil, fmt.Errorf("validate decision: %w", err)
	}
	respBody, err := client.Post(constants.APIPaths.AuthPlatformEnrollmentDecision, decisionReq)
	if err != nil {
		return nil, fmt.Errorf("post decision: %w", err)
	}
	return auth.DecodePlatformEnrollmentDecision(respBody)
}

// PostPlatformEnrollmentBatchDecision sends one atomic, governed decision.
func PostPlatformEnrollmentBatchDecision(client APIClient, req models.PlatformEnrollmentBatchDecisionRequest) (*models.PlatformEnrollmentBatchDecisionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate batch decision: %w", err)
	}
	body, err := client.Post(constants.APIPaths.AuthPlatformEnrollmentBatchDecision, req)
	if err != nil {
		return nil, fmt.Errorf("post batch decision: %w", err)
	}
	var resp models.PlatformEnrollmentBatchDecisionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode batch decision: %w", err)
	}
	if len(resp.Requests) != len(req.Requests) || resp.ReceiptID == "" {
		return nil, constants.ErrPlatformEnrollmentInvalidDecision
	}
	for i, result := range resp.Requests {
		expected := models.PlatformEnrollmentStateApproved
		if req.Decision == models.PlatformEnrollmentDecisionDeny {
			expected = models.PlatformEnrollmentStateDenied
		}
		if result.RequestID != req.Requests[i].RequestID || result.State != expected {
			return nil, constants.ErrPlatformEnrollmentInvalidDecision
		}
	}
	return &resp, nil
}

// selectPendingRequests resolves each selector against the pending list by exact
// request ID, exact instance ID, or case-insensitive hostname. A selector that
// matches several requests (for example a hostname running both an operator and
// an ensemble) selects all of them. Results are de-duplicated and keep pending
// list order. Selectors that match nothing are returned in unmatched (they may
// have been decided, expired, or completed since the list was fetched).
func selectPendingRequests(requests []models.PlatformEnrollmentPendingRequest, selectors []string) (selected []models.PlatformEnrollmentPendingRequest, unmatched []string) {
	chosen := make(map[string]struct{}, len(requests))
	for _, selector := range selectors {
		selector = strings.TrimSpace(selector)
		matched := false
		for i := range requests {
			req := &requests[i]
			if req.RequestID == selector || req.InstanceID == selector || strings.EqualFold(req.Hostname, selector) {
				chosen[req.RequestID] = struct{}{}
				matched = true
			}
		}
		if !matched {
			unmatched = append(unmatched, selector)
		}
	}
	for _, req := range requests {
		if _, ok := chosen[req.RequestID]; ok {
			selected = append(selected, req)
		}
	}
	return selected, unmatched
}

// printPlatformEnrollmentRequestDetails displays the owner-visible metadata for
// a pending platform enrollment request: component kind, instance ID, hostname,
// system fingerprint (if present), state, creation time, expiry, and CSR
// fingerprints. It never prints requester tokens, CSR PEM, or certificates.
func PrintPlatformEnrollmentRequestDetails(cmd *cobra.Command, req *models.PlatformEnrollmentPendingRequest) {
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

	fingerprints := auth.PlatformEnrollmentFingerprints(req.Fingerprints)
	if len(fingerprints) > 0 {
		cmd.Printf("  Key fingerprints (compare with the workload output):\n")
		for _, fp := range fingerprints {
			cmd.Printf("    %s: %s\n", fp.Label, fp.Value)
		}
	}
}
