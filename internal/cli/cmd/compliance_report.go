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
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
		Short: "Generate and verify canonical compliance reports",
	}
	cmd.AddCommand(
		complianceReportGenerateCmdWithConfig(newFileSvc, defaultProvenanceSourceFactory),
		complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, time.Now),
	)
	return cmd
}

type complianceReportBundleInput struct {
	bundle      *compliancev1.ComplianceReportBundle
	reader      compliancereport.BundleArtifactReader
	trustPolicy *compliancev1.ComplianceReportTrustPolicy
	close       func() error
}

type complianceReportBundleInputLoader func(context.Context, string, string) (complianceReportBundleInput, error)
type complianceReportBundleVerifier func(context.Context, compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error)

func complianceReportVerifyCmdWithConfig(loader complianceReportBundleInputLoader, verifier complianceReportBundleVerifier, nowFunc func() time.Time) *cobra.Command {
	var trustPolicyPath string
	cmd := &cobra.Command{
		Use:   "verify <bundle>",
		Short: "Independently verify a complete signed report bundle offline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("%w: exactly one bundle argument is required", constants.ErrValidationFailed)
			}
			if strings.TrimSpace(trustPolicyPath) == "" {
				return fmt.Errorf("%w: --trust-policy is required", constants.ErrValidationFailed)
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			input, err := loader(ctx, args[0], trustPolicyPath)
			if err != nil {
				return err
			}
			report, verifyErr := verifier(ctx, compliancereport.BundleVerificationRequest{
				Bundle:      input.bundle,
				Reader:      input.reader,
				TrustPolicy: input.trustPolicy,
				VerifiedAt:  nowFunc().UTC(),
			})
			var closeErr error
			if input.close != nil {
				closeErr = input.close()
			}
			if verifyErr != nil {
				return fmt.Errorf("%w: %w", constants.ErrReportVerificationFailed, verifyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("%w: close bundle reader: %w", constants.ErrReportVerificationFailed, closeErr)
			}
			body, err := compliancev1.MarshalCanonical(report)
			if err != nil {
				return fmt.Errorf("compliance report: marshal verification report: %w", err)
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(body)); err != nil {
				return fmt.Errorf("compliance report: write verification report: %w", err)
			}
			if !report.GetValid() {
				return constants.ErrReportVerificationFailed
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&trustPolicyPath, "trust-policy", "", "Path to externally assessed compliance report trust policy")
	return cmd
}

type complianceBundleRootReader struct {
	root *os.Root
}

func (r *complianceBundleRootReader) ReadFile(_ context.Context, bundlePath string) ([]byte, error) {
	file, err := r.root.Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrNotFound, err)
	}
	body, readErr := io.ReadAll(io.LimitReader(file, constants.ComplianceBundleMaxArtifactBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrReportVerificationFailed, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrReportVerificationFailed, closeErr)
	}
	if int64(len(body)) > constants.ComplianceBundleMaxArtifactBytes {
		return nil, constants.ErrEvidenceArtifactTooLarge
	}
	return body, nil
}

func loadComplianceReportBundleInput(ctx context.Context, bundlePath, trustPolicyPath string) (complianceReportBundleInput, error) {
	if err := ctx.Err(); err != nil {
		return complianceReportBundleInput{}, err
	}
	resolvedBundlePath, err := filepath.EvalSymlinks(bundlePath)
	if err != nil {
		return complianceReportBundleInput{}, fmt.Errorf("%w: resolve bundle descriptor: %w", constants.ErrReportVerificationFailed, err)
	}
	resolvedBundlePath, err = filepath.Abs(resolvedBundlePath)
	if err != nil {
		return complianceReportBundleInput{}, fmt.Errorf("%w: resolve bundle descriptor: %w", constants.ErrReportVerificationFailed, err)
	}
	resolvedTrustPath, err := filepath.EvalSymlinks(trustPolicyPath)
	if err != nil {
		return complianceReportBundleInput{}, fmt.Errorf("%w: resolve assessed trust policy: %w", constants.ErrEvidenceTrustNotAssessed, err)
	}
	resolvedTrustPath, err = filepath.Abs(resolvedTrustPath)
	if err != nil {
		return complianceReportBundleInput{}, fmt.Errorf("%w: resolve assessed trust policy: %w", constants.ErrEvidenceTrustNotAssessed, err)
	}
	bundleRoot := filepath.Dir(resolvedBundlePath)
	if pathWithinRoot(bundleRoot, resolvedTrustPath) {
		return complianceReportBundleInput{}, fmt.Errorf("%w: trust policy must be external to the report bundle", constants.ErrEvidenceTrustNotAssessed)
	}
	bundleBody, err := readComplianceReportInputFile(resolvedBundlePath)
	if err != nil {
		return complianceReportBundleInput{}, err
	}
	bundle := &compliancev1.ComplianceReportBundle{}
	if err := compliancev1.UnmarshalCanonical(bundleBody, bundle); err != nil {
		return complianceReportBundleInput{}, fmt.Errorf("%w: decode canonical bundle descriptor: %w", constants.ErrReportVerificationFailed, err)
	}
	trustBody, err := readComplianceReportInputFile(resolvedTrustPath)
	if err != nil {
		return complianceReportBundleInput{}, fmt.Errorf("%w: %w", constants.ErrEvidenceTrustNotAssessed, err)
	}
	trustPolicy := &compliancev1.ComplianceReportTrustPolicy{}
	if err := compliancev1.UnmarshalCanonical(trustBody, trustPolicy); err != nil {
		return complianceReportBundleInput{}, fmt.Errorf("%w: decode canonical trust policy: %w", constants.ErrEvidenceTrustNotAssessed, err)
	}
	root, err := os.OpenRoot(bundleRoot)
	if err != nil {
		return complianceReportBundleInput{}, fmt.Errorf("%w: open bundle root: %w", constants.ErrReportVerificationFailed, err)
	}
	return complianceReportBundleInput{
		bundle:      bundle,
		reader:      &complianceBundleRootReader{root: root},
		trustPolicy: trustPolicy,
		close:       root.Close,
	}, nil
}

func readComplianceReportInputFile(inputPath string) ([]byte, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: open %s: %w", constants.ErrReportVerificationFailed, inputPath, err)
	}
	body, readErr := io.ReadAll(io.LimitReader(file, constants.ComplianceBundleMaxArtifactBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("%w: read %s: %w", constants.ErrReportVerificationFailed, inputPath, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("%w: close %s: %w", constants.ErrReportVerificationFailed, inputPath, closeErr)
	}
	if int64(len(body)) > constants.ComplianceBundleMaxArtifactBytes {
		return nil, constants.ErrEvidenceArtifactTooLarge
	}
	return body, nil
}

func pathWithinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || relative != constants.PathParentDir && !strings.HasPrefix(relative, constants.PathParentDir+string(filepath.Separator))
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
		outputFormat     string
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate canonical analysis from persisted evidence",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := compliancereport.ParseFormat(outputFormat)
			if err != nil {
				return err
			}
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
			rendered, err := compliancereport.RenderComplianceAnalysis(result.Analysis, format)
			if err != nil {
				return err
			}
			if _, err := cmd.OutOrStdout().Write(rendered.Body); err != nil {
				return fmt.Errorf("compliance report: write %s output: %w", format, err)
			}
			if len(rendered.Body) == 0 || rendered.Body[len(rendered.Body)-1] != '\n' {
				if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
					return fmt.Errorf("compliance report: terminate %s output: %w", format, err)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&scopeID, "scope-id", "", "Assessment scope ID")
	cmd.Flags().StringSliceVar(&demoRuns, "demo-run", nil, "Demo evidence run ID (repeatable)")
	cmd.Flags().StringSliceVar(&evalRuns, "eval-run", nil, "Eval bundle run ID (repeatable)")
	cmd.Flags().Int64Var(&windowStartMilli, "window-start-unix-ms", 0, "Evidence window start as Unix milliseconds")
	cmd.Flags().Int64Var(&windowEndMilli, "window-end-unix-ms", 0, "Evidence window end as Unix milliseconds")
	cmd.Flags().StringVar(&outputFormat, "format", string(compliancereport.FormatJSON), "Output format: json, oscal, markdown, html, or cli")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root directory (defaults to cwd)")
	return cmd
}
