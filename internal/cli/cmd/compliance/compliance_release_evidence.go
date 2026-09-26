// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliancecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// complianceReleaseEvidenceProjectionCmdWithConfig creates the canonical
// bundle-backed `compliance release-evidence` projection command. It verifies
// the signed bundle before copying its public-safe canonical Markdown and CSV
// renderings into the release-notes directory.
func complianceReleaseEvidenceProjectionCmdWithConfig(
	loader complianceReportBundleInputLoader,
	verifier complianceReportBundleVerifier,
	nowFunc func() time.Time,
) *cobra.Command {
	var (
		outDir            string
		trustPolicyPath   string
		evidenceTrustPath string
	)

	cmd := &cobra.Command{
		Use:   "release-evidence <bundle>",
		Short: "Verify a signed compliance bundle and project release evidence",
		Args:  cobra.ExactArgs(1),
		Long: `Verify the complete signed compliance bundle offline, then copy its
canonical public-safe Markdown and CSV renderings into the release-notes
output directory. The release version comes from the protected assessment
scope; this command does not accept an arbitrary version or regrade evidence.`,
		RunE: func(cmd *cobra.Command, args []string) (runErr error) {
			if strings.TrimSpace(outDir) == "" {
				return fmt.Errorf("%w: --out is required", constants.ErrValidationFailed)
			}
			if strings.TrimSpace(trustPolicyPath) == "" {
				return fmt.Errorf("%w: --trust-policy is required", constants.ErrValidationFailed)
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			input, err := loader(ctx, args[0], trustPolicyPath, evidenceTrustPath)
			if err != nil {
				return err
			}
			closed := false
			if input.close != nil {
				defer func() {
					if closed {
						return
					}
					if closeErr := input.close(); closeErr != nil {
						runErr = errors.Join(runErr, fmt.Errorf("%w: close bundle reader: %w", constants.ErrReportVerificationFailed, closeErr))
					}
				}()
			}
			verification, err := verifier(ctx, compliancereport.BundleVerificationRequest{
				Bundle:        input.bundle,
				Reader:        input.reader,
				TrustPolicy:   input.trustPolicy,
				EvidenceTrust: input.evidenceTrust,
				VerifiedAt:    nowFunc().UTC(),
			})
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrReportVerificationFailed, err)
			}
			if verification == nil || !verification.GetValid() {
				return constants.ErrReportVerificationFailed
			}
			if input.bundle.GetManifest().GetBundleProfile() != constants.ComplianceBundleProfilePublic {
				return fmt.Errorf("%w: release projection requires a public bundle", constants.ErrBundleProfileUnsupported)
			}
			scopeBody, err := input.reader.ReadFile(ctx, constants.ComplianceBundleScopeFilename)
			if err != nil {
				return fmt.Errorf("%w: read protected assessment scope: %w", constants.ErrReportVerificationFailed, err)
			}
			scope := &compliancev1.AssessmentScope{}
			if err := compliancev1.UnmarshalCanonical(scopeBody, scope); err != nil {
				return fmt.Errorf("%w: decode protected assessment scope: %w", constants.ErrReportVerificationFailed, err)
			}
			releaseVersion := "v" + strings.TrimPrefix(scope.GetProductVersion(), "v")
			if err := validateReleaseVersion(releaseVersion); err != nil {
				return fmt.Errorf("%w: protected assessment scope product version: %w", constants.ErrValidationFailed, err)
			}
			markdown, err := input.reader.ReadFile(ctx, constants.ComplianceBundleMarkdownPath)
			if err != nil {
				return fmt.Errorf("%w: read canonical markdown projection: %w", constants.ErrReportVerificationFailed, err)
			}
			csvBody, err := input.reader.ReadFile(ctx, constants.ComplianceBundleCSVPath)
			if err != nil {
				return fmt.Errorf("%w: read canonical CSV projection: %w", constants.ErrReportVerificationFailed, err)
			}
			if input.close != nil {
				if err := input.close(); err != nil {
					closed = true
					return fmt.Errorf("%w: close bundle reader: %w", constants.ErrReportVerificationFailed, err)
				}
				closed = true
			}
			if err := writeReleaseProjectionArtifacts(outDir, releaseVersion, markdown, csvBody); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", filepath.Join(outDir, releaseVersion+constants.ReleaseEvidenceMarkdownSuffix))
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", filepath.Join(outDir, releaseVersion+constants.ReleaseEvidenceCSVSuffix))
			return nil
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for the release Markdown and CSV")
	cmd.Flags().StringVar(&trustPolicyPath, "trust-policy", "", "Path to externally assessed compliance report trust policy")
	cmd.Flags().StringVar(&evidenceTrustPath, "evidence-trust", "", "Path to externally assessed source evidence signer trust policy")
	return cmd
}

func writeReleaseProjectionArtifacts(outDir, releaseVersion string, markdown, csvBody []byte) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("%w: --out is required", constants.ErrValidationFailed)
	}
	if err := validateReleaseVersion(releaseVersion); err != nil {
		return err
	}
	cleanOutDir := filepath.Clean(outDir)
	if err := os.MkdirAll(cleanOutDir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrReportOutputDirFailed, err)
	}
	for _, artifact := range []struct {
		suffix string
		body   []byte
	}{
		{suffix: constants.ReleaseEvidenceMarkdownSuffix, body: markdown},
		{suffix: constants.ReleaseEvidenceCSVSuffix, body: csvBody},
	} {
		artifactPath := filepath.Join(cleanOutDir, releaseVersion+artifact.suffix)
		if err := os.WriteFile(artifactPath, artifact.body, constants.PermFileReadOnly); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrReportWriteFailed, err)
		}
	}
	return nil
}

var releaseVersionRegexp = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// validateReleaseVersion checks that the version flag is a non-empty string
// beginning with the 'v' prefix and matching the semantic versioning contract.
// It explicitly rejects path separators and parent directory references.
func validateReleaseVersion(version string) error {
	if version == "" {
		return fmt.Errorf("%w: release version is required", constants.ErrValidationFailed)
	}
	if !strings.HasPrefix(version, "v") {
		return fmt.Errorf("%w: release version must begin with 'v' (got %q)", constants.ErrValidationFailed, version)
	}
	if strings.ContainsAny(version, `/\`) || strings.Contains(version, "..") {
		return fmt.Errorf("%w: release version must not contain path separators or parent directory references", constants.ErrValidationFailed)
	}
	if !releaseVersionRegexp.MatchString(version) {
		return fmt.Errorf("%w: release version must be valid semantic version starting with 'v' (e.g. v2.1.3): %q", constants.ErrValidationFailed, version)
	}
	return nil
}
