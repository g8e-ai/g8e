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

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/pathutil"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
)

func complianceEvidenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evidence",
		Short: "Export source evidence from the selected runtime",
	}
	cmd.AddCommand(complianceEvidenceExportCmdWithConfig(newFileSvc))
	return cmd
}

func complianceEvidenceExportCmdWithConfig(fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var scopePath string
	var outputDir string
	var maxRows int

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export bounded operational evidence without changing runtime state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if scopePath == "" || outputDir == "" {
				return fmt.Errorf("%w: --scope and --out are required", constants.ErrValidationFailed)
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			scope, err := loadComplianceAssessmentScope(scopePath)
			if err != nil {
				return err
			}
			if len(scope.GetSourceAdmissions()) != 1 {
				return fmt.Errorf("%w: operational export requires exactly one source admission", constants.ErrValidationFailed)
			}
			admission := scope.GetSourceAdmissions()[0]
			dbPath := pathutil.ResolveDBPath(fileSvc.Resolve(constants.DataDirname), constants.DbFilename)
			reader, err := storage.OpenReadOnlyOperationalEvidence(dbPath, slog.Default())
			if err != nil {
				return err
			}
			defer func() { _ = reader.Close() }()
			snapshot, err := reader.Snapshot(ctx, storage.OperationalEvidenceQuery{
				WindowStart: scope.GetAssessmentWindowStart().AsTime(),
				WindowEnd:   scope.GetAssessmentWindowEnd().AsTime(),
				MaxRows:     maxRows,
			})
			if err != nil {
				return err
			}
			inventory, err := evidence.ExportOperationalEvidence(ctx, snapshot, evidence.OperationalExportRequest{
				ScopeID:              scope.GetScopeId(),
				OwnerRuntimeBoundary: admission.GetOwnerRuntimeBoundary(),
				AcquisitionBoundary:  admission.GetAcquisitionBoundary(),
				WindowStart:          scope.GetAssessmentWindowStart().AsTime(),
				WindowEnd:            scope.GetAssessmentWindowEnd().AsTime(),
				MaxRows:              maxRows,
				OutputDir:            outputDir,
			})
			if err != nil {
				return err
			}
			body, err := json.Marshal(inventory)
			if err != nil {
				return fmt.Errorf("compliance evidence: marshal export inventory: %w", err)
			}
			if _, err := cmd.OutOrStdout().Write(append(body, '\n')); err != nil {
				return fmt.Errorf("compliance evidence: write export inventory: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scopePath, "scope", "", "Canonical assessment scope selecting the runtime source and evidence window")
	cmd.Flags().StringVar(&outputDir, "out", "", "Output directory for the operational source package")
	cmd.Flags().IntVar(&maxRows, "max-rows", constants.ComplianceOperationalExportDefaultMaxRows, "Maximum receipts and commitments to export")
	return cmd
}
