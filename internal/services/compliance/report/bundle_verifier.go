// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type BundleArtifactReader interface {
	ReadFile(context.Context, string) ([]byte, error)
}

type BundleVerificationRequest struct {
	Bundle      *compliancev1.ComplianceReportBundle
	Reader      BundleArtifactReader
	TrustPolicy *compliancev1.ComplianceReportTrustPolicy
	VerifiedAt  time.Time
}

func VerifyComplianceReportBundle(ctx context.Context, request BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Bundle == nil || request.Reader == nil || request.TrustPolicy == nil || request.VerifiedAt.IsZero() {
		return nil, fmt.Errorf("%w: bundle, artifact reader, assessed trust policy, and verification time are required", constants.ErrReportVerificationFailed)
	}
	manifest := request.Bundle.GetManifest()
	if manifest == nil || manifest.GetReportId() == "" {
		return nil, fmt.Errorf("%w: bundle manifest and report ID are required", constants.ErrReportVerificationFailed)
	}
	report := &compliancev1.ComplianceVerificationReport{
		ReportId:        manifest.GetReportId(),
		VerifiedAt:      timestamppb.New(request.VerifiedAt.UTC()),
		VerifierId:      constants.ComplianceBundleVerifierID,
		VerifierVersion: constants.ComplianceBundleVerifierVersion,
	}
	verifier := bundleVerifier{request: request, report: report, bodies: make(map[string][]byte, len(request.Bundle.GetArtifacts()))}
	verifier.verify(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report.Valid = len(report.GetFailures()) == 0
	if err := catalog.ValidateVerificationReport(report); err != nil {
		return nil, fmt.Errorf("%w: validate verification report: %w", constants.ErrReportVerificationFailed, err)
	}
	return report, nil
}

type bundleVerifier struct {
	request BundleVerificationRequest
	report  *compliancev1.ComplianceVerificationReport
	bodies  map[string][]byte
}

func (v *bundleVerifier) verify(ctx context.Context) {
	bundle := v.request.Bundle
	frameworks := frameworkCatalogForManifest(bundle.GetManifest())
	if err := catalog.ValidateComplianceReportBundle(bundle, frameworks); err != nil {
		v.fail(err, constants.ComplianceBundleManifestPath, "bundle structure is invalid")
	}
	v.verifyManifestDigest()
	v.verifyBindings()
	v.verifyArtifactBodies(ctx)
	v.verifyChecksumRoot()
	v.verifySignatures()
	v.verifyTypedArtifacts()
	v.verifyRenderedFormats()
}

func (v *bundleVerifier) verifyManifestDigest() {
	manifest := v.request.Bundle.GetManifest()
	body, err := canonicalManifestBytes(manifest)
	if err != nil {
		v.fail(constants.ErrEvidenceArtifactMalformed, constants.ComplianceBundleManifestPath, err.Error())
		return
	}
	digest := sha256.Sum256(body)
	if manifest.GetManifestSha256() != hex.EncodeToString(digest[:]) {
		v.fail(constants.ErrChecksumMismatch, constants.ComplianceBundleManifestPath, "manifest SHA-256 does not match canonical manifest content")
	}
}

func (v *bundleVerifier) verifyBindings() {
	bundle := v.request.Bundle
	manifest := bundle.GetManifest()
	if manifest.GetReportSchemaVersion() != constants.ComplianceBundleSchemaVersion {
		v.fail(constants.ErrEvidenceSchemaMismatch, constants.ComplianceBundleManifestPath, "manifest schema version is unsupported")
	}
	if manifest.GetGeneratorIdentity() != constants.ComplianceBundleAssemblerID || manifest.GetGeneratorVersion() != constants.ComplianceBundleAssemblerVersion {
		v.fail(constants.ErrEvidenceProducerUnverified, constants.ComplianceBundleManifestPath, "bundle assembler identity is unsupported")
	}
	if bundle.GetAnalysis().GetScopeRef() != manifest.GetScopeRef() {
		v.fail(constants.ErrEvidenceScopeMismatch, constants.ComplianceBundleAnalysisPath, "analysis scope does not match manifest scope")
	}
	for _, artifact := range bundle.GetArtifacts() {
		if artifact != nil && manifest.GetBundleProfile() == constants.ComplianceBundleProfilePublic && artifact.GetProfile() != constants.ComplianceBundleProfilePublic {
			v.fail(constants.ErrBundleProfileUnsupported, artifact.GetBundlePath(), "public bundle contains a restricted artifact")
		}
	}
	for _, profile := range bundle.GetProfiles() {
		if profile != nil && profile.GetAnalysisRef() != bundle.GetAnalysis().GetAnalysisId() {
			bundlePath := fmt.Sprintf("%s/%s.json", constants.ComplianceBundleProfilesDirname, profile.GetProfileId())
			v.fail(constants.ErrUnresolvedReference, bundlePath, "framework profile does not reference the bundled analysis")
		}
	}
}

func (v *bundleVerifier) verifyArtifactBodies(ctx context.Context) {
	for _, artifact := range v.request.Bundle.GetArtifacts() {
		if err := ctx.Err(); err != nil {
			v.fail(err, artifact.GetBundlePath(), "verification cancelled")
			return
		}
		if artifact == nil || artifact.GetBundlePath() == "" {
			continue
		}
		body, err := v.request.Reader.ReadFile(ctx, artifact.GetBundlePath())
		if err != nil {
			v.fail(constants.ErrBundleArtifactMissing, artifact.GetBundlePath(), err.Error())
			continue
		}
		if int64(len(body)) > constants.ComplianceBundleMaxArtifactBytes {
			v.fail(constants.ErrEvidenceArtifactTooLarge, artifact.GetBundlePath(), "artifact exceeds verification size limit")
			continue
		}
		v.bodies[artifact.GetBundlePath()] = body
		digest := sha256.Sum256(body)
		if artifact.GetSha256() != hex.EncodeToString(digest[:]) {
			v.fail(constants.ErrChecksumMismatch, artifact.GetBundlePath(), "artifact SHA-256 does not match protected bytes")
		}
		if artifact.GetByteLength() != int64(len(body)) {
			v.fail(constants.ErrChecksumMismatch, artifact.GetBundlePath(), "artifact byte length does not match protected bytes")
		}
	}
}

func (v *bundleVerifier) verifyChecksumRoot() {
	entries := make([]*compliancev1.ChecksumEntry, 0, len(v.request.Bundle.GetArtifacts()))
	for _, artifact := range v.request.Bundle.GetArtifacts() {
		if artifact == nil {
			continue
		}
		entries = append(entries, &compliancev1.ChecksumEntry{BundlePath: artifact.GetBundlePath(), Sha256: artifact.GetSha256()})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].GetBundlePath() < entries[j].GetBundlePath() })
	root, err := computeChecksumRoot(entries)
	if err != nil {
		v.fail(constants.ErrBundleChecksumRootFailed, constants.ComplianceBundleChecksumsPath, err.Error())
		v.report.ReproducedChecksumRoot = zeroSHA256()
		return
	}
	v.report.ReproducedChecksumRoot = root
	bundle := v.request.Bundle
	if root != bundle.GetChecksumRoot() {
		v.fail(constants.ErrChecksumMismatch, constants.ComplianceBundleChecksumsPath, "reproduced checksum root does not match bundle checksum root")
	}
	if bundle.GetManifest().GetChecksumRoot() != bundle.GetChecksumRoot() {
		v.fail(constants.ErrChecksumMismatch, constants.ComplianceBundleManifestPath, "manifest checksum root does not match bundle checksum root")
	}
}

func (v *bundleVerifier) verifySignatures() {
	manifest := v.request.Bundle.GetManifest()
	if manifest.GetGeneratedAt() == nil || manifest.GetGeneratedAt().CheckValid() != nil {
		v.fail(constants.ErrReportSignatureFailed, constants.ComplianceBundleManifestPath, "manifest signing time is invalid")
		return
	}
	signedAt := manifest.GetGeneratedAt().AsTime()
	if err := VerifyComplianceReportSignature(manifest.GetSignature(), v.request.TrustPolicy, manifest.GetScopeRef(), signedAt); err != nil {
		v.fail(classifySignatureError(err), constants.ComplianceBundleManifestPath, err.Error())
	}
	if err := VerifyComplianceReportSignature(v.request.Bundle.GetChecksumRootSignature(), v.request.TrustPolicy, manifest.GetScopeRef(), signedAt); err != nil {
		v.fail(classifySignatureError(err), constants.ComplianceBundleChecksumsPath, err.Error())
	}
}

func (v *bundleVerifier) verifyTypedArtifacts() {
	analysisBody, ok := v.bodies[constants.ComplianceBundleAnalysisPath]
	if !ok {
		v.fail(constants.ErrBundleArtifactMissing, constants.ComplianceBundleAnalysisPath, "canonical analysis artifact is missing")
	} else {
		expected, err := compliancev1.MarshalCanonical(v.request.Bundle.GetAnalysis())
		if err != nil || !bytes.Equal(analysisBody, expected) {
			v.fail(constants.ErrRendererMismatch, constants.ComplianceBundleAnalysisPath, "analysis artifact does not match the typed canonical analysis")
		}
	}
	for _, profile := range v.request.Bundle.GetProfiles() {
		if profile == nil {
			continue
		}
		bundlePath := fmt.Sprintf("%s/%s.json", constants.ComplianceBundleProfilesDirname, profile.GetProfileId())
		body, ok := v.bodies[bundlePath]
		if !ok {
			v.fail(constants.ErrBundleArtifactMissing, bundlePath, "framework profile artifact is missing")
			continue
		}
		expected, err := compliancev1.MarshalCanonical(profile)
		if err != nil || !bytes.Equal(body, expected) {
			v.fail(constants.ErrRendererMismatch, bundlePath, "framework profile artifact does not match the typed canonical profile")
		}
	}
}

func (v *bundleVerifier) verifyRenderedFormats() {
	for _, entry := range v.request.Bundle.GetRenderedFormats() {
		if entry == nil {
			continue
		}
		format, err := ParseFormat(entry.GetFormat())
		if err != nil {
			v.fail(constants.ErrRendererMismatch, entry.GetBundlePath(), err.Error())
			continue
		}
		rendered, err := RenderComplianceAnalysis(v.request.Bundle.GetAnalysis(), format)
		if err != nil {
			v.fail(constants.ErrRendererMismatch, entry.GetBundlePath(), err.Error())
			continue
		}
		body, ok := v.bodies[entry.GetBundlePath()]
		if !ok {
			continue
		}
		if entry.GetMediaType() != rendered.MediaType || !bytes.Equal(body, rendered.Body) {
			v.fail(constants.ErrRendererMismatch, entry.GetBundlePath(), "rendered artifact does not reproduce from the canonical analysis")
		}
	}
}

func (v *bundleVerifier) fail(code error, subject, reason string) {
	v.report.Failures = append(v.report.Failures, &compliancev1.VerificationFailure{Code: code.Error(), SubjectRef: subject, Reason: reason})
}

func frameworkCatalogForManifest(manifest *compliancev1.ComplianceReportManifest) *compliancev1.FrameworkCatalog {
	catalogValue := &compliancev1.FrameworkCatalog{}
	if manifest == nil {
		return catalogValue
	}
	for _, reference := range manifest.GetFrameworkRefs() {
		if reference == nil {
			continue
		}
		catalogValue.Frameworks = append(catalogValue.Frameworks, &compliancev1.FrameworkDefinition{FrameworkId: reference.GetId(), FrameworkVersion: reference.GetVersion()})
	}
	return catalogValue
}

func classifySignatureError(err error) error {
	if errors.Is(err, constants.ErrEvidenceTrustNotAssessed) {
		return constants.ErrEvidenceTrustNotAssessed
	}
	return constants.ErrReportSignatureFailed
}

func zeroSHA256() string {
	return hex.EncodeToString(make([]byte, sha256.Size))
}
