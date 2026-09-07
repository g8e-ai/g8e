// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// complianceEvidenceGraphCmd creates the `compliance evidence-graph`
// subcommand tree. The evidence graph imports and cross-validates
// evidence from multiple sources (demo runs, eval bundles) into a single
// content-addressed graph with duplicate detection, reference resolution,
// cycle detection, scope binding, freshness, encryption, digest, and trust
// validation.
func complianceEvidenceGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evidence-graph",
		Short: "Build and validate the cross-source evidence graph",
		Long: `Import evidence from persisted demo runs and eval bundles into a single
content-addressed evidence graph, then run the full validation gauntlet:
duplicate ID/content conflict detection, reference resolution, prohibited
cycle detection, scope/run/attempt/scenario binding, freshness window,
encryption metadata authentication, canonical digest verification, and
assessed trust validation. Prints a typed EvidenceGraphReport as canonical
JSON. Returns nonzero when the graph is invalid or any importer fails.`,
	}
	cmd.AddCommand(complianceEvidenceGraphVerifyCmdWithConfig(newFileSvc, defaultProvenanceSourceFactory))
	return cmd
}

// complianceEvidenceGraphVerifyCmdWithConfig creates the `compliance
// evidence-graph verify` subcommand with injectable dependencies for
// testing. The fileSvcFactory produces a RuntimeFileService that serves
// as the ArtifactReader for all importers. The provenanceSourceFactory
// produces a ProvenanceSource for demo-run manifest provenance
// verification.
func complianceEvidenceGraphVerifyCmdWithConfig(
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	provenanceSourceFactory func(string) evidence.ProvenanceSource,
) *cobra.Command {
	var (
		projectRoot string
		demoRuns    []string
		evalRuns    []string
	)

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Build and validate the evidence graph from persisted runs",
		Long: `Import evidence from the specified demo runs and eval bundles into a
single content-addressed evidence graph and run the full validation
gauntlet. At least one --demo-run or --eval-run must be supplied.

Demo runs are read from data/compliance/demo-evidence/<run-id>/ under the
runtime tree. Eval bundles are read from
data/compliance/eval-runs/<run-id>/ under the runtime tree.

Prints a typed EvidenceGraphReport as canonical JSON. Returns nonzero when
the graph is invalid or any importer fails.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(demoRuns) == 0 && len(evalRuns) == 0 {
				return fmt.Errorf("%w: at least one --demo-run or --eval-run is required", constants.ErrValidationFailed)
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			fileSvc, err := fileSvcFactory(projectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			source := provenanceSourceFactory(projectRoot)

			importers, err := buildEvidenceGraphImporters(ctx, fileSvc, source, demoRuns, evalRuns)
			if err != nil {
				return err
			}

			report := evidence.BuildAndValidateGraph(ctx, importers, time.Time{}, time.Time{}, time.Now().UTC())

			body, err := json.Marshal(report)
			if err != nil {
				return fmt.Errorf("compliance: marshal evidence graph report: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(body))

			if !report.Valid {
				return constants.ErrReportVerificationFailed
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&demoRuns, "demo-run", nil, "Demo evidence run ID (repeatable)")
	cmd.Flags().StringSliceVar(&evalRuns, "eval-run", nil, "Eval bundle run ID (repeatable)")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root directory (defaults to cwd)")

	return cmd
}

// buildEvidenceGraphImporters constructs the read-only importer set for
// the specified demo and eval run IDs. Each run ID is validated as a safe
// path element before constructing the importer.
func buildEvidenceGraphImporters(
	_ context.Context,
	fileSvc fs.RuntimeFileService,
	source evidence.ProvenanceSource,
	demoRuns []string,
	evalRuns []string,
) ([]evidence.EvidenceImporter, error) {
	importers := make([]evidence.EvidenceImporter, 0, len(demoRuns)+len(evalRuns))

	for _, runID := range demoRuns {
		if !evidence.ValidPathElement(runID) {
			return nil, fmt.Errorf("%w: invalid demo run ID %q", constants.ErrPathValidation, runID)
		}
		importers = append(importers, evidence.NewDemoRunImporter(fileSvc, runID, source))
	}

	for _, runID := range evalRuns {
		if !evidence.ValidPathElement(runID) {
			return nil, fmt.Errorf("%w: invalid eval run ID %q", constants.ErrPathValidation, runID)
		}
		runDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.EvalRunsDirname, runID)
		importers = append(importers, evidence.NewEvalBundleImporter(fileSvc, runID, runDir))
	}

	return importers, nil
}
