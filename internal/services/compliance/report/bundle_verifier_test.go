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
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type bundleArtifactReaderStub struct {
	bodies  map[string][]byte
	listErr error
	readErr error
}

type assessedEvidenceSignerStub struct {
	keys map[string]ed25519.PublicKey
}

func (s *assessedEvidenceSignerStub) GetTrustedSignerPublicKey(_ context.Context, keyID string) (ed25519.PublicKey, error) {
	key, exists := s.keys[keyID]
	if !exists {
		return nil, constants.ErrTrustedSignerKeyNotFound
	}
	return key, nil
}

type authorizedPlaintextReaderStub struct {
	plaintext []byte
	err       error
	calls     int
}

func (r *authorizedPlaintextReaderStub) ReadPlaintext(_ context.Context, _ *compliancev1.BundleArtifact, _ []byte) ([]byte, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return append([]byte(nil), r.plaintext...), nil
}

func (r *bundleArtifactReaderStub) ReadFile(_ context.Context, bundlePath string) ([]byte, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	body, ok := r.bodies[bundlePath]
	if !ok {
		return nil, constants.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func (r *bundleArtifactReaderStub) ListFiles(context.Context) ([]string, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	paths := make([]string, 0, len(r.bodies))
	for bundlePath := range r.bodies {
		paths = append(paths, bundlePath)
	}
	sort.Strings(paths)
	return paths, nil
}

func demoReplaySourceFixture(t *testing.T, runID string, generatedAt time.Time) []SourceArtifact {
	t.Helper()
	assertions, frameworks, _, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	scenarios, err := catalog.LoadDemoScenarioCatalog(assertions, frameworks)
	require.NoError(t, err)
	definitions := make([]*compliancev1.DemoScenarioDefinition, 0)
	definitionRefs := make([]*compliancev1.VersionedReference, 0)
	frameworkRefs := make(map[string]*compliancev1.FrameworkControlReference)
	for _, definition := range scenarios.GetDefinitions() {
		if !strings.HasPrefix(definition.GetScenarioId(), constants.DemosOrgFedRAMP+"-") {
			continue
		}
		definitions = append(definitions, definition)
		definitionRefs = append(definitionRefs, &compliancev1.VersionedReference{Id: definition.GetScenarioId(), Version: definition.GetScenarioVersion()})
		for _, reference := range definition.GetFrameworkControlRefs() {
			key := reference.GetFrameworkRef().GetId() + ":" + reference.GetFrameworkRef().GetVersion() + ":" + reference.GetControlId()
			frameworkRefs[key] = reference
		}
	}
	require.NotEmpty(t, definitions)
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].GetScenarioId() < definitions[j].GetScenarioId() })
	sort.Slice(definitionRefs, func(i, j int) bool { return definitionRefs[i].GetId() < definitionRefs[j].GetId() })
	frameworkKeys := make([]string, 0, len(frameworkRefs))
	for key := range frameworkRefs {
		frameworkKeys = append(frameworkKeys, key)
	}
	sort.Strings(frameworkKeys)
	manifestFrameworkRefs := make([]*compliancev1.FrameworkControlReference, 0, len(frameworkKeys))
	for _, key := range frameworkKeys {
		manifestFrameworkRefs = append(manifestFrameworkRefs, frameworkRefs[key])
	}
	provenanceBody := []byte("services: {}")
	provenanceDigest := sha256.Sum256(provenanceBody)
	manifest := &compliancev1.DemoManifest{
		DemoId:                 constants.DemosOrgFedRAMP,
		DemoVersion:            constants.DemoVersion,
		RunId:                  runID,
		ScopeId:                constants.DemoScopeFedRAMP,
		GeneratedAt:            timestamppb.New(generatedAt),
		ScenarioDefinitionRefs: definitionRefs,
		ProvenanceHashes:       []*compliancev1.NamedDigest{{Name: constants.DemosComposeFile, Sha256: hex.EncodeToString(provenanceDigest[:])}},
		RequiredEnvironment:    []string{"docker", "g8e-binary"},
		FrameworkControlRefs:   manifestFrameworkRefs,
		SupportedLanes:         []string{"automated", "manual-notary"},
	}
	definition := definitions[0]
	result := &compliancev1.DemoScenarioResult{
		ResultId:             runID + ":" + definition.GetScenarioId(),
		ScenarioRef:          &compliancev1.VersionedReference{Id: definition.GetScenarioId(), Version: definition.GetScenarioVersion()},
		DemoId:               constants.DemosOrgFedRAMP,
		ScopeId:              constants.DemoScopeFedRAMP,
		RunId:                runID,
		StartedAt:            timestamppb.New(generatedAt.Add(time.Second)),
		CompletedAt:          timestamppb.New(generatedAt.Add(2 * time.Second)),
		Status:               "failed",
		Failure:              "expected fixture failure",
		VerificationStatus:   "unverifiable",
		DisplayNumber:        definition.GetDisplayNumber(),
		Title:                definition.GetTitle(),
		AssertionRefs:        definition.GetAssertionRefs(),
		FrameworkControlRefs: definition.GetFrameworkControlRefs(),
		StepResults: []*compliancev1.DemoStepResult{{
			StepId: "step-1", Operation: "fixture", StartedAt: timestamppb.New(generatedAt.Add(time.Second)), CompletedAt: timestamppb.New(generatedAt.Add(2 * time.Second)), Status: "failed", Failure: "expected fixture failure", Required: true,
		}},
	}
	manifestBody, err := compliancev1.MarshalCanonical(manifest)
	require.NoError(t, err)
	resultBody, err := compliancev1.MarshalCanonical(result)
	require.NoError(t, err)
	definitionBodies := make([]string, 0, len(definitions))
	for _, value := range definitions {
		body, err := compliancev1.MarshalCanonical(value)
		require.NoError(t, err)
		definitionBodies = append(definitionBodies, string(body))
	}
	base := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID)
	return []SourceArtifact{
		{BundlePath: path.Join(base, constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunManifestFilename), Body: manifestBody, MediaType: constants.MediaTypeJSON},
		{BundlePath: path.Join(base, constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunResultsFilename), Body: resultBody, MediaType: constants.MediaTypeJSON},
		{BundlePath: path.Join(base, constants.ComplianceBundleSourceProvenanceDirname, constants.ComplianceBundleSourceArtifactsDirname, constants.DemosComposeFile), Body: provenanceBody, MediaType: constants.MediaTypeText},
		{BundlePath: path.Join(base, constants.ComplianceBundleSourceProvenanceDirname, constants.DemoRunDefinitionsFilename), Body: []byte(strings.Join(definitionBodies, "\n")), MediaType: constants.MediaTypeJSON},
	}
}

func signedBundleVerificationFixture(t *testing.T) (*compliancev1.ComplianceReportBundle, *bundleArtifactReaderStub, *compliancev1.ComplianceReportTrustPolicy, time.Time) {
	t.Helper()
	return signedBundleVerificationFixtureWithRequest(t, nil)
}

func signedBundleVerificationFixtureWithRequest(t *testing.T, mutate func(*BundleAssemblyRequest)) (*compliancev1.ComplianceReportBundle, *bundleArtifactReaderStub, *compliancev1.ComplianceReportTrustPolicy, time.Time) {
	t.Helper()
	request, _ := bundleAssemblyFixture(t)
	request.Analysis = rendererTestAnalysis()
	request.Analysis.EvidenceResources = []*compliancev1.ComplianceEvidenceReference{{
		ArtifactId:         string(evidence.ArtifactTypeDemoManifest) + ":sha256:" + strings.Repeat("a", 64),
		ArtifactType:       string(evidence.ArtifactTypeDemoManifest),
		Sha256:             strings.Repeat("a", 64),
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.compliance.v1.DemoManifest",
		ProducerIdentity:   "demo-run-1",
		ProducedAt:         timestamppb.New(request.GeneratedAt),
		ScopeId:            request.ScopeRef,
		RunId:              "demo-run-1",
		VerificationStatus: string(evidence.VerificationStatusVerified),
		VerifierId:         constants.DemoRunVerifierID,
		VerifierVersion:    constants.DemoRunVerifierVersion,
		VerifiedAt:         timestamppb.New(request.GeneratedAt),
		BundlePath:         path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.DemoRunManifestFilename),
	}}
	request.Profiles[0].AnalysisRef = request.Analysis.GetAnalysisId()
	renderedFormats, err := renderAllFormats(request.Analysis)
	require.NoError(t, err)
	request.RenderedFormats = renderedFormats
	controlAssessmentRef := path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleControlAssessmentsFilename)
	request.AssessmentRefs = append(request.AssessmentRefs, controlAssessmentRef)
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	request.SourceArtifacts, err = canonicalReportSourceArtifacts(GenerationRequest{Assertions: assertions, Frameworks: frameworks, Crosswalks: crosswalks}, &GenerationResult{Analysis: request.Analysis})
	require.NoError(t, err)
	sourceReportBody, err := compliancev1.MarshalCanonical(&compliancev1.ComplianceVerificationReport{
		ReportId:        "demo-run-1",
		Valid:           true,
		VerifiedAt:      timestamppb.New(request.GeneratedAt),
		VerifierId:      constants.DemoRunVerifierID,
		VerifierVersion: constants.DemoRunVerifierVersion,
		Checks: []*compliancev1.VerificationCheckResult{evidence.NewVerificationCheckResult(
			constants.DemoRunVerificationCheck,
			constants.DemoRunVerifierID,
			constants.DemoRunVerifierVersion,
			[]string{"demo-run-1"},
			nil,
		)},
	})
	require.NoError(t, err)
	demoSourceBase := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1")
	request.SourceArtifacts = append(request.SourceArtifacts, demoReplaySourceFixture(t, "demo-run-1", request.GeneratedAt)...)
	request.SourceArtifacts = append(request.SourceArtifacts, SourceArtifact{BundlePath: path.Join(demoSourceBase, constants.ComplianceBundleSourceVerificationFilename), Body: sourceReportBody, MediaType: constants.MediaTypeJSON})
	if mutate != nil {
		mutate(&request)
	}
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))
	bodies := make(map[string][]byte, len(result.ArtifactBodies))
	for _, artifact := range result.ArtifactBodies {
		bodies[artifact.BundlePath] = append([]byte(nil), artifact.Body...)
	}
	reader := &bundleArtifactReaderStub{bodies: bodies}
	publicKey := identity.privateKey.Public().(ed25519.PublicKey)
	policy := &compliancev1.ComplianceReportTrustPolicy{
		PolicyId:      "policy-1",
		PolicyVersion: "1.0.0",
		TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{
			Metadata:         proto.Clone(identity.metadata).(*compliancev1.ComplianceReportSigningKeyMetadata),
			PublicKey:        hex.EncodeToString(publicKey),
			AssessmentId:     "assessment-1",
			AssessorIdentity: "assessor-1",
			AssessedAt:       timestamppb.New(request.GeneratedAt),
			AllowedScopeRefs: []string{request.ScopeRef},
		}},
	}
	return result.Bundle, reader, policy, request.GeneratedAt.Add(time.Hour)
}

type ksiSourcePaths struct {
	history string
	results string
}

func addKSIHistorySourceFixture(t *testing.T, request *BundleAssemblyRequest) ksiSourcePaths {
	t.Helper()
	runID := "ksi-run-1"
	binding := compliance.EvaluationBinding{
		ScopeID:            request.ScopeRef,
		RunID:              runID,
		WindowStartUnixMs:  request.GeneratedAt.Add(-time.Hour).UnixMilli(),
		WindowEndUnixMs:    request.GeneratedAt.UnixMilli(),
		EvaluatorID:        constants.KSIEvaluatorID,
		EvaluatorVersion:   constants.KSIEvaluatorVersion,
		MethodDefinitionID: constants.KSIMethodDefinitionVersion,
		AssertionAssessments: compliance.AssertionAssessmentScope{
			AssessmentIDs: []string{"assessment-1"},
		},
	}
	resultSet := compliance.KSIResultSet{
		Class:         compliance.ClassC,
		EvaluatedAtMs: request.GeneratedAt.UnixMilli(),
		Binding:       binding,
		Results: []compliance.KSIResult{{
			ID:                  "KSI-CMT-01",
			Status:              compliance.KSIStatusSatisfied,
			Outcome:             compliance.KSIOutcomeSatisfied,
			LastValidatedUnixMs: request.GeneratedAt.Add(-time.Second).UnixMilli(),
			MethodCount:         2,
			Binding:             binding,
		}},
	}
	body, err := json.Marshal(resultSet)
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	paths := ksiSourcePaths{
		history: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, request.ScopeRef, runID, constants.ComplianceBundleKSIHistoryFilename),
		results: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, request.ScopeRef, runID, constants.ComplianceBundleKSIResultsFilename),
	}
	request.Analysis.EvidenceResources = append(request.Analysis.EvidenceResources, &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         constants.KSIResultReferencePrefix + ":sha256:" + digestHex,
		ArtifactType:       string(evidence.ArtifactTypeKSIResult),
		Sha256:             digestHex,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.compliance.KSIResultSet@" + constants.KSIHistorySchemaVersion,
		ProducerIdentity:   constants.KSIEvaluatorID,
		ProducedAt:         timestamppb.New(request.GeneratedAt),
		ScopeId:            request.ScopeRef,
		RunId:              runID,
		VerificationStatus: string(evidence.VerificationStatusUnverified),
		BundlePath:         paths.history,
	})
	request.SourceArtifacts = append(request.SourceArtifacts,
		SourceArtifact{BundlePath: paths.history, Body: body, MediaType: constants.MediaTypeJSON},
		SourceArtifact{BundlePath: paths.results, Body: body, MediaType: constants.MediaTypeJSON},
	)
	evidenceIndexPath := path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)
	evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
	require.NoError(t, err)
	replaceSourceArtifactBody(t, request.SourceArtifacts, evidenceIndexPath, evidenceIndex)
	request.RenderedFormats, err = renderAllFormats(request.Analysis)
	require.NoError(t, err)
	return paths
}

type commitmentSourceFixture struct {
	bundlePath  string
	privateKey  ed25519.PrivateKey
	attestation *operatorv1.CommitmentAttestation
	trust       *assessedEvidenceSignerStub
}

func addCommitmentSourceFixture(t *testing.T, request *BundleAssemblyRequest) commitmentSourceFixture {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := hex.EncodeToString(publicKey)
	attestation := &operatorv1.CommitmentAttestation{
		TransactionId:               "transaction-1",
		TransactionHash:             strings.Repeat("1", 64),
		PriorCommitmentHash:         strings.Repeat("2", 64),
		StateRootAtCommit:           strings.Repeat("3", 64),
		L2SignatureDigest:           strings.Repeat("4", 64),
		WardenIntentSignatureDigest: strings.Repeat("5", 64),
		HumanSignatureDigest:        strings.Repeat("6", 64),
		ActionType:                  "FILE_EDIT",
		TargetResource:              constants.DemosTargetDataDir,
		CommittedAtUnixMs:           request.GeneratedAt.Add(-time.Second).UnixMilli(),
		AuditorKeyId:                keyID,
	}
	signCommitmentFixture(t, attestation, privateKey)
	body, err := compliancev1.MarshalCanonical(attestation)
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, request.ScopeRef, "commitment-run-1", constants.ComplianceBundleCommitmentsFilename)
	request.Analysis.EvidenceResources = append(request.Analysis.EvidenceResources, &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         constants.CommitmentReferencePrefix + ":sha256:" + digestHex,
		ArtifactType:       string(evidence.ArtifactTypeCommitment),
		Sha256:             digestHex,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.operator.v1.CommitmentAttestation",
		ProducerIdentity:   keyID,
		ProducedAt:         timestamppb.New(time.UnixMilli(attestation.GetCommittedAtUnixMs())),
		ScopeId:            request.ScopeRef,
		RunId:              "commitment-run-1",
		AttemptId:          "attempt-1",
		ScenarioId:         "scenario-1",
		TransactionId:      attestation.GetTransactionId(),
		VerificationStatus: string(evidence.VerificationStatusVerified),
		VerifierId:         constants.CommitmentEvidenceVerifierID,
		VerifierVersion:    constants.CommitmentEvidenceVerifierVersion,
		VerifiedAt:         timestamppb.New(request.GeneratedAt),
		BundlePath:         bundlePath,
	})
	sort.Slice(request.Analysis.EvidenceResources, func(i, j int) bool {
		return request.Analysis.EvidenceResources[i].GetArtifactId() < request.Analysis.EvidenceResources[j].GetArtifactId()
	})
	request.SourceArtifacts = append(request.SourceArtifacts, SourceArtifact{BundlePath: bundlePath, Body: body, MediaType: constants.MediaTypeJSON})
	evidenceIndexPath := path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)
	evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
	require.NoError(t, err)
	replaceSourceArtifactBody(t, request.SourceArtifacts, evidenceIndexPath, evidenceIndex)
	request.RenderedFormats, err = renderAllFormats(request.Analysis)
	require.NoError(t, err)
	return commitmentSourceFixture{bundlePath: bundlePath, privateKey: privateKey, attestation: attestation, trust: &assessedEvidenceSignerStub{keys: map[string]ed25519.PublicKey{keyID: publicKey}}}
}

func signCommitmentFixture(t *testing.T, attestation *operatorv1.CommitmentAttestation, privateKey ed25519.PrivateKey) {
	t.Helper()
	attestation.Hash = ""
	attestation.Signature = ""
	payload, err := governance.CanonicalizeCommitmentAttestation(attestation)
	require.NoError(t, err)
	digest := sha256.Sum256(payload)
	attestation.Hash = hex.EncodeToString(digest[:])
	attestation.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
}

type attestationSourceRecord struct {
	SchemaVersion    string   `json:"schema_version"`
	AttestationID    string   `json:"attestation_id"`
	AttesterType     string   `json:"attester_type"`
	AttesterIdentity string   `json:"attester_identity"`
	SignerKeyID      string   `json:"signer_key_id"`
	IssuedAtUTC      string   `json:"issued_at_utc"`
	ValidFromUTC     string   `json:"valid_from_utc"`
	ValidUntilUTC    string   `json:"valid_until_utc"`
	ScopeID          string   `json:"scope_id"`
	RunID            string   `json:"run_id"`
	AssertionIDs     []string `json:"assertion_ids"`
	EvidenceRefs     []string `json:"evidence_refs,omitempty"`
	Statement        string   `json:"statement"`
	Revoked          bool     `json:"revoked"`
	RevokedAtUTC     string   `json:"revoked_at_utc,omitempty"`
	Signature        string   `json:"signature,omitempty"`
}

type attestationSourceFixture struct {
	bundlePath string
	privateKey ed25519.PrivateKey
	record     attestationSourceRecord
	trust      *assessedEvidenceSignerStub
}

func addAttestationSourceFixture(t *testing.T, request *BundleAssemblyRequest) attestationSourceFixture {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := hex.EncodeToString(publicKey)
	record := attestationSourceRecord{
		SchemaVersion:    constants.AttestationSchemaVersion,
		AttestationID:    "customer-attestation-1",
		AttesterType:     "customer",
		AttesterIdentity: "customer-1",
		SignerKeyID:      keyID,
		IssuedAtUTC:      request.GeneratedAt.Add(-time.Hour).Format(time.RFC3339),
		ValidFromUTC:     request.GeneratedAt.Add(-time.Hour).Format(time.RFC3339),
		ValidUntilUTC:    request.GeneratedAt.Add(time.Hour).Format(time.RFC3339),
		ScopeID:          request.ScopeRef,
		RunID:            "attestation-run-1",
		AssertionIDs:     []string{"assertion-1"},
		Statement:        "Customer-operated control is in effect.",
	}
	signAttestationSourceFixture(t, &record, privateKey)
	body := marshalAttestationSourceFixture(t, record)
	line := bytes.TrimSuffix(body, []byte{'\n'})
	digest := sha256.Sum256(line)
	digestHex := hex.EncodeToString(digest[:])
	bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, request.ScopeRef, record.RunID, constants.ComplianceBundleAttestationsFilename)
	request.Analysis.EvidenceResources = append(request.Analysis.EvidenceResources, &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         constants.CustomerAttestationReferencePrefix + ":sha256:" + digestHex,
		ArtifactType:       string(evidence.ArtifactTypeCustomerAttestation),
		Sha256:             digestHex,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.evidence.CustomerAttestation@" + constants.AttestationSchemaVersion,
		ProducerIdentity:   record.AttesterIdentity,
		ProducedAt:         timestamppb.New(request.GeneratedAt.Add(-time.Hour)),
		ScopeId:            request.ScopeRef,
		RunId:              record.RunID,
		VerificationStatus: string(evidence.VerificationStatusVerified),
		VerifierId:         constants.AttestationEvidenceVerifierID,
		VerifierVersion:    constants.AttestationEvidenceVerifierVersion,
		VerifiedAt:         timestamppb.New(request.GeneratedAt),
		BundlePath:         bundlePath,
	})
	sort.Slice(request.Analysis.EvidenceResources, func(i, j int) bool {
		return request.Analysis.EvidenceResources[i].GetArtifactId() < request.Analysis.EvidenceResources[j].GetArtifactId()
	})
	request.SourceArtifacts = append(request.SourceArtifacts, SourceArtifact{BundlePath: bundlePath, Body: body, MediaType: constants.MediaTypeJSON})
	evidenceIndexPath := path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)
	evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
	require.NoError(t, err)
	replaceSourceArtifactBody(t, request.SourceArtifacts, evidenceIndexPath, evidenceIndex)
	request.RenderedFormats, err = renderAllFormats(request.Analysis)
	require.NoError(t, err)
	return attestationSourceFixture{bundlePath: bundlePath, privateKey: privateKey, record: record, trust: &assessedEvidenceSignerStub{keys: map[string]ed25519.PublicKey{keyID: publicKey}}}
}

func signAttestationSourceFixture(t *testing.T, record *attestationSourceRecord, privateKey ed25519.PrivateKey) {
	t.Helper()
	record.Signature = ""
	payload, err := json.Marshal(record)
	require.NoError(t, err)
	record.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
}

func marshalAttestationSourceFixture(t *testing.T, record attestationSourceRecord) []byte {
	t.Helper()
	body, err := json.Marshal(record)
	require.NoError(t, err)
	return append(body, '\n')
}

type auditSourceFixture struct {
	bundlePath string
	event      *operatorv1.AuditEvent
}

func addAuditSourceFixture(t *testing.T, request *BundleAssemblyRequest) auditSourceFixture {
	t.Helper()
	event := &operatorv1.AuditEvent{
		Id:                  41,
		OperatorSessionId:   "operator-session-1",
		Timestamp:           timesvc.FormatTimestamp(request.GeneratedAt.Add(-time.Second)),
		Type:                string(constants.EventAppTaskCompleted),
		CommandRaw:          "printf test",
		CommandExitCode:     0,
		ExecutionDurationMs: 12,
		StoredLocally:       true,
	}
	body, err := compliancev1.MarshalCanonical(event)
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	runID := "audit-run-1"
	bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, request.ScopeRef, runID, constants.AuditRecordsDirname, digestHex+constants.FileExtJSON)
	request.Analysis.EvidenceResources = append(request.Analysis.EvidenceResources, &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         constants.AuditRecordReferencePrefix + ":sha256:" + digestHex,
		ArtifactType:       string(evidence.ArtifactTypeAuditRecord),
		Sha256:             digestHex,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.operator.v1.AuditEvent",
		ProducerIdentity:   event.GetOperatorSessionId(),
		ProducedAt:         timestamppb.New(request.GeneratedAt.Add(-time.Second)),
		ScopeId:            request.ScopeRef,
		RunId:              runID,
		AttemptId:          "attempt-1",
		ScenarioId:         "scenario-1",
		VerificationStatus: string(evidence.VerificationStatusUnverified),
		BundlePath:         bundlePath,
	})
	sort.Slice(request.Analysis.EvidenceResources, func(i, j int) bool {
		return request.Analysis.EvidenceResources[i].GetArtifactId() < request.Analysis.EvidenceResources[j].GetArtifactId()
	})
	request.SourceArtifacts = append(request.SourceArtifacts, SourceArtifact{BundlePath: bundlePath, Body: body, MediaType: constants.MediaTypeJSON})
	evidenceIndexPath := path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)
	evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
	require.NoError(t, err)
	replaceSourceArtifactBody(t, request.SourceArtifacts, evidenceIndexPath, evidenceIndex)
	request.RenderedFormats, err = renderAllFormats(request.Analysis)
	require.NoError(t, err)
	return auditSourceFixture{bundlePath: bundlePath, event: event}
}

func TestVerifyComplianceReportBundle_AcceptsCompleteSignedBundleOffline(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{
		Bundle:      bundle,
		Reader:      reader,
		TrustPolicy: policy,
		VerifiedAt:  verifiedAt,
	})

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.True(t, report.GetValid())
	assert.Empty(t, report.GetFailures())
	assert.Equal(t, bundle.GetManifest().GetReportId(), report.GetReportId())
	assert.Equal(t, constants.ComplianceBundleVerifierID, report.GetVerifierId())
	assert.Equal(t, constants.ComplianceBundleVerifierVersion, report.GetVerifierVersion())
	assert.Equal(t, bundle.GetChecksumRoot(), report.GetReproducedChecksumRoot())
	require.Len(t, report.GetChecks(), 10)
	for _, check := range report.GetChecks() {
		assert.Equal(t, compliancev1.VerificationCheckStatus_VERIFICATION_CHECK_STATUS_PASSED, check.GetStatus())
		assert.NotEmpty(t, check.GetEvidenceRefs())
		assert.Equal(t, report.GetVerifierId(), check.GetVerifierId())
		assert.Equal(t, report.GetVerifierVersion(), check.GetVerifierVersion())
		assert.Empty(t, check.GetFailures())
	}
}

func TestVerifyComplianceReportBundle_RejectsSignedCommitmentThatDiffersFromAnalysis(t *testing.T) {
	var fixture commitmentSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addCommitmentSourceFixture(t, request)
		fixture.attestation.TargetResource += "-substituted"
		signCommitmentFixture(t, fixture.attestation, fixture.privateKey)
		body, err := compliancev1.MarshalCanonical(fixture.attestation)
		require.NoError(t, err)
		replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, body)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid(), "REGRESSION: AFTER FIX")
	assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_AcceptsCompleteCommitmentSource(t *testing.T) {
	var fixture commitmentSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addCommitmentSourceFixture(t, request)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.True(t, report.GetValid())
	assert.Empty(t, report.GetFailures())
}

func TestVerifyComplianceReportBundle_RejectsMissingCommitmentSource(t *testing.T) {
	var fixture commitmentSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addCommitmentSourceFixture(t, request)
		request.SourceArtifacts = removeSourceArtifact(request.SourceArtifacts, fixture.bundlePath)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsCommitmentWithoutAssessedEvidenceTrust(t *testing.T) {
	var fixture commitmentSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addCommitmentSourceFixture(t, request)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrEvidenceTrustNotAssessed, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsCommitmentWithUnassessedSigner(t *testing.T) {
	var fixture commitmentSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addCommitmentSourceFixture(t, request)
		fixture.trust.keys = map[string]ed25519.PublicKey{}
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrEvidenceTrustNotAssessed, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsCommitmentAnalysisBindingMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.ComplianceEvidenceReference)
	}{
		{name: "run differs from source inventory", mutate: func(resource *compliancev1.ComplianceEvidenceReference) { resource.RunId = "other-run" }},
		{name: "transaction differs from attestation", mutate: func(resource *compliancev1.ComplianceEvidenceReference) { resource.TransactionId = "other-transaction" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var fixture commitmentSourceFixture
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
				fixture = addCommitmentSourceFixture(t, request)
				for _, resource := range request.Analysis.GetEvidenceResources() {
					if resource.GetArtifactType() == string(evidence.ArtifactTypeCommitment) {
						test.mutate(resource)
						break
					}
				}
				evidenceIndexPath := path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)
				evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
				require.NoError(t, err)
				replaceSourceArtifactBody(t, request.SourceArtifacts, evidenceIndexPath, evidenceIndex)
				request.RenderedFormats, err = renderAllFormats(request.Analysis)
				require.NoError(t, err)
			})

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
		})
	}
}

func TestVerifyComplianceReportBundle_RejectsSignedAttestationThatDiffersFromAnalysis(t *testing.T) {
	var fixture attestationSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addAttestationSourceFixture(t, request)
		fixture.record.Statement += " Substituted after analysis."
		signAttestationSourceFixture(t, &fixture.record, fixture.privateKey)
		replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, marshalAttestationSourceFixture(t, fixture.record))
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_AcceptsCompleteAttestationSource(t *testing.T) {
	var fixture attestationSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addAttestationSourceFixture(t, request)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.True(t, report.GetValid())
	assert.Empty(t, report.GetFailures())
}

func TestVerifyComplianceReportBundle_RejectsMissingAttestationSource(t *testing.T) {
	var fixture attestationSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addAttestationSourceFixture(t, request)
		request.SourceArtifacts = removeSourceArtifact(request.SourceArtifacts, fixture.bundlePath)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsAttestationWithoutAssessedEvidenceTrust(t *testing.T) {
	var fixture attestationSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addAttestationSourceFixture(t, request)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrEvidenceTrustNotAssessed, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsAttestationWithUnassessedSigner(t *testing.T) {
	var fixture attestationSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addAttestationSourceFixture(t, request)
		fixture.trust.keys = map[string]ed25519.PublicKey{}
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrEvidenceTrustNotAssessed, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsAttestationAnalysisBindingMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.ComplianceEvidenceReference)
	}{
		{name: "run differs from source inventory", mutate: func(resource *compliancev1.ComplianceEvidenceReference) { resource.RunId = "other-run" }},
		{name: "producer differs from attester", mutate: func(resource *compliancev1.ComplianceEvidenceReference) { resource.ProducerIdentity = "other-customer" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var fixture attestationSourceFixture
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
				fixture = addAttestationSourceFixture(t, request)
				for _, resource := range request.Analysis.GetEvidenceResources() {
					if resource.GetArtifactType() == string(evidence.ArtifactTypeCustomerAttestation) {
						test.mutate(resource)
						break
					}
				}
				evidenceIndexPath := path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)
				evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
				require.NoError(t, err)
				replaceSourceArtifactBody(t, request.SourceArtifacts, evidenceIndexPath, evidenceIndex)
				request.RenderedFormats, err = renderAllFormats(request.Analysis)
				require.NoError(t, err)
			})

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
		})
	}
}

func TestVerifyComplianceReportBundle_RejectsOrphanedAttestationSource(t *testing.T) {
	var fixture attestationSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addAttestationSourceFixture(t, request)
		resources := request.Analysis.EvidenceResources[:0]
		for _, resource := range request.Analysis.GetEvidenceResources() {
			if resource.GetArtifactType() != string(evidence.ArtifactTypeCustomerAttestation) {
				resources = append(resources, resource)
			}
		}
		request.Analysis.EvidenceResources = resources
		evidenceIndexPath := path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)
		evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
		require.NoError(t, err)
		replaceSourceArtifactBody(t, request.SourceArtifacts, evidenceIndexPath, evidenceIndex)
		request.RenderedFormats, err = renderAllFormats(request.Analysis)
		require.NoError(t, err)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, EvidenceTrust: fixture.trust, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrUnresolvedReference, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsSignedAuditRecordThatDiffersFromAnalysis(t *testing.T) {
	var fixture auditSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addAuditSourceFixture(t, request)
		fixture.event.CommandRaw = "printf substituted"
		body, err := compliancev1.MarshalCanonical(fixture.event)
		require.NoError(t, err)
		replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, body)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_AcceptsCompleteAuditRecordSource(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		addAuditSourceFixture(t, request)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.True(t, report.GetValid())
	assert.Empty(t, report.GetFailures())
}

func TestVerifyComplianceReportBundle_RejectsMissingAuditRecordSource(t *testing.T) {
	var fixture auditSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addAuditSourceFixture(t, request)
		request.SourceArtifacts = removeSourceArtifact(request.SourceArtifacts, fixture.bundlePath)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsAuditRecordAnalysisBindingMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.ComplianceEvidenceReference)
	}{
		{name: "run differs from source inventory", mutate: func(resource *compliancev1.ComplianceEvidenceReference) { resource.RunId = "other-run" }},
		{name: "producer differs from operator session", mutate: func(resource *compliancev1.ComplianceEvidenceReference) { resource.ProducerIdentity = "other-session" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var fixture auditSourceFixture
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
				fixture = addAuditSourceFixture(t, request)
				for _, resource := range request.Analysis.GetEvidenceResources() {
					if resource.GetArtifactType() == string(evidence.ArtifactTypeAuditRecord) {
						test.mutate(resource)
						break
					}
				}
				evidenceIndexPath := path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)
				evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
				require.NoError(t, err)
				replaceSourceArtifactBody(t, request.SourceArtifacts, evidenceIndexPath, evidenceIndex)
				request.RenderedFormats, err = renderAllFormats(request.Analysis)
				require.NoError(t, err)
			})

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
		})
	}
}

func TestVerifyComplianceReportBundle_RejectsOrphanedAuditRecordSource(t *testing.T) {
	var fixture auditSourceFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addAuditSourceFixture(t, request)
		resources := request.Analysis.EvidenceResources[:0]
		for _, resource := range request.Analysis.GetEvidenceResources() {
			if resource.GetArtifactType() != string(evidence.ArtifactTypeAuditRecord) {
				resources = append(resources, resource)
			}
		}
		request.Analysis.EvidenceResources = resources
		evidenceIndexPath := path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)
		evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
		require.NoError(t, err)
		replaceSourceArtifactBody(t, request.SourceArtifacts, evidenceIndexPath, evidenceIndex)
		request.RenderedFormats, err = renderAllFormats(request.Analysis)
		require.NoError(t, err)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrUnresolvedReference, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_AcceptsCompleteKSIResultAndHistorySources(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		addKSIHistorySourceFixture(t, request)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.True(t, report.GetValid())
	assert.Empty(t, report.GetFailures())
}

func TestVerifyComplianceReportBundle_RejectsSignedKSISourceMutations(t *testing.T) {
	tests := []struct {
		name          string
		resultsSource bool
	}{
		{name: "history snapshot differs from analysis"},
		{name: "current results differ from latest history", resultsSource: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var targetPath string
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
				paths := addKSIHistorySourceFixture(t, request)
				targetPath = paths.history
				if test.resultsSource {
					targetPath = paths.results
				}
				resultSet := compliance.KSIResultSet{}
				require.NoError(t, json.Unmarshal(sourceArtifactBody(t, request.SourceArtifacts, targetPath), &resultSet))
				resultSet.Results[0].LastValidatedUnixMs--
				body, err := json.Marshal(resultSet)
				require.NoError(t, err)
				replaceSourceArtifactBody(t, request.SourceArtifacts, targetPath, body)
				if !test.resultsSource {
					replaceSourceArtifactBody(t, request.SourceArtifacts, paths.results, body)
				}
			})

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

			require.NoError(t, err)
			assert.False(t, report.GetValid(), "REGRESSION: AFTER FIX")
			assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, targetPath)
		})
	}
}

func TestVerifyComplianceReportBundle_RejectsIncompleteKSISourceInventories(t *testing.T) {
	tests := []struct {
		name          string
		removeResults bool
	}{
		{name: "missing history"},
		{name: "missing current results", removeResults: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var missingPath string
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
				paths := addKSIHistorySourceFixture(t, request)
				missingPath = paths.history
				if test.removeResults {
					missingPath = paths.results
				}
				request.SourceArtifacts = removeSourceArtifact(request.SourceArtifacts, missingPath)
			})

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, missingPath)
		})
	}
}

func signedRestrictedBundleVerificationFixture(t *testing.T, plaintext []byte) (*compliancev1.ComplianceReportBundle, *bundleArtifactReaderStub, *compliancev1.ComplianceReportTrustPolicy, time.Time) {
	t.Helper()
	plaintextDigest := sha256.Sum256(plaintext)
	return signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		request.Profile = ProfileRestricted
		request.RestrictedArtifacts = []RestrictedArtifact{{
			BundlePath: constants.ComplianceBundleRestrictedEvidenceTestPath,
			Body:       []byte(`{"ciphertext":"authenticated"}`),
			MediaType:  constants.MediaTypeJSON,
			Encryption: &compliancev1.EvidenceEncryptionMetadata{
				Algorithm:                   constants.EvalEvidenceEncryptionAES256GCM,
				KeyId:                       "key-1",
				AuthorizationScope:          constants.EvalRestrictedEvidenceScope,
				PlaintextSha256:             hex.EncodeToString(plaintextDigest[:]),
				AuthenticatedMetadataSha256: strings.Repeat("b", 64),
			},
		}}
	})
}

func TestVerifyComplianceReportBundle_RejectsSignedCanonicalReportSourceMutations(t *testing.T) {
	assertionPath := path.Join(constants.ComplianceBundleAssertionsDirname, constants.ComplianceBundleAssertionCatalogFilename)
	frameworkPath := path.Join(constants.ComplianceBundleFrameworkCatalogsDirname, constants.ComplianceBundleFrameworkCatalogFilename)
	crosswalkPath := path.Join(constants.ComplianceBundleCrosswalksDirname, constants.ComplianceBundleCrosswalkFilename)
	assertionAssessmentsPath := path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleAssertionAssessmentsFilename)
	controlAssessmentsPath := path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleControlAssessmentsFilename)
	tests := []struct {
		name        string
		bundlePath  string
		failureCode error
		mutate      func(*BundleAssemblyRequest) []byte
	}{
		{name: "assertion catalog semantics", bundlePath: assertionPath, failureCode: constants.ErrInvalidEvidenceGraph, mutate: func(request *BundleAssemblyRequest) []byte {
			catalogValue := &compliancev1.ControlAssertionCatalog{}
			require.NoError(t, compliancev1.UnmarshalCanonical(sourceArtifactBody(t, request.SourceArtifacts, assertionPath), catalogValue))
			catalogValue.Sha256 = "invalid"
			body, err := compliancev1.MarshalCanonical(catalogValue)
			require.NoError(t, err)
			return body
		}},
		{name: "framework catalog semantics", bundlePath: frameworkPath, failureCode: constants.ErrInvalidEvidenceGraph, mutate: func(request *BundleAssemblyRequest) []byte {
			catalogValue := &compliancev1.FrameworkCatalog{}
			require.NoError(t, compliancev1.UnmarshalCanonical(sourceArtifactBody(t, request.SourceArtifacts, frameworkPath), catalogValue))
			catalogValue.Sha256 = "invalid"
			body, err := compliancev1.MarshalCanonical(catalogValue)
			require.NoError(t, err)
			return body
		}},
		{name: "crosswalk catalog semantics", bundlePath: crosswalkPath, failureCode: constants.ErrInvalidEvidenceGraph, mutate: func(request *BundleAssemblyRequest) []byte {
			catalogValue := &compliancev1.ControlCrosswalkCatalog{}
			require.NoError(t, compliancev1.UnmarshalCanonical(sourceArtifactBody(t, request.SourceArtifacts, crosswalkPath), catalogValue))
			catalogValue.Sha256 = "invalid"
			body, err := compliancev1.MarshalCanonical(catalogValue)
			require.NoError(t, err)
			return body
		}},
		{name: "assertion assessment projection", bundlePath: assertionAssessmentsPath, failureCode: constants.ErrRendererMismatch, mutate: func(request *BundleAssemblyRequest) []byte {
			assessments := cloneAssertionAssessments(request.Analysis.GetAssertionAssessments())
			assessments[0].AssessmentId = "mutated-assessment"
			body, err := marshalCanonicalMessages(assessments)
			require.NoError(t, err)
			return body
		}},
		{name: "control assessment projection", bundlePath: controlAssessmentsPath, failureCode: constants.ErrRendererMismatch, mutate: func(request *BundleAssemblyRequest) []byte {
			assessments := cloneControlAssessments(request.Analysis.GetFrameworkAssessments())
			assessments[0].AssessmentId = "mutated-assessment"
			body, err := marshalCanonicalMessages(assessments)
			require.NoError(t, err)
			return body
		}},
		{name: "evidence index projection", bundlePath: path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename), failureCode: constants.ErrRendererMismatch, mutate: func(request *BundleAssemblyRequest) []byte {
			resources := cloneEvidenceReferences(request.Analysis.GetEvidenceResources())
			resources[0].RunId = "mutated-run"
			body, err := marshalCanonicalMessages(resources)
			require.NoError(t, err)
			return body
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
				replaceSourceArtifactBody(t, request.SourceArtifacts, test.bundlePath, test.mutate(request))
			})

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

			require.NoError(t, err)
			assert.False(t, report.GetValid(), "REGRESSION: AFTER FIX")
			assertVerificationFailure(t, report, test.failureCode, test.bundlePath)
		})
	}
}

func sourceArtifactBody(t *testing.T, artifacts []SourceArtifact, bundlePath string) []byte {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.BundlePath == bundlePath {
			return artifact.Body
		}
	}
	require.FailNow(t, "source artifact is missing", bundlePath)
	return nil
}

func replaceSourceArtifactBody(t *testing.T, artifacts []SourceArtifact, bundlePath string, body []byte) {
	t.Helper()
	for index := range artifacts {
		if artifacts[index].BundlePath == bundlePath {
			artifacts[index].Body = body
			return
		}
	}
	require.FailNow(t, "source artifact is missing", bundlePath)
}

func removeSourceArtifact(artifacts []SourceArtifact, bundlePath string) []SourceArtifact {
	result := make([]SourceArtifact, 0, len(artifacts)-1)
	for _, artifact := range artifacts {
		if artifact.BundlePath != bundlePath {
			result = append(result, artifact)
		}
	}
	return result
}

func cloneAssertionAssessments(values []*compliancev1.ControlAssertionAssessment) []*compliancev1.ControlAssertionAssessment {
	clones := make([]*compliancev1.ControlAssertionAssessment, len(values))
	for index, value := range values {
		clones[index] = proto.Clone(value).(*compliancev1.ControlAssertionAssessment)
	}
	return clones
}

func cloneControlAssessments(values []*compliancev1.FrameworkControlAssessment) []*compliancev1.FrameworkControlAssessment {
	clones := make([]*compliancev1.FrameworkControlAssessment, len(values))
	for index, value := range values {
		clones[index] = proto.Clone(value).(*compliancev1.FrameworkControlAssessment)
	}
	return clones
}

func cloneEvidenceReferences(values []*compliancev1.ComplianceEvidenceReference) []*compliancev1.ComplianceEvidenceReference {
	clones := make([]*compliancev1.ComplianceEvidenceReference, len(values))
	for index, value := range values {
		clones[index] = proto.Clone(value).(*compliancev1.ComplianceEvidenceReference)
	}
	return clones
}

func TestVerifyComplianceReportBundle_AcceptsRestrictedBundleWithoutPlaintextAccess(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedRestrictedBundleVerificationFixture(t, []byte(`{"secret":"value"}`))

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.True(t, report.GetValid())
	assert.Empty(t, report.GetFailures())
}

func TestVerifyComplianceReportBundle_VerifiesRestrictedPlaintextDigestWhenAuthorizedReaderProvided(t *testing.T) {
	plaintext := []byte(`{"secret":"value"}`)
	bundle, reader, policy, verifiedAt := signedRestrictedBundleVerificationFixture(t, plaintext)
	plaintextReader := &authorizedPlaintextReaderStub{plaintext: plaintext}

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt, AuthorizedPlaintextReader: plaintextReader})

	require.NoError(t, err)
	assert.True(t, report.GetValid())
	assert.Empty(t, report.GetFailures())
	assert.Equal(t, 1, plaintextReader.calls)
}

func TestVerifyComplianceReportBundle_RejectsAuthorizedPlaintextDigestMismatch(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedRestrictedBundleVerificationFixture(t, []byte(`{"secret":"expected"}`))
	plaintextReader := &authorizedPlaintextReaderStub{plaintext: []byte(`{"secret":"mutated"}`)}

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt, AuthorizedPlaintextReader: plaintextReader})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assert.Contains(t, report.GetFailures(), &compliancev1.VerificationFailure{Code: constants.ErrEvidenceEncryptionInvalid.Error(), SubjectRef: constants.ComplianceBundleRestrictedEvidenceTestPath, Reason: "authorized plaintext SHA-256 does not match authenticated encryption metadata"})
	assert.Equal(t, 1, plaintextReader.calls)
}

func TestVerifyComplianceReportBundle_RejectsRestrictedEncryptionMetadataMutation(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedRestrictedBundleVerificationFixture(t, []byte(`{"secret":"value"}`))
	for _, artifact := range bundle.GetArtifacts() {
		if artifact.GetBundlePath() == constants.ComplianceBundleRestrictedEvidenceTestPath {
			artifact.Encryption.AuthenticatedMetadataSha256 = strings.Repeat("c", 64)
		}
	}

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assert.Contains(t, report.GetFailures(), &compliancev1.VerificationFailure{Code: constants.ErrChecksumMismatch.Error(), SubjectRef: constants.ComplianceBundleChecksumsPath, Reason: "reproduced checksum root does not match bundle checksum root"})
}

func TestVerifyComplianceReportBundle_RejectsAuthorizedPlaintextReaderFailure(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedRestrictedBundleVerificationFixture(t, []byte(`{"secret":"value"}`))
	plaintextReader := &authorizedPlaintextReaderStub{err: constants.ErrEvidenceTrustNotAssessed}

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt, AuthorizedPlaintextReader: plaintextReader})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assert.Contains(t, report.GetFailures(), &compliancev1.VerificationFailure{Code: constants.ErrEvidenceEncryptionInvalid.Error(), SubjectRef: constants.ComplianceBundleRestrictedEvidenceTestPath, Reason: constants.ErrEvidenceTrustNotAssessed.Error()})
	assert.Equal(t, 1, plaintextReader.calls)
}

func TestVerifyComplianceReportBundle_ReportsArtifactAndSignatureMutations(t *testing.T) {
	tests := []struct {
		name           string
		mutate         func(*compliancev1.ComplianceReportBundle, *bundleArtifactReaderStub, *compliancev1.ComplianceReportTrustPolicy)
		failureCode    error
		failureSubject string
	}{
		{
			name: "unsupported bundle schema",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.ReportSchemaVersion = "2.0.0"
			},
			failureCode:    constants.ErrEvidenceSchemaMismatch,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "unsupported assembler identity",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.GeneratorIdentity = "unassessed-assembler"
			},
			failureCode:    constants.ErrEvidenceProducerUnverified,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "unsupported manifest bundle profile",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.BundleProfile = "unsupported"
			},
			failureCode:    constants.ErrBundleProfileUnsupported,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "public manifest contains restricted artifact",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Artifacts[0].Profile = constants.ComplianceBundleProfileRestricted
			},
			failureCode:    constants.ErrBundleProfileUnsupported,
			failureSubject: constants.ComplianceBundleAnalysisPath,
		},
		{
			name: "analysis scope mismatch",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Analysis.ScopeRef = "other-scope"
			},
			failureCode:    constants.ErrEvidenceScopeMismatch,
			failureSubject: constants.ComplianceBundleAnalysisPath,
		},
		{
			name: "framework profile analysis mismatch",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Profiles[0].AnalysisRef = "analysis:sha256:" + strings.Repeat("0", 64)
			},
			failureCode:    constants.ErrUnresolvedReference,
			failureSubject: constants.ComplianceBundleFrameworkProfileTestPath,
		},
		{
			name: "profile framework does not bind manifest",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Profiles[0].FrameworkRef.Version = "unsupported"
			},
			failureCode:    constants.ErrUnresolvedReference,
			failureSubject: constants.ComplianceBundleFrameworkProfileTestPath,
		},
		{
			name: "manifest framework lacks profile",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Profiles = nil
			},
			failureCode:    constants.ErrUnresolvedReference,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "unresolved assertion catalog reference",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.AssertionCatalogRef = constants.ComplianceBundleUnexpectedTestPath
			},
			failureCode:    constants.ErrUnresolvedReference,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "unresolved crosswalk reference",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.CrosswalkRefs[0] = constants.ComplianceBundleUnexpectedTestPath
			},
			failureCode:    constants.ErrUnresolvedReference,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "unresolved assessment reference",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.AssessmentRefs[0] = constants.ComplianceBundleUnexpectedTestPath
			},
			failureCode:    constants.ErrUnresolvedReference,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "unresolved evidence index reference",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.EvidenceIndexRef = constants.ComplianceBundleUnexpectedTestPath
			},
			failureCode:    constants.ErrUnresolvedReference,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "artifact body digest mismatch",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				reader.bodies[constants.ComplianceBundleAnalysisPath] = []byte(`{"tampered":true}`)
			},
			failureCode:    constants.ErrChecksumMismatch,
			failureSubject: constants.ComplianceBundleAnalysisPath,
		},
		{
			name: "missing artifact body",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				delete(reader.bodies, constants.ComplianceBundleAnalysisPath)
			},
			failureCode:    constants.ErrBundleArtifactMissing,
			failureSubject: constants.ComplianceBundleAnalysisPath,
		},
		{
			name: "descriptor digest mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Artifacts[0].Sha256 = strings.Repeat("0", 64)
			},
			failureCode:    constants.ErrChecksumMismatch,
			failureSubject: constants.ComplianceBundleAnalysisPath,
		},
		{
			name: "artifact length mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Artifacts[0].ByteLength++
			},
			failureCode:    constants.ErrChecksumMismatch,
			failureSubject: constants.ComplianceBundleAnalysisPath,
		},
		{
			name: "renderer media type mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.RenderedFormats[0].MediaType = constants.MediaTypeText
			},
			failureCode:    constants.ErrRendererMismatch,
			failureSubject: constants.ComplianceBundleAnalysisPath,
		},
		{
			name: "checksum root mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.ChecksumRoot = strings.Repeat("0", 64)
			},
			failureCode:    constants.ErrChecksumMismatch,
			failureSubject: constants.ComplianceBundleChecksumsPath,
		},
		{
			name: "manifest content mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.ReportId = "tampered-report"
			},
			failureCode:    constants.ErrChecksumMismatch,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "malformed generation timestamp",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.GeneratedAt = &timestamppb.Timestamp{Seconds: 253402300800}
			},
			failureCode:    constants.ErrReportSignatureFailed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "manifest signature mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.Signature.Signature = strings.Repeat("0", ed25519.SignatureSize*2)
			},
			failureCode:    constants.ErrReportSignatureFailed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "checksum signature mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.ChecksumRootSignature.Signature = strings.Repeat("0", ed25519.SignatureSize*2)
			},
			failureCode:    constants.ErrReportSignatureFailed,
			failureSubject: constants.ComplianceBundleChecksumsPath,
		},
		{
			name: "unassessed packaged signer",
			mutate: func(_ *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, policy *compliancev1.ComplianceReportTrustPolicy) {
				policy.TrustedKeys = nil
			},
			failureCode:    constants.ErrEvidenceTrustNotAssessed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "signer scope not assessed",
			mutate: func(_ *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, policy *compliancev1.ComplianceReportTrustPolicy) {
				policy.TrustedKeys[0].AllowedScopeRefs = []string{"other-scope"}
			},
			failureCode:    constants.ErrEvidenceTrustNotAssessed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "signer revoked before generation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, policy *compliancev1.ComplianceReportTrustPolicy) {
				policy.TrustedKeys[0].RevokedAt = timestamppb.New(bundle.Manifest.GeneratedAt.AsTime())
			},
			failureCode:    constants.ErrEvidenceTrustNotAssessed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "signer validity expired before generation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, policy *compliancev1.ComplianceReportTrustPolicy) {
				policy.TrustedKeys[0].Metadata.ExpiresAt = timestamppb.New(bundle.Manifest.GeneratedAt.AsTime().Add(-time.Second))
			},
			failureCode:    constants.ErrEvidenceTrustNotAssessed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "assessed signer public key digest mismatch",
			mutate: func(_ *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, policy *compliancev1.ComplianceReportTrustPolicy) {
				policy.TrustedKeys[0].Metadata.PublicKeySha256 = strings.Repeat("0", 64)
			},
			failureCode:    constants.ErrEvidenceTrustNotAssessed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "missing signer assessment identifier",
			mutate: func(_ *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, policy *compliancev1.ComplianceReportTrustPolicy) {
				policy.TrustedKeys[0].AssessmentId = ""
			},
			failureCode:    constants.ErrEvidenceTrustNotAssessed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "missing signer assessor identity",
			mutate: func(_ *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, policy *compliancev1.ComplianceReportTrustPolicy) {
				policy.TrustedKeys[0].AssessorIdentity = ""
			},
			failureCode:    constants.ErrEvidenceTrustNotAssessed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "malformed signer assessment timestamp",
			mutate: func(_ *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, policy *compliancev1.ComplianceReportTrustPolicy) {
				policy.TrustedKeys[0].AssessedAt = &timestamppb.Timestamp{Seconds: 253402300800}
			},
			failureCode:    constants.ErrEvidenceTrustNotAssessed,
			failureSubject: constants.ComplianceBundleManifestPath,
		},
		{
			name: "demo source inventory run binding mismatch",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Analysis.EvidenceResources[0].RunId = "other-demo-run"
			},
			failureCode:    constants.ErrUnresolvedReference,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename),
		},
		{
			name: "missing eval source inventory",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Analysis.EvidenceResources = append(bundle.Analysis.EvidenceResources, &compliancev1.ComplianceEvidenceReference{ArtifactType: string(evidence.ArtifactTypeEvalManifest), RunId: "eval-run-1"})
			},
			failureCode:    constants.ErrEvalRunVerificationFailed,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, "eval-run-1"),
		},
		{
			name: "missing demo source verification",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename)
				delete(reader.bodies, bundlePath)
			},
			failureCode:    constants.ErrDemoRunVerificationFailed,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename),
		},
		{
			name: "demo source verification lacks typed check",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename)
				report := &compliancev1.ComplianceVerificationReport{}
				require.NoError(t, compliancev1.UnmarshalCanonical(reader.bodies[bundlePath], report))
				report.Checks = nil
				body, err := compliancev1.MarshalCanonical(report)
				require.NoError(t, err)
				reader.bodies[bundlePath] = body
			},
			failureCode:    constants.ErrDemoRunVerificationFailed,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename),
		},
		{
			name: "invalid demo source verification",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename)
				report := &compliancev1.ComplianceVerificationReport{ReportId: "demo-run-1", VerifiedAt: timestamppb.Now(), VerifierId: constants.DemoRunVerifierID, VerifierVersion: constants.DemoRunVerifierVersion}
				body, err := compliancev1.MarshalCanonical(report)
				require.NoError(t, err)
				reader.bodies[bundlePath] = body
			},
			failureCode:    constants.ErrDemoRunVerificationFailed,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename),
		},
		{
			name: "malformed demo source verification timestamp",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename)
				reader.bodies[bundlePath] = bytes.Replace(reader.bodies[bundlePath], []byte(`"verified_at":"2023-11-14T22:13:20Z"`), []byte(`"verified_at":"10000-01-01T00:00:00Z"`), 1)
			},
			failureCode:    constants.ErrEvidenceArtifactMalformed,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename),
		},
		{
			name: "missing demo runtime manifest source",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunManifestFilename)
				delete(reader.bodies, bundlePath)
			},
			failureCode:    constants.ErrDemoRunVerificationFailed,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1"),
		},
		{
			name: "tampered demo runtime results source",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunResultsFilename)
				reader.bodies[bundlePath] = append(reader.bodies[bundlePath], '\n')
			},
			failureCode:    constants.ErrDemoRunVerificationFailed,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename),
		},
		{
			name: "tampered demo provenance source",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceProvenanceDirname, constants.ComplianceBundleSourceArtifactsDirname, constants.DemosComposeFile)
				reader.bodies[bundlePath] = append(reader.bodies[bundlePath], '\n')
			},
			failureCode:    constants.ErrDemoRunVerificationFailed,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename),
		},
		{
			name: "tampered demo scenario definitions source",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceProvenanceDirname, constants.DemoRunDefinitionsFilename)
				reader.bodies[bundlePath] = append(reader.bodies[bundlePath], '\n')
			},
			failureCode:    constants.ErrDemoRunVerificationFailed,
			failureSubject: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, "demo-run-1", constants.ComplianceBundleSourceVerificationFilename),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
			test.mutate(bundle, reader, policy)

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{
				Bundle:      bundle,
				Reader:      reader,
				TrustPolicy: policy,
				VerifiedAt:  verifiedAt,
			})

			require.NoError(t, err)
			require.NotNil(t, report)
			assert.False(t, report.GetValid())
			require.NotEmpty(t, report.GetFailures())
			for _, failure := range report.GetFailures() {
				assert.NotEmpty(t, failure.GetCode())
				assert.NotEmpty(t, failure.GetSubjectRef())
			}
			assertVerificationFailure(t, report, test.failureCode, test.failureSubject)
		})
	}
}

func TestVerifyComplianceReportBundle_RejectsEveryProtectedReportBodyMutation(t *testing.T) {
	tests := []struct {
		name       string
		bundlePath string
	}{
		{name: "canonical JSON renderer", bundlePath: constants.ComplianceBundleJSONPath},
		{name: "OSCAL renderer", bundlePath: constants.ComplianceBundleOSCALPath},
		{name: "Markdown renderer", bundlePath: constants.ComplianceBundleMarkdownPath},
		{name: "HTML renderer", bundlePath: constants.ComplianceBundleHTMLPath},
		{name: "CLI renderer", bundlePath: constants.ComplianceBundleCLIPath},
		{name: "assertion catalog", bundlePath: path.Join(constants.ComplianceBundleAssertionsDirname, constants.ComplianceBundleAssertionCatalogFilename)},
		{name: "framework catalog", bundlePath: path.Join(constants.ComplianceBundleFrameworkCatalogsDirname, constants.ComplianceBundleFrameworkCatalogFilename)},
		{name: "crosswalk catalog", bundlePath: path.Join(constants.ComplianceBundleCrosswalksDirname, constants.ComplianceBundleCrosswalkFilename)},
		{name: "assertion assessments", bundlePath: path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleAssertionAssessmentsFilename)},
		{name: "control assessments", bundlePath: path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleControlAssessmentsFilename)},
		{name: "evidence index", bundlePath: path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
			require.Contains(t, reader.bodies, test.bundlePath)
			reader.bodies[test.bundlePath] = append(reader.bodies[test.bundlePath], '\n')

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertVerificationFailure(t, report, constants.ErrChecksumMismatch, test.bundlePath)
		})
	}
}

func TestBundleVerifier_SourceVerificationRejectsMalformedTimestamps(t *testing.T) {
	tests := []struct {
		name            string
		sourceDir       string
		verifierID      string
		verifierVersion string
		verificationErr error
		source          string
	}{
		{name: "demo source verification timestamp", sourceDir: constants.ComplianceBundleSourceDemosDirname, verifierID: constants.DemoRunVerifierID, verifierVersion: constants.DemoRunVerifierVersion, verificationErr: constants.ErrDemoRunVerificationFailed, source: "demo"},
		{name: "eval source verification timestamp", sourceDir: constants.ComplianceBundleSourceEvalsDirname, verifierID: constants.EvalRunVerifierID, verifierVersion: constants.EvalRunVerifierVersion, verificationErr: constants.ErrEvalRunVerificationFailed, source: "eval"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generatedAt := time.Unix(1_700_000_000, 0).UTC()
			body, err := compliancev1.MarshalCanonical(&compliancev1.ComplianceVerificationReport{
				ReportId:        "source-run-1",
				Valid:           true,
				VerifiedAt:      timestamppb.New(generatedAt),
				VerifierId:      test.verifierID,
				VerifierVersion: test.verifierVersion,
			})
			require.NoError(t, err)
			malformed := bytes.Replace(body, []byte(`"verified_at":"2023-11-14T22:13:20Z"`), []byte(`"verified_at":"10000-01-01T00:00:00Z"`), 1)
			require.NotEqual(t, body, malformed)
			bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, test.sourceDir, "source-run-1", constants.ComplianceBundleSourceVerificationFilename)
			verifier := &bundleVerifier{
				request: BundleVerificationRequest{Bundle: &compliancev1.ComplianceReportBundle{Manifest: &compliancev1.ComplianceReportManifest{GeneratedAt: timestamppb.New(generatedAt)}}},
				report:  &compliancev1.ComplianceVerificationReport{},
			}

			assert.Nil(t, verifier.verifySourceVerificationReport(bundlePath, malformed, "source-run-1", test.verifierID, test.verifierVersion, test.verificationErr, test.source))
			assertVerificationFailure(t, verifier.report, constants.ErrEvidenceArtifactMalformed, bundlePath)
		})
	}
}

func TestBundleVerifier_SourceVerificationRejectsReportsWithoutAnalysisEvidenceRuns(t *testing.T) {
	tests := []struct {
		name            string
		sourceDir       string
		verifierID      string
		verifierVersion string
	}{
		{name: "demo source", sourceDir: constants.ComplianceBundleSourceDemosDirname, verifierID: constants.DemoRunVerifierID, verifierVersion: constants.DemoRunVerifierVersion},
		{name: "eval source", sourceDir: constants.ComplianceBundleSourceEvalsDirname, verifierID: constants.EvalRunVerifierID, verifierVersion: constants.EvalRunVerifierVersion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generatedAt := time.Unix(1_700_000_000, 0).UTC()
			body, err := compliancev1.MarshalCanonical(&compliancev1.ComplianceVerificationReport{
				ReportId:        "undeclared-run",
				Valid:           true,
				VerifiedAt:      timestamppb.New(generatedAt),
				VerifierId:      test.verifierID,
				VerifierVersion: test.verifierVersion,
			})
			require.NoError(t, err)
			bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, test.sourceDir, "undeclared-run", constants.ComplianceBundleSourceVerificationFilename)
			verifier := bundleVerifier{
				request: BundleVerificationRequest{Bundle: &compliancev1.ComplianceReportBundle{
					Manifest: &compliancev1.ComplianceReportManifest{GeneratedAt: timestamppb.New(generatedAt)},
					Analysis: &compliancev1.ComplianceAnalysis{},
				}},
				report: &compliancev1.ComplianceVerificationReport{},
				bodies: map[string][]byte{bundlePath: body},
			}

			verifier.verifySourceVerificationReports(context.Background())

			assertVerificationFailure(t, verifier.report, constants.ErrUnresolvedReference, bundlePath)
		})
	}
}

func TestVerifyComplianceReportBundle_RejectsUnexpectedDirectoryArtifacts(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
	reader.bodies[constants.ComplianceBundleUnexpectedTestPath] = []byte("unexpected")

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{
		Bundle:      bundle,
		Reader:      reader,
		TrustPolicy: policy,
		VerifiedAt:  verifiedAt,
	})

	require.NoError(t, err)
	assert.False(t, report.GetValid())
	assertVerificationFailure(t, report, constants.ErrUnexpectedEvidenceArtifact, constants.ComplianceBundleUnexpectedTestPath)
}

func TestVerifyComplianceReportBundle_OrdersFailuresDeterministically(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
	reader.bodies[constants.ComplianceBundleAnalysisPath] = []byte(`{"tampered":true}`)
	bundle.ChecksumRoot = strings.Repeat("0", 64)

	first, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})
	require.NoError(t, err)
	second, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})
	require.NoError(t, err)

	assert.Equal(t, first.GetFailures(), second.GetFailures())
	for index := 1; index < len(first.GetFailures()); index++ {
		previous := first.GetFailures()[index-1]
		current := first.GetFailures()[index]
		assert.LessOrEqual(t, previous.GetSubjectRef()+previous.GetCode()+previous.GetReason(), current.GetSubjectRef()+current.GetCode()+current.GetReason())
	}
}

func TestVerifyComplianceReportBundle_ReportsArtifactReaderFailuresWithStableCodes(t *testing.T) {
	tests := []struct {
		name            string
		listErr         error
		readErr         error
		expectedCode    error
		expectedSubject string
	}{
		{name: "directory read failure", listErr: errors.New("read failed"), expectedCode: constants.ErrDirectoryRead, expectedSubject: constants.ComplianceBundleManifestPath},
		{name: "directory resource limit", listErr: constants.ErrEvidenceDirectoryLimitExceeded, expectedCode: constants.ErrEvidenceDirectoryLimitExceeded, expectedSubject: constants.ComplianceBundleManifestPath},
		{name: "artifact read failure", readErr: errors.New("read failed"), expectedCode: constants.ErrFileReadFailed, expectedSubject: constants.ComplianceBundleAnalysisPath},
		{name: "oversized artifact read", readErr: constants.ErrEvidenceArtifactTooLarge, expectedCode: constants.ErrEvidenceArtifactTooLarge, expectedSubject: constants.ComplianceBundleAnalysisPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
			reader.listErr = test.listErr
			reader.readErr = test.readErr

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertVerificationFailure(t, report, test.expectedCode, test.expectedSubject)
		})
	}
}

func TestVerifyComplianceReportBundle_RejectsInvalidVerifierConfiguration(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
	tests := []struct {
		name    string
		request BundleVerificationRequest
	}{
		{name: "missing bundle", request: BundleVerificationRequest{Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt}},
		{name: "missing reader", request: BundleVerificationRequest{Bundle: bundle, TrustPolicy: policy, VerifiedAt: verifiedAt}},
		{name: "missing trust policy", request: BundleVerificationRequest{Bundle: bundle, Reader: reader, VerifiedAt: verifiedAt}},
		{name: "missing verification time", request: BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := VerifyComplianceReportBundle(context.Background(), test.request)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
			assert.Nil(t, report)
		})
	}
}

func TestEvidenceVerificationRoutes_ClassifyEverySupportedArtifactType(t *testing.T) {
	supported := evidence.SupportedArtifactTypes()
	represented := make(map[evidence.ArtifactType]struct{}, len(supported))
	for _, artifactType := range supported {
		_, exists := evidenceVerificationRoutes[artifactType]
		assert.True(t, exists, "artifact type %q has no complete-bundle verification route", artifactType)
		represented[artifactType] = struct{}{}
	}
	assert.Len(t, evidenceVerificationRoutes, len(represented))
	for artifactType, route := range evidenceVerificationRoutes {
		assert.Contains(t, represented, artifactType)
		assert.NotEmpty(t, route)
	}
}

func TestVerifyComplianceReportBundle_PreservesCancellation(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := VerifyComplianceReportBundle(ctx, BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, report)
}

type ledgerReplayCommitRecord struct {
	SchemaVersion    string `json:"schema_version"`
	ProducerIdentity string `json:"producer_identity"`
	CommitHash       string `json:"commit_hash"`
	ParentHash       string `json:"parent_hash"`
	TimestampUTC     string `json:"timestamp_utc"`
	Message          string `json:"message"`
	FilesChanged     int    `json:"files_changed"`
	DiffStat         string `json:"diff_stat"`
}

type ledgerReplayStateRecord struct {
	SchemaVersion    string `json:"schema_version"`
	ProducerIdentity string `json:"producer_identity"`
	MerkleRoot       string `json:"merkle_root"`
	CapturedAtUTC    string `json:"captured_at_utc"`
}

type ledgerReplayFixture struct {
	commitsPath string
	statePath   string
	commits     []ledgerReplayCommitRecord
	state       ledgerReplayStateRecord
}

func addLedgerReplayFixture(t *testing.T, request *BundleAssemblyRequest) ledgerReplayFixture {
	t.Helper()
	runID := "ledger-run-1"
	base := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, request.ScopeRef, runID, constants.LedgerEvidenceDirname)
	fixture := ledgerReplayFixture{
		commitsPath: path.Join(base, constants.LedgerCommitsFilename),
		statePath:   path.Join(base, constants.LedgerStateFilename),
		commits: []ledgerReplayCommitRecord{
			{SchemaVersion: constants.LedgerEvidenceSchemaVersion, ProducerIdentity: "gateway-1", CommitHash: strings.Repeat("1", 40), TimestampUTC: "2026-09-06T10:00:00Z", Message: "bootstrap", FilesChanged: 1, DiffStat: "1 file changed"},
			{SchemaVersion: constants.LedgerEvidenceSchemaVersion, ProducerIdentity: "gateway-1", CommitHash: strings.Repeat("2", 40), ParentHash: strings.Repeat("1", 40), TimestampUTC: "2026-09-06T10:01:00Z", Message: "governed mutation", FilesChanged: 1, DiffStat: "1 file changed"},
		},
		state: ledgerReplayStateRecord{SchemaVersion: constants.LedgerEvidenceSchemaVersion, ProducerIdentity: "gateway-1", MerkleRoot: strings.Repeat("2", 40), CapturedAtUTC: "2026-09-06T10:02:00Z"},
	}
	commitsBody := marshalJSONLines(t, fixture.commits)
	stateBody, err := json.Marshal(fixture.state)
	require.NoError(t, err)
	binding := evidence.LedgerImportBinding{
		CommitsReference: evidence.ContentReferenceForBody(constants.LedgerCommitCollectionReferencePrefix, commitsBody),
		StateReference:   evidence.ContentReferenceForBody(constants.LedgerStateReferencePrefix, stateBody),
		CommitsPath:      fixture.commitsPath,
		StatePath:        fixture.statePath,
		ScopeID:          request.ScopeRef,
		RunID:            runID,
		AttemptID:        "attempt-1",
		ScenarioID:       "scenario-1",
	}
	reader := &bundledSourceArtifactReader{bodies: map[string][]byte{fixture.commitsPath: commitsBody, fixture.statePath: stateBody}}
	nodes, err := evidence.NewLedgerImporter(reader, binding).Import(context.Background())
	require.NoError(t, err)
	for index := range nodes {
		request.Analysis.EvidenceResources = append(request.Analysis.EvidenceResources, nodes[index].ToProto())
	}
	request.SourceArtifacts = append(request.SourceArtifacts,
		SourceArtifact{BundlePath: fixture.commitsPath, Body: commitsBody, MediaType: constants.MediaTypeJSON},
		SourceArtifact{BundlePath: fixture.statePath, Body: stateBody, MediaType: constants.MediaTypeJSON},
	)
	refreshBundleAnalysisArtifacts(t, request)
	return fixture
}

type buildConfigReplayRecord struct {
	SchemaVersion       string                    `json:"schema_version"`
	AttestationType     string                    `json:"attestation_type"`
	ProducerIdentity    string                    `json:"producer_identity"`
	ProducedAtUTC       string                    `json:"produced_at_utc"`
	ScopeID             string                    `json:"scope_id"`
	RunID               string                    `json:"run_id"`
	BuildIdentity       string                    `json:"build_identity"`
	SourceRevision      string                    `json:"source_revision"`
	ImageDigests        []buildConfigReplayDigest `json:"image_digests,omitempty"`
	ComponentInventory  []buildConfigReplayItem   `json:"component_inventory,omitempty"`
	ConfigurationHashes []buildConfigReplayDigest `json:"configuration_hashes,omitempty"`
}

type buildConfigReplayDigest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type buildConfigReplayItem struct {
	ComponentID   string `json:"component_id"`
	ComponentType string `json:"component_type"`
	Version       string `json:"version"`
	Digest        string `json:"digest"`
}

type buildConfigReplayFixture struct {
	bundlePath string
	records    []buildConfigReplayRecord
}

func addBuildConfigReplayFixture(t *testing.T, request *BundleAssemblyRequest) buildConfigReplayFixture {
	t.Helper()
	runID := "build-run-1"
	fixture := buildConfigReplayFixture{
		bundlePath: path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, request.ScopeRef, runID, constants.BuildConfigAttestationsFilename),
		records: []buildConfigReplayRecord{
			{SchemaVersion: constants.BuildAttestationSchemaVersion, AttestationType: "build", ProducerIdentity: "build-system-1", ProducedAtUTC: "2026-09-06T10:00:00Z", ScopeID: request.ScopeRef, RunID: runID, BuildIdentity: "build-1", SourceRevision: "revision-1", ImageDigests: []buildConfigReplayDigest{{Name: "gateway", SHA256: strings.Repeat("1", 64)}}, ComponentInventory: []buildConfigReplayItem{{ComponentID: "gateway", ComponentType: "service", Version: "2.1.7", Digest: strings.Repeat("2", 64)}}},
			{SchemaVersion: constants.BuildAttestationSchemaVersion, AttestationType: "configuration", ProducerIdentity: "build-system-1", ProducedAtUTC: "2026-09-06T10:01:00Z", ScopeID: request.ScopeRef, RunID: runID, BuildIdentity: "build-1", SourceRevision: "revision-1", ConfigurationHashes: []buildConfigReplayDigest{{Name: "gateway", SHA256: strings.Repeat("3", 64)}}},
		},
	}
	body := marshalJSONLines(t, fixture.records)
	binding := evidence.BuildConfigImportBinding{Reference: evidence.ContentReferenceForBody(constants.BuildAttestationReferencePrefix, body), Path: fixture.bundlePath, ScopeID: request.ScopeRef, RunID: runID, BuildIdentity: "build-1", SourceRevision: "revision-1", ProducerIdentity: "build-system-1"}
	reader := &bundledSourceArtifactReader{bodies: map[string][]byte{fixture.bundlePath: body}}
	nodes, err := evidence.NewBuildConfigImporter(reader, binding).Import(context.Background())
	require.NoError(t, err)
	for index := range nodes {
		request.Analysis.EvidenceResources = append(request.Analysis.EvidenceResources, nodes[index].ToProto())
	}
	request.SourceArtifacts = append(request.SourceArtifacts, SourceArtifact{BundlePath: fixture.bundlePath, Body: body, MediaType: constants.MediaTypeJSON})
	refreshBundleAnalysisArtifacts(t, request)
	return fixture
}

func marshalJSONLines[T any](t *testing.T, records []T) []byte {
	t.Helper()
	lines := make([][]byte, 0, len(records))
	for _, record := range records {
		body, err := json.Marshal(record)
		require.NoError(t, err)
		lines = append(lines, body)
	}
	return append(bytes.Join(lines, []byte{'\n'}), '\n')
}

func refreshBundleAnalysisArtifacts(t *testing.T, request *BundleAssemblyRequest) {
	t.Helper()
	sort.Slice(request.Analysis.EvidenceResources, func(i, j int) bool {
		return request.Analysis.EvidenceResources[i].GetArtifactId() < request.Analysis.EvidenceResources[j].GetArtifactId()
	})
	evidenceIndex, err := marshalCanonicalMessages(request.Analysis.GetEvidenceResources())
	require.NoError(t, err)
	replaceSourceArtifactBody(t, request.SourceArtifacts, path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename), evidenceIndex)
	request.RenderedFormats, err = renderAllFormats(request.Analysis)
	require.NoError(t, err)
}

func TestVerifyComplianceReportBundle_ReplaysLedgerSources(t *testing.T) {
	var fixture ledgerReplayFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addLedgerReplayFixture(t, request)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.True(t, report.GetValid(), report.GetFailures())
	assert.NotEmpty(t, fixture.commitsPath)
}

func TestVerifyComplianceReportBundle_RejectsSignedLedgerSemanticSubstitution(t *testing.T) {
	var fixture ledgerReplayFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addLedgerReplayFixture(t, request)
		fixture.commits[1].Message = "semantically substituted"
		replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.commitsPath, marshalJSONLines(t, fixture.commits))
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid(), "REGRESSION: AFTER FIX")
	assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.commitsPath)
}

func TestVerifyComplianceReportBundle_ReplaysBuildConfigurationSources(t *testing.T) {
	var fixture buildConfigReplayFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addBuildConfigReplayFixture(t, request)
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.True(t, report.GetValid(), report.GetFailures())
	assert.NotEmpty(t, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsSignedBuildConfigurationSemanticSubstitution(t *testing.T) {
	var fixture buildConfigReplayFixture
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
		fixture = addBuildConfigReplayFixture(t, request)
		fixture.records[0].ImageDigests[0].SHA256 = strings.Repeat("9", 64)
		replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, marshalJSONLines(t, fixture.records))
	})

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	require.NoError(t, err)
	assert.False(t, report.GetValid(), "REGRESSION: AFTER FIX")
	assertVerificationFailure(t, report, constants.ErrInvalidEvidenceGraph, fixture.bundlePath)
}

func TestVerifyComplianceReportBundle_RejectsLedgerInventoryAndBindingMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *BundleAssemblyRequest, *ledgerReplayFixture)
	}{
		{name: "missing commit inventory", mutate: func(_ *testing.T, request *BundleAssemblyRequest, fixture *ledgerReplayFixture) {
			request.SourceArtifacts = removeSourceArtifact(request.SourceArtifacts, fixture.commitsPath)
		}},
		{name: "missing state inventory", mutate: func(_ *testing.T, request *BundleAssemblyRequest, fixture *ledgerReplayFixture) {
			request.SourceArtifacts = removeSourceArtifact(request.SourceArtifacts, fixture.statePath)
		}},
		{name: "empty commit inventory", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *ledgerReplayFixture) {
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.commitsPath, nil)
		}},
		{name: "empty state inventory", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *ledgerReplayFixture) {
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.statePath, nil)
		}},
		{name: "duplicate resource identity under conflicting metadata", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *ledgerReplayFixture) {
			fixture.commits[1].CommitHash = fixture.commits[0].CommitHash
			fixture.commits[1].Message = "conflicting duplicate"
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.commitsPath, marshalJSONLines(t, fixture.commits))
		}},
		{name: "orphaned inventory", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *ledgerReplayFixture) {
			resources := request.Analysis.EvidenceResources[:0]
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() != string(evidence.ArtifactTypeLedgerCommit) && resource.GetArtifactType() != string(evidence.ArtifactTypeLedgerState) {
					resources = append(resources, resource)
				}
			}
			request.Analysis.EvidenceResources = resources
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "wrong canonical path", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *ledgerReplayFixture) {
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() == string(evidence.ArtifactTypeLedgerState) {
					resource.BundlePath = constants.ComplianceBundleUnexpectedTestPath
					break
				}
			}
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "cross-scope path transplantation", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *ledgerReplayFixture) {
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() == string(evidence.ArtifactTypeLedgerCommit) {
					resource.BundlePath = path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundlePlatformEvidenceDirname, constants.TestNestedDirname, resource.GetRunId(), constants.LedgerEvidenceDirname, constants.LedgerCommitsFilename)
					break
				}
			}
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "wrong run", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *ledgerReplayFixture) {
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() == string(evidence.ArtifactTypeLedgerCommit) {
					resource.RunId = "other-run"
					break
				}
			}
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "producer mismatch", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *ledgerReplayFixture) {
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() == string(evidence.ArtifactTypeLedgerCommit) {
					resource.ProducerIdentity = "other-producer"
					break
				}
			}
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "conflicting attempt binding", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *ledgerReplayFixture) {
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() == string(evidence.ArtifactTypeLedgerState) {
					resource.AttemptId = "other-attempt"
					break
				}
			}
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "commit chain mismatch", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *ledgerReplayFixture) {
			fixture.commits[1].ParentHash = strings.Repeat("9", 40)
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.commitsPath, marshalJSONLines(t, fixture.commits))
		}},
		{name: "state root mismatch", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *ledgerReplayFixture) {
			fixture.state.MerkleRoot = strings.Repeat("9", 40)
			body, err := json.Marshal(fixture.state)
			require.NoError(t, err)
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.statePath, body)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
				fixture := addLedgerReplayFixture(t, request)
				test.mutate(t, request, &fixture)
			})
			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})
			require.NoError(t, err)
			assert.False(t, report.GetValid())
			require.NotEmpty(t, report.GetFailures())
			for _, failure := range report.GetFailures() {
				assert.NotEmpty(t, failure.GetCode())
				assert.NotEmpty(t, failure.GetSubjectRef())
			}
		})
	}
}

func TestVerifyComplianceReportBundle_RejectsBuildConfigurationInventoryAndBindingMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *BundleAssemblyRequest, *buildConfigReplayFixture)
	}{
		{name: "missing inventory", mutate: func(_ *testing.T, request *BundleAssemblyRequest, fixture *buildConfigReplayFixture) {
			request.SourceArtifacts = removeSourceArtifact(request.SourceArtifacts, fixture.bundlePath)
		}},
		{name: "empty inventory", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *buildConfigReplayFixture) {
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, nil)
		}},
		{name: "duplicate build identity", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *buildConfigReplayFixture) {
			fixture.records = append(fixture.records, fixture.records[0])
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, marshalJSONLines(t, fixture.records))
		}},
		{name: "orphaned inventory", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *buildConfigReplayFixture) {
			resources := request.Analysis.EvidenceResources[:0]
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() != string(evidence.ArtifactTypeBuildAttestation) && resource.GetArtifactType() != string(evidence.ArtifactTypeConfigAttestation) {
					resources = append(resources, resource)
				}
			}
			request.Analysis.EvidenceResources = resources
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "source scope mutation", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *buildConfigReplayFixture) {
			for index := range fixture.records {
				fixture.records[index].ScopeID = "other-scope"
			}
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, marshalJSONLines(t, fixture.records))
		}},
		{name: "wrong run", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *buildConfigReplayFixture) {
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() == string(evidence.ArtifactTypeConfigAttestation) {
					resource.RunId = "other-run"
					break
				}
			}
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "wrong canonical path", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *buildConfigReplayFixture) {
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() == string(evidence.ArtifactTypeBuildAttestation) {
					resource.BundlePath = constants.ComplianceBundleUnexpectedTestPath
					break
				}
			}
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "producer mismatch", mutate: func(t *testing.T, request *BundleAssemblyRequest, _ *buildConfigReplayFixture) {
			for _, resource := range request.Analysis.GetEvidenceResources() {
				if resource.GetArtifactType() == string(evidence.ArtifactTypeConfigAttestation) {
					resource.ProducerIdentity = "other-producer"
					break
				}
			}
			refreshBundleAnalysisArtifacts(t, request)
		}},
		{name: "source revision mutation", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *buildConfigReplayFixture) {
			for index := range fixture.records {
				fixture.records[index].SourceRevision = "revision-2"
			}
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, marshalJSONLines(t, fixture.records))
		}},
		{name: "configuration digest mutation", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *buildConfigReplayFixture) {
			fixture.records[1].ConfigurationHashes[0].SHA256 = strings.Repeat("8", 64)
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, marshalJSONLines(t, fixture.records))
		}},
		{name: "component inventory mutation", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *buildConfigReplayFixture) {
			fixture.records[0].ComponentInventory[0].Version = "2.1.8"
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, marshalJSONLines(t, fixture.records))
		}},
		{name: "schema mutation", mutate: func(t *testing.T, request *BundleAssemblyRequest, fixture *buildConfigReplayFixture) {
			fixture.records[0].SchemaVersion = "unsupported"
			replaceSourceArtifactBody(t, request.SourceArtifacts, fixture.bundlePath, marshalJSONLines(t, fixture.records))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixtureWithRequest(t, func(request *BundleAssemblyRequest) {
				fixture := addBuildConfigReplayFixture(t, request)
				test.mutate(t, request, &fixture)
			})
			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})
			require.NoError(t, err)
			assert.False(t, report.GetValid())
			require.NotEmpty(t, report.GetFailures())
			for _, failure := range report.GetFailures() {
				assert.NotEmpty(t, failure.GetCode())
				assert.NotEmpty(t, failure.GetSubjectRef())
			}
		})
	}
}

func assertVerificationFailure(t *testing.T, report *compliancev1.ComplianceVerificationReport, code error, subject string) {
	t.Helper()
	for _, failure := range report.GetFailures() {
		if failure.GetCode() == code.Error() && failure.GetSubjectRef() == subject {
			return
		}
	}
	assert.Fail(t, "expected verification failure", "code=%q subject=%q failures=%v", code.Error(), subject, report.GetFailures())
}
