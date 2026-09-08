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
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type BundleArtifactReader interface {
	ReadFile(context.Context, string) ([]byte, error)
	ListFiles(context.Context) ([]string, error)
}

type AuthorizedPlaintextReader interface {
	ReadPlaintext(context.Context, *compliancev1.BundleArtifact, []byte) ([]byte, error)
}

type BundleVerificationRequest struct {
	Bundle                    *compliancev1.ComplianceReportBundle
	Reader                    BundleArtifactReader
	TrustPolicy               *compliancev1.ComplianceReportTrustPolicy
	AuthorizedPlaintextReader AuthorizedPlaintextReader
	VerifiedAt                time.Time
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
	sort.Slice(report.Failures, func(i, j int) bool {
		left := report.Failures[i].GetSubjectRef() + report.Failures[i].GetCode() + report.Failures[i].GetReason()
		right := report.Failures[j].GetSubjectRef() + report.Failures[j].GetCode() + report.Failures[j].GetReason()
		return left < right
	})
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
	v.check(constants.ComplianceBundleCheckCatalog, []string{constants.ComplianceBundleManifestPath}, func() {
		frameworks := frameworkCatalogForManifest(bundle.GetManifest())
		if err := catalog.ValidateComplianceReportBundle(bundle, frameworks); err != nil {
			v.fail(err, constants.ComplianceBundleManifestPath, err.Error())
		}
	})
	v.check(constants.ComplianceBundleCheckManifestDigest, []string{constants.ComplianceBundleManifestPath}, v.verifyManifestDigest)
	v.check(constants.ComplianceBundleCheckBindings, []string{constants.ComplianceBundleManifestPath, constants.ComplianceBundleAnalysisPath}, v.verifyBindings)
	v.check(constants.ComplianceBundleCheckDirectoryInventory, []string{constants.ComplianceBundleManifestPath}, func() { v.verifyDirectoryInventory(ctx) })
	v.check(constants.ComplianceBundleCheckArtifactBodies, []string{constants.ComplianceBundleManifestPath}, func() { v.verifyArtifactBodies(ctx) })
	v.check(constants.ComplianceBundleCheckChecksumRoot, []string{constants.ComplianceBundleChecksumsPath}, v.verifyChecksumRoot)
	v.check(constants.ComplianceBundleCheckSignatures, []string{constants.ComplianceBundleManifestPath, constants.ComplianceBundleChecksumsPath}, v.verifySignatures)
	v.check(constants.ComplianceBundleCheckTypedArtifacts, []string{constants.ComplianceBundleAnalysisPath}, v.verifyTypedArtifacts)
	v.check(constants.ComplianceBundleCheckSourceVerification, []string{constants.ComplianceBundleSourcesDirname}, func() { v.verifySourceVerificationReports(ctx) })
	v.check(constants.ComplianceBundleCheckRenderedFormats, []string{constants.ComplianceBundleAnalysisPath}, v.verifyRenderedFormats)
}

func (v *bundleVerifier) check(checkID string, evidenceRefs []string, verify func()) {
	failureStart := len(v.report.GetFailures())
	verify()
	failures := v.report.GetFailures()[failureStart:]
	v.report.Checks = append(v.report.Checks, evidence.NewVerificationCheckResult(checkID, constants.ComplianceBundleVerifierID, constants.ComplianceBundleVerifierVersion, evidenceRefs, failures))
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
	manifestFrameworks := make(map[string]struct{}, len(manifest.GetFrameworkRefs()))
	for _, reference := range manifest.GetFrameworkRefs() {
		manifestFrameworks[versionedReferenceKey(reference)] = struct{}{}
	}
	profileFrameworks := make(map[string]struct{}, len(bundle.GetProfiles()))
	for _, profile := range bundle.GetProfiles() {
		if profile == nil {
			continue
		}
		bundlePath := fmt.Sprintf("%s/%s.json", constants.ComplianceBundleProfilesDirname, profile.GetProfileId())
		if profile.GetAnalysisRef() != bundle.GetAnalysis().GetAnalysisId() {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "framework profile does not reference the bundled analysis")
		}
		frameworkKey := versionedReferenceKey(profile.GetFrameworkRef())
		if _, exists := manifestFrameworks[frameworkKey]; !exists {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "framework profile does not reference a manifest framework")
			continue
		}
		profileFrameworks[frameworkKey] = struct{}{}
	}
	for frameworkKey := range manifestFrameworks {
		if _, exists := profileFrameworks[frameworkKey]; !exists {
			v.fail(constants.ErrUnresolvedReference, constants.ComplianceBundleManifestPath, "manifest framework lacks a bundled framework profile")
		}
	}
}

func (v *bundleVerifier) verifyDirectoryInventory(ctx context.Context) {
	paths, err := v.request.Reader.ListFiles(ctx)
	if err != nil {
		v.fail(classifyDirectoryReadError(err), constants.ComplianceBundleManifestPath, err.Error())
		return
	}
	if len(paths) > constants.ComplianceBundleMaxArtifacts {
		v.fail(constants.ErrEvidenceArtifactTooLarge, constants.ComplianceBundleManifestPath, "bundle directory exceeds the artifact-count limit")
		return
	}
	expected := make(map[string]struct{}, len(v.request.Bundle.GetArtifacts()))
	for _, artifact := range v.request.Bundle.GetArtifacts() {
		if artifact != nil && artifact.GetBundlePath() != "" {
			expected[artifact.GetBundlePath()] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(paths))
	for _, bundlePath := range paths {
		if _, duplicate := seen[bundlePath]; duplicate {
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "bundle directory enumerated a duplicate artifact")
			continue
		}
		seen[bundlePath] = struct{}{}
		if _, exists := expected[bundlePath]; !exists {
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "bundle directory contains an undeclared artifact")
		}
	}
	for bundlePath := range expected {
		if _, exists := seen[bundlePath]; !exists {
			v.fail(constants.ErrBundleArtifactMissing, bundlePath, "declared artifact is absent from the bundle directory")
		}
	}
}

func (v *bundleVerifier) verifyArtifactBodies(ctx context.Context) {
	for _, artifact := range v.request.Bundle.GetArtifacts() {
		if artifact == nil || artifact.GetBundlePath() == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			v.fail(err, artifact.GetBundlePath(), "verification cancelled")
			return
		}
		body, err := v.request.Reader.ReadFile(ctx, artifact.GetBundlePath())
		if err != nil {
			v.fail(classifyArtifactReadError(err), artifact.GetBundlePath(), err.Error())
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
		if artifact.GetProfile() == constants.ComplianceBundleProfileRestricted && v.request.AuthorizedPlaintextReader != nil {
			plaintext, err := v.request.AuthorizedPlaintextReader.ReadPlaintext(ctx, artifact, append([]byte(nil), body...))
			if err != nil {
				v.fail(constants.ErrEvidenceEncryptionInvalid, artifact.GetBundlePath(), err.Error())
				continue
			}
			if int64(len(plaintext)) > constants.ComplianceBundleMaxArtifactBytes {
				v.fail(constants.ErrEvidenceArtifactTooLarge, artifact.GetBundlePath(), "authorized plaintext exceeds verification size limit")
				continue
			}
			plaintextDigest := sha256.Sum256(plaintext)
			if artifact.GetEncryption().GetPlaintextSha256() != hex.EncodeToString(plaintextDigest[:]) {
				v.fail(constants.ErrEvidenceEncryptionInvalid, artifact.GetBundlePath(), "authorized plaintext SHA-256 does not match authenticated encryption metadata")
			}
		}
	}
}

func (v *bundleVerifier) verifyChecksumRoot() {
	entries, err := artifactDescriptorChecksumEntries(v.request.Bundle.GetArtifacts())
	if err != nil {
		v.fail(constants.ErrBundleChecksumRootFailed, constants.ComplianceBundleChecksumsPath, err.Error())
		v.report.ReproducedChecksumRoot = zeroSHA256()
		return
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

func (v *bundleVerifier) verifyCanonicalReportSources() {
	manifest := v.request.Bundle.GetManifest()
	assertionPath := manifest.GetAssertionCatalogRef()
	assertions := &compliancev1.ControlAssertionCatalog{}
	if !v.decodeCanonicalSource(assertionPath, assertions) {
		return
	}
	if err := catalog.ValidateAssertionCatalog(assertions); err != nil {
		v.fail(err, assertionPath, err.Error())
		return
	}
	frameworkPath := path.Join(constants.ComplianceBundleFrameworkCatalogsDirname, constants.ComplianceBundleFrameworkCatalogFilename)
	frameworks := &compliancev1.FrameworkCatalog{}
	if !v.decodeCanonicalSource(frameworkPath, frameworks) {
		return
	}
	if err := catalog.ValidateFrameworkCatalog(frameworks); err != nil {
		v.fail(err, frameworkPath, err.Error())
		return
	}
	for _, reference := range manifest.GetFrameworkRefs() {
		if reference == nil || catalog.FindFramework(frameworks, reference.GetId(), reference.GetVersion()) == nil {
			v.fail(constants.ErrUnsupportedFramework, frameworkPath, "manifest framework is absent from the protected framework catalog")
		}
	}
	for _, crosswalkPath := range manifest.GetCrosswalkRefs() {
		crosswalks := &compliancev1.ControlCrosswalkCatalog{}
		if !v.decodeCanonicalSource(crosswalkPath, crosswalks) {
			continue
		}
		if err := catalog.ValidateCatalogSet(assertions, frameworks, crosswalks); err != nil {
			v.fail(err, crosswalkPath, err.Error())
		}
	}
	assertionAssessments, err := marshalCanonicalMessages(v.request.Bundle.GetAnalysis().GetAssertionAssessments())
	v.verifyCanonicalSourceProjection(path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleAssertionAssessmentsFilename), assertionAssessments, err)
	controlAssessments, err := marshalCanonicalMessages(v.request.Bundle.GetAnalysis().GetFrameworkAssessments())
	v.verifyCanonicalSourceProjection(path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleControlAssessmentsFilename), controlAssessments, err)
	evidenceIndex, err := marshalCanonicalMessages(v.request.Bundle.GetAnalysis().GetEvidenceResources())
	v.verifyCanonicalSourceProjection(manifest.GetEvidenceIndexRef(), evidenceIndex, err)
}

func (v *bundleVerifier) decodeCanonicalSource(bundlePath string, message proto.Message) bool {
	body, exists := v.bodies[bundlePath]
	if !exists {
		v.fail(constants.ErrBundleArtifactMissing, bundlePath, "canonical report source is missing")
		return false
	}
	if err := compliancev1.UnmarshalCanonical(body, message); err != nil {
		v.fail(constants.ErrEvidenceArtifactMalformed, bundlePath, err.Error())
		return false
	}
	return true
}

func (v *bundleVerifier) verifyCanonicalSourceProjection(bundlePath string, expected []byte, err error) {
	if err != nil {
		v.fail(constants.ErrEvidenceArtifactMalformed, bundlePath, err.Error())
		return
	}
	body, exists := v.bodies[bundlePath]
	if !exists {
		v.fail(constants.ErrBundleArtifactMissing, bundlePath, "canonical report source is missing")
		return
	}
	if !bytes.Equal(body, expected) {
		v.fail(constants.ErrRendererMismatch, bundlePath, "canonical report source does not reproduce from the typed analysis")
	}
}

func (v *bundleVerifier) verifyTypedArtifacts() {
	v.verifyCanonicalReportSources()
	analysisBody, ok := v.bodies[constants.ComplianceBundleAnalysisPath]
	if !ok {
		v.fail(constants.ErrBundleArtifactMissing, constants.ComplianceBundleAnalysisPath, "canonical analysis artifact is missing")
	} else {
		matches, err := evidence.CanonicalProtoBodyEqual(analysisBody, v.request.Bundle.GetAnalysis())
		if err != nil || !matches {
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
		matches, err := evidence.CanonicalProtoBodyEqual(body, profile)
		if err != nil || !matches {
			v.fail(constants.ErrRendererMismatch, bundlePath, "framework profile artifact does not match the typed canonical profile")
		}
	}
}

type demoSourceInventory struct {
	verificationReport *compliancev1.ComplianceVerificationReport
	runtimeManifest    bool
	runtimeResults     bool
	provenanceArtifact bool
	definitions        bool
}

func (v *bundleVerifier) verifySourceVerificationReports(ctx context.Context) {
	v.verifyDemoSourceVerificationReports(ctx)
	v.verifyEvalSourceVerificationReports(ctx)
}

func (v *bundleVerifier) verifyDemoSourceVerificationReports(ctx context.Context) {
	expectedRuns := make(map[string]struct{})
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource != nil && resource.GetArtifactType() == string(evidence.ArtifactTypeDemoManifest) && resource.GetRunId() != "" {
			expectedRuns[resource.GetRunId()] = struct{}{}
		}
	}
	inventories := make(map[string]*demoSourceInventory, len(expectedRuns))
	for runID := range expectedRuns {
		inventories[runID] = &demoSourceInventory{}
	}
	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) < 4 || parts[0] != constants.ComplianceBundleSourcesDirname || parts[1] != constants.ComplianceBundleSourceDemosDirname || parts[2] == "" {
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "demo source path is unsupported")
			continue
		}
		runID := parts[2]
		inventory, expected := inventories[runID]
		if !expected {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "demo source artifact does not bind an analysis evidence run")
			continue
		}
		switch parts[3] {
		case constants.ComplianceBundleSourceVerificationFilename:
			if len(parts) != 4 {
				v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "demo source verification path is unsupported")
				continue
			}
			inventory.verificationReport = v.verifySourceVerificationReport(bundlePath, body, runID, constants.DemoRunVerifierID, constants.DemoRunVerifierVersion, constants.ErrDemoRunVerificationFailed, "demo")
		case constants.ComplianceBundleSourceRuntimeDirname:
			if len(parts) < 5 {
				v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "demo runtime source path is incomplete")
				continue
			}
			if len(parts) == 5 && parts[4] == constants.DemoRunManifestFilename {
				inventory.runtimeManifest = true
			}
			if len(parts) == 5 && parts[4] == constants.DemoRunResultsFilename {
				inventory.runtimeResults = true
			}
		case constants.ComplianceBundleSourceProvenanceDirname:
			if len(parts) == 5 && parts[4] == constants.DemoRunDefinitionsFilename {
				inventory.definitions = true
				continue
			}
			if len(parts) >= 6 && parts[4] == constants.ComplianceBundleSourceArtifactsDirname {
				inventory.provenanceArtifact = true
				continue
			}
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "demo provenance source path is unsupported")
		default:
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "demo source path is unsupported")
		}
	}
	for runID, inventory := range inventories {
		if inventory.verificationReport == nil {
			bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceVerificationFilename)
			v.fail(constants.ErrDemoRunVerificationFailed, bundlePath, "demo evidence run lacks a valid independent source verification report")
		}
		complete := inventory.runtimeManifest && inventory.runtimeResults && inventory.provenanceArtifact && inventory.definitions
		if !complete {
			bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID)
			v.fail(constants.ErrDemoRunVerificationFailed, bundlePath, "demo evidence run lacks a complete protected runtime and provenance source inventory")
		}
		if complete && inventory.verificationReport != nil {
			v.replayDemoSourceVerification(ctx, runID, inventory.verificationReport)
		}
	}
}

type evalSourceInventory struct {
	verificationReport *compliancev1.ComplianceVerificationReport
	manifest           bool
	tasks              bool
	attempts           bool
	receipts           bool
	stages             bool
	metrics            bool
	evidenceIndex      bool
}

func (v *bundleVerifier) verifyEvalSourceVerificationReports(ctx context.Context) {
	expectedRuns := make(map[string]struct{})
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource != nil && resource.GetArtifactType() == string(evidence.ArtifactTypeEvalManifest) && resource.GetRunId() != "" {
			expectedRuns[resource.GetRunId()] = struct{}{}
		}
	}
	inventories := make(map[string]*evalSourceInventory, len(expectedRuns))
	for runID := range expectedRuns {
		inventories[runID] = &evalSourceInventory{}
	}
	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) < 4 || parts[0] != constants.ComplianceBundleSourcesDirname || parts[1] != constants.ComplianceBundleSourceEvalsDirname || parts[2] == "" {
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "eval source path is unsupported")
			continue
		}
		runID := parts[2]
		inventory, expected := inventories[runID]
		if !expected {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "eval source artifact does not bind an analysis evidence run")
			continue
		}
		switch parts[3] {
		case constants.ComplianceBundleSourceVerificationFilename:
			if len(parts) != 4 {
				v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "eval source verification path is unsupported")
				continue
			}
			inventory.verificationReport = v.verifySourceVerificationReport(bundlePath, body, runID, constants.EvalRunVerifierID, constants.EvalRunVerifierVersion, constants.ErrEvalRunVerificationFailed, "eval")
		case constants.ComplianceBundleSourceRuntimeDirname:
			if len(parts) < 5 {
				v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "eval runtime source path is incomplete")
				continue
			}
			if len(parts) > 5 {
				if parts[4] != constants.EvalRunEvidenceDirname {
					v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "eval runtime nested source path is unsupported")
				}
				continue
			}
			switch parts[4] {
			case constants.EvalRunManifestFilename:
				inventory.manifest = true
			case constants.EvalRunTasksFilename:
				inventory.tasks = true
			case constants.EvalRunAttemptsFilename:
				inventory.attempts = true
			case constants.EvalRunReceiptsFilename:
				inventory.receipts = true
			case constants.EvalRunStagesFilename:
				inventory.stages = true
			case constants.EvalRunMetricsFilename:
				inventory.metrics = true
			case constants.EvalRunEvidenceIndexFilename:
				inventory.evidenceIndex = true
			default:
				v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "eval runtime root source path is unsupported")
			}
		default:
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "eval source path is unsupported")
		}
	}
	for runID, inventory := range inventories {
		if inventory.verificationReport == nil {
			bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, runID, constants.ComplianceBundleSourceVerificationFilename)
			v.fail(constants.ErrEvalRunVerificationFailed, bundlePath, "eval evidence run lacks a valid independent source verification report")
		}
		complete := inventory.manifest && inventory.tasks && inventory.attempts && inventory.receipts && inventory.stages && inventory.metrics && inventory.evidenceIndex
		if !complete {
			bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, runID)
			v.fail(constants.ErrEvalRunVerificationFailed, bundlePath, "eval evidence run lacks a complete protected runtime source inventory")
		}
		if complete && inventory.verificationReport != nil {
			v.replayEvalSourceVerification(ctx, runID, inventory.verificationReport)
		}
	}
}

func (v *bundleVerifier) verifySourceVerificationReport(bundlePath string, body []byte, runID, verifierID, verifierVersion string, verificationErr error, source string) *compliancev1.ComplianceVerificationReport {
	report, err := evidence.ValidateVerificationReport(body, runID, verifierID, verifierVersion, v.request.Bundle.GetManifest().GetGeneratedAt().AsTime())
	if err == nil {
		return report
	}
	if errors.Is(err, constants.ErrEvidenceArtifactMalformed) {
		v.fail(constants.ErrEvidenceArtifactMalformed, bundlePath, source+" source verification report is not canonical")
		return nil
	}
	v.fail(verificationErr, bundlePath, source+" source verification report is invalid or does not bind the declared run")
	return nil
}

func (v *bundleVerifier) replayEvalSourceVerification(ctx context.Context, runID string, expected *compliancev1.ComplianceVerificationReport) {
	runtimeRoot := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.EvalRunsDirname, runID)
	reader := &bundledRuntimeArtifactReader{bodies: v.bodies, runID: runID, sourceDir: constants.ComplianceBundleSourceEvalsDirname, runtimeRoot: runtimeRoot}
	replayed, err := evidence.VerifyEvalRun(ctx, reader, runID, runtimeRoot, expected.GetVerifiedAt().AsTime())
	bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, runID, constants.ComplianceBundleSourceVerificationFilename)
	if err != nil {
		v.fail(constants.ErrEvalRunVerificationFailed, bundlePath, err.Error())
		return
	}
	matches, matchErr := evidence.CanonicalProtosEqual(expected, replayed)
	if matchErr != nil || !replayed.GetValid() || !matches {
		v.fail(constants.ErrEvalRunVerificationFailed, bundlePath, "replayed eval verification does not match the protected source verification report")
	}
}

func (v *bundleVerifier) replayDemoSourceVerification(ctx context.Context, runID string, expected *compliancev1.ComplianceVerificationReport) {
	runtimeRoot := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, runID)
	reader := &bundledRuntimeArtifactReader{bodies: v.bodies, runID: runID, sourceDir: constants.ComplianceBundleSourceDemosDirname, runtimeRoot: runtimeRoot}
	replayed, err := evidence.VerifyDemoRun(ctx, reader, runID, &bundledDemoProvenanceSource{bodies: v.bodies, runID: runID}, expected.GetVerifiedAt().AsTime())
	bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceVerificationFilename)
	if err != nil {
		v.fail(constants.ErrDemoRunVerificationFailed, bundlePath, err.Error())
		return
	}
	matches, matchErr := evidence.CanonicalProtosEqual(expected, replayed)
	if matchErr != nil || !replayed.GetValid() || !matches {
		v.fail(constants.ErrDemoRunVerificationFailed, bundlePath, "replayed demo verification does not match the protected source verification report")
	}
}

type bundledRuntimeArtifactReader struct {
	bodies      map[string][]byte
	runID       string
	sourceDir   string
	runtimeRoot string
}

func (r *bundledRuntimeArtifactReader) ReadFile(ctx context.Context, sourcePath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bundlePath, ok := r.bundlePath(sourcePath)
	if !ok {
		return nil, constants.ErrUnexpectedEvidenceArtifact
	}
	body, exists := r.bodies[bundlePath]
	if !exists {
		return nil, constants.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func (r *bundledRuntimeArtifactReader) ReadDir(ctx context.Context, sourcePath string) ([]os.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bundlePath, ok := r.bundlePath(sourcePath)
	if !ok {
		return nil, constants.ErrUnexpectedEvidenceArtifact
	}
	prefix := bundlePath + "/"
	entries := make(map[string]bool)
	for candidate := range r.bodies {
		if !strings.HasPrefix(candidate, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(candidate, prefix)
		parts := strings.SplitN(remainder, "/", 2)
		if parts[0] == "" {
			continue
		}
		entries[parts[0]] = len(parts) == 2 || entries[parts[0]]
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]os.DirEntry, 0, len(names))
	for _, name := range names {
		result = append(result, bundledRuntimeDirEntry{name: name, directory: entries[name]})
	}
	return result, nil
}

func (r *bundledRuntimeArtifactReader) bundlePath(sourcePath string) (string, bool) {
	relative, err := filepath.Rel(r.runtimeRoot, sourcePath)
	if err != nil || relative == constants.PathParentDir || strings.HasPrefix(relative, constants.PathParentDir+string(filepath.Separator)) {
		return "", false
	}
	bundleRoot := path.Join(constants.ComplianceBundleSourcesDirname, r.sourceDir, r.runID, constants.ComplianceBundleSourceRuntimeDirname)
	if relative == "." {
		return bundleRoot, true
	}
	return path.Join(bundleRoot, filepath.ToSlash(relative)), true
}

type bundledRuntimeDirEntry struct {
	name      string
	directory bool
}

func (e bundledRuntimeDirEntry) Name() string { return e.name }
func (e bundledRuntimeDirEntry) IsDir() bool  { return e.directory }
func (e bundledRuntimeDirEntry) Type() os.FileMode {
	if e.directory {
		return os.ModeDir
	}
	return 0
}
func (e bundledRuntimeDirEntry) Info() (os.FileInfo, error) { return nil, nil }

type bundledDemoProvenanceSource struct {
	bodies map[string][]byte
	runID  string
}

func (s *bundledDemoProvenanceSource) Artifacts(ctx context.Context, _ string) ([]evidence.ProvenanceArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, s.runID, constants.ComplianceBundleSourceProvenanceDirname, constants.ComplianceBundleSourceArtifactsDirname) + "/"
	artifacts := make([]evidence.ProvenanceArtifact, 0)
	for bundlePath, body := range s.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		name := strings.TrimPrefix(bundlePath, prefix)
		if name == "" {
			return nil, constants.ErrUnexpectedEvidenceArtifact
		}
		artifacts = append(artifacts, evidence.ProvenanceArtifact{Name: filepath.FromSlash(name), Body: append([]byte(nil), body...)})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Name < artifacts[j].Name })
	return artifacts, nil
}

func (s *bundledDemoProvenanceSource) Definitions(ctx context.Context, _ string) ([]evidence.DemoDefinitionArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, s.runID, constants.ComplianceBundleSourceProvenanceDirname, constants.DemoRunDefinitionsFilename)
	body, exists := s.bodies[bundlePath]
	if !exists {
		return nil, constants.ErrNotFound
	}
	lines := bytes.Split(body, []byte{'\n'})
	definitions := make([]evidence.DemoDefinitionArtifact, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			return nil, constants.ErrEvidenceArtifactMalformed
		}
		definitions = append(definitions, evidence.DemoDefinitionArtifact{Body: append([]byte(nil), line...)})
	}
	return definitions, nil
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
	stableCode := stableVerificationFailureCode(code)
	v.report.Failures = append(v.report.Failures, &compliancev1.VerificationFailure{Code: stableCode.Error(), SubjectRef: subject, Reason: reason})
}

func stableVerificationFailureCode(err error) error {
	knownCodes := [...]error{
		constants.ErrUnsupportedFramework,
		constants.ErrUnsupportedAssertion,
		constants.ErrUnsupportedVerifier,
		constants.ErrUnsupportedGrader,
		constants.ErrFrameworkProfileInvalid,
		constants.ErrStaleEvidence,
		constants.ErrEvidenceScopeMismatch,
		constants.ErrUnresolvedReference,
		constants.ErrRendererMismatch,
		constants.ErrChecksumMismatch,
		constants.ErrReportSignatureFailed,
		constants.ErrEvidenceArtifactMalformed,
		constants.ErrUnexpectedEvidenceArtifact,
		constants.ErrEvidenceArtifactTooLarge,
		constants.ErrEvidenceDirectoryLimitExceeded,
		constants.ErrDemoRunVerificationFailed,
		constants.ErrEvalRunVerificationFailed,
		constants.ErrEvidenceDuplicateID,
		constants.ErrEvidenceDuplicateContent,
		constants.ErrEvidenceCycleDetected,
		constants.ErrEvidenceSchemaMismatch,
		constants.ErrEvidenceMediaTypeUnsupported,
		constants.ErrEvidenceTrustNotAssessed,
		constants.ErrEvidenceEncryptionInvalid,
		constants.ErrEvidenceImporterFailed,
		constants.ErrEvidenceProducerUnverified,
		constants.ErrEvidenceVerifierUnverified,
		constants.ErrBundleAssemblyFailed,
		constants.ErrBundlePersistenceFailed,
		constants.ErrBundleProfileUnsupported,
		constants.ErrBundleArtifactMissing,
		constants.ErrBundleChecksumRootFailed,
		constants.ErrDirectoryRead,
		constants.ErrFileReadFailed,
		constants.ErrInvalidEvidenceGraph,
	}
	for _, code := range knownCodes {
		if errors.Is(err, code) {
			return code
		}
	}
	return constants.ErrReportVerificationFailed
}

func classifyDirectoryReadError(err error) error {
	for _, code := range [...]error{constants.ErrEvidenceDirectoryLimitExceeded, constants.ErrEvidenceArtifactTooLarge, constants.ErrUnexpectedEvidenceArtifact} {
		if errors.Is(err, code) {
			return code
		}
	}
	return constants.ErrDirectoryRead
}

func classifyArtifactReadError(err error) error {
	for _, code := range [...]error{constants.ErrEvidenceArtifactTooLarge, constants.ErrUnexpectedEvidenceArtifact} {
		if errors.Is(err, code) {
			return code
		}
	}
	if errors.Is(err, constants.ErrNotFound) {
		return constants.ErrBundleArtifactMissing
	}
	return constants.ErrFileReadFailed
}

func versionedReferenceKey(reference *compliancev1.VersionedReference) string {
	if reference == nil {
		return ""
	}
	return reference.GetId() + "\x00" + reference.GetVersion()
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
