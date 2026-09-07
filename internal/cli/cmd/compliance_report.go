// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func complianceReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate canonical compliance reports",
	}
	cmd.AddCommand(complianceReportGenerateCmdWithConfig(newFileSvc, defaultProvenanceSourceFactory))
	return cmd
}

func complianceReportGenerateCmdWithConfig(
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	provenanceSourceFactory func(string) evidence.ProvenanceSource,
) *cobra.Command {
	var (
		projectRoot      string
		scopeID          string
		demoRuns         []string
		evalRuns         []string
		windowStartMilli int64
		windowEndMilli   int64
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate canonical analysis from persisted evidence",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if scopeID == "" {
				return fmt.Errorf("%w: --scope-id is required", constants.ErrValidationFailed)
			}
			if len(demoRuns) == 0 && len(evalRuns) == 0 {
				return fmt.Errorf("%w: at least one --demo-run or --eval-run is required", constants.ErrValidationFailed)
			}
			if windowStartMilli <= 0 || windowEndMilli <= 0 || windowEndMilli < windowStartMilli {
				return fmt.Errorf("%w: a valid evidence window is required", constants.ErrValidationFailed)
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			fileSvc, err := fileSvcFactory(projectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			importers, err := buildEvidenceGraphImporters(ctx, fileSvc, provenanceSourceFactory(projectRoot), demoRuns, evalRuns)
			if err != nil {
				return err
			}
			assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
			if err != nil {
				return fmt.Errorf("compliance report: load canonical catalogs: %w", err)
			}
			windowStart := time.UnixMilli(windowStartMilli).UTC()
			windowEnd := time.UnixMilli(windowEndMilli).UTC()
			result, err := compliancereport.GenerateComplianceAnalysis(ctx, compliancereport.GenerationRequest{
				ScopeID:     scopeID,
				WindowStart: windowStart,
				WindowEnd:   windowEnd,
				EvaluatedAt: windowEnd,
				Importers:   importers,
				Assertions:  assertions,
				Frameworks:  frameworks,
				Crosswalks:  crosswalks,
			})
			if err != nil {
				return err
			}
			body, err := compliancev1.MarshalCanonical(result.Analysis)
			if err != nil {
				return fmt.Errorf("compliance report: marshal canonical analysis: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(body))
			return nil
		},
	}

	cmd.Flags().StringVar(&scopeID, "scope-id", "", "Assessment scope ID")
	cmd.Flags().StringSliceVar(&demoRuns, "demo-run", nil, "Demo evidence run ID (repeatable)")
	cmd.Flags().StringSliceVar(&evalRuns, "eval-run", nil, "Eval bundle run ID (repeatable)")
	cmd.Flags().Int64Var(&windowStartMilli, "window-start-unix-ms", 0, "Evidence window start as Unix milliseconds")
	cmd.Flags().Int64Var(&windowEndMilli, "window-end-unix-ms", 0, "Evidence window end as Unix milliseconds")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root directory (defaults to cwd)")
	return cmd
}
