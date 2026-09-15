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
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// evalCampaignCmd returns the production `eval campaign` command tree
// with real dependencies.
func evalCampaignCmd() *cobra.Command {
	return evalCampaignCmdWithDeps(evalStartDepsFromLeaseDeps())
}

// evalCampaignCmdWithDeps returns the `eval campaign` command tree wired
// with the supplied dependencies for testability. The draft subcommand
// uses the draft subset of the lease deps; the start subcommand uses the
// full lease deps for lease verification and engine invocation.
func evalCampaignCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "campaign",
		Short: "Campaign lifecycle (draft, plan, check, start, status, stop, verify, publish)",
		Long: `campaign owns the authoritative multi-arm, multi-cohort campaign
lifecycle. A campaign creates one campaign identity, one report
directory, one assignment manifest, one randomized schedule, and one
final canonical analysis.

Draft creates the typed config. Plan and check are provider-free; check
requires an active lease and validates its exact candidate and inventory
bindings. Start is the only provider-backed campaign operation.`,
	}
	draftDeps := evalDraftDeps{
		configLoader:   deps.configLoader,
		stat:           deps.stat,
		runner:         deps.runner,
		tempFileWriter: deps.tempFileWriter,
	}
	cmd.AddCommand(
		evalCampaignDraftCmdWithDeps(draftDeps),
		evalCampaignPlanCmdWithDeps(deps),
		evalCampaignCheckCmdWithDeps(deps),
		evalCampaignStartCmdWithDeps(deps),
		evalCampaignStatusCmdWithDeps(deps),
		evalCampaignStopCmdWithDeps(deps),
		evalCampaignVerifyCmdWithDeps(deps),
	)
	return cmd
}

// evalCampaignDraftRequest is the typed JSON request the Go facade sends
// to the Python draft module for a campaign draft.
type evalCampaignDraftRequest struct {
	Kind             string                      `json:"kind"`
	Preset           string                      `json:"preset"`
	OperationID      string                      `json:"operation_id"`
	Revision         string                      `json:"revision"`
	ReportRoot       string                      `json:"report_root"`
	GoldSet          evalAuthorityRefJSON        `json:"gold_set"`
	EvidenceKey      evalEvidenceKeyRefJSON      `json:"evidence_key"`
	ProviderEndpoint evalProviderEndpointRefJSON `json:"provider_endpoint"`
	CampaignID       string                      `json:"campaign_id"`
	ReleaseVersion   string                      `json:"release_version"`
	Preregistration  evalAuthorityRefJSON        `json:"preregistration"`
	Profile          evalAuthorityRefJSON        `json:"profile"`
	ModelRegistry    evalAuthorityRefJSON        `json:"model_registry"`
	CohortIDs        []string                    `json:"cohort_ids"`
	ModelTags        *evalAuthorityRefJSON       `json:"model_tags,omitempty"`
	CampaignSetPlan  *evalAuthorityRefJSON       `json:"campaign_set_plan,omitempty"`
	ReplacementRule  *evalAuthorityRefJSON       `json:"replacement_rule,omitempty"`
	OutputPath       string                      `json:"output_path"`
}

func evalCampaignDraftCmdWithDeps(deps evalDraftDeps) *cobra.Command {
	var preset, outPath, operationID, revision, reportRoot string
	var goldSetPath, goldSetSHA256, evidenceKeyPath, evidenceKeyID string
	var provider, endpointClass string
	var campaignID, releaseVersion string
	var preregPath, preregSHA256, profilePath, profileSHA256, registryPath, registrySHA256 string
	var cohortIDsStr string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "draft",
		Short: "Create a new campaign config from a preset (read-only; creates new config only)",
		Long: `draft creates a new campaign config at a path that does not exist.
It consumes a repository-owned named preset and explicit concise overrides.
Draft generation performs no provider call, model pull, report-root
creation, lease issuance, or publication.

The generated config has no active lease and cannot be started; lease
lifecycle remains a separate typed record so the content-addressed config
does not mutate during authorization.

This command is read-only with respect to existing state; it creates a
new config file only.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := commandContext(cmd)
			cohortIDs := strings.Split(cohortIDsStr, ",")
			for i, c := range cohortIDs {
				cohortIDs[i] = strings.TrimSpace(c)
			}
			req := evalCampaignDraftRequest{
				Kind:             "campaign",
				Preset:           preset,
				OperationID:      operationID,
				Revision:         revision,
				ReportRoot:       reportRoot,
				GoldSet:          evalAuthorityRefJSON{Path: goldSetPath, SHA256: goldSetSHA256},
				EvidenceKey:      evalEvidenceKeyRefJSON{Path: evidenceKeyPath, KeyID: evidenceKeyID},
				ProviderEndpoint: evalProviderEndpointRefJSON{Provider: provider, EndpointClass: endpointClass},
				CampaignID:       campaignID,
				ReleaseVersion:   releaseVersion,
				Preregistration:  evalAuthorityRefJSON{Path: preregPath, SHA256: preregSHA256},
				Profile:          evalAuthorityRefJSON{Path: profilePath, SHA256: profileSHA256},
				ModelRegistry:    evalAuthorityRefJSON{Path: registryPath, SHA256: registrySHA256},
				CohortIDs:        cohortIDs,
				OutputPath:       outPath,
			}
			requestJSON, err := json.Marshal(req)
			if err != nil {
				return fmt.Errorf("%w: marshal draft request: %w", constants.ErrEvalConfigInvalid, err)
			}
			summary, err := runEvalDraft(ctx, deps, requestJSON, cmd.OutOrStderr())
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitEvalJSON(cmd, summary)
			}
			printEvalDraftSummaryHuman(cmd.OutOrStdout(), summary)
			return nil
		},
	}

	cmd.Flags().StringVar(&preset, "preset", "", "Repository-owned preset name (required)")
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "Output path for the new config (required; must not exist)")
	cmd.Flags().StringVar(&operationID, "operation-id", "", "Stable operation ID (required)")
	cmd.Flags().StringVar(&revision, "revision", "rev-1", "Fresh revision identifier")
	cmd.Flags().StringVar(&reportRoot, "report-root", "", "Repository-relative report root (required)")
	cmd.Flags().StringVar(&goldSetPath, "gold-set-path", "", "Repository-relative path to the gold set authority (required)")
	cmd.Flags().StringVar(&goldSetSHA256, "gold-set-sha256", "", "SHA-256 of the gold set authority (required)")
	cmd.Flags().StringVar(&evidenceKeyPath, "evidence-key-path", "", "Repository-relative path to the evidence key (required)")
	cmd.Flags().StringVar(&evidenceKeyID, "evidence-key-id", "", "Expected evidence key ID (required)")
	cmd.Flags().StringVar(&provider, "provider", "", "Provider name (required)")
	cmd.Flags().StringVar(&endpointClass, "endpoint-class", "", "Provider endpoint class (required)")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Campaign ID (required)")
	cmd.Flags().StringVar(&releaseVersion, "release-version", "", "Release version (required)")
	cmd.Flags().StringVar(&preregPath, "preregistration-path", "", "Repository-relative path to the preregistration authority (required)")
	cmd.Flags().StringVar(&preregSHA256, "preregistration-sha256", "", "SHA-256 of the preregistration authority (required)")
	cmd.Flags().StringVar(&profilePath, "profile-path", "", "Repository-relative path to the campaign profile authority (required)")
	cmd.Flags().StringVar(&profileSHA256, "profile-sha256", "", "SHA-256 of the campaign profile authority (required)")
	cmd.Flags().StringVar(&registryPath, "model-registry-path", "", "Repository-relative path to the model registry authority (required)")
	cmd.Flags().StringVar(&registrySHA256, "model-registry-sha256", "", "SHA-256 of the model registry authority (required)")
	cmd.Flags().StringVar(&cohortIDsStr, "cohort-ids", "", "Comma-separated cohort IDs (required)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	_ = cmd.MarkFlagRequired("preset")
	_ = cmd.MarkFlagRequired("out")
	_ = cmd.MarkFlagRequired("operation-id")
	_ = cmd.MarkFlagRequired("report-root")
	_ = cmd.MarkFlagRequired("gold-set-path")
	_ = cmd.MarkFlagRequired("gold-set-sha256")
	_ = cmd.MarkFlagRequired("evidence-key-path")
	_ = cmd.MarkFlagRequired("evidence-key-id")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("endpoint-class")
	_ = cmd.MarkFlagRequired("campaign-id")
	_ = cmd.MarkFlagRequired("release-version")
	_ = cmd.MarkFlagRequired("preregistration-path")
	_ = cmd.MarkFlagRequired("preregistration-sha256")
	_ = cmd.MarkFlagRequired("profile-path")
	_ = cmd.MarkFlagRequired("profile-sha256")
	_ = cmd.MarkFlagRequired("model-registry-path")
	_ = cmd.MarkFlagRequired("model-registry-sha256")
	_ = cmd.MarkFlagRequired("cohort-ids")
	return cmd
}

// evalCampaignStartCmdWithDeps returns the `eval campaign start` command
// wired with the supplied dependencies. Start is provider-backed and
// requires a valid active lease; it routes through runEvalStart so lease
// verification cannot be bypassed.
func evalCampaignStartCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	var jsonOutput, verbose, yes bool

	cmd := &cobra.Command{
		Use:   "start <config>",
		Short: "Start a provider-backed campaign run (requires lease)",
		Long: `start launches a multi-arm, multi-cohort campaign run against the
provider. It is provider-backed: it requires a valid active lease bound
to the exact typed request digest of the supplied operation config.
Issue a lease with 'eval lease issue' before start.

The command verifies the lease before report-root creation or engine
launch. A missing, inactive, expired, consumed, or mismatched lease
fails closed with a typed error. After the engine reaches a terminal
state the lease is transitioned to completed (success) or stopped
(failure/interruption).

This command requires --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("%w: --yes is required for campaign start", constants.ErrEvalLeaseMissing)
			}
			ctx := commandContext(cmd)
			result, err := runEvalStart(ctx, deps, args[0], models.EvalOperationCampaignStart, evalCommandFamilyCampaignRun, jsonOutput, verbose, cmd.OutOrStdout(), cmd.OutOrStderr())
			if err != nil {
				if jsonOutput {
					return emitEvalJSON(cmd, result)
				}
				return err
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalStartHuman(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the provider-backed launch")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Add authority and per-assignment detail to human output")
	return cmd
}
