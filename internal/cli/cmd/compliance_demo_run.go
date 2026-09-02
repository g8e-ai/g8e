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
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// complianceDemoRunCmd creates the `compliance demo-run` subcommand tree.
func complianceDemoRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "demo-run",
		Short: "Verify persisted demo evidence runs",
		Long: `Read and independently verify persisted demo evidence runs under the
runtime compliance evidence tree. Verification is read-only and fail-closed;
it never mutates assessed state.`,
	}
	cmd.AddCommand(complianceDemoRunVerifyCmdWithConfig(newFileSvc, defaultProvenanceSourceFactory))
	return cmd
}

// defaultProvenanceSourceFactory constructs a DemoDirectoryProvenanceSource
// rooted at the current working directory. This is the production factory used
// when the command is invoked directly.
func defaultProvenanceSourceFactory(projectRoot string) evidence.ProvenanceSource {
	if projectRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return evidence.NewDemoDirectoryProvenanceSource("")
		}
		projectRoot = cwd
	}
	return evidence.NewDemoDirectoryProvenanceSource(projectRoot)
}

// complianceDemoRunVerifyCmdWithConfig creates the `compliance demo-run verify`
// subcommand with injectable dependencies for testing. The fileSvcFactory
// produces a RuntimeFileService that serves as the ArtifactReader for the
// verifier. The provenanceSourceFactory produces a ProvenanceSource for
// manifest provenance verification.
func complianceDemoRunVerifyCmdWithConfig(
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	provenanceSourceFactory func(string) evidence.ProvenanceSource,
) *cobra.Command {
	var projectRoot string

	cmd := &cobra.Command{
		Use:   "verify <run-id>",
		Short: "Independently verify a persisted demo evidence run",
		Long: `Read the canonical DemoManifest and DemoScenarioResult records persisted
under data/compliance/demo-evidence/<run-id>/ and independently verify run
correlation, provenance digests, content-addressed receipt and persistence
bodies, cryptographic signatures, deterministic-stage protocol chains,
state-observation bindings, and artifact directory integrity. Prints a typed
ComplianceVerificationReport as canonical JSON. Returns nonzero when the
report is invalid.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("%w: exactly one run ID argument is required", constants.ErrValidationFailed)
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			runID := args[0]

			fileSvc, err := fileSvcFactory(projectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			source := provenanceSourceFactory(projectRoot)
			report, verifyErr := evidence.VerifyDemoRun(ctx, fileSvc, runID, source, time.Now().UTC())
			if verifyErr != nil {
				return fmt.Errorf("%w: %w", constants.ErrReportVerificationFailed, verifyErr)
			}

			body, err := compliancev1.MarshalCanonical(report)
			if err != nil {
				return fmt.Errorf("compliance: marshal verification report: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(body))

			if !report.GetValid() {
				return constants.ErrReportVerificationFailed
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root directory (defaults to cwd)")

	return cmd
}
