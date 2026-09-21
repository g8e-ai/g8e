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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func complianceReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate and verify canonical compliance reports",
	}
	cmd.AddCommand(
		complianceReportGenerateCmdWithConfig(newFileSvc, defaultProvenanceSourceFactory, loadComplianceReportSigningIdentity, time.Now),
		complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, time.Now),
	)
	return cmd
}

type complianceReportBundleInput struct {
	bundle        *compliancev1.ComplianceReportBundle
	reader        compliancereport.BundleArtifactReader
	trustPolicy   *compliancev1.ComplianceReportTrustPolicy
	evidenceTrust evidence.AssessedSignerSource
	close         func() error
}

type complianceReportBundleInputLoader func(context.Context, string, string, string) (complianceReportBundleInput, error)
type complianceReportBundleVerifier func(context.Context, compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error)

func complianceReportVerifyCmdWithConfig(loader complianceReportBundleInputLoader, verifier complianceReportBundleVerifier, nowFunc func() time.Time) *cobra.Command {
	var trustPolicyPath string
	var evidenceTrustPath string
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
			input, err := loader(ctx, args[0], trustPolicyPath, evidenceTrustPath)
			if err != nil {
				return err
			}
			report, verifyErr := verifier(ctx, compliancereport.BundleVerificationRequest{
				Bundle:        input.bundle,
				Reader:        input.reader,
				TrustPolicy:   input.trustPolicy,
				EvidenceTrust: input.evidenceTrust,
				VerifiedAt:    nowFunc().UTC(),
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
	cmd.Flags().StringVar(&evidenceTrustPath, "evidence-trust", "", "Path to externally assessed source evidence signer trust policy")
	return cmd
}

type complianceBundleRootReader struct {
	root *os.Root
}

type recursiveDirectoryBudget struct {
	entries int
}

func (b *recursiveDirectoryBudget) consume(depth, entries int) error {
	if depth > constants.ComplianceBundleMaxDirectoryDepth {
		return fmt.Errorf("%w: depth %d exceeds %d", constants.ErrEvidenceDirectoryLimitExceeded, depth, constants.ComplianceBundleMaxDirectoryDepth)
	}
	if b.entries+entries > constants.ComplianceBundleMaxEnumeratedEntries {
		return fmt.Errorf("%w: entry count exceeds %d", constants.ErrEvidenceDirectoryLimitExceeded, constants.ComplianceBundleMaxEnumeratedEntries)
	}
	b.entries += entries
	return nil
}

func (r *complianceBundleRootReader) ReadFile(ctx context.Context, bundlePath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
	budget := recursiveDirectoryBudget{}
	if err := r.listFiles(ctx, ".", 0, &budget, &paths); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func (r *complianceBundleRootReader) listFiles(ctx context.Context, directory string, depth int, budget *recursiveDirectoryBudget, paths *[]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := budget.consume(depth, 0); err != nil {
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
	if err := budget.consume(depth, len(entries)); err != nil {
		return err
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
			if err := r.listFiles(ctx, entryPath, depth+1, budget, paths); err != nil {
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

func loadComplianceReportBundleInput(ctx context.Context, bundlePath, trustPolicyPath, evidenceTrustPath string) (complianceReportBundleInput, error) {
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
	resolvedEvidenceTrustPath := ""
	if strings.TrimSpace(evidenceTrustPath) != "" {
		resolvedEvidenceTrustPath, err = filepath.EvalSymlinks(evidenceTrustPath)
		if err != nil {
			return complianceReportBundleInput{}, fmt.Errorf("%w: resolve assessed evidence trust: %w", constants.ErrEvidenceTrustNotAssessed, err)
		}
		resolvedEvidenceTrustPath, err = filepath.Abs(resolvedEvidenceTrustPath)
		if err != nil {
			return complianceReportBundleInput{}, fmt.Errorf("%w: resolve assessed evidence trust: %w", constants.ErrEvidenceTrustNotAssessed, err)
		}
		if pathWithinRoot(bundleRoot, resolvedEvidenceTrustPath) || resolvedEvidenceTrustPath == resolvedTrustPath {
			return complianceReportBundleInput{}, fmt.Errorf("%w: evidence trust must be external to the report bundle and distinct from report trust", constants.ErrEvidenceTrustNotAssessed)
		}
	}
	bundleBody, err := readComplianceReportInputFile(resolvedBundlePath)
	if err != nil {
		return complianceReportBundleInput{}, err
	}
	bundle := &compliancev1.ComplianceReportBundle{}
	if err := compliancev1.UnmarshalCanonical(bundleBody, bundle); err != nil {
		return complianceReportBundleInput{}, fmt.Errorf("%w: decode canonical bundle descriptor: %w", constants.ErrReportVerificationFailed, err)
	}
	if bundleRequiresEvidenceTrust(bundle) && resolvedEvidenceTrustPath == "" {
		return complianceReportBundleInput{}, fmt.Errorf("%w: --evidence-trust is required for represented signed source evidence", constants.ErrEvidenceTrustNotAssessed)
	}
	var evidenceTrust evidence.AssessedSignerSource
	if resolvedEvidenceTrustPath != "" {
		evidenceTrustBody, err := readComplianceReportInputFile(resolvedEvidenceTrustPath)
		if err != nil {
			return complianceReportBundleInput{}, fmt.Errorf("%w: %w", constants.ErrEvidenceTrustNotAssessed, err)
		}
		policy := &compliancev1.ComplianceEvidenceTrustPolicy{}
		if err := compliancev1.UnmarshalCanonical(evidenceTrustBody, policy); err != nil {
			return complianceReportBundleInput{}, fmt.Errorf("%w: decode canonical evidence trust policy: %w", constants.ErrEvidenceTrustNotAssessed, err)
		}
		assessmentAsOf, err := loadProtectedAssessmentTime(ctx, bundleRoot)
		if err != nil {
			return complianceReportBundleInput{}, err
		}
		evidenceTrust, err = newAssessedEvidenceTrust(policy, bundle.GetManifest().GetScopeRef(), timestamppb.New(assessmentAsOf))
		if err != nil {
			return complianceReportBundleInput{}, err
		}
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
		bundle:        bundle,
		reader:        &complianceBundleRootReader{root: root},
		trustPolicy:   trustPolicy,
		evidenceTrust: evidenceTrust,
		close:         root.Close,
	}, nil
}

func loadProtectedAssessmentTime(ctx context.Context, bundleRoot string) (time.Time, error) {
	root, err := os.OpenRoot(bundleRoot)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: open bundle root: %w", constants.ErrReportVerificationFailed, err)
	}
	body, readErr := (&complianceBundleRootReader{root: root}).ReadFile(ctx, constants.ComplianceBundleScopeFilename)
	closeErr := root.Close()
	if readErr != nil {
		return time.Time{}, fmt.Errorf("%w: read protected assessment scope: %w", constants.ErrReportVerificationFailed, readErr)
	}
	if closeErr != nil {
		return time.Time{}, fmt.Errorf("%w: close bundle root: %w", constants.ErrReportVerificationFailed, closeErr)
	}
	scope := &compliancev1.AssessmentScope{}
	if err := compliancev1.UnmarshalCanonical(body, scope); err != nil {
		return time.Time{}, fmt.Errorf("%w: decode protected assessment scope: %w", constants.ErrReportVerificationFailed, err)
	}
	if err := catalog.ValidateAssessmentScope(scope); err != nil {
		return time.Time{}, fmt.Errorf("%w: validate protected assessment scope: %w", constants.ErrReportVerificationFailed, err)
	}
	return scope.GetAssessmentAsOf().AsTime(), nil
}

type assessedEvidenceTrust struct {
	keys map[string]ed25519.PublicKey
}

func (t *assessedEvidenceTrust) GetTrustedSignerPublicKey(ctx context.Context, keyID string) (ed25519.PublicKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, exists := t.keys[keyID]
	if !exists {
		return nil, constants.ErrTrustedSignerKeyNotFound
	}
	return append(ed25519.PublicKey(nil), key...), nil
}

func bundleRequiresEvidenceTrust(bundle *compliancev1.ComplianceReportBundle) bool {
	if bundle == nil || bundle.GetAnalysis() == nil {
		return false
	}
	for _, resource := range bundle.GetAnalysis().GetEvidenceResources() {
		if resource == nil {
			continue
		}
		switch evidence.ArtifactType(resource.GetArtifactType()) {
		case evidence.ArtifactTypeCommitment, evidence.ArtifactTypeCustomerAttestation, evidence.ArtifactTypeAssessorAttestation:
			return true
		}
	}
	return false
}

func loadAssessedEvidenceTrust(inputPath, scopeID string, signedAt time.Time) (evidence.AssessedSignerSource, error) {
	body, err := readComplianceReportInputFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: read assessed evidence trust: %w", constants.ErrEvidenceTrustNotAssessed, err)
	}
	policy := &compliancev1.ComplianceEvidenceTrustPolicy{}
	if err := compliancev1.UnmarshalCanonical(body, policy); err != nil {
		return nil, fmt.Errorf("%w: decode canonical evidence trust policy: %w", constants.ErrEvidenceTrustNotAssessed, err)
	}
	return newAssessedEvidenceTrust(policy, scopeID, timestamppb.New(signedAt))
}

func newAssessedEvidenceTrust(policy *compliancev1.ComplianceEvidenceTrustPolicy, scopeID string, signedAt *timestamppb.Timestamp) (evidence.AssessedSignerSource, error) {
	if policy == nil || policy.GetPolicyId() == "" || policy.GetPolicyVersion() == "" || len(policy.GetTrustedKeys()) == 0 || len(policy.GetTrustedKeys()) > constants.ComplianceEvidenceTrustMaxKeys || scopeID == "" || signedAt == nil || signedAt.CheckValid() != nil {
		return nil, fmt.Errorf("%w: evidence trust policy, scope, and signed time are incomplete", constants.ErrEvidenceTrustNotAssessed)
	}
	at := signedAt.AsTime()
	keys := make(map[string]ed25519.PublicKey, len(policy.GetTrustedKeys()))
	for _, trusted := range policy.GetTrustedKeys() {
		if trusted == nil || trusted.GetKeyId() == "" || trusted.GetAssessmentId() == "" || trusted.GetAssessorIdentity() == "" || trusted.GetAssessedAt() == nil || trusted.GetValidFrom() == nil || trusted.GetValidUntil() == nil {
			return nil, fmt.Errorf("%w: assessed evidence signer metadata is incomplete", constants.ErrEvidenceTrustNotAssessed)
		}
		if _, exists := keys[trusted.GetKeyId()]; exists {
			return nil, fmt.Errorf("%w: duplicate assessed evidence signer %s", constants.ErrEvidenceTrustNotAssessed, trusted.GetKeyId())
		}
		if trusted.GetAssessedAt().CheckValid() != nil || trusted.GetValidFrom().CheckValid() != nil || trusted.GetValidUntil().CheckValid() != nil || trusted.GetRevokedAt() != nil && trusted.GetRevokedAt().CheckValid() != nil {
			return nil, fmt.Errorf("%w: assessed evidence signer %s has an invalid timestamp", constants.ErrEvidenceTrustNotAssessed, trusted.GetKeyId())
		}
		if trusted.GetAssessedAt().AsTime().After(at) || at.Before(trusted.GetValidFrom().AsTime()) || at.After(trusted.GetValidUntil().AsTime()) || trusted.GetRevokedAt() != nil && !at.Before(trusted.GetRevokedAt().AsTime()) {
			return nil, fmt.Errorf("%w: assessed evidence signer %s is ineligible at bundle signing time", constants.ErrEvidenceTrustNotAssessed, trusted.GetKeyId())
		}
		allowed := false
		for _, allowedScope := range trusted.GetAllowedScopeRefs() {
			if allowedScope == scopeID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("%w: assessed evidence signer %s is not eligible for scope %s", constants.ErrEvidenceTrustNotAssessed, trusted.GetKeyId(), scopeID)
		}
		publicKey, err := hex.DecodeString(trusted.GetPublicKey())
		if err != nil || len(publicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: assessed evidence signer %s has an invalid Ed25519 public key", constants.ErrEvidenceTrustNotAssessed, trusted.GetKeyId())
		}
		digest := sha256.Sum256(publicKey)
		if trusted.GetPublicKeySha256() != hex.EncodeToString(digest[:]) {
			return nil, fmt.Errorf("%w: assessed evidence signer %s public key digest mismatch", constants.ErrEvidenceTrustNotAssessed, trusted.GetKeyId())
		}
		keys[trusted.GetKeyId()] = ed25519.PublicKey(publicKey)
	}
	return &assessedEvidenceTrust{keys: keys}, nil
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

func loadComplianceAssessmentScope(scopePath string) (*compliancev1.AssessmentScope, error) {
	body, err := readComplianceReportInputFile(scopePath)
	if err != nil {
		return nil, fmt.Errorf("compliance report: read assessment scope: %w", err)
	}
	scope := &compliancev1.AssessmentScope{}
	if err := compliancev1.UnmarshalCanonical(body, scope); err != nil {
		return nil, fmt.Errorf("%w: decode canonical assessment scope: %w", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := catalog.ValidateAssessmentScope(scope); err != nil {
		return nil, fmt.Errorf("compliance report: validate assessment scope: %w", err)
	}
	return scope, nil
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
	artifacts, err := collectRuntimeSourceArtifacts(ctx, reader, runtimeRoot, runtimeRoot, bundleRoot, false)
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

func buildEvaluationReportSources(ctx context.Context, fileSvc fs.RuntimeFileService, scope *compliancev1.AssessmentScope, runIDs []string, verifiedAt time.Time) ([]compliancereport.GenerationSource, []compliancereport.SourceArtifact, error) {
	sources := make([]compliancereport.GenerationSource, 0, len(runIDs))
	artifacts := make([]compliancereport.SourceArtifact, 0, len(runIDs))
	store := evaluation.NewStore(fileSvc)
	selected := make(map[string]struct{}, len(runIDs))
	for _, runID := range runIDs {
		if !evidence.ValidPathElement(runID) {
			return nil, nil, fmt.Errorf("%w: invalid eval run ID %q", constants.ErrPathValidation, runID)
		}
		if _, exists := selected[runID]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate eval run %s", constants.ErrValidationFailed, runID)
		}
		selected[runID] = struct{}{}
		admission, err := selectedEvaluationAdmission(scope, runID)
		if err != nil {
			return nil, nil, err
		}
		inventory, err := store.InspectRun(ctx, runID)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: inspect eval run %s: %w", constants.ErrEvalRunVerificationFailed, runID, err)
		}
		switch inventory.Kind {
		case evaluation.RunKindCampaign:
			if admission.GetSourceKind() != constants.EvaluationSourceKindCampaign || admission.GetSourceVersion() != constants.EvaluationSourceVersion || admission.GetVerifierRef().GetId() != constants.CampaignVerifierID || admission.GetVerifierRef().GetVersion() != constants.CampaignVerifierVersion {
				return nil, nil, fmt.Errorf("%w: campaign eval run %s does not match protected source admission %s", constants.ErrEvidenceScopeMismatch, runID, admission.GetAdmissionId())
			}
			campaignSource, campaignArtifacts, err := buildCampaignReportSource(ctx, fileSvc, admission, verifiedAt)
			if err != nil {
				return nil, nil, err
			}
			sources = append(sources, campaignSource)
			artifacts = append(artifacts, campaignArtifacts...)
			continue
		case evaluation.RunKindIncomplete:
			return nil, nil, fmt.Errorf("%w: eval run %s is incomplete: %s", constants.ErrEvalRunVerificationFailed, runID, inventory.Reason)
		case evaluation.RunKindUnsupported:
			return nil, nil, fmt.Errorf("%w: eval run %s is unsupported: %s", constants.ErrUnsupportedVerifier, runID, inventory.Reason)
		case evaluation.RunKindMalformed:
			return nil, nil, fmt.Errorf("%w: eval run %s is malformed: %s", constants.ErrEvidenceArtifactMalformed, runID, inventory.Reason)
		case evaluation.RunKindNative:
		default:
			return nil, nil, fmt.Errorf("%w: eval run %s has unknown inventory disposition %q", constants.ErrUnsupportedVerifier, runID, inventory.Kind)
		}
		if admission.GetSourceKind() != constants.EvaluationSourceKindNative || admission.GetSourceVersion() != constants.EvaluationSourceVersion || admission.GetVerifierRef().GetId() != constants.EvalRunVerifierID || admission.GetVerifierRef().GetVersion() != constants.EvalRunVerifierVersion {
			return nil, nil, fmt.Errorf("%w: native eval run %s does not match protected source admission %s", constants.ErrEvidenceScopeMismatch, runID, admission.GetAdmissionId())
		}
		runtimeRoot := path.Join(constants.DataDirname, constants.EvaluationDirname, constants.EvaluationRunsDirname, runID)
		bundleRoot := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, runID)
		report, err := evaluation.NewVerifier(fileSvc, evaluation.NewRegistry(), func() time.Time { return verifiedAt }).Verify(ctx, runID)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: verify eval run %s: %w", constants.ErrEvalRunVerificationFailed, runID, err)
		}
		if !report.GetValid() {
			return nil, nil, fmt.Errorf("%w: eval run %s has %d verification failures", constants.ErrEvalRunVerificationFailed, runID, len(report.GetFailures()))
		}
		rawArtifacts, err := collectRuntimeSourceArtifacts(ctx, fileSvc, runtimeRoot, runtimeRoot, bundleRoot, true)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: collect eval runtime %s: %w", constants.ErrEvalRunVerificationFailed, runID, err)
		}
		artifacts = append(artifacts, rawArtifacts...)
		body, err := compliancev1.MarshalCanonical(report)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: canonicalize eval verification report %s: %w", constants.ErrEvalRunVerificationFailed, runID, err)
		}
		artifacts = append(artifacts, compliancereport.SourceArtifact{
			BundlePath: path.Join(bundleRoot, constants.ComplianceBundleSourceVerificationFilename),
			Body:       body,
			MediaType:  constants.MediaTypeJSON,
		})
		sources = append(sources, compliancereport.GenerationSource{AdmissionID: admission.GetAdmissionId(), Importer: evaluation.NewEvidenceImporter(fileSvc, runID, func() time.Time { return verifiedAt })})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].BundlePath < artifacts[j].BundlePath })
	return sources, artifacts, nil
}

func campaignVerificationPolicyForAdmission(admission *compliancev1.AssessmentSourceAdmission, assessmentAsOf time.Time) (evaluation.CampaignVerificationPolicy, error) {
	if admission == nil || admission.GetSourceKind() != constants.EvaluationSourceKindCampaign {
		return evaluation.CampaignVerificationPolicy{}, fmt.Errorf("%w: campaign source admission is required", constants.ErrEvidenceScopeMismatch)
	}
	providerPolicy, err := campaignProviderObservationPolicy(admission.GetProviderObservationPolicy())
	if err != nil {
		return evaluation.CampaignVerificationPolicy{}, err
	}
	provenancePolicy, err := campaignModelProvenancePolicy(admission.GetModelProvenancePolicy())
	if err != nil {
		return evaluation.CampaignVerificationPolicy{}, err
	}
	return evaluation.CampaignVerificationPolicy{
		VerifierReleaseVersion: constants.EvaluationSourceVersion,
		ProviderObservation:    providerPolicy,
		ModelProvenance:        provenancePolicy,
		AssessmentTime:         func() time.Time { return assessmentAsOf },
	}, nil
}

func campaignProviderObservationPolicy(policy compliancev1.AssessmentWitnessPolicy) (evaluation.ProviderObservationPolicy, error) {
	switch policy {
	case compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_INTERIM:
		return evaluation.ProviderObservationPolicyInterim, nil
	case compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_STRICT:
		return evaluation.ProviderObservationPolicyStrict, nil
	default:
		return evaluation.ProviderObservationPolicyInterim, fmt.Errorf("%w: unsupported campaign provider-observation witness policy %s", constants.ErrUnsupportedVerifier, policy)
	}
}

func campaignModelProvenancePolicy(policy compliancev1.AssessmentWitnessPolicy) (evaluation.ModelProvenancePolicy, error) {
	switch policy {
	case compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_INTERIM:
		return evaluation.ModelProvenancePolicyInterim, nil
	case compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_STRICT:
		return evaluation.ModelProvenancePolicyStrict, nil
	default:
		return evaluation.ModelProvenancePolicyInterim, fmt.Errorf("%w: unsupported campaign model-provenance witness policy %s", constants.ErrUnsupportedVerifier, policy)
	}
}

func campaignWitnessPolicyProto(policy compliancev1.AssessmentWitnessPolicy) evalv1.EvaluationWitnessPolicy {
	if policy == compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_STRICT {
		return evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT
	}
	return evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM
}

func buildCampaignReportSource(ctx context.Context, fileSvc fs.RuntimeFileService, admission *compliancev1.AssessmentSourceAdmission, assessmentAsOf time.Time) (compliancereport.GenerationSource, []compliancereport.SourceArtifact, error) {
	runID := admission.GetRunId()
	store := evaluation.NewStore(fileSvc)
	run, err := store.LoadRun(ctx, runID)
	if err != nil {
		return compliancereport.GenerationSource{}, nil, err
	}
	campaignID := run.GetCampaignBinding().GetCampaignId()
	bundleRoot := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, admission.GetAdmissionId())
	runtimeBundleRoot := path.Join(bundleRoot, constants.ComplianceBundleSourceRuntimeDirname)
	roots := []string{
		path.Join(constants.DataDirname, constants.EvaluationDirname, constants.EvaluationRunsDirname, runID),
		path.Join(constants.DataDirname, constants.EvaluationDirname, constants.EvaluationCampaignsDirname, campaignID),
	}
	artifacts := make([]compliancereport.SourceArtifact, 0)
	bodies := make(map[string][]byte)
	for _, root := range roots {
		captured, captureErr := collectRuntimeSourceArtifacts(ctx, fileSvc, constants.PathCurrentDir, root, bundleRoot, false)
		if captureErr != nil {
			return compliancereport.GenerationSource{}, nil, fmt.Errorf("%w: capture campaign source %s: %w", constants.ErrEvalRunVerificationFailed, runID, captureErr)
		}
		for _, artifact := range captured {
			runtimePath := strings.TrimPrefix(artifact.BundlePath, runtimeBundleRoot+"/")
			bodies[filepath.FromSlash(runtimePath)] = append([]byte(nil), artifact.Body...)
		}
		artifacts = append(artifacts, captured...)
	}
	assignments, err := store.ListAssignments(ctx, runID)
	if err != nil {
		return compliancereport.GenerationSource{}, nil, err
	}
	attemptIDs := make(map[string]struct{})
	for _, assignment := range assignments {
		exists, existsErr := store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId())
		if existsErr != nil {
			return compliancereport.GenerationSource{}, nil, existsErr
		}
		if !exists {
			continue
		}
		result, loadErr := store.LoadAssignmentResult(ctx, runID, assignment.GetAssignmentId())
		if loadErr != nil {
			return compliancereport.GenerationSource{}, nil, loadErr
		}
		for _, inferenceRecord := range result.GetModelInferences() {
			if inferenceRecord.GetProviderAttemptId() != "" {
				attemptIDs[inferenceRecord.GetProviderAttemptId()] = struct{}{}
			}
		}
	}
	attemptPaths := make([]string, 0, len(attemptIDs)*3)
	for attemptID := range attemptIDs {
		attemptPaths = append(attemptPaths,
			path.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceAttemptsDirname, attemptID+constants.FileExtJSON),
			path.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceProviderObserverDirname, constants.InferenceProviderObserverWindowsDirname, attemptID+constants.FileExtJSON),
			path.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceModelProvenanceDirname, constants.InferenceModelProvenanceWindowsDirname, attemptID+constants.FileExtJSON),
		)
	}
	sort.Strings(attemptPaths)
	for _, runtimePath := range attemptPaths {
		exists, existsErr := fileSvc.FileExists(ctx, filepath.FromSlash(runtimePath))
		if existsErr != nil {
			return compliancereport.GenerationSource{}, nil, existsErr
		}
		if !exists {
			continue
		}
		body, readErr := fileSvc.ReadFile(ctx, filepath.FromSlash(runtimePath))
		if readErr != nil {
			return compliancereport.GenerationSource{}, nil, readErr
		}
		bodies[filepath.FromSlash(runtimePath)] = append([]byte(nil), body...)
		artifacts = append(artifacts, compliancereport.SourceArtifact{BundlePath: path.Join(runtimeBundleRoot, runtimePath), Body: body, MediaType: constants.MediaTypeJSON})
	}
	policy, err := campaignVerificationPolicyForAdmission(admission, assessmentAsOf)
	if err != nil {
		return compliancereport.GenerationSource{}, nil, err
	}
	inventory := &evalv1.CampaignComplianceSourceInventory{SchemaVersion: constants.CampaignSourceInventoryVersion, AdmissionId: admission.GetAdmissionId(), RunId: runID, CampaignId: campaignID, ProviderObservationPolicy: campaignWitnessPolicyProto(admission.GetProviderObservationPolicy()), ModelProvenancePolicy: campaignWitnessPolicyProto(admission.GetModelProvenancePolicy())}
	runtimePaths := make([]string, 0, len(bodies))
	for runtimePath := range bodies {
		runtimePaths = append(runtimePaths, filepath.ToSlash(runtimePath))
	}
	sort.Strings(runtimePaths)
	for _, runtimePath := range runtimePaths {
		digest := sha256.Sum256(bodies[filepath.FromSlash(runtimePath)])
		inventory.Artifacts = append(inventory.Artifacts, &evalv1.CampaignComplianceSourceArtifact{RuntimePath: runtimePath, Sha256: hex.EncodeToString(digest[:]), MediaType: constants.MediaTypeJSON})
	}
	inventoryBody, err := evalv1.MarshalCanonical(inventory)
	if err != nil {
		return compliancereport.GenerationSource{}, nil, err
	}
	artifacts = append(artifacts, compliancereport.SourceArtifact{BundlePath: path.Join(bundleRoot, constants.CampaignSourceInventoryFilename), Body: inventoryBody, MediaType: constants.MediaTypeJSON})
	reader := &explicitSourceReader{bodies: bodies}
	importer := evaluation.NewCampaignImporter(reader, runID, policy)
	nodes, err := importer.Import(ctx)
	if err != nil {
		return compliancereport.GenerationSource{}, nil, err
	}
	if len(nodes) != 1 {
		return compliancereport.GenerationSource{}, nil, fmt.Errorf("%w: campaign importer returned an invalid manifest population", constants.ErrInvalidEvidenceGraph)
	}
	artifacts = append(artifacts, compliancereport.SourceArtifact{BundlePath: path.Join(bundleRoot, constants.ComplianceBundleSourceVerificationFilename), Body: nodes[0].CanonicalBytes, MediaType: constants.MediaTypeJSON})
	return compliancereport.GenerationSource{AdmissionID: admission.GetAdmissionId(), Importer: importer}, artifacts, nil
}

func buildEvaluationSelectionDiagnostics(ctx context.Context, store *evaluation.Store, scope *compliancev1.AssessmentScope, selectedRunIDs []string) ([]*compliancev1.AssessmentDiagnostic, *compliancereport.SourceArtifact, error) {
	inventory, err := store.ListRunInventory(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: list eval run inventory: %w", constants.ErrEvalRunVerificationFailed, err)
	}
	selected := make(map[string]*compliancev1.AssessmentSourceAdmission, len(selectedRunIDs))
	for _, runID := range selectedRunIDs {
		admission, selectionErr := selectedEvaluationAdmission(scope, runID)
		if selectionErr != nil {
			return nil, nil, selectionErr
		}
		selected[runID] = admission
	}
	diagnostics := make([]*compliancev1.AssessmentDiagnostic, 0, len(inventory))
	for _, candidate := range inventory {
		code := "evaluation_candidate_" + string(candidate.Kind)
		severity := "info"
		message := "evaluation candidate is " + string(candidate.Kind)
		if selected[candidate.RunID] == nil && (candidate.Kind == evaluation.RunKindNative || candidate.Kind == evaluation.RunKindCampaign) {
			inWindow, timeKnown, windowErr := evaluationCandidateInWindow(ctx, store, candidate, scope.GetAssessmentWindowStart().AsTime(), scope.GetAssessmentWindowEnd().AsTime())
			if windowErr != nil {
				return nil, nil, windowErr
			}
			if timeKnown && !inWindow {
				code = "evaluation_candidate_outside_window"
				message = "evaluation candidate is outside the protected assessment window"
			}
		}
		if candidate.Reason != "" {
			message += ": " + candidate.Reason
		}
		if candidate.Kind == evaluation.RunKindIncomplete || candidate.Kind == evaluation.RunKindUnsupported {
			severity = "warning"
		}
		if candidate.Kind == evaluation.RunKindMalformed {
			severity = "error"
		}
		diagnostic := &compliancev1.AssessmentDiagnostic{
			Code:     code,
			Severity: severity,
			Subject:  &compliancev1.AssessmentSubjectSelection{RunId: candidate.RunID},
			Message:  message,
		}
		if admission := selected[candidate.RunID]; admission != nil {
			diagnostic.SourceAdmissionId = admission.GetAdmissionId()
		}
		diagnostics = append(diagnostics, diagnostic)
	}
	body, err := compliancereport.MarshalAssessmentDiagnostics(diagnostics)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: canonicalize eval selection diagnostics: %w", constants.ErrEvalRunVerificationFailed, err)
	}
	artifact := &compliancereport.SourceArtifact{
		BundlePath: path.Join(constants.ComplianceBundleSourcesDirname, constants.EvaluationSelectionDiagnosticsFilename),
		Body:       body,
		MediaType:  constants.MediaTypeJSON,
	}
	return diagnostics, artifact, nil
}

func evaluationCandidateInWindow(ctx context.Context, store *evaluation.Store, candidate evaluation.RunInventoryEntry, windowStart, windowEnd time.Time) (bool, bool, error) {
	var run *evalv1.EvaluationRun
	switch candidate.Kind {
	case evaluation.RunKindNative:
		report, err := store.LoadReport(ctx, candidate.RunID)
		if err != nil {
			return false, false, fmt.Errorf("%w: load native eval candidate %s: %w", constants.ErrEvalRunVerificationFailed, candidate.RunID, err)
		}
		run = report.GetRun()
	case evaluation.RunKindCampaign:
		loaded, err := store.LoadRun(ctx, candidate.RunID)
		if err != nil {
			return false, false, fmt.Errorf("%w: load campaign eval candidate %s: %w", constants.ErrEvalRunVerificationFailed, candidate.RunID, err)
		}
		run = loaded
	default:
		return false, false, nil
	}
	if run.GetStartedAt() == nil && run.GetCompletedAt() == nil {
		return false, false, nil
	}
	var startedAt, completedAt time.Time
	if run.GetStartedAt() != nil {
		startedAt = run.GetStartedAt().AsTime()
	}
	if run.GetCompletedAt() != nil {
		completedAt = run.GetCompletedAt().AsTime()
	}
	if startedAt.IsZero() {
		startedAt = completedAt
	}
	if completedAt.IsZero() {
		completedAt = startedAt
	}
	return !completedAt.Before(windowStart) && !startedAt.After(windowEnd), true, nil
}

func selectedEvaluationAdmission(scope *compliancev1.AssessmentScope, runID string) (*compliancev1.AssessmentSourceAdmission, error) {
	var selected *compliancev1.AssessmentSourceAdmission
	for _, admission := range scope.GetSourceAdmissions() {
		if admission.GetRunId() != runID {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("%w: eval run %s matches multiple protected source admissions", constants.ErrValidationFailed, runID)
		}
		selected = admission
	}
	if selected == nil {
		return nil, fmt.Errorf("%w: eval run %s has no protected source admission", constants.ErrEvidenceScopeMismatch, runID)
	}
	return selected, nil
}

func collectRuntimeSourceArtifacts(ctx context.Context, reader evidence.ArtifactReader, runtimeRoot, currentPath, bundleRoot string, allowEmpty bool) ([]compliancereport.SourceArtifact, error) {
	budget := recursiveDirectoryBudget{}
	return collectRuntimeSourceArtifactsRecursive(ctx, reader, runtimeRoot, currentPath, bundleRoot, allowEmpty, 0, &budget)
}

func collectRuntimeSourceArtifactsRecursive(ctx context.Context, reader evidence.ArtifactReader, runtimeRoot, currentPath, bundleRoot string, allowEmpty bool, depth int, budget *recursiveDirectoryBudget) ([]compliancereport.SourceArtifact, error) {
	if err := budget.consume(depth, 0); err != nil {
		return nil, err
	}
	entries, err := reader.ReadDir(ctx, currentPath)
	if err != nil {
		return nil, err
	}
	if len(entries) > constants.DemoRunMaxArtifactsPerDirectory {
		return nil, constants.ErrEvidenceArtifactTooLarge
	}
	if err := budget.consume(depth, len(entries)); err != nil {
		return nil, err
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
			nested, err := collectRuntimeSourceArtifactsRecursive(ctx, reader, runtimeRoot, sourcePath, bundleRoot, allowEmpty, depth+1, budget)
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
		if !allowEmpty && len(body) == 0 || int64(len(body)) > constants.ComplianceBundleMaxArtifactBytes {
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

type explicitSourceReader struct {
	bodies map[string][]byte
}

func (r *explicitSourceReader) ReadFile(ctx context.Context, sourcePath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, exists := r.bodies[sourcePath]
	if !exists {
		return nil, constants.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func (r *explicitSourceReader) ReadDir(ctx context.Context, sourcePath string) ([]os.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(sourcePath, string(filepath.Separator)) + string(filepath.Separator)
	entries := make(map[string]bool)
	for candidate := range r.bodies {
		if !strings.HasPrefix(candidate, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(candidate, prefix)
		parts := strings.SplitN(remainder, string(filepath.Separator), 2)
		if parts[0] != "" {
			entries[parts[0]] = len(parts) == 2
		}
	}
	if len(entries) == 0 {
		return nil, constants.ErrNotFound
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]os.DirEntry, 0, len(names))
	for _, name := range names {
		result = append(result, explicitSourceDirEntry{name: name, directory: entries[name]})
	}
	return result, nil
}

type explicitSourceDirEntry struct {
	name      string
	directory bool
}

func (e explicitSourceDirEntry) Name() string      { return e.name }
func (e explicitSourceDirEntry) IsDir() bool       { return e.directory }
func (e explicitSourceDirEntry) Type() os.FileMode { return 0 }
func (e explicitSourceDirEntry) Info() (os.FileInfo, error) {
	return nil, constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) FileExists(ctx context.Context, sourcePath string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, exists := r.bodies[sourcePath]
	return exists, nil
}
func (r *explicitSourceReader) MkdirAll(context.Context, string, os.FileMode) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) CreateRuntimeTree(context.Context) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) Stat(context.Context, string) (os.FileInfo, error) {
	return nil, constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) Lstat(context.Context, string) (os.FileInfo, error) {
	return nil, constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) WriteFile(context.Context, string, []byte, os.FileMode) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) OpenForAppend(context.Context, string, os.FileMode) (*os.File, error) {
	return nil, constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) OpenForRead(context.Context, string) (*os.File, error) {
	return nil, constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) Remove(context.Context, string) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) RemoveAll(context.Context, string) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) Rename(context.Context, string, string) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) EnforceDirPermissions(context.Context, string, os.FileMode) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) EnforceFilePermissions(context.Context, string, os.FileMode) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *explicitSourceReader) Resolve(sourcePath string) string             { return sourcePath }
func (r *explicitSourceReader) Rel(sourcePath string) (string, error)        { return sourcePath, nil }
func (r *explicitSourceReader) RelFromAbs(sourcePath string) (string, error) { return sourcePath, nil }

type standaloneReportSourceInput struct {
	scopeID              string
	verifiedAt           time.Time
	evidenceTrust        evidence.AssessedSignerSource
	ksiRunID             string
	ksiHistory           string
	ksiResults           string
	commitmentRunID      string
	commitment           string
	commitmentAttemptID  string
	commitmentScenarioID string
	attestationRunID     string
	attestations         string
	auditRunID           string
	auditRecords         []string
	auditAttemptID       string
	auditScenarioID      string
	ledgerRunID          string
	ledgerCommits        string
	ledgerState          string
	ledgerAttemptID      string
	ledgerScenarioID     string
	buildRunID           string
	buildConfig          string
}

func buildOperationalReportSources(ctx context.Context, sourceDirs []string, scope *compliancev1.AssessmentScope, trust evidence.AssessedSignerSource, assessmentAsOf time.Time) ([]compliancereport.GenerationSource, []compliancereport.SourceArtifact, error) {
	if len(sourceDirs) == 0 {
		return nil, nil, nil
	}
	if scope == nil || trust == nil || assessmentAsOf.IsZero() {
		return nil, nil, fmt.Errorf("%w: operational sources require protected scope, assessed evidence trust, and assessment time", constants.ErrValidationFailed)
	}
	admissions := make(map[string]*compliancev1.AssessmentSourceAdmission, len(scope.GetSourceAdmissions()))
	for _, admission := range scope.GetSourceAdmissions() {
		admissions[admission.GetAdmissionId()] = admission
	}
	sources := make([]compliancereport.GenerationSource, 0, len(sourceDirs))
	artifacts := make([]compliancereport.SourceArtifact, 0)
	seen := make(map[string]struct{}, len(sourceDirs))
	for _, sourceDir := range sourceDirs {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		inventoryBody, err := readComplianceReportInputFile(filepath.Join(sourceDir, constants.ComplianceOperationalInventoryFilename))
		if err != nil {
			return nil, nil, fmt.Errorf("%w: read operational source inventory: %w", constants.ErrEvidenceImporterFailed, err)
		}
		inventory := &evidence.OperationalSourceInventory{}
		decoder := json.NewDecoder(bytes.NewReader(inventoryBody))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(inventory); err != nil {
			return nil, nil, fmt.Errorf("%w: decode operational source inventory: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		admission := admissions[inventory.AdmissionID]
		if admission == nil {
			return nil, nil, fmt.Errorf("%w: operational source admission %s is not protected by the assessment scope", constants.ErrEvidenceScopeMismatch, inventory.AdmissionID)
		}
		if _, duplicate := seen[inventory.AdmissionID]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate operational source admission %s", constants.ErrEvidenceDuplicateID, inventory.AdmissionID)
		}
		seen[inventory.AdmissionID] = struct{}{}
		sourceRoot := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, inventory.AdmissionID)
		inventoryPath := path.Join(sourceRoot, constants.ComplianceOperationalInventoryFilename)
		bodies := map[string][]byte{inventoryPath: inventoryBody}
		sourceArtifacts := []compliancereport.SourceArtifact{{BundlePath: inventoryPath, Body: inventoryBody, MediaType: constants.MediaTypeJSON}}
		for _, artifact := range inventory.Artifacts {
			if !evidence.ValidRelativePath(artifact.RelativePath) {
				return nil, nil, fmt.Errorf("%w: invalid operational artifact path %s", constants.ErrPathValidation, artifact.RelativePath)
			}
			body, err := readComplianceReportInputFile(filepath.Join(sourceDir, artifact.RelativePath))
			if err != nil {
				return nil, nil, fmt.Errorf("%w: read operational source artifact: %w", constants.ErrEvidenceImporterFailed, err)
			}
			bundlePath := path.Join(sourceRoot, artifact.RelativePath)
			if _, duplicate := bodies[bundlePath]; duplicate {
				return nil, nil, fmt.Errorf("%w: duplicate operational source path %s", constants.ErrEvidenceDuplicateID, bundlePath)
			}
			bodies[bundlePath] = body
			sourceArtifacts = append(sourceArtifacts, compliancereport.SourceArtifact{BundlePath: bundlePath, Body: body, MediaType: constants.MediaTypeJSON})
		}
		reader := &explicitSourceReader{bodies: bodies}
		importer := evidence.NewOperationalExportImporter(reader, trust, inventoryPath, sourceRoot, scope.GetScopeId(), admission, assessmentAsOf, func() time.Time { return assessmentAsOf })
		sources = append(sources, compliancereport.GenerationSource{AdmissionID: admission.GetAdmissionId(), Importer: importer})
		artifacts = append(artifacts, sourceArtifacts...)
	}
	return sources, artifacts, nil
}

func appendStandaloneReportSourceArtifact(artifacts *[]compliancereport.SourceArtifact, bundlePath string, body []byte) error {
	for _, artifact := range *artifacts {
		if artifact.BundlePath == bundlePath {
			return fmt.Errorf("%w: duplicate standalone source path %s", constants.ErrEvidenceDuplicateID, bundlePath)
		}
	}
	*artifacts = append(*artifacts, compliancereport.SourceArtifact{BundlePath: bundlePath, Body: body, MediaType: constants.MediaTypeJSON})
	return nil
}

func buildStandaloneReportSources(ctx context.Context, input standaloneReportSourceInput) ([]evidence.EvidenceImporter, []compliancereport.SourceArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	importers := make([]evidence.EvidenceImporter, 0, 8)
	artifacts := make([]compliancereport.SourceArtifact, 0, 10)
	ksiRequested := input.ksiRunID != "" || input.ksiHistory != "" || input.ksiResults != ""
	if ksiRequested {
		if !evidence.ValidPathElement(input.ksiRunID) || input.ksiHistory == "" || input.ksiResults == "" {
			return nil, nil, fmt.Errorf("%w: --ksi-run-id, --ksi-history, and --ksi-results are required together", constants.ErrValidationFailed)
		}
		historyBody, err := readComplianceReportInputFile(input.ksiHistory)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: read KSI history: %w", constants.ErrEvidenceImporterFailed, err)
		}
		resultsBody, err := readComplianceReportInputFile(input.ksiResults)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: read current KSI results: %w", constants.ErrEvidenceImporterFailed, err)
		}
		resultSet, latestBody, err := inspectKSIHistorySource(historyBody)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: inspect KSI history: %w", constants.ErrEvidenceImporterFailed, err)
		}
		if !bytes.Equal(resultsBody, latestBody) || resultSet.Binding.ScopeID != input.scopeID || resultSet.Binding.RunID != input.ksiRunID {
			return nil, nil, fmt.Errorf("%w: KSI source scope, run, or current result binding does not match", constants.ErrEvidenceScopeMismatch)
		}
		historyPath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, input.scopeID, input.ksiRunID, constants.ComplianceBundleKSIHistoryFilename)
		resultsPath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, input.scopeID, input.ksiRunID, constants.ComplianceBundleKSIResultsFilename)
		reader := &explicitSourceReader{bodies: map[string][]byte{historyPath: historyBody}}
		importers = append(importers, evidence.NewKSIHistoryImporter(reader, evidence.KSIHistoryImportBinding{Reference: evidence.ContentReferenceForBody(constants.KSIHistoryReferencePrefix, historyBody), Path: historyPath, ScopeID: input.scopeID, RunID: input.ksiRunID, Class: resultSet.Class, ProducerIdentity: constants.KSIEvaluatorID, AssertionAssessments: resultSet.Binding.AssertionAssessments}))
		if err := appendStandaloneReportSourceArtifact(&artifacts, historyPath, historyBody); err != nil {
			return nil, nil, err
		}
		if err := appendStandaloneReportSourceArtifact(&artifacts, resultsPath, resultsBody); err != nil {
			return nil, nil, err
		}
	}
	commitmentRequested := input.commitmentRunID != "" || input.commitment != ""
	if commitmentRequested {
		if !evidence.ValidPathElement(input.commitmentRunID) || input.commitment == "" || input.evidenceTrust == nil || input.verifiedAt.IsZero() {
			return nil, nil, fmt.Errorf("%w: commitment source, run, verification time, and assessed evidence trust are required", constants.ErrValidationFailed)
		}
		body, err := readComplianceReportInputFile(input.commitment)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: read commitment: %w", constants.ErrEvidenceImporterFailed, err)
		}
		attestation := &operatorv1.CommitmentAttestation{}
		if err := compliancev1.UnmarshalCanonical(body, attestation); err != nil || attestation.GetTransactionId() == "" {
			return nil, nil, fmt.Errorf("%w: decode commitment", constants.ErrEvidenceArtifactMalformed)
		}
		bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, input.scopeID, input.commitmentRunID, constants.ComplianceBundleCommitmentsFilename)
		reader := &explicitSourceReader{bodies: map[string][]byte{bundlePath: body}}
		importers = append(importers, evidence.NewCommitmentImporter(reader, input.evidenceTrust, evidence.CommitmentImportBinding{Reference: evidence.ContentReferenceForBody(constants.CommitmentReferencePrefix, body), Path: bundlePath, ScopeID: input.scopeID, RunID: input.commitmentRunID, AttemptID: input.commitmentAttemptID, ScenarioID: input.commitmentScenarioID, TransactionID: attestation.GetTransactionId()}, input.verifiedAt))
		if err := appendStandaloneReportSourceArtifact(&artifacts, bundlePath, body); err != nil {
			return nil, nil, err
		}
	}
	attestationRequested := input.attestationRunID != "" || input.attestations != ""
	if attestationRequested {
		if !evidence.ValidPathElement(input.attestationRunID) || input.attestations == "" || input.evidenceTrust == nil || input.verifiedAt.IsZero() {
			return nil, nil, fmt.Errorf("%w: attestation source, run, verification time, and assessed evidence trust are required", constants.ErrValidationFailed)
		}
		body, err := readComplianceReportInputFile(input.attestations)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: read attestations: %w", constants.ErrEvidenceImporterFailed, err)
		}
		bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, input.scopeID, input.attestationRunID, constants.ComplianceBundleAttestationsFilename)
		reader := &explicitSourceReader{bodies: map[string][]byte{bundlePath: body}}
		importers = append(importers, evidence.NewAttestationImporter(reader, input.evidenceTrust, evidence.AttestationImportBinding{Reference: evidence.ContentReferenceForBody(constants.AttestationCollectionReferencePrefix, body), Path: bundlePath, ScopeID: input.scopeID, RunID: input.attestationRunID}, input.verifiedAt))
		if err := appendStandaloneReportSourceArtifact(&artifacts, bundlePath, body); err != nil {
			return nil, nil, err
		}
	}
	auditRequested := input.auditRunID != "" || len(input.auditRecords) != 0
	if auditRequested {
		if !evidence.ValidPathElement(input.auditRunID) || len(input.auditRecords) == 0 {
			return nil, nil, fmt.Errorf("%w: --audit-run-id and at least one --audit-record are required together", constants.ErrValidationFailed)
		}
		for _, sourcePath := range input.auditRecords {
			body, err := readComplianceReportInputFile(sourcePath)
			if err != nil {
				return nil, nil, fmt.Errorf("%w: read audit record: %w", constants.ErrEvidenceImporterFailed, err)
			}
			event := &operatorv1.AuditEvent{}
			if err := compliancev1.UnmarshalCanonical(body, event); err != nil || event.GetOperatorSessionId() == "" {
				return nil, nil, fmt.Errorf("%w: decode audit record", constants.ErrEvidenceArtifactMalformed)
			}
			reference := evidence.ContentReferenceForBody(constants.AuditRecordReferencePrefix, body)
			_, digest, _ := evidence.ParseExpectedContentReference(reference, constants.AuditRecordReferencePrefix)
			bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, input.scopeID, input.auditRunID, constants.AuditRecordsDirname, digest+constants.FileExtJSON)
			reader := &explicitSourceReader{bodies: map[string][]byte{bundlePath: body}}
			importers = append(importers, evidence.NewAuditRecordImporter(reader, evidence.AuditRecordImportBinding{Reference: reference, Path: bundlePath, ScopeID: input.scopeID, RunID: input.auditRunID, AttemptID: input.auditAttemptID, ScenarioID: input.auditScenarioID, OperatorSessionID: event.GetOperatorSessionId()}))
			if err := appendStandaloneReportSourceArtifact(&artifacts, bundlePath, body); err != nil {
				return nil, nil, err
			}
		}
	}
	ledgerRequested := input.ledgerRunID != "" || input.ledgerCommits != "" || input.ledgerState != ""
	if ledgerRequested {
		if !evidence.ValidPathElement(input.ledgerRunID) || input.ledgerCommits == "" || input.ledgerState == "" {
			return nil, nil, fmt.Errorf("%w: --ledger-run-id, --ledger-commits, and --ledger-state are required together", constants.ErrValidationFailed)
		}
		commitsBody, err := readComplianceReportInputFile(input.ledgerCommits)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: read ledger commits: %w", constants.ErrEvidenceImporterFailed, err)
		}
		stateBody, err := readComplianceReportInputFile(input.ledgerState)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: read ledger state: %w", constants.ErrEvidenceImporterFailed, err)
		}
		base := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, input.scopeID, input.ledgerRunID, constants.LedgerEvidenceDirname)
		commitsPath := path.Join(base, constants.LedgerCommitsFilename)
		statePath := path.Join(base, constants.LedgerStateFilename)
		reader := &explicitSourceReader{bodies: map[string][]byte{commitsPath: commitsBody, statePath: stateBody}}
		importers = append(importers, evidence.NewLedgerImporter(reader, evidence.LedgerImportBinding{CommitsReference: evidence.ContentReferenceForBody(constants.LedgerCommitCollectionReferencePrefix, commitsBody), StateReference: evidence.ContentReferenceForBody(constants.LedgerStateReferencePrefix, stateBody), CommitsPath: commitsPath, StatePath: statePath, ScopeID: input.scopeID, RunID: input.ledgerRunID, AttemptID: input.ledgerAttemptID, ScenarioID: input.ledgerScenarioID}))
		if err := appendStandaloneReportSourceArtifact(&artifacts, commitsPath, commitsBody); err != nil {
			return nil, nil, err
		}
		if err := appendStandaloneReportSourceArtifact(&artifacts, statePath, stateBody); err != nil {
			return nil, nil, err
		}
	}
	buildRequested := input.buildRunID != "" || input.buildConfig != ""
	if buildRequested {
		if !evidence.ValidPathElement(input.buildRunID) || input.buildConfig == "" {
			return nil, nil, fmt.Errorf("%w: --build-run-id and --build-config-attestations are required together", constants.ErrValidationFailed)
		}
		body, err := readComplianceReportInputFile(input.buildConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: read build and configuration attestations: %w", constants.ErrEvidenceImporterFailed, err)
		}
		metadata, err := evidence.InspectBuildConfigSource(body)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: inspect build and configuration attestations: %w", constants.ErrEvidenceImporterFailed, err)
		}
		bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, input.scopeID, input.buildRunID, constants.BuildConfigAttestationsFilename)
		reader := &explicitSourceReader{bodies: map[string][]byte{bundlePath: body}}
		importers = append(importers, evidence.NewBuildConfigImporter(reader, evidence.BuildConfigImportBinding{Reference: evidence.ContentReferenceForBody(constants.BuildAttestationReferencePrefix, body), Path: bundlePath, ScopeID: input.scopeID, RunID: input.buildRunID, BuildIdentity: metadata.BuildIdentity, SourceRevision: metadata.SourceRevision, ProducerIdentity: metadata.ProducerIdentity}))
		if err := appendStandaloneReportSourceArtifact(&artifacts, bundlePath, body); err != nil {
			return nil, nil, err
		}
	}
	return importers, artifacts, nil
}

func inspectKSIHistorySource(body []byte) (*compliance.KSIResultSet, []byte, error) {
	var first *compliance.KSIResultSet
	var latest []byte
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		if err := evidence.ValidateCanonicalJSON(line); err != nil {
			return nil, nil, err
		}
		resultSet := &compliance.KSIResultSet{}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(resultSet); err != nil {
			return nil, nil, err
		}
		if first == nil {
			first = resultSet
		}
		latest = append([]byte(nil), line...)
	}
	if first == nil {
		return nil, nil, constants.ErrEvidenceArtifactMalformed
	}
	return first, latest, nil
}

func complianceReportGenerateCmdWithConfig(
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	provenanceSourceFactory func(string) evidence.ProvenanceSource,
	signingIdentityLoader complianceReportSigningIdentityLoader,
	nowFunc func() time.Time,
) *cobra.Command {
	var (
		projectRoot          string
		scopePath            string
		demoRuns             []string
		evalRuns             []string
		discoverEvalRuns     bool
		operationalSources   []string
		reportID             string
		bundleProfile        string
		signingMetadata      string
		signingPrivateKey    string
		evidenceTrustPath    string
		ksiRunID             string
		ksiHistory           string
		ksiResults           string
		commitmentRunID      string
		commitment           string
		commitmentAttemptID  string
		commitmentScenarioID string
		attestationRunID     string
		attestations         string
		auditRunID           string
		auditRecords         []string
		auditAttemptID       string
		auditScenarioID      string
		ledgerRunID          string
		ledgerCommits        string
		ledgerState          string
		ledgerAttemptID      string
		ledgerScenarioID     string
		buildRunID           string
		buildConfig          string
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate canonical analysis from persisted evidence",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if scopePath == "" {
				return fmt.Errorf("%w: --scope is required", constants.ErrValidationFailed)
			}
			if len(demoRuns) == 0 && len(evalRuns) == 0 && len(operationalSources) == 0 && ksiRunID == "" && ksiHistory == "" && ksiResults == "" && commitmentRunID == "" && commitment == "" && attestationRunID == "" && attestations == "" && auditRunID == "" && len(auditRecords) == 0 && ledgerRunID == "" && ledgerCommits == "" && ledgerState == "" && buildRunID == "" && buildConfig == "" {
				return fmt.Errorf("%w: at least one demo, eval, or standalone platform source is required", constants.ErrValidationFailed)
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
			scope, err := loadComplianceAssessmentScope(scopePath)
			if err != nil {
				return err
			}
			identity, err := signingIdentityLoader(ctx, signingMetadata, signingPrivateKey)
			if err != nil {
				return err
			}
			scopeID := scope.GetScopeId()
			assessmentAsOf := scope.GetAssessmentAsOf().AsTime()
			var evidenceTrust evidence.AssessedSignerSource
			if evidenceTrustPath != "" {
				evidenceTrust, err = loadAssessedEvidenceTrust(evidenceTrustPath, scopeID, assessmentAsOf)
				if err != nil {
					return err
				}
			}
			operationalGenerationSources, operationalSourceArtifacts, err := buildOperationalReportSources(ctx, operationalSources, scope, evidenceTrust, assessmentAsOf)
			if err != nil {
				return err
			}
			source := provenanceSourceFactory(projectRoot)
			importers, err := buildEvidenceGraphImporters(ctx, fileSvc, source, demoRuns, nil, func() time.Time { return assessmentAsOf })
			if err != nil {
				return err
			}
			evaluationSources, evalSourceArtifacts, err := buildEvaluationReportSources(ctx, fileSvc, scope, evalRuns, assessmentAsOf)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrReportVerificationFailed, err)
			}
			var evaluationDiagnostics []*compliancev1.AssessmentDiagnostic
			if discoverEvalRuns {
				diagnostics, artifact, diagnosticsErr := buildEvaluationSelectionDiagnostics(ctx, evaluation.NewStore(fileSvc), scope, evalRuns)
				if diagnosticsErr != nil {
					return diagnosticsErr
				}
				evaluationDiagnostics = diagnostics
				evalSourceArtifacts = append(evalSourceArtifacts, *artifact)
			}
			standaloneImporters, standaloneSourceArtifacts, err := buildStandaloneReportSources(ctx, standaloneReportSourceInput{
				scopeID:              scopeID,
				verifiedAt:           assessmentAsOf,
				evidenceTrust:        evidenceTrust,
				ksiRunID:             ksiRunID,
				ksiHistory:           ksiHistory,
				ksiResults:           ksiResults,
				commitmentRunID:      commitmentRunID,
				commitment:           commitment,
				commitmentAttemptID:  commitmentAttemptID,
				commitmentScenarioID: commitmentScenarioID,
				attestationRunID:     attestationRunID,
				attestations:         attestations,
				auditRunID:           auditRunID,
				auditRecords:         auditRecords,
				auditAttemptID:       auditAttemptID,
				auditScenarioID:      auditScenarioID,
				ledgerRunID:          ledgerRunID,
				ledgerCommits:        ledgerCommits,
				ledgerState:          ledgerState,
				ledgerAttemptID:      ledgerAttemptID,
				ledgerScenarioID:     ledgerScenarioID,
				buildRunID:           buildRunID,
				buildConfig:          buildConfig,
			})
			if err != nil {
				return err
			}
			importers = append(importers, standaloneImporters...)
			assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
			if err != nil {
				return fmt.Errorf("compliance report: load canonical catalogs: %w", err)
			}
			sourceArtifacts, err := buildDemoVerificationArtifacts(ctx, fileSvc, source, demoRuns, assessmentAsOf)
			if err != nil {
				return err
			}
			sourceArtifacts = append(sourceArtifacts, evalSourceArtifacts...)
			sourceArtifacts = append(sourceArtifacts, operationalSourceArtifacts...)
			sourceArtifacts = append(sourceArtifacts, standaloneSourceArtifacts...)
			sort.Slice(sourceArtifacts, func(i, j int) bool { return sourceArtifacts[i].BundlePath < sourceArtifacts[j].BundlePath })
			if len(importers)+len(evaluationSources)+len(operationalGenerationSources) != len(scope.GetSourceAdmissions()) {
				return fmt.Errorf("%w: selected source count does not match protected source admissions", constants.ErrValidationFailed)
			}
			explicitSourcesByAdmission := make(map[string]compliancereport.GenerationSource, len(operationalGenerationSources)+len(evaluationSources))
			for _, source := range append(operationalGenerationSources, evaluationSources...) {
				if _, exists := explicitSourcesByAdmission[source.AdmissionID]; exists {
					return fmt.Errorf("%w: duplicate selected source admission %s", constants.ErrValidationFailed, source.AdmissionID)
				}
				explicitSourcesByAdmission[source.AdmissionID] = source
			}
			generationSources := make([]compliancereport.GenerationSource, 0, len(scope.GetSourceAdmissions()))
			importerIndex := 0
			for _, admission := range scope.GetSourceAdmissions() {
				if source, exists := explicitSourcesByAdmission[admission.GetAdmissionId()]; exists {
					generationSources = append(generationSources, source)
					continue
				}
				if importerIndex >= len(importers) {
					return fmt.Errorf("%w: source admission %s has no selected importer", constants.ErrValidationFailed, admission.GetAdmissionId())
				}
				generationSources = append(generationSources, compliancereport.GenerationSource{AdmissionID: admission.GetAdmissionId(), Importer: importers[importerIndex]})
				importerIndex++
			}
			result, err := compliancereport.GenerateSignedComplianceBundle(ctx, compliancereport.SignedBundleGenerationRequest{
				Generation: compliancereport.GenerationRequest{
					Scope:       scope,
					Sources:     generationSources,
					Assertions:  assertions,
					Frameworks:  frameworks,
					Crosswalks:  crosswalks,
					Diagnostics: evaluationDiagnostics,
				},
				Profile:         profile,
				ReportID:        reportID,
				GeneratedAt:     nowFunc().UTC(),
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

	cmd.Flags().StringVar(&scopePath, "scope", "", "Path to canonical protected assessment scope")
	cmd.Flags().StringSliceVar(&demoRuns, "demo-run", nil, "Demo evidence run ID (repeatable)")
	cmd.Flags().StringSliceVar(&evalRuns, "eval-run", nil, "Eval bundle run ID (repeatable)")
	cmd.Flags().BoolVar(&discoverEvalRuns, "discover-eval-runs", false, "Freeze all local eval run candidate dispositions into report diagnostics")
	cmd.Flags().StringSliceVar(&operationalSources, "source", nil, "Operational evidence source package directory (repeatable)")
	cmd.Flags().StringVar(&reportID, "report-id", "", "Immutable report bundle ID")
	cmd.Flags().StringVar(&bundleProfile, "profile", string(compliancereport.ProfilePublic), "Bundle profile: public or restricted")
	cmd.Flags().StringVar(&signingMetadata, "signing-metadata", "", "Path to canonical compliance report signing-key metadata")
	cmd.Flags().StringVar(&signingPrivateKey, "signing-private-key", "", "Path to hex-encoded Ed25519 compliance report private key")
	cmd.Flags().StringVar(&evidenceTrustPath, "evidence-trust", "", "Path to externally assessed source evidence signer trust policy")
	cmd.Flags().StringVar(&ksiRunID, "ksi-run-id", "", "Run ID for protected KSI history evidence")
	cmd.Flags().StringVar(&ksiHistory, "ksi-history", "", "Path to canonical KSI history JSONL")
	cmd.Flags().StringVar(&ksiResults, "ksi-results", "", "Path to canonical current KSI result JSON")
	cmd.Flags().StringVar(&commitmentRunID, "commitment-run-id", "", "Run ID for the protected commitment source")
	cmd.Flags().StringVar(&commitment, "commitment", "", "Path to a canonical signed commitment attestation")
	cmd.Flags().StringVar(&commitmentAttemptID, "commitment-attempt-id", "", "Optional attempt ID for commitment evidence")
	cmd.Flags().StringVar(&commitmentScenarioID, "commitment-scenario-id", "", "Optional scenario ID for commitment evidence")
	cmd.Flags().StringVar(&attestationRunID, "attestation-run-id", "", "Run ID for protected customer and assessor attestations")
	cmd.Flags().StringVar(&attestations, "attestations", "", "Path to canonical customer and assessor attestation JSONL")
	cmd.Flags().StringVar(&auditRunID, "audit-run-id", "", "Run ID for protected audit records")
	cmd.Flags().StringSliceVar(&auditRecords, "audit-record", nil, "Path to a canonical audit record (repeatable)")
	cmd.Flags().StringVar(&auditAttemptID, "audit-attempt-id", "", "Optional attempt ID for audit evidence")
	cmd.Flags().StringVar(&auditScenarioID, "audit-scenario-id", "", "Optional scenario ID for audit evidence")
	cmd.Flags().StringVar(&ledgerRunID, "ledger-run-id", "", "Run ID for the protected ledger source")
	cmd.Flags().StringVar(&ledgerCommits, "ledger-commits", "", "Path to canonical ledger commit JSONL")
	cmd.Flags().StringVar(&ledgerState, "ledger-state", "", "Path to canonical ledger state JSON")
	cmd.Flags().StringVar(&ledgerAttemptID, "ledger-attempt-id", "", "Optional attempt ID for ledger evidence")
	cmd.Flags().StringVar(&ledgerScenarioID, "ledger-scenario-id", "", "Optional scenario ID for ledger evidence")
	cmd.Flags().StringVar(&buildRunID, "build-run-id", "", "Run ID for protected build and configuration evidence")
	cmd.Flags().StringVar(&buildConfig, "build-config-attestations", "", "Path to canonical build and configuration attestation JSONL")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root directory (defaults to cwd)")
	return cmd
}
