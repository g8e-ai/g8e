// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
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
		complianceReportGenerateCmdWithConfig(newFileSvc, defaultProvenanceSourceFactory, loadComplianceReportSigningIdentity),
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

func (r *complianceBundleRootReader) ListFiles(ctx context.Context) ([]string, error) {
	paths := make([]string, 0)
	if err := r.listFiles(ctx, ".", &paths); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func (r *complianceBundleRootReader) listFiles(ctx context.Context, directory string, paths *[]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := r.root.Open(directory)
	if err != nil {
		return fmt.Errorf("%w: open bundle directory: %w", constants.ErrDirectoryRead, err)
	}
	entries, readErr := file.ReadDir(constants.ComplianceBundleMaxArtifacts + 1)
	closeErr := file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("%w: enumerate bundle directory: %w", constants.ErrDirectoryRead, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("%w: close bundle directory: %w", constants.ErrDirectoryRead, closeErr)
	}
	if len(entries) > constants.ComplianceBundleMaxArtifacts {
		return constants.ErrEvidenceArtifactTooLarge
	}
	for _, entry := range entries {
		entryPath := entry.Name()
		if directory != "." {
			entryPath = path.Join(directory, entry.Name())
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink %s", constants.ErrUnexpectedEvidenceArtifact, entryPath)
		}
		if entry.IsDir() {
			if err := r.listFiles(ctx, entryPath, paths); err != nil {
				return err
			}
			continue
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%w: unsupported entry %s", constants.ErrUnexpectedEvidenceArtifact, entryPath)
		}
		if entryPath != constants.ComplianceBundleManifestPath {
			*paths = append(*paths, entryPath)
		}
	}
	return nil
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

type complianceReportSigningIdentityLoader func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error)

func loadComplianceReportSigningIdentity(ctx context.Context, metadataPath, privateKeyPath string) (*compliancereport.ComplianceReportSigningIdentity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	metadataBody, err := readComplianceReportInputFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("%w: read signing metadata: %w", constants.ErrReportSignatureFailed, err)
	}
	metadata := &compliancev1.ComplianceReportSigningKeyMetadata{}
	if err := compliancev1.UnmarshalCanonical(metadataBody, metadata); err != nil {
		return nil, fmt.Errorf("%w: decode canonical signing metadata: %w", constants.ErrReportSignatureFailed, err)
	}
	privateKeyBody, err := readComplianceReportInputFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("%w: read signing key: %w", constants.ErrReportSignatureFailed, err)
	}
	privateKey, err := hex.DecodeString(strings.TrimSpace(string(privateKeyBody)))
	if err != nil || len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: signing key must be a hex-encoded Ed25519 private key", constants.ErrReportSignatureFailed)
	}
	return compliancereport.NewComplianceReportSigningIdentity(metadata, ed25519.PrivateKey(privateKey))
}

func buildDemoVerificationArtifacts(ctx context.Context, reader evidence.ArtifactReader, source evidence.ProvenanceSource, runIDs []string, verifiedAt time.Time) ([]compliancereport.SourceArtifact, error) {
	artifacts := make([]compliancereport.SourceArtifact, 0, len(runIDs))
	for _, runID := range runIDs {
		report, err := evidence.VerifyDemoRun(ctx, reader, runID, source, verifiedAt)
		if err != nil {
			return nil, fmt.Errorf("%w: verify demo run %s: %w", constants.ErrDemoRunVerificationFailed, runID, err)
		}
		if !report.GetValid() {
			return nil, fmt.Errorf("%w: demo run %s has %d verification failures", constants.ErrDemoRunVerificationFailed, runID, len(report.GetFailures()))
		}
		rawArtifacts, err := buildDemoRawSourceArtifacts(ctx, reader, source, runID)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, rawArtifacts...)
		body, err := compliancev1.MarshalCanonical(report)
		if err != nil {
			return nil, fmt.Errorf("%w: canonicalize demo verification report %s: %w", constants.ErrDemoRunVerificationFailed, runID, err)
		}
		artifacts = append(artifacts, compliancereport.SourceArtifact{
			BundlePath: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceVerificationFilename),
			Body:       body,
			MediaType:  constants.MediaTypeJSON,
		})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].BundlePath < artifacts[j].BundlePath })
	return artifacts, nil
}

func buildDemoRawSourceArtifacts(ctx context.Context, reader evidence.ArtifactReader, source evidence.ProvenanceSource, runID string) ([]compliancereport.SourceArtifact, error) {
	runtimeRoot := path.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, runID)
	bundleRoot := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID)
	artifacts, err := collectDemoRuntimeArtifacts(ctx, reader, runtimeRoot, runtimeRoot, bundleRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: collect demo runtime %s: %w", constants.ErrDemoRunVerificationFailed, runID, err)
	}
	manifestBody, err := reader.ReadFile(ctx, path.Join(runtimeRoot, constants.DemoRunManifestFilename))
	if err != nil {
		return nil, fmt.Errorf("%w: read demo manifest %s: %w", constants.ErrDemoRunVerificationFailed, runID, err)
	}
	manifest := &compliancev1.DemoManifest{}
	if err := compliancev1.UnmarshalCanonical(manifestBody, manifest); err != nil {
		return nil, fmt.Errorf("%w: decode demo manifest %s: %w", constants.ErrDemoRunVerificationFailed, runID, err)
	}
	provenanceArtifacts, err := source.Artifacts(ctx, manifest.GetDemoId())
	if err != nil {
		return nil, fmt.Errorf("%w: load demo provenance %s: %w", constants.ErrDemoRunVerificationFailed, runID, err)
	}
	for _, artifact := range provenanceArtifacts {
		name := path.Clean(filepath.ToSlash(artifact.Name))
		if !validDemoSourceRelativePath(name) || len(artifact.Body) == 0 || int64(len(artifact.Body)) > constants.ComplianceBundleMaxArtifactBytes {
			return nil, fmt.Errorf("%w: invalid demo provenance artifact %s", constants.ErrDemoRunVerificationFailed, artifact.Name)
		}
		artifacts = append(artifacts, compliancereport.SourceArtifact{
			BundlePath: path.Join(bundleRoot, constants.ComplianceBundleSourceProvenanceDirname, constants.ComplianceBundleSourceArtifactsDirname, name),
			Body:       append([]byte(nil), artifact.Body...),
			MediaType:  constants.MediaTypeText,
		})
	}
	definitions, err := source.Definitions(ctx, manifest.GetDemoId())
	if err != nil {
		return nil, fmt.Errorf("%w: load demo definitions %s: %w", constants.ErrDemoRunVerificationFailed, runID, err)
	}
	definitionBodies := make([][]byte, 0, len(definitions))
	for _, definition := range definitions {
		if len(definition.Body) == 0 {
			return nil, fmt.Errorf("%w: empty demo definition for %s", constants.ErrDemoRunVerificationFailed, runID)
		}
		definitionBodies = append(definitionBodies, definition.Body)
	}
	if len(definitionBodies) == 0 {
		return nil, fmt.Errorf("%w: demo definitions are missing for %s", constants.ErrDemoRunVerificationFailed, runID)
	}
	artifacts = append(artifacts, compliancereport.SourceArtifact{
		BundlePath: path.Join(bundleRoot, constants.ComplianceBundleSourceProvenanceDirname, constants.DemoRunDefinitionsFilename),
		Body:       bytes.Join(definitionBodies, []byte{'\n'}),
		MediaType:  constants.MediaTypeJSON,
	})
	return artifacts, nil
}

func collectDemoRuntimeArtifacts(ctx context.Context, reader evidence.ArtifactReader, runtimeRoot, currentPath, bundleRoot string) ([]compliancereport.SourceArtifact, error) {
	entries, err := reader.ReadDir(ctx, currentPath)
	if err != nil {
		return nil, err
	}
	if len(entries) > constants.DemoRunMaxArtifactsPerDirectory {
		return nil, constants.ErrEvidenceArtifactTooLarge
	}
	artifacts := make([]compliancereport.SourceArtifact, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, constants.ErrUnexpectedEvidenceArtifact
		}
		sourcePath := path.Join(currentPath, entry.Name())
		if entry.IsDir() {
			nested, err := collectDemoRuntimeArtifacts(ctx, reader, runtimeRoot, sourcePath, bundleRoot)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, nested...)
			continue
		}
		if !entry.Type().IsRegular() {
			return nil, constants.ErrUnexpectedEvidenceArtifact
		}
		relativePath, err := filepath.Rel(runtimeRoot, sourcePath)
		if err != nil || !validDemoSourceRelativePath(filepath.ToSlash(relativePath)) {
			return nil, constants.ErrUnexpectedEvidenceArtifact
		}
		body, err := reader.ReadFile(ctx, sourcePath)
		if err != nil {
			return nil, err
		}
		if len(body) == 0 || int64(len(body)) > constants.ComplianceBundleMaxArtifactBytes {
			return nil, constants.ErrEvidenceArtifactTooLarge
		}
		artifacts = append(artifacts, compliancereport.SourceArtifact{
			BundlePath: path.Join(bundleRoot, constants.ComplianceBundleSourceRuntimeDirname, filepath.ToSlash(relativePath)),
			Body:       append([]byte(nil), body...),
			MediaType:  constants.MediaTypeJSON,
		})
	}
	return artifacts, nil
}

func validDemoSourceRelativePath(value string) bool {
	return value != "" && value != "." && value != constants.PathParentDir && !path.IsAbs(value) && !strings.HasPrefix(value, constants.PathParentDir+"/") && path.Clean(value) == value
}

func complianceReportGenerateCmdWithConfig(
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	provenanceSourceFactory func(string) evidence.ProvenanceSource,
	signingIdentityLoader complianceReportSigningIdentityLoader,
) *cobra.Command {
	var (
		projectRoot       string
		scopeID           string
		demoRuns          []string
		evalRuns          []string
		windowStartMilli  int64
		windowEndMilli    int64
		reportID          string
		bundleProfile     string
		signingMetadata   string
		signingPrivateKey string
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
			if reportID == "" || signingMetadata == "" || signingPrivateKey == "" {
				return fmt.Errorf("%w: --report-id, --signing-metadata, and --signing-private-key are required", constants.ErrValidationFailed)
			}
			profile, err := compliancereport.ParseBundleProfile(bundleProfile)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			fileSvc, err := fileSvcFactory(projectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			identity, err := signingIdentityLoader(ctx, signingMetadata, signingPrivateKey)
			if err != nil {
				return err
			}
			source := provenanceSourceFactory(projectRoot)
			importers, err := buildEvidenceGraphImporters(ctx, fileSvc, source, demoRuns, evalRuns)
			if err != nil {
				return err
			}
			assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
			if err != nil {
				return fmt.Errorf("compliance report: load canonical catalogs: %w", err)
			}
			windowStart := time.UnixMilli(windowStartMilli).UTC()
			windowEnd := time.UnixMilli(windowEndMilli).UTC()
			sourceArtifacts, err := buildDemoVerificationArtifacts(ctx, fileSvc, source, demoRuns, windowEnd)
			if err != nil {
				return err
			}
			result, err := compliancereport.GenerateSignedComplianceBundle(ctx, compliancereport.SignedBundleGenerationRequest{
				Generation: compliancereport.GenerationRequest{
					ScopeID:     scopeID,
					WindowStart: windowStart,
					WindowEnd:   windowEnd,
					EvaluatedAt: windowEnd,
					Importers:   importers,
					Assertions:  assertions,
					Frameworks:  frameworks,
					Crosswalks:  crosswalks,
				},
				Profile:         profile,
				ReportID:        reportID,
				SigningIdentity: identity,
				SourceArtifacts: sourceArtifacts,
			})
			if err != nil {
				return err
			}
			descriptorPath, err := compliancereport.PersistBundle(ctx, fileSvc, result)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), fileSvc.Resolve(descriptorPath)); err != nil {
				return fmt.Errorf("compliance report: write persisted bundle path: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&scopeID, "scope-id", "", "Assessment scope ID")
	cmd.Flags().StringSliceVar(&demoRuns, "demo-run", nil, "Demo evidence run ID (repeatable)")
	cmd.Flags().StringSliceVar(&evalRuns, "eval-run", nil, "Eval bundle run ID (repeatable)")
	cmd.Flags().Int64Var(&windowStartMilli, "window-start-unix-ms", 0, "Evidence window start as Unix milliseconds")
	cmd.Flags().Int64Var(&windowEndMilli, "window-end-unix-ms", 0, "Evidence window end as Unix milliseconds")
	cmd.Flags().StringVar(&reportID, "report-id", "", "Immutable report bundle ID")
	cmd.Flags().StringVar(&bundleProfile, "profile", string(compliancereport.ProfilePublic), "Bundle profile: public or restricted")
	cmd.Flags().StringVar(&signingMetadata, "signing-metadata", "", "Path to canonical compliance report signing-key metadata")
	cmd.Flags().StringVar(&signingPrivateKey, "signing-private-key", "", "Path to hex-encoded Ed25519 compliance report private key")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root directory (defaults to cwd)")
	return cmd
}
