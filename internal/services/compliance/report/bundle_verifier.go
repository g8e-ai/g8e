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
	"encoding/json"
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
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
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
	EvidenceTrust             evidence.AssessedSignerSource
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
	verifier := bundleVerifier{
		request:                  request,
		report:                   report,
		bodies:                   make(map[string][]byte, len(request.Bundle.GetArtifacts())),
		replayedNodesByAdmission: make(map[string][]evidence.EvidenceNode),
		operationalAdmissions:    make(map[string]struct{}),
	}
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
	request                  BundleVerificationRequest
	report                   *compliancev1.ComplianceVerificationReport
	bodies                   map[string][]byte
	replayedNodesByAdmission map[string][]evidence.EvidenceNode
	operationalAdmissions    map[string]struct{}
	evaluationDiagnostics    []*compliancev1.AssessmentDiagnostic
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
	v.check(constants.ComplianceBundleCheckDecisionReplay, []string{constants.ComplianceBundleScopeFilename, constants.ComplianceBundleAnalysisPath}, func() { v.verifyDecisionReplay(ctx) })
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
	scope := &compliancev1.AssessmentScope{}
	if v.decodeCanonicalSource(constants.ComplianceBundleScopeFilename, scope) {
		if err := catalog.ValidateAssessmentScope(scope); err != nil {
			v.fail(err, constants.ComplianceBundleScopeFilename, err.Error())
		} else {
			if scope.GetScopeId() != manifest.GetScopeRef() {
				v.fail(constants.ErrEvidenceScopeMismatch, constants.ComplianceBundleScopeFilename, "protected assessment scope does not match the manifest scope")
			}
			scopeBody := v.bodies[constants.ComplianceBundleScopeFilename]
			digest := sha256.Sum256(scopeBody)
			if v.request.Bundle.GetAnalysis().GetAssessmentScopeSha256() != hex.EncodeToString(digest[:]) {
				v.fail(constants.ErrChecksumMismatch, constants.ComplianceBundleAnalysisPath, "analysis assessment-scope digest does not match the protected scope")
			}
			v.verifySourceAdmissionBindings(scope)
		}
	}
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

func (v *bundleVerifier) verifySourceAdmissionBindings(scope *compliancev1.AssessmentScope) {
	admissions := make(map[string]*compliancev1.AssessmentSourceAdmission, len(scope.GetSourceAdmissions()))
	for _, admission := range scope.GetSourceAdmissions() {
		admissions[admission.GetAdmissionId()] = admission
	}
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource.GetSourceAdmissionId() == "" {
			if evidence.ArtifactType(resource.GetArtifactType()) == evidence.ArtifactTypeDemoDefinition {
				continue
			}
			v.fail(constants.ErrEvidenceScopeMismatch, resource.GetArtifactId(), "analysis evidence resource does not bind a protected source admission")
			continue
		}
		admission := admissions[resource.GetSourceAdmissionId()]
		if admission == nil {
			v.fail(constants.ErrEvidenceScopeMismatch, resource.GetArtifactId(), "analysis evidence resource does not bind a protected source admission")
			continue
		}
		if admission.GetRunId() != "" && admission.GetRunId() != resource.GetRunId() {
			v.fail(constants.ErrEvidenceScopeMismatch, resource.GetArtifactId(), "analysis evidence resource run does not match its protected source admission")
		}
		if len(admission.GetArtifactIds()) == 0 {
			continue
		}
		selected := false
		for _, artifactID := range admission.GetArtifactIds() {
			if artifactID == resource.GetArtifactId() {
				selected = true
				break
			}
		}
		if !selected {
			v.fail(constants.ErrEvidenceScopeMismatch, resource.GetArtifactId(), "analysis evidence resource is not selected by its protected source admission")
		}
	}
}

func (v *bundleVerifier) protectedAssessmentTime() (time.Time, error) {
	body, exists := v.bodies[constants.ComplianceBundleScopeFilename]
	if !exists {
		return time.Time{}, fmt.Errorf("%w: protected assessment scope is missing", constants.ErrBundleArtifactMissing)
	}
	scope := &compliancev1.AssessmentScope{}
	if err := compliancev1.UnmarshalCanonical(body, scope); err != nil {
		return time.Time{}, fmt.Errorf("%w: decode protected assessment scope: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	if scope.GetAssessmentAsOf() == nil || scope.GetAssessmentAsOf().CheckValid() != nil {
		return time.Time{}, fmt.Errorf("%w: protected assessment time is invalid", constants.ErrInvalidEvidenceGraph)
	}
	return scope.GetAssessmentAsOf().AsTime(), nil
}

func replayedNodeMatches(resource *compliancev1.ComplianceEvidenceReference, node *evidence.EvidenceNode) bool {
	if resource == nil || node == nil {
		return false
	}
	replayed := *node
	replayed.SourceAdmissionID = resource.GetSourceAdmissionId()
	return proto.Equal(resource, replayed.ToProto())
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

type protectedDecisionImporter struct {
	admissionID string
	nodes       []evidence.EvidenceNode
}

func (i protectedDecisionImporter) Import(context.Context) ([]evidence.EvidenceNode, error) {
	return append([]evidence.EvidenceNode(nil), i.nodes...), nil
}

func (i protectedDecisionImporter) SourceID() string {
	return i.admissionID
}

func (v *bundleVerifier) retainReplayedNodes(nodes []evidence.EvidenceNode) error {
	pending := make(map[string][]evidence.EvidenceNode)
	for index := range nodes {
		node := nodes[index]
		resource := v.analysisResource(node.ArtifactID)
		if resource == nil {
			return fmt.Errorf("%w: replayed evidence %s is absent from the protected analysis", constants.ErrInvalidEvidenceGraph, node.ArtifactID)
		}
		if !replayedNodeMatches(resource, &node) {
			return fmt.Errorf("%w: replayed evidence %s does not match the protected analysis", constants.ErrInvalidEvidenceGraph, node.ArtifactID)
		}
		node.SourceAdmissionID = resource.GetSourceAdmissionId()
		pending[node.SourceAdmissionID] = append(pending[node.SourceAdmissionID], node)
	}
	for admissionID, admissionNodes := range pending {
		v.replayedNodesByAdmission[admissionID] = append(v.replayedNodesByAdmission[admissionID], admissionNodes...)
	}
	return nil
}

func (v *bundleVerifier) analysisResource(artifactID string) *compliancev1.ComplianceEvidenceReference {
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource != nil && resource.GetArtifactId() == artifactID {
			return resource
		}
	}
	return nil
}

func (v *bundleVerifier) verifyDecisionReplay(ctx context.Context) {
	if len(v.report.GetFailures()) > 0 {
		return
	}
	scope := &compliancev1.AssessmentScope{}
	if !v.decodeCanonicalSource(constants.ComplianceBundleScopeFilename, scope) {
		return
	}
	assertions := &compliancev1.ControlAssertionCatalog{}
	if !v.decodeCanonicalSource(v.request.Bundle.GetManifest().GetAssertionCatalogRef(), assertions) {
		return
	}
	frameworks := &compliancev1.FrameworkCatalog{}
	if !v.decodeCanonicalSource(path.Join(constants.ComplianceBundleFrameworkCatalogsDirname, constants.ComplianceBundleFrameworkCatalogFilename), frameworks) {
		return
	}
	crosswalkRefs := v.request.Bundle.GetManifest().GetCrosswalkRefs()
	if len(crosswalkRefs) != 1 {
		v.fail(constants.ErrInvalidEvidenceGraph, constants.ComplianceBundleManifestPath, "decision replay requires exactly one protected crosswalk catalog")
		return
	}
	crosswalks := &compliancev1.ControlCrosswalkCatalog{}
	if !v.decodeCanonicalSource(crosswalkRefs[0], crosswalks) {
		return
	}
	for _, admission := range scope.GetSourceAdmissions() {
		if _, exists := v.replayedNodesByAdmission[admission.GetAdmissionId()]; !exists {
			v.fail(constants.ErrInvalidEvidenceGraph, admission.GetAdmissionId(), "protected source admission has no exact importer replay")
		}
	}
	if len(v.replayedNodesByAdmission) == 0 && len(v.request.Bundle.GetAnalysis().GetEvidenceResources()) > 0 {
		v.fail(constants.ErrInvalidEvidenceGraph, constants.ComplianceBundleAnalysisPath, "protected analysis has no exact importer replay")
		return
	}
	sharedNodes := append([]evidence.EvidenceNode(nil), v.replayedNodesByAdmission[""]...)
	sources := make([]GenerationSource, 0, len(scope.GetSourceAdmissions()))
	for _, admission := range scope.GetSourceAdmissions() {
		nodes := append([]evidence.EvidenceNode(nil), v.replayedNodesByAdmission[admission.GetAdmissionId()]...)
		nodes = append(nodes, sharedNodes...)
		sources = append(sources, GenerationSource{AdmissionID: admission.GetAdmissionId(), Importer: protectedDecisionImporter{admissionID: admission.GetAdmissionId(), nodes: nodes}})
	}
	replayed, err := GenerateComplianceAnalysis(ctx, GenerationRequest{Scope: scope, Sources: sources, Assertions: assertions, Frameworks: frameworks, Crosswalks: crosswalks, Diagnostics: v.evaluationDiagnostics})
	if err != nil {
		v.fail(constants.ErrReportVerificationFailed, constants.ComplianceBundleAnalysisPath, fmt.Sprintf("replay canonical compliance analysis: %v", err))
		return
	}
	if !proto.Equal(v.request.Bundle.GetAnalysis(), replayed.Analysis) {
		v.fail(constants.ErrRendererMismatch, constants.ComplianceBundleAnalysisPath, "canonical analysis does not reproduce from protected scope, catalogs, and verified evidence")
	}
	expectedProfiles := make(map[string]*compliancev1.FrameworkProfile, len(v.request.Bundle.GetProfiles()))
	for _, profile := range v.request.Bundle.GetProfiles() {
		expectedProfiles[profile.GetProfileId()] = profile
	}
	if len(expectedProfiles) != len(replayed.Profiles) {
		v.fail(constants.ErrRendererMismatch, constants.ComplianceBundleProfilesDirname, "framework profile count does not reproduce from protected analysis")
		return
	}
	for _, profile := range replayed.Profiles {
		if !proto.Equal(expectedProfiles[profile.GetProfileId()], profile) {
			v.fail(constants.ErrRendererMismatch, path.Join(constants.ComplianceBundleProfilesDirname, profile.GetProfileId()+constants.FileExtJSON), "framework profile does not reproduce from protected analysis")
		}
	}
}

type evidenceVerificationRoute string

const (
	evidenceVerificationRouteDemo        evidenceVerificationRoute = "demo"
	evidenceVerificationRouteEval        evidenceVerificationRoute = "eval"
	evidenceVerificationRouteKSI         evidenceVerificationRoute = "ksi"
	evidenceVerificationRouteCommitment  evidenceVerificationRoute = "commitment"
	evidenceVerificationRouteAttestation evidenceVerificationRoute = "attestation"
	evidenceVerificationRouteAudit       evidenceVerificationRoute = "audit"
	evidenceVerificationRouteLedger      evidenceVerificationRoute = "ledger"
	evidenceVerificationRouteBuildConfig evidenceVerificationRoute = "build-config"
)

var evidenceVerificationRoutes = map[evidence.ArtifactType]evidenceVerificationRoute{
	evidence.ArtifactTypeDemoManifest:        evidenceVerificationRouteDemo,
	evidence.ArtifactTypeDemoResult:          evidenceVerificationRouteDemo,
	evidence.ArtifactTypeDemoStepResult:      evidenceVerificationRouteDemo,
	evidence.ArtifactTypeDemoDefinition:      evidenceVerificationRouteDemo,
	evidence.ArtifactTypeActionReceipt:       evidenceVerificationRouteDemo,
	evidence.ArtifactTypeReceiptPersistence:  evidenceVerificationRouteDemo,
	evidence.ArtifactTypeStateObservation:    evidenceVerificationRouteDemo,
	evidence.ArtifactTypeDemoMetric:          evidenceVerificationRouteDemo,
	evidence.ArtifactTypeProtocolChain:       evidenceVerificationRouteDemo,
	evidence.ArtifactTypeEvalManifest:        evidenceVerificationRouteEval,
	evidence.ArtifactTypeEvalTask:            evidenceVerificationRouteEval,
	evidence.ArtifactTypeEvalAttempt:         evidenceVerificationRouteEval,
	evidence.ArtifactTypeEvalMetric:          evidenceVerificationRouteEval,
	evidence.ArtifactTypeEvalObservation:     evidenceVerificationRouteEval,
	evidence.ArtifactTypeEvalExchange:        evidenceVerificationRouteEval,
	evidence.ArtifactTypeEvalStage:           evidenceVerificationRouteEval,
	evidence.ArtifactTypeEvalReceipt:         evidenceVerificationRouteEval,
	evidence.ArtifactTypeAuditRecord:         evidenceVerificationRouteAudit,
	evidence.ArtifactTypeLedgerCommit:        evidenceVerificationRouteLedger,
	evidence.ArtifactTypeLedgerState:         evidenceVerificationRouteLedger,
	evidence.ArtifactTypeCommitment:          evidenceVerificationRouteCommitment,
	evidence.ArtifactTypeAuditChainEntry:     evidenceVerificationRouteCommitment,
	evidence.ArtifactTypeKSIResult:           evidenceVerificationRouteKSI,
	evidence.ArtifactTypeBuildAttestation:    evidenceVerificationRouteBuildConfig,
	evidence.ArtifactTypeConfigAttestation:   evidenceVerificationRouteBuildConfig,
	evidence.ArtifactTypeCustomerAttestation: evidenceVerificationRouteAttestation,
	evidence.ArtifactTypeAssessorAttestation: evidenceVerificationRouteAttestation,
}

type demoSourceInventory struct {
	verificationReport *compliancev1.ComplianceVerificationReport
	runtimeManifest    bool
	runtimeResults     bool
	provenanceArtifact bool
	definitions        bool
}

func (v *bundleVerifier) verifySourceVerificationReports(ctx context.Context) {
	v.verifyEvidenceArtifactRoutes()
	v.verifyEvaluationSelectionDiagnostics()
	v.verifyDemoSourceVerificationReports(ctx)
	v.verifyEvalSourceVerificationReports(ctx)
	v.verifyOperationalSources(ctx)
	v.verifyKSIHistorySources(ctx)
	v.verifyCommitmentSources(ctx)
	v.verifyAttestationSources(ctx)
	v.verifyAuditRecordSources(ctx)
	v.verifyLedgerSources(ctx)
	v.verifyBuildConfigSources(ctx)
}

func (v *bundleVerifier) verifyEvidenceArtifactRoutes() {
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource == nil {
			v.fail(constants.ErrInvalidEvidenceGraph, constants.ComplianceBundleAnalysisPath, "analysis contains a nil evidence resource")
			continue
		}
		if _, exists := evidenceVerificationRoutes[evidence.ArtifactType(resource.GetArtifactType())]; !exists {
			v.fail(constants.ErrInvalidEvidenceGraph, resource.GetArtifactId(), "evidence artifact type has no complete-bundle verification route")
		}
	}
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
	admission            *compliancev1.AssessmentSourceAdmission
	verificationReport   *compliancev1.ComplianceVerificationReport
	campaignReport       *evalv1.EvaluationVerificationReport
	campaignInventory    *evalv1.CampaignComplianceSourceInventory
	report               bool
	evidence             bool
	runtimeArtifacts     int
	runtimeArtifactPaths map[string]struct{}
}

func (v *bundleVerifier) verifyEvaluationSelectionDiagnostics() {
	bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.EvaluationSelectionDiagnosticsFilename)
	body, exists := v.bodies[bundlePath]
	if !exists {
		return
	}
	diagnostics, err := UnmarshalAssessmentDiagnostics(body)
	if err != nil {
		v.fail(constants.ErrEvidenceArtifactMalformed, bundlePath, err.Error())
		return
	}
	scope := &compliancev1.AssessmentScope{}
	if !v.decodeCanonicalSource(constants.ComplianceBundleScopeFilename, scope) {
		return
	}
	admissions := make(map[string]*compliancev1.AssessmentSourceAdmission, len(scope.GetSourceAdmissions()))
	selectedEvalAdmissions := make(map[string]string)
	for _, admission := range scope.GetSourceAdmissions() {
		admissions[admission.GetAdmissionId()] = admission
		if admission.GetSourceKind() == constants.EvaluationSourceKindNative {
			selectedEvalAdmissions[admission.GetAdmissionId()] = "evaluation_candidate_native"
		}
		if admission.GetSourceKind() == constants.EvaluationSourceKindCampaign {
			selectedEvalAdmissions[admission.GetAdmissionId()] = "evaluation_candidate_campaign"
		}
	}
	expectedSeverity := map[string]string{
		"evaluation_candidate_native":         "info",
		"evaluation_candidate_campaign":       "info",
		"evaluation_candidate_incomplete":     "warning",
		"evaluation_candidate_unsupported":    "warning",
		"evaluation_candidate_malformed":      "error",
		"evaluation_candidate_outside_window": "info",
	}
	seen := make(map[string]struct{}, len(diagnostics))
	seenAdmissions := make(map[string]struct{}, len(selectedEvalAdmissions))
	for _, diagnostic := range diagnostics {
		runID := diagnostic.GetSubject().GetRunId()
		severity, supported := expectedSeverity[diagnostic.GetCode()]
		if diagnostic == nil || !supported || diagnostic.GetSeverity() != severity || runID == "" || diagnostic.GetMessage() == "" {
			v.fail(constants.ErrEvidenceArtifactMalformed, bundlePath, "evaluation selection diagnostic is incomplete or unsupported")
			return
		}
		if _, exists := seen[runID]; exists {
			v.fail(constants.ErrEvidenceDuplicateID, bundlePath, "evaluation selection diagnostic repeats a run candidate")
			return
		}
		seen[runID] = struct{}{}
		if diagnostic.GetSourceAdmissionId() != "" {
			admission := admissions[diagnostic.GetSourceAdmissionId()]
			if admission == nil || admission.GetRunId() != runID || selectedEvalAdmissions[admission.GetAdmissionId()] != diagnostic.GetCode() {
				v.fail(constants.ErrEvidenceScopeMismatch, bundlePath, "evaluation selection diagnostic does not match its protected admission")
				return
			}
			seenAdmissions[admission.GetAdmissionId()] = struct{}{}
		}
	}
	for admissionID := range selectedEvalAdmissions {
		if _, exists := seenAdmissions[admissionID]; !exists {
			v.fail(constants.ErrEvidenceScopeMismatch, bundlePath, "evaluation selection diagnostics omit a protected eval admission")
			return
		}
	}
	v.evaluationDiagnostics = diagnostics
}

func (v *bundleVerifier) verifyEvalSourceVerificationReports(ctx context.Context) {
	scope := &compliancev1.AssessmentScope{}
	if !v.decodeCanonicalSource(constants.ComplianceBundleScopeFilename, scope) {
		prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname) + "/"
		for bundlePath := range v.bodies {
			if strings.HasPrefix(bundlePath, prefix) {
				v.fail(constants.ErrUnresolvedReference, bundlePath, "eval source artifact does not bind a protected admission")
			}
		}
		return
	}
	admissionsByID := make(map[string]*compliancev1.AssessmentSourceAdmission, len(scope.GetSourceAdmissions()))
	for _, admission := range scope.GetSourceAdmissions() {
		admissionsByID[admission.GetAdmissionId()] = admission
	}
	evalAdmissions := make(map[string]struct{})
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource.GetArtifactType() != string(evidence.ArtifactTypeEvalManifest) || resource.GetVerifierId() == constants.DemoRunVerifierID {
			continue
		}
		if admissionsByID[resource.GetSourceAdmissionId()] == nil {
			v.fail(constants.ErrEvalRunVerificationFailed, path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, resource.GetRunId()), "eval evidence run does not bind a protected source admission")
			continue
		}
		evalAdmissions[resource.GetSourceAdmissionId()] = struct{}{}
	}
	inventories := make(map[string]*evalSourceInventory)
	for _, admission := range scope.GetSourceAdmissions() {
		_, hasEvalManifest := evalAdmissions[admission.GetAdmissionId()]
		if hasEvalManifest && (admission.GetSourceKind() == constants.EvaluationSourceKindNative || admission.GetSourceKind() == constants.EvaluationSourceKindCampaign) {
			key := admission.GetRunId()
			if admission.GetSourceKind() == constants.EvaluationSourceKindCampaign {
				key = admission.GetAdmissionId()
			}
			inventories[key] = &evalSourceInventory{admission: admission, runtimeArtifactPaths: make(map[string]struct{})}
		}
	}
	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) < 4 || parts[2] == "" {
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "eval source path is unsupported")
			continue
		}
		inventory := inventories[parts[2]]
		if inventory == nil {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "eval source artifact does not bind a protected admission")
			continue
		}
		campaign := inventory.admission.GetSourceKind() == constants.EvaluationSourceKindCampaign
		switch parts[3] {
		case constants.ComplianceBundleSourceVerificationFilename:
			if campaign {
				report := &evalv1.EvaluationVerificationReport{}
				if err := evalv1.UnmarshalCanonical(body, report); err != nil || report.GetRunId() != inventory.admission.GetRunId() || report.GetVerifierContractVersion() != constants.CampaignVerifierVersion {
					v.fail(constants.ErrEvalRunVerificationFailed, bundlePath, "campaign verification report is invalid")
				} else {
					inventory.campaignReport = report
				}
			} else {
				inventory.verificationReport = v.verifySourceVerificationReport(bundlePath, body, inventory.admission.GetRunId(), constants.EvalRunVerifierID, constants.EvalRunVerifierVersion, constants.ErrEvalRunVerificationFailed, "eval")
			}
		case constants.CampaignSourceInventoryFilename:
			if !campaign {
				v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "native eval source cannot carry a campaign inventory")
				continue
			}
			manifest := &evalv1.CampaignComplianceSourceInventory{}
			if err := evalv1.UnmarshalCanonical(body, manifest); err != nil {
				v.fail(constants.ErrEvidenceArtifactMalformed, bundlePath, "campaign source inventory is not canonical")
				continue
			}
			if err := validateCampaignSourceInventory(manifest, inventory.admission); err != nil {
				v.fail(constants.ErrEvidenceArtifactMalformed, bundlePath, err.Error())
			} else {
				inventory.campaignInventory = manifest
			}
		case constants.ComplianceBundleSourceRuntimeDirname:
			if campaign {
				if len(parts) < 5 {
					v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "campaign runtime source path is incomplete")
					continue
				}
				runtimePath := strings.Join(parts[4:], "/")
				if !evidence.ValidRelativePath(runtimePath) {
					v.fail(constants.ErrPathValidation, bundlePath, "campaign runtime source path is invalid")
					continue
				}
				if _, exists := inventory.runtimeArtifactPaths[runtimePath]; exists {
					v.fail(constants.ErrEvidenceDuplicateID, bundlePath, "campaign runtime source path is duplicated")
					continue
				}
				inventory.runtimeArtifactPaths[runtimePath] = struct{}{}
				inventory.runtimeArtifacts++
			}
			if !campaign && len(parts) == 5 && parts[4] == constants.EvaluationReportFilename {
				inventory.report = true
			}
			if !campaign && len(parts) == 6 && parts[4] == constants.EvaluationEvidenceDirname {
				inventory.evidence = true
			}
		default:
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "eval source path is unsupported")
		}
	}
	for key, inventory := range inventories {
		if inventory.admission.GetSourceKind() == constants.EvaluationSourceKindCampaign {
			if inventory.campaignReport == nil || inventory.campaignInventory == nil || inventory.runtimeArtifacts != len(inventory.campaignInventory.GetArtifacts()) || !campaignRuntimePathsMatchInventory(inventory) {
				v.fail(constants.ErrEvalRunVerificationFailed, key, "campaign source inventory is incomplete or does not enumerate the protected runtime exactly")
				continue
			}
			v.replayCampaignSourceVerification(ctx, key, inventory)
			continue
		}
		if inventory.verificationReport == nil || !inventory.report || !inventory.evidence {
			v.fail(constants.ErrEvalRunVerificationFailed, key, "eval evidence run lacks a complete protected runtime source inventory")
			continue
		}
		v.replayEvalSourceVerification(ctx, inventory.admission.GetRunId(), inventory.verificationReport)
	}
}

type ledgerSourceInventory struct {
	commitsPath string
	commitsBody []byte
	statePath   string
	stateBody   []byte
	scopeID     string
	runID       string
	attemptID   string
	scenarioID  string
	resources   map[string]*compliancev1.ComplianceEvidenceReference
}

func (v *bundleVerifier) verifyLedgerSources(ctx context.Context) {
	inventories := make(map[string]*ledgerSourceInventory)
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource == nil || (resource.GetArtifactType() != string(evidence.ArtifactTypeLedgerCommit) && resource.GetArtifactType() != string(evidence.ArtifactTypeLedgerState)) {
			continue
		}
		if !evidence.ValidPathElement(resource.GetScopeId()) || !evidence.ValidPathElement(resource.GetRunId()) {
			v.fail(constants.ErrEvidenceScopeMismatch, resource.GetArtifactId(), "ledger evidence requires canonical scope and run identifiers")
			continue
		}
		base := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, resource.GetScopeId(), resource.GetRunId(), constants.LedgerEvidenceDirname)
		commitsPath := path.Join(base, constants.LedgerCommitsFilename)
		statePath := path.Join(base, constants.LedgerStateFilename)
		expectedPath := commitsPath
		if resource.GetArtifactType() == string(evidence.ArtifactTypeLedgerState) {
			expectedPath = statePath
		}
		if resource.GetBundlePath() != expectedPath {
			v.fail(constants.ErrUnresolvedReference, resource.GetArtifactId(), "ledger evidence does not reference its canonical protected source")
			continue
		}
		key := resource.GetScopeId() + "\x00" + resource.GetRunId()
		inventory := inventories[key]
		if inventory == nil {
			inventory = &ledgerSourceInventory{commitsPath: commitsPath, statePath: statePath, scopeID: resource.GetScopeId(), runID: resource.GetRunId(), attemptID: resource.GetAttemptId(), scenarioID: resource.GetScenarioId(), resources: make(map[string]*compliancev1.ComplianceEvidenceReference)}
			inventories[key] = inventory
		}
		if resource.GetAttemptId() != inventory.attemptID || resource.GetScenarioId() != inventory.scenarioID {
			v.fail(constants.ErrInvalidEvidenceGraph, resource.GetArtifactId(), "ledger evidence inventory has conflicting attempt or scenario bindings")
			continue
		}
		if _, exists := inventory.resources[resource.GetArtifactId()]; exists {
			v.fail(constants.ErrEvidenceDuplicateID, resource.GetArtifactId(), "ledger evidence resource is duplicated")
			continue
		}
		inventory.resources[resource.GetArtifactId()] = resource
	}

	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) < 5 || parts[4] != constants.LedgerEvidenceDirname {
			continue
		}
		key := parts[2] + "\x00" + parts[3]
		inventory := inventories[key]
		if inventory == nil {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "ledger source does not bind analysis evidence")
			continue
		}
		switch bundlePath {
		case inventory.commitsPath:
			inventory.commitsBody = body
		case inventory.statePath:
			inventory.stateBody = body
		default:
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "ledger source path is unsupported")
		}
	}

	for _, inventory := range inventories {
		if len(inventory.commitsBody) == 0 {
			v.fail(constants.ErrInvalidEvidenceGraph, inventory.commitsPath, "ledger commit source inventory is missing or empty")
			continue
		}
		if len(inventory.stateBody) == 0 {
			v.fail(constants.ErrInvalidEvidenceGraph, inventory.statePath, "ledger state source inventory is missing or empty")
			continue
		}
		v.replayLedgerSource(ctx, inventory)
	}
}

func (v *bundleVerifier) replayLedgerSource(ctx context.Context, inventory *ledgerSourceInventory) {
	binding := evidence.LedgerImportBinding{
		CommitsReference: evidence.ContentReferenceForBody(constants.LedgerCommitCollectionReferencePrefix, inventory.commitsBody),
		StateReference:   evidence.ContentReferenceForBody(constants.LedgerStateReferencePrefix, inventory.stateBody),
		CommitsPath:      inventory.commitsPath,
		StatePath:        inventory.statePath,
		ScopeID:          inventory.scopeID,
		RunID:            inventory.runID,
		AttemptID:        inventory.attemptID,
		ScenarioID:       inventory.scenarioID,
	}
	nodes, err := evidence.NewLedgerImporter(&bundledSourceArtifactReader{bodies: v.bodies}, binding).Import(ctx)
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.commitsPath, err.Error())
		return
	}
	if len(nodes) != len(inventory.resources) {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.commitsPath, "replayed ledger inventory does not match the protected analysis evidence count")
		return
	}
	if err := v.retainReplayedNodes(nodes); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.commitsPath, err.Error())
	}
}

type buildConfigSourceInventory struct {
	path      string
	body      []byte
	scopeID   string
	runID     string
	resources map[string]*compliancev1.ComplianceEvidenceReference
}

func (v *bundleVerifier) verifyBuildConfigSources(ctx context.Context) {
	inventories := make(map[string]*buildConfigSourceInventory)
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource == nil || (resource.GetArtifactType() != string(evidence.ArtifactTypeBuildAttestation) && resource.GetArtifactType() != string(evidence.ArtifactTypeConfigAttestation)) {
			continue
		}
		if !evidence.ValidPathElement(resource.GetScopeId()) || !evidence.ValidPathElement(resource.GetRunId()) {
			v.fail(constants.ErrEvidenceScopeMismatch, resource.GetArtifactId(), "build and configuration evidence requires canonical scope and run identifiers")
			continue
		}
		expectedPath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, resource.GetScopeId(), resource.GetRunId(), constants.BuildConfigAttestationsFilename)
		if resource.GetBundlePath() != expectedPath {
			v.fail(constants.ErrUnresolvedReference, resource.GetArtifactId(), "build or configuration evidence does not reference its canonical protected source")
			continue
		}
		key := resource.GetScopeId() + "\x00" + resource.GetRunId()
		inventory := inventories[key]
		if inventory == nil {
			inventory = &buildConfigSourceInventory{path: expectedPath, scopeID: resource.GetScopeId(), runID: resource.GetRunId(), resources: make(map[string]*compliancev1.ComplianceEvidenceReference)}
			inventories[key] = inventory
		}
		if _, exists := inventory.resources[resource.GetArtifactId()]; exists {
			v.fail(constants.ErrEvidenceDuplicateID, resource.GetArtifactId(), "build or configuration evidence resource is duplicated")
			continue
		}
		inventory.resources[resource.GetArtifactId()] = resource
	}

	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) != 5 || parts[4] != constants.BuildConfigAttestationsFilename {
			continue
		}
		inventory := inventories[parts[2]+"\x00"+parts[3]]
		if inventory == nil {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "build and configuration source does not bind analysis evidence")
			continue
		}
		inventory.body = body
	}

	for _, inventory := range inventories {
		if len(inventory.body) == 0 {
			v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, "build and configuration source inventory is missing or empty")
			continue
		}
		v.replayBuildConfigSource(ctx, inventory)
	}
}

func (v *bundleVerifier) replayBuildConfigSource(ctx context.Context, inventory *buildConfigSourceInventory) {
	metadata, err := evidence.InspectBuildConfigSource(inventory.body)
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, err.Error())
		return
	}
	binding := evidence.BuildConfigImportBinding{
		Reference:        evidence.ContentReferenceForBody(constants.BuildAttestationReferencePrefix, inventory.body),
		Path:             inventory.path,
		ScopeID:          inventory.scopeID,
		RunID:            inventory.runID,
		BuildIdentity:    metadata.BuildIdentity,
		SourceRevision:   metadata.SourceRevision,
		ProducerIdentity: metadata.ProducerIdentity,
	}
	nodes, err := evidence.NewBuildConfigImporter(&bundledSourceArtifactReader{bodies: v.bodies}, binding).Import(ctx)
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, err.Error())
		return
	}
	if len(nodes) != len(inventory.resources) {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, "replayed build and configuration inventory does not match the protected analysis evidence count")
		return
	}
	if err := v.retainReplayedNodes(nodes); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, err.Error())
	}
}

func (v *bundleVerifier) verifyOperationalSources(ctx context.Context) {
	scope := &compliancev1.AssessmentScope{}
	if !v.decodeCanonicalSource(constants.ComplianceBundleScopeFilename, scope) {
		return
	}
	admissions := make(map[string]*compliancev1.AssessmentSourceAdmission, len(scope.GetSourceAdmissions()))
	for _, admission := range scope.GetSourceAdmissions() {
		admissions[admission.GetAdmissionId()] = admission
	}
	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) || path.Base(bundlePath) != constants.ComplianceOperationalInventoryFilename {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) != 4 || parts[0] != constants.ComplianceBundleSourcesDirname || parts[1] != constants.ComplianceOperationalExportDirname || parts[2] == "" {
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "operational source inventory path is unsupported")
			continue
		}
		admissionID := parts[2]
		admission := admissions[admissionID]
		if admission == nil {
			v.fail(constants.ErrEvidenceScopeMismatch, bundlePath, "operational source inventory has no protected source admission")
			continue
		}
		if _, duplicate := v.operationalAdmissions[admissionID]; duplicate {
			v.fail(constants.ErrEvidenceDuplicateID, bundlePath, "operational source admission has multiple inventories")
			continue
		}
		v.operationalAdmissions[admissionID] = struct{}{}
		inventory := &evidence.OperationalSourceInventory{}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(inventory); err != nil {
			v.fail(constants.ErrEvidenceArtifactMalformed, bundlePath, err.Error())
			continue
		}
		sourceRoot := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, admissionID)
		expectedPaths := map[string]struct{}{bundlePath: {}}
		validInventory := true
		for _, artifact := range inventory.Artifacts {
			if !evidence.ValidRelativePath(artifact.RelativePath) {
				v.fail(constants.ErrPathValidation, bundlePath, "operational source inventory contains an invalid artifact path")
				validInventory = false
				continue
			}
			expectedPaths[path.Join(sourceRoot, artifact.RelativePath)] = struct{}{}
		}
		for protectedPath := range v.bodies {
			if strings.HasPrefix(protectedPath, sourceRoot+"/") {
				if _, expected := expectedPaths[protectedPath]; !expected {
					v.fail(constants.ErrUnexpectedEvidenceArtifact, protectedPath, "operational source contains an artifact absent from its inventory")
					validInventory = false
				}
			}
		}
		for expectedPath := range expectedPaths {
			if _, present := v.bodies[expectedPath]; !present {
				v.fail(constants.ErrBundleArtifactMissing, expectedPath, "operational inventory artifact is absent from the protected bundle")
				validInventory = false
			}
		}
		if !validInventory {
			continue
		}
		if v.request.EvidenceTrust == nil {
			v.fail(constants.ErrEvidenceTrustNotAssessed, bundlePath, "operational source requires explicit external assessed evidence trust")
			continue
		}
		assessmentAsOf, err := v.protectedAssessmentTime()
		if err != nil {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, err.Error())
			continue
		}
		importer := evidence.NewOperationalExportImporter(&bundledSourceArtifactReader{bodies: v.bodies}, v.request.EvidenceTrust, bundlePath, sourceRoot, scope.GetScopeId(), admission, assessmentAsOf, func() time.Time { return assessmentAsOf })
		nodes, err := importer.Import(ctx)
		if err != nil {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, err.Error())
			continue
		}
		expectedCount := 0
		for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
			if resource.GetSourceAdmissionId() == admissionID {
				expectedCount++
			}
		}
		if len(nodes) != expectedCount {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, "replayed operational inventory does not match the protected analysis evidence count")
			continue
		}
		if err := v.retainReplayedNodes(nodes); err != nil {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, err.Error())
		}
	}
}

type commitmentSourceInventory struct {
	path     string
	body     []byte
	resource *compliancev1.ComplianceEvidenceReference
}

func (v *bundleVerifier) verifyCommitmentSources(ctx context.Context) {
	inventories := make(map[string]*commitmentSourceInventory)
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource == nil || resource.GetArtifactType() != string(evidence.ArtifactTypeCommitment) {
			continue
		}
		if _, operational := v.operationalAdmissions[resource.GetSourceAdmissionId()]; operational {
			continue
		}
		bundlePath := resource.GetBundlePath()
		if !evidence.ValidPathElement(resource.GetScopeId()) || !evidence.ValidPathElement(resource.GetRunId()) {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, "commitment evidence requires canonical scope and run identifiers")
			continue
		}
		expectedPath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, resource.GetScopeId(), resource.GetRunId(), constants.ComplianceBundleCommitmentsFilename)
		if bundlePath != expectedPath {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, "commitment evidence does not reference its protected source inventory")
			continue
		}
		key := resource.GetScopeId() + "\x00" + resource.GetRunId()
		if _, exists := inventories[key]; exists {
			v.fail(constants.ErrEvidenceDuplicateID, bundlePath, "commitment source inventory binds multiple analysis resources")
			continue
		}
		inventories[key] = &commitmentSourceInventory{path: bundlePath, resource: resource}
	}

	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) != 5 || parts[4] != constants.ComplianceBundleCommitmentsFilename {
			continue
		}
		key := parts[2] + "\x00" + parts[3]
		inventory := inventories[key]
		if inventory == nil {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "commitment source does not bind analysis evidence")
			continue
		}
		inventory.body = body
	}

	for _, inventory := range inventories {
		if len(inventory.body) == 0 {
			v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, "commitment source inventory is missing or empty")
			continue
		}
		if v.request.EvidenceTrust == nil {
			v.fail(constants.ErrEvidenceTrustNotAssessed, inventory.path, "commitment source requires explicit external assessed evidence trust")
			continue
		}
		v.replayCommitmentSource(ctx, inventory)
	}
}

func (v *bundleVerifier) replayCommitmentSource(ctx context.Context, inventory *commitmentSourceInventory) {
	resource := inventory.resource
	binding := evidence.CommitmentImportBinding{
		Reference:     resource.GetArtifactId(),
		Path:          inventory.path,
		ScopeID:       resource.GetScopeId(),
		RunID:         resource.GetRunId(),
		AttemptID:     resource.GetAttemptId(),
		ScenarioID:    resource.GetScenarioId(),
		TransactionID: resource.GetTransactionId(),
	}
	assessmentAsOf, err := v.protectedAssessmentTime()
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, err.Error())
		return
	}
	reader := &bundledSourceArtifactReader{bodies: v.bodies}
	nodes, err := evidence.NewCommitmentImporter(reader, v.request.EvidenceTrust, binding, assessmentAsOf).Import(ctx)
	if err != nil {
		failure := constants.ErrInvalidEvidenceGraph
		if errors.Is(err, constants.ErrEvidenceTrustNotAssessed) {
			failure = constants.ErrEvidenceTrustNotAssessed
		}
		v.fail(failure, inventory.path, err.Error())
		return
	}
	if len(nodes) != 1 {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, "replayed commitment does not produce exactly one evidence resource")
		return
	}
	if nodes[0].VerificationStatus == evidence.VerificationStatusUnverified {
		v.fail(constants.ErrEvidenceTrustNotAssessed, inventory.path, "commitment signer is not present in external assessed evidence trust")
		return
	}
	if err := v.retainReplayedNodes(nodes); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, err.Error())
	}
}

type attestationSourceInventory struct {
	path      string
	body      []byte
	scopeID   string
	runID     string
	resources map[string]*compliancev1.ComplianceEvidenceReference
}

func (v *bundleVerifier) verifyAttestationSources(ctx context.Context) {
	inventories := make(map[string]*attestationSourceInventory)
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource == nil || (resource.GetArtifactType() != string(evidence.ArtifactTypeCustomerAttestation) && resource.GetArtifactType() != string(evidence.ArtifactTypeAssessorAttestation)) {
			continue
		}
		bundlePath := resource.GetBundlePath()
		if !evidence.ValidPathElement(resource.GetScopeId()) || !evidence.ValidPathElement(resource.GetRunId()) {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, "attestation evidence requires canonical scope and run identifiers")
			continue
		}
		expectedPath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, resource.GetScopeId(), resource.GetRunId(), constants.ComplianceBundleAttestationsFilename)
		if bundlePath != expectedPath {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, "attestation evidence does not reference its protected source inventory")
			continue
		}
		key := resource.GetScopeId() + "\x00" + resource.GetRunId()
		inventory := inventories[key]
		if inventory == nil {
			inventory = &attestationSourceInventory{path: bundlePath, scopeID: resource.GetScopeId(), runID: resource.GetRunId(), resources: make(map[string]*compliancev1.ComplianceEvidenceReference)}
			inventories[key] = inventory
		}
		if _, exists := inventory.resources[resource.GetArtifactId()]; exists {
			v.fail(constants.ErrEvidenceDuplicateID, bundlePath, "attestation evidence resource is duplicated")
			continue
		}
		inventory.resources[resource.GetArtifactId()] = resource
	}

	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) != 5 || parts[4] != constants.ComplianceBundleAttestationsFilename {
			continue
		}
		key := parts[2] + "\x00" + parts[3]
		inventory := inventories[key]
		if inventory == nil {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "attestation source does not bind analysis evidence")
			continue
		}
		inventory.body = body
	}

	for _, inventory := range inventories {
		if len(inventory.body) == 0 {
			v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, "attestation source inventory is missing or empty")
			continue
		}
		if v.request.EvidenceTrust == nil {
			v.fail(constants.ErrEvidenceTrustNotAssessed, inventory.path, "attestation source requires explicit external assessed evidence trust")
			continue
		}
		v.replayAttestationSource(ctx, inventory)
	}
}

func (v *bundleVerifier) replayAttestationSource(ctx context.Context, inventory *attestationSourceInventory) {
	assessmentAsOf, err := v.protectedAssessmentTime()
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, err.Error())
		return
	}
	binding := evidence.AttestationImportBinding{
		Reference: evidence.ContentReferenceForBody(constants.AttestationCollectionReferencePrefix, inventory.body),
		Path:      inventory.path,
		ScopeID:   inventory.scopeID,
		RunID:     inventory.runID,
	}
	reader := &bundledSourceArtifactReader{bodies: v.bodies}
	nodes, err := evidence.NewAttestationImporter(reader, v.request.EvidenceTrust, binding, assessmentAsOf).Import(ctx)
	if err != nil {
		failure := constants.ErrInvalidEvidenceGraph
		if errors.Is(err, constants.ErrEvidenceTrustNotAssessed) {
			failure = constants.ErrEvidenceTrustNotAssessed
		}
		v.fail(failure, inventory.path, err.Error())
		return
	}
	if len(nodes) != len(inventory.resources) {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, "replayed attestation inventory does not match the protected analysis evidence count")
		return
	}
	for index := range nodes {
		if nodes[index].VerificationStatus == evidence.VerificationStatusUnverified {
			v.fail(constants.ErrEvidenceTrustNotAssessed, inventory.path, "attestation signer is not present in external assessed evidence trust")
			return
		}
	}
	if err := v.retainReplayedNodes(nodes); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, err.Error())
	}
}

type auditRecordSourceInventory struct {
	path     string
	body     []byte
	resource *compliancev1.ComplianceEvidenceReference
}

func (v *bundleVerifier) verifyAuditRecordSources(ctx context.Context) {
	inventories := make(map[string]*auditRecordSourceInventory)
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource == nil || resource.GetArtifactType() != string(evidence.ArtifactTypeAuditRecord) {
			continue
		}
		bundlePath := resource.GetBundlePath()
		if !evidence.ValidPathElement(resource.GetScopeId()) || !evidence.ValidPathElement(resource.GetRunId()) {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, "audit record evidence requires canonical scope and run identifiers")
			continue
		}
		expectedPath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, resource.GetScopeId(), resource.GetRunId(), constants.AuditRecordsDirname, resource.GetSha256()+constants.FileExtJSON)
		if bundlePath != expectedPath {
			v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, "audit record evidence does not reference its protected content-addressed source")
			continue
		}
		if _, exists := inventories[bundlePath]; exists {
			v.fail(constants.ErrEvidenceDuplicateID, bundlePath, "audit record source binds multiple analysis resources")
			continue
		}
		inventories[bundlePath] = &auditRecordSourceInventory{path: bundlePath, resource: resource}
	}

	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) != 6 || parts[4] != constants.AuditRecordsDirname {
			continue
		}
		inventory := inventories[bundlePath]
		if inventory == nil {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "audit record source does not bind analysis evidence")
			continue
		}
		inventory.body = body
	}

	for _, inventory := range inventories {
		if len(inventory.body) == 0 {
			v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, "audit record source is missing or empty")
			continue
		}
		v.replayAuditRecordSource(ctx, inventory)
	}
}

func (v *bundleVerifier) replayAuditRecordSource(ctx context.Context, inventory *auditRecordSourceInventory) {
	resource := inventory.resource
	binding := evidence.AuditRecordImportBinding{
		Reference:         resource.GetArtifactId(),
		Path:              inventory.path,
		ScopeID:           resource.GetScopeId(),
		RunID:             resource.GetRunId(),
		AttemptID:         resource.GetAttemptId(),
		ScenarioID:        resource.GetScenarioId(),
		OperatorSessionID: resource.GetProducerIdentity(),
	}
	reader := &bundledSourceArtifactReader{bodies: v.bodies}
	nodes, err := evidence.NewAuditRecordImporter(reader, binding).Import(ctx)
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, err.Error())
		return
	}
	if len(nodes) != 1 {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, "replayed audit record does not produce exactly one evidence resource")
		return
	}
	if err := v.retainReplayedNodes(nodes); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.path, err.Error())
	}
}

type ksiHistorySourceInventory struct {
	historyPath string
	historyBody []byte
	resultsPath string
	resultsBody []byte
	resources   map[string]*compliancev1.ComplianceEvidenceReference
}

func (v *bundleVerifier) verifyKSIHistorySources(ctx context.Context) {
	inventories := make(map[string]*ksiHistorySourceInventory)
	for _, resource := range v.request.Bundle.GetAnalysis().GetEvidenceResources() {
		if resource == nil || resource.GetArtifactType() != string(evidence.ArtifactTypeKSIResult) {
			continue
		}
		if !evidence.ValidPathElement(resource.GetScopeId()) || !evidence.ValidPathElement(resource.GetRunId()) {
			v.fail(constants.ErrEvidenceScopeMismatch, resource.GetArtifactId(), "KSI history evidence requires canonical scope and run identifiers")
			continue
		}
		historyPath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, resource.GetScopeId(), resource.GetRunId(), constants.ComplianceBundleKSIHistoryFilename)
		if resource.GetBundlePath() != historyPath {
			v.fail(constants.ErrUnresolvedReference, resource.GetArtifactId(), "KSI history evidence does not reference its protected source inventory")
			continue
		}
		key := resource.GetScopeId() + "\x00" + resource.GetRunId()
		inventory := inventories[key]
		if inventory == nil {
			inventory = &ksiHistorySourceInventory{
				historyPath: historyPath,
				resultsPath: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, resource.GetScopeId(), resource.GetRunId(), constants.ComplianceBundleKSIResultsFilename),
				resources:   make(map[string]*compliancev1.ComplianceEvidenceReference),
			}
			inventories[key] = inventory
		}
		if _, exists := inventory.resources[resource.GetArtifactId()]; exists {
			v.fail(constants.ErrEvidenceDuplicateID, resource.GetArtifactId(), "KSI history evidence resource is duplicated")
			continue
		}
		inventory.resources[resource.GetArtifactId()] = resource
	}

	prefix := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname) + "/"
	for bundlePath, body := range v.bodies {
		if !strings.HasPrefix(bundlePath, prefix) {
			continue
		}
		parts := strings.Split(bundlePath, "/")
		if len(parts) != 5 || parts[0] != constants.ComplianceBundleSourcesDirname || parts[1] != constants.ComplianceBundlePlatformEvidenceDirname || parts[2] == "" || parts[3] == "" {
			continue
		}
		if parts[4] != constants.ComplianceBundleKSIHistoryFilename && parts[4] != constants.ComplianceBundleKSIResultsFilename {
			continue
		}
		key := parts[2] + "\x00" + parts[3]
		inventory := inventories[key]
		if inventory == nil {
			v.fail(constants.ErrUnresolvedReference, bundlePath, "platform source does not bind analysis evidence")
			continue
		}
		switch bundlePath {
		case inventory.historyPath:
			inventory.historyBody = body
		case inventory.resultsPath:
			inventory.resultsBody = body
		default:
			v.fail(constants.ErrUnexpectedEvidenceArtifact, bundlePath, "platform source path is unsupported")
		}
	}

	for _, inventory := range inventories {
		if len(inventory.historyBody) == 0 {
			v.fail(constants.ErrInvalidEvidenceGraph, inventory.historyPath, "KSI history source inventory is missing or empty")
			continue
		}
		if len(inventory.resultsBody) == 0 {
			v.fail(constants.ErrInvalidEvidenceGraph, inventory.resultsPath, "KSI result source inventory is missing or empty")
			continue
		}
		v.replayKSIHistorySource(ctx, inventory)
	}
}

func (v *bundleVerifier) replayKSIHistorySource(ctx context.Context, inventory *ksiHistorySourceInventory) {
	resultSet, err := firstKSIHistoryResultSet(inventory.historyBody)
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.historyPath, err.Error())
		return
	}
	latestBody, err := lastKSIHistoryResultBody(inventory.historyBody)
	if err != nil || !bytes.Equal(latestBody, inventory.resultsBody) {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.resultsPath, "current KSI results do not match the latest protected history snapshot")
		return
	}
	producerIdentity := constants.KSIEvaluatorID
	for _, resource := range inventory.resources {
		if resource.GetProducerIdentity() != producerIdentity {
			v.fail(constants.ErrEvidenceProducerUnverified, inventory.historyPath, "KSI history resource producer identity is unsupported")
			return
		}
	}
	binding := evidence.KSIHistoryImportBinding{
		Reference:            evidence.ContentReferenceForBody(constants.KSIHistoryReferencePrefix, inventory.historyBody),
		Path:                 inventory.historyPath,
		ScopeID:              resultSet.Binding.ScopeID,
		RunID:                resultSet.Binding.RunID,
		Class:                resultSet.Class,
		ProducerIdentity:     producerIdentity,
		AssertionAssessments: resultSet.Binding.AssertionAssessments,
	}
	reader := &bundledSourceArtifactReader{bodies: v.bodies}
	nodes, err := evidence.NewKSIHistoryImporter(reader, binding).Import(ctx)
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.historyPath, err.Error())
		return
	}
	if len(nodes) != len(inventory.resources) {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.historyPath, "replayed KSI history does not match the analysis evidence inventory")
		return
	}
	if err := v.retainReplayedNodes(nodes); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, inventory.historyPath, err.Error())
	}
}

func lastKSIHistoryResultBody(body []byte) ([]byte, error) {
	var latest []byte
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		if len(line) != 0 {
			latest = line
		}
	}
	if len(latest) == 0 {
		return nil, constants.ErrEvidenceArtifactMalformed
	}
	return latest, nil
}

func firstKSIHistoryResultSet(body []byte) (*compliance.KSIResultSet, error) {
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		resultSet := &compliance.KSIResultSet{}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(resultSet); err != nil {
			return nil, fmt.Errorf("decode KSI history binding: %w", err)
		}
		return resultSet, nil
	}
	return nil, constants.ErrEvidenceArtifactMalformed
}

type bundledSourceArtifactReader struct {
	bodies map[string][]byte
}

func (r *bundledSourceArtifactReader) ReadFile(ctx context.Context, sourcePath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, exists := r.bodies[sourcePath]
	if !exists {
		return nil, constants.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func (r *bundledSourceArtifactReader) ReadDir(ctx context.Context, _ string) ([]os.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, constants.ErrUnexpectedEvidenceArtifact
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

func (v *bundleVerifier) replayCampaignSourceVerification(ctx context.Context, admissionID string, inventory *evalSourceInventory) {
	bundleRoot := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, admissionID)
	for _, artifact := range inventory.campaignInventory.GetArtifacts() {
		if !evidence.ValidRelativePath(artifact.GetRuntimePath()) {
			v.fail(constants.ErrPathValidation, artifact.GetRuntimePath(), "campaign runtime path is invalid")
			return
		}
		body, exists := v.bodies[path.Join(bundleRoot, constants.ComplianceBundleSourceRuntimeDirname, artifact.GetRuntimePath())]
		if !exists {
			v.fail(constants.ErrBundleArtifactMissing, artifact.GetRuntimePath(), "campaign runtime artifact is missing")
			return
		}
		digest := sha256.Sum256(body)
		if hex.EncodeToString(digest[:]) != artifact.GetSha256() {
			v.fail(constants.ErrChecksumMismatch, artifact.GetRuntimePath(), "campaign runtime artifact digest does not match inventory")
			return
		}
	}
	assessmentAsOf, err := v.protectedAssessmentTime()
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, admissionID, err.Error())
		return
	}
	reader := &bundledRuntimeArtifactReader{bodies: v.bodies, runID: admissionID, sourceDir: constants.ComplianceBundleSourceEvalsDirname, runtimeRoot: constants.PathCurrentDir}
	run, err := evaluation.NewStore(reader).LoadRun(ctx, inventory.admission.GetRunId())
	if err != nil {
		v.fail(constants.ErrEvalRunVerificationFailed, admissionID, fmt.Sprintf("load protected campaign run: %v", err))
		return
	}
	if run.GetCampaignBinding().GetCampaignId() != inventory.campaignInventory.GetCampaignId() {
		v.fail(constants.ErrEvidenceScopeMismatch, admissionID, "campaign source inventory campaign ID does not match the protected run")
		return
	}
	policy := evaluation.CampaignVerificationPolicy{
		VerifierReleaseVersion: inventory.campaignReport.GetVerifierReleaseVersion(),
		ProviderObservation:    campaignProviderPolicy(inventory.campaignInventory.GetProviderObservationPolicy()),
		ModelProvenance:        campaignProvenancePolicy(inventory.campaignInventory.GetModelProvenancePolicy()),
		AssessmentTime:         func() time.Time { return assessmentAsOf },
	}
	nodes, err := evaluation.NewCampaignImporter(reader, inventory.admission.GetRunId(), policy).Import(ctx)
	if err != nil {
		v.fail(constants.ErrEvalRunVerificationFailed, admissionID, err.Error())
		return
	}
	if len(nodes) != 1 || !bytes.Equal(nodes[0].CanonicalBytes, v.bodies[path.Join(bundleRoot, constants.ComplianceBundleSourceVerificationFilename)]) {
		v.fail(constants.ErrEvalRunVerificationFailed, admissionID, "campaign verification does not reproduce from protected source bytes")
		return
	}
	if err := v.retainReplayedNodes(nodes); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, admissionID, err.Error())
	}
}

func validateCampaignSourceInventory(manifest *evalv1.CampaignComplianceSourceInventory, admission *compliancev1.AssessmentSourceAdmission) error {
	if manifest == nil || admission == nil || manifest.GetSchemaVersion() != constants.CampaignSourceInventoryVersion || manifest.GetAdmissionId() != admission.GetAdmissionId() || manifest.GetRunId() != admission.GetRunId() || !recognizedCampaignWitnessPolicy(manifest.GetProviderObservationPolicy()) || !recognizedCampaignWitnessPolicy(manifest.GetModelProvenancePolicy()) || !campaignWitnessPoliciesMatchAdmission(manifest, admission) {
		return fmt.Errorf("campaign source inventory is invalid or does not match the protected source admission")
	}
	seenPaths := make(map[string]struct{}, len(manifest.GetArtifacts()))
	for _, artifact := range manifest.GetArtifacts() {
		if artifact == nil || !evidence.ValidRelativePath(artifact.GetRuntimePath()) || artifact.GetMediaType() != constants.MediaTypeJSON || len(artifact.GetSha256()) != sha256.Size*2 || strings.ToLower(artifact.GetSha256()) != artifact.GetSha256() {
			return fmt.Errorf("campaign source inventory contains an incomplete artifact")
		}
		if _, err := hex.DecodeString(artifact.GetSha256()); err != nil {
			return fmt.Errorf("campaign source inventory contains an invalid artifact digest")
		}
		if _, exists := seenPaths[artifact.GetRuntimePath()]; exists {
			return fmt.Errorf("campaign source inventory contains a duplicate runtime path %s", artifact.GetRuntimePath())
		}
		seenPaths[artifact.GetRuntimePath()] = struct{}{}
	}
	return nil
}

func campaignRuntimePathsMatchInventory(inventory *evalSourceInventory) bool {
	if inventory == nil || inventory.campaignInventory == nil || len(inventory.runtimeArtifactPaths) != len(inventory.campaignInventory.GetArtifacts()) {
		return false
	}
	for _, artifact := range inventory.campaignInventory.GetArtifacts() {
		if _, exists := inventory.runtimeArtifactPaths[artifact.GetRuntimePath()]; !exists {
			return false
		}
	}
	return true
}

func recognizedCampaignWitnessPolicy(policy evalv1.EvaluationWitnessPolicy) bool {
	return policy == evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM || policy == evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT
}

func campaignWitnessPolicyFromAdmission(policy compliancev1.AssessmentWitnessPolicy) evalv1.EvaluationWitnessPolicy {
	if policy == compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_STRICT {
		return evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT
	}
	if policy == compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_INTERIM {
		return evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM
	}
	return evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_UNSPECIFIED
}

func campaignWitnessPoliciesMatchAdmission(manifest *evalv1.CampaignComplianceSourceInventory, admission *compliancev1.AssessmentSourceAdmission) bool {
	return manifest != nil && admission != nil && manifest.GetProviderObservationPolicy() == campaignWitnessPolicyFromAdmission(admission.GetProviderObservationPolicy()) && manifest.GetModelProvenancePolicy() == campaignWitnessPolicyFromAdmission(admission.GetModelProvenancePolicy())
}

func campaignProviderPolicy(policy evalv1.EvaluationWitnessPolicy) evaluation.ProviderObservationPolicy {
	if policy == evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT {
		return evaluation.ProviderObservationPolicyStrict
	}
	return evaluation.ProviderObservationPolicyInterim
}

func campaignProvenancePolicy(policy evalv1.EvaluationWitnessPolicy) evaluation.ModelProvenancePolicy {
	if policy == evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT {
		return evaluation.ModelProvenancePolicyStrict
	}
	return evaluation.ModelProvenancePolicyInterim
}

func (v *bundleVerifier) replayEvalSourceVerification(ctx context.Context, runID string, expected *compliancev1.ComplianceVerificationReport) {
	runtimeRoot := filepath.Join(constants.DataDirname, constants.EvaluationDirname, constants.EvaluationRunsDirname, runID)
	reader := &bundledRuntimeArtifactReader{bodies: v.bodies, runID: runID, sourceDir: constants.ComplianceBundleSourceEvalsDirname, runtimeRoot: runtimeRoot}
	assessmentAsOf, err := v.protectedAssessmentTime()
	bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, runID, constants.ComplianceBundleSourceVerificationFilename)
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, err.Error())
		return
	}
	replayed, err := evaluation.NewVerifier(reader, evaluation.NewRegistry(), func() time.Time { return assessmentAsOf }).Verify(ctx, runID)
	if err != nil {
		v.fail(constants.ErrEvalRunVerificationFailed, bundlePath, err.Error())
		return
	}
	matches, matchErr := evidence.CanonicalProtosEqual(expected, replayed)
	if matchErr != nil || !replayed.GetValid() || !matches {
		v.fail(constants.ErrEvalRunVerificationFailed, bundlePath, "replayed eval verification does not match the protected source verification report")
		return
	}
	nodes, err := evaluation.NewEvidenceImporter(reader, runID, func() time.Time { return assessmentAsOf }).Import(ctx)
	if err != nil {
		v.fail(constants.ErrEvalRunVerificationFailed, bundlePath, fmt.Sprintf("replay eval evidence importer: %v", err))
		return
	}
	if err := v.retainReplayedNodes(nodes); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, err.Error())
	}
}

func (v *bundleVerifier) replayDemoSourceVerification(ctx context.Context, runID string, expected *compliancev1.ComplianceVerificationReport) {
	runtimeRoot := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, runID)
	reader := &bundledRuntimeArtifactReader{bodies: v.bodies, runID: runID, sourceDir: constants.ComplianceBundleSourceDemosDirname, runtimeRoot: runtimeRoot}
	assessmentAsOf, err := v.protectedAssessmentTime()
	bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceVerificationFilename)
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, err.Error())
		return
	}
	replayed, err := evidence.VerifyDemoRun(ctx, reader, runID, &bundledDemoProvenanceSource{bodies: v.bodies, runID: runID}, assessmentAsOf)
	if err != nil {
		v.fail(constants.ErrDemoRunVerificationFailed, bundlePath, err.Error())
		return
	}
	matches, matchErr := evidence.CanonicalProtosEqual(expected, replayed)
	if matchErr != nil || !replayed.GetValid() || !matches {
		v.fail(constants.ErrDemoRunVerificationFailed, bundlePath, "replayed demo verification does not match the protected source verification report")
		return
	}
	nodes, err := evidence.NewDemoRunImporterAt(reader, runID, &bundledDemoProvenanceSource{bodies: v.bodies, runID: runID}, func() time.Time { return assessmentAsOf }).Import(ctx)
	if err != nil {
		v.fail(constants.ErrDemoRunVerificationFailed, bundlePath, fmt.Sprintf("replay demo evidence importer: %v", err))
		return
	}
	if err := v.retainReplayedNodes(nodes); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, bundlePath, err.Error())
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

func (r *bundledRuntimeArtifactReader) FileExists(ctx context.Context, sourcePath string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	bundlePath, ok := r.bundlePath(sourcePath)
	if !ok {
		return false, constants.ErrUnexpectedEvidenceArtifact
	}
	_, exists := r.bodies[bundlePath]
	return exists, nil
}

func (r *bundledRuntimeArtifactReader) MkdirAll(context.Context, string, os.FileMode) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) CreateRuntimeTree(context.Context) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) Stat(context.Context, string) (os.FileInfo, error) {
	return nil, constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) Lstat(context.Context, string) (os.FileInfo, error) {
	return nil, constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) WriteFile(context.Context, string, []byte, os.FileMode) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) OpenForAppend(context.Context, string, os.FileMode) (*os.File, error) {
	return nil, constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) OpenForRead(context.Context, string) (*os.File, error) {
	return nil, constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) Remove(context.Context, string) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) RemoveAll(context.Context, string) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) Rename(context.Context, string, string) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) EnforceDirPermissions(context.Context, string, os.FileMode) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) EnforceFilePermissions(context.Context, string, os.FileMode) error {
	return constants.ErrReadOnlyEvidenceSource
}
func (r *bundledRuntimeArtifactReader) Resolve(sourcePath string) string      { return sourcePath }
func (r *bundledRuntimeArtifactReader) Rel(sourcePath string) (string, error) { return sourcePath, nil }
func (r *bundledRuntimeArtifactReader) RelFromAbs(sourcePath string) (string, error) {
	return sourcePath, nil
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
	profile, err := ParseBundleProfile(v.request.Bundle.GetManifest().GetBundleProfile())
	if err != nil {
		v.fail(constants.ErrRendererMismatch, constants.ComplianceBundleManifestPath, err.Error())
		return
	}
	for _, entry := range v.request.Bundle.GetRenderedFormats() {
		if entry == nil {
			continue
		}
		format, err := ParseFormat(entry.GetFormat())
		if err != nil {
			v.fail(constants.ErrRendererMismatch, entry.GetBundlePath(), err.Error())
			continue
		}
		rendered, err := renderBundleFormat(v.request.Bundle.GetAnalysis(), format, profile)
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
