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

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// evalDiagnosticCmd returns the production `eval diagnostic` command tree
// with real dependencies.
func evalDiagnosticCmd() *cobra.Command {
	return evalDiagnosticCmdWithDeps(evalDraftDeps{
		configLoader:   config.Load,
		stat:           realEvalFileStat{},
		runner:         realEvalCommandRunner{},
		tempFileWriter: realEvalTempFileWriter{},
	})
}

// evalDiagnosticCmdWithDeps returns the `eval diagnostic` command tree
// wired with the supplied dependencies for testability.
func evalDiagnosticCmdWithDeps(deps evalDraftDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diagnostic",
		Short: "Single-arm diagnostic lifecycle (draft, plan, check, start, status, stop, verify)",
		Long: `diagnostic owns the single-arm diagnostic lifecycle. A diagnostic
executes one arm against one model and produces one run. It does not
create a campaign identity, assignment manifest, or randomized schedule.

U5 implements the draft subcommand. Later phases add plan, check, start,
status, stop, and verify.`,
	}
	cmd.AddCommand(evalDiagnosticDraftCmdWithDeps(deps))
	return cmd
}

// evalDiagnosticDraftRequest is the typed JSON request the Go facade
// sends to the Python draft module for a diagnostic draft.
type evalDiagnosticDraftRequest struct {
	Kind            string                 `json:"kind"`
	Preset          string                 `json:"preset"`
	OperationID     string                 `json:"operation_id"`
	Revision        string                 `json:"revision"`
	ReportRoot      string                 `json:"report_root"`
	GoldSet         evalAuthorityRefJSON   `json:"gold_set"`
	EvidenceKey     evalEvidenceKeyRefJSON  `json:"evidence_key"`
	ProviderEndpoint evalProviderEndpointRefJSON `json:"provider_endpoint"`
	ModelVariantID  string                 `json:"model_variant_id"`
	OutputPath      string                 `json:"output_path"`
}

func evalDiagnosticDraftCmdWithDeps(deps evalDraftDeps) *cobra.Command {
	var preset, outPath, operationID, revision, reportRoot string
	var goldSetPath, goldSetSHA256, evidenceKeyPath, evidenceKeyID string
	var provider, endpointClass, modelVariantID string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "draft",
		Short: "Create a new diagnostic config from a preset (read-only; creates new config only)",
		Long: `draft creates a new diagnostic config at a path that does not exist.
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
			req := evalDiagnosticDraftRequest{
				Kind:           "diagnostic",
				Preset:         preset,
				OperationID:    operationID,
				Revision:       revision,
				ReportRoot:     reportRoot,
				GoldSet:        evalAuthorityRefJSON{Path: goldSetPath, SHA256: goldSetSHA256},
				EvidenceKey:    evalEvidenceKeyRefJSON{Path: evidenceKeyPath, KeyID: evidenceKeyID},
				ProviderEndpoint: evalProviderEndpointRefJSON{Provider: provider, EndpointClass: endpointClass},
				ModelVariantID: modelVariantID,
				OutputPath:     outPath,
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
	cmd.Flags().StringVar(&modelVariantID, "model-variant-id", "", "Model variant ID (required)")
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
	_ = cmd.MarkFlagRequired("model-variant-id")
	return cmd
}
