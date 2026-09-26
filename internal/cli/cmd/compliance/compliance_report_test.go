// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliancecmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func complianceReportSigningFixtureForTest(t *testing.T, scopeID string) (*compliancereport.ComplianceReportSigningIdentity, *compliancev1.ComplianceReportTrustPolicy, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	digest := sha256.Sum256(publicKey)
	createdAt := time.Now().UTC().Add(-time.Hour)
	metadata := &compliancev1.ComplianceReportSigningKeyMetadata{
		KeyId:           "report-key-1",
		Algorithm:       constants.ComplianceReportSignatureAlgorithm,
		Purpose:         constants.ComplianceReportSigningPurpose,
		PublicKeySha256: hex.EncodeToString(digest[:]),
		CreatedAt:       timestamppb.New(createdAt),
		ExpiresAt:       timestamppb.New(createdAt.Add(24 * time.Hour)),
	}
	identity, err := compliancereport.NewComplianceReportSigningIdentity(metadata, privateKey)
	require.NoError(t, err)
	policy := &compliancev1.ComplianceReportTrustPolicy{
		PolicyId:      "policy-1",
		PolicyVersion: "1.0.0",
		TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{
			Metadata:         metadata,
			PublicKey:        hex.EncodeToString(publicKey),
			AssessmentId:     "assessment-1",
			AssessorIdentity: "assessor-1",
			AssessedAt:       timestamppb.New(createdAt),
			AllowedScopeRefs: []string{scopeID},
		}},
	}
	return identity, policy, privateKey
}

func complianceReportSigningIdentityForTest(t *testing.T) *compliancereport.ComplianceReportSigningIdentity {
	identity, _, _ := complianceReportSigningFixtureForTest(t, evidence.EvalScopeID("evidence-graph-suite"))
	return identity
}

func complianceReportSigningIdentityLoaderForTest(t *testing.T) complianceReportSigningIdentityLoader {
	t.Helper()
	identity := complianceReportSigningIdentityForTest(t)
	return func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
		return identity, nil
	}
}

func writeComplianceAssessmentScopeForTest(t *testing.T, scopeID string, runIDs []string, assessmentAsOf time.Time) string {
	t.Helper()
	admissions := make([]*compliancev1.AssessmentSourceAdmission, 0, len(runIDs))
	for index, runID := range runIDs {
		admissions = append(admissions, &compliancev1.AssessmentSourceAdmission{
			AdmissionId:              fmt.Sprintf("source-%d", index+1),
			SourceKind:               constants.EvaluationSourceKindNative,
			SourceVersion:            constants.EvaluationSourceVersion,
			SourceScopeId:            scopeID,
			OwnerRuntimeBoundary:     "evaluation-owner",
			AcquisitionBoundary:      "owner-local-run",
			RunId:                    runID,
			VerifierRef:              &compliancev1.VersionedReference{Id: constants.EvalRunVerifierID, Version: constants.EvalRunVerifierVersion},
			DisclosureClassification: constants.ComplianceBundleProfileRestricted,
		})
	}
	scope := &compliancev1.AssessmentScope{
		ScopeId:               scopeID,
		OrganizationId:        "org-1",
		DeploymentId:          "deployment-1",
		ProductVersion:        "2.1.12",
		BuildIdentity:         "build-1",
		SourceRevision:        "revision-1",
		ComponentInventory:    []*compliancev1.ComponentInventoryEntry{{ComponentId: "operator-1", ComponentType: "operator", Version: "2.1.12", Digest: strings.Repeat("a", 64)}},
		NetworkTopologyHash:   strings.Repeat("b", 64),
		ConfigurationHashes:   []*compliancev1.NamedDigest{{Name: "operator-1", Sha256: strings.Repeat("c", 64)}},
		DoctrineBundleHashes:  []*compliancev1.NamedDigest{{Name: "doctrine", Sha256: strings.Repeat("d", 64)}},
		TrustAnchorIds:        []string{"root-1"},
		CryptographicMode:     "standard",
		AssessmentWindowStart: timestamppb.New(assessmentAsOf.Add(-time.Hour)),
		AssessmentWindowEnd:   timestamppb.New(assessmentAsOf),
		ActivePosture:         constants.PostureDoctrine,
		SourceAdmissions:      admissions,
		Applicability:         &compliancev1.AssessmentApplicabilitySelection{Components: []string{"operator"}, ActionClasses: []string{"governed_mutation"}, Arms: []string{"governed"}},
		SelectedPopulation:    &compliancev1.AssessmentPopulationSelection{},
		AssessmentAsOf:        timestamppb.New(assessmentAsOf),
	}
	body, err := compliancev1.MarshalCanonical(scope)
	require.NoError(t, err)
	scopePath := filepath.Join(t.TempDir(), constants.ComplianceBundleScopeFilename)
	require.NoError(t, os.WriteFile(scopePath, body, constants.PermFilePrivate))
	return scopePath
}

func markAssessmentContextUnavailable(scope *compliancev1.AssessmentScope) {
	scope.BuildIdentity = ""
	scope.SourceRevision = ""
	scope.ImageDigests = nil
	scope.ComponentInventory = nil
	scope.NetworkTopologyHash = ""
	scope.ConfigurationHashes = nil
	scope.DoctrineBundleHashes = nil
	scope.TrustAnchorIds = nil
	scope.UnavailableContext = []*compliancev1.UnavailableAssessmentContext{
		{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_BUILD_IDENTITY, Reason: "build provenance was not captured"},
		{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_SOURCE_REVISION, Reason: "source revision evidence was not captured"},
		{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_COMPONENT_INVENTORY, Reason: "component inventory evidence was not captured"},
		{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_NETWORK_TOPOLOGY, Reason: "network topology evidence was not captured"},
		{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_CONFIGURATION, Reason: "configuration evidence was not captured"},
		{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_DOCTRINE_BUNDLES, Reason: "doctrine bundle evidence was not captured"},
		{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_TRUST_ANCHORS, Reason: "trust-anchor evidence was not captured"},
	}
}

func configureComplianceReportGenerateCommand(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	require.NoError(t, cmd.Flags().Set("report-id", "report-1"))
	require.NoError(t, cmd.Flags().Set("signing-metadata", constants.ComplianceReportSigningMetadataTestFilename))
	require.NoError(t, cmd.Flags().Set("signing-private-key", constants.ComplianceReportSigningPrivateKeyTestFilename))
}

func runComplianceReportGenerateCommand(t *testing.T, evalRuns []string) ([]byte, error) {
	t.Helper()
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t), time.Now)
	configureComplianceReportGenerateCommand(t, cmd)
	assessmentAsOf := time.Unix(1_700_000_100, 0).UTC()
	scopePath := writeComplianceAssessmentScopeForTest(t, evidence.EvalScopeID("evidence-graph-suite"), evalRuns, assessmentAsOf)
	require.NoError(t, cmd.Flags().Set("scope", scopePath))
	for _, runID := range evalRuns {
		require.NoError(t, cmd.Flags().Set("eval-run", runID))
	}
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	return bytes.TrimSpace(output.Bytes()), cmd.RunE(cmd, nil)
}

func TestComplianceReportGenerateCmdWithConfig_DefaultsToRestrictedProfile(t *testing.T) {
	cmd := complianceReportGenerateCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t), time.Now)

	profile := cmd.Flags().Lookup("profile")
	require.NotNil(t, profile)
	assert.Equal(t, string(compliancereport.ProfileRestricted), profile.DefValue)
}

func TestComplianceReportCmd_ContainsGenerateAndVerifySubcommands(t *testing.T) {
	cmd := complianceReportCmd()
	require.Len(t, cmd.Commands(), 2)
	assert.Equal(t, "generate", cmd.Commands()[0].Name())
	assert.Equal(t, "verify", cmd.Commands()[1].Name())
}

func TestBuildStandaloneReportSources_ProtectsReplayableLedgerAndBuildConfigurationBytes(t *testing.T) {
	root := t.TempDir()
	commitsPath := filepath.Join(root, constants.LedgerCommitsFilename)
	statePath := filepath.Join(root, constants.LedgerStateFilename)
	buildPath := filepath.Join(root, constants.BuildConfigAttestationsFilename)
	commitsBody := []byte(`{"schema_version":"1.0.0","producer_identity":"gateway-1","commit_hash":"1111111111111111111111111111111111111111","parent_hash":"","timestamp_utc":"2026-09-06T10:00:00Z","message":"bootstrap","files_changed":1,"diff_stat":"1 file changed"}
{"schema_version":"1.0.0","producer_identity":"gateway-1","commit_hash":"2222222222222222222222222222222222222222","parent_hash":"1111111111111111111111111111111111111111","timestamp_utc":"2026-09-06T10:01:00Z","message":"mutation","files_changed":1,"diff_stat":"1 file changed"}
`)
	stateBody := []byte(`{"schema_version":"1.0.0","producer_identity":"gateway-1","merkle_root":"2222222222222222222222222222222222222222","captured_at_utc":"2026-09-06T10:02:00Z"}`)
	buildBody := []byte(`{"schema_version":"1.0.0","attestation_type":"build","producer_identity":"build-system-1","produced_at_utc":"2026-09-06T10:00:00Z","scope_id":"scope-1","run_id":"build-run-1","build_identity":"build-1","source_revision":"revision-1","image_digests":[{"name":"gateway","sha256":"1111111111111111111111111111111111111111111111111111111111111111"}],"component_inventory":[{"component_id":"gateway","component_type":"service","version":"2.1.7","digest":"2222222222222222222222222222222222222222222222222222222222222222"}]}
{"schema_version":"1.0.0","attestation_type":"configuration","producer_identity":"build-system-1","produced_at_utc":"2026-09-06T10:01:00Z","scope_id":"scope-1","run_id":"build-run-1","build_identity":"build-1","source_revision":"revision-1","configuration_hashes":[{"name":"gateway","sha256":"3333333333333333333333333333333333333333333333333333333333333333"}]}
`)
	require.NoError(t, os.WriteFile(commitsPath, commitsBody, constants.PermFileReadOnly))
	require.NoError(t, os.WriteFile(statePath, stateBody, constants.PermFileReadOnly))
	require.NoError(t, os.WriteFile(buildPath, buildBody, constants.PermFileReadOnly))

	importers, artifacts, err := buildStandaloneReportSources(context.Background(), standaloneReportSourceInput{scopeID: "scope-1", ledgerRunID: "ledger-run-1", ledgerCommits: commitsPath, ledgerState: statePath, buildRunID: "build-run-1", buildConfig: buildPath})

	require.NoError(t, err)
	require.Len(t, importers, 2)
	assert.Len(t, artifacts, 3)
	ledgerNodes, err := importers[0].Import(context.Background())
	require.NoError(t, err)
	assert.Len(t, ledgerNodes, 3)
	buildNodes, err := importers[1].Import(context.Background())
	require.NoError(t, err)
	assert.Len(t, buildNodes, 2)
	assert.Equal(t, commitsBody, artifacts[0].Body)
	assert.Equal(t, stateBody, artifacts[1].Body)
	assert.Equal(t, buildBody, artifacts[2].Body)
}

func TestBuildOperationalReportSources_BindsProtectedAdmissionAndBundlePaths(t *testing.T) {
	assessmentAsOf := time.UnixMilli(1_700_000_100_000).UTC()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signerKeyID := hex.EncodeToString(publicKey)
	receipt := &operatorv1.ActionReceipt{TransactionId: "transaction-1", TransactionHash: "hash-1", SignerKeyId: signerKeyID, ExecutedAtUnixMs: assessmentAsOf.Add(-time.Minute).UnixMilli()}
	payload, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	receiptBody, err := compliancev1.MarshalCanonical(receipt)
	require.NoError(t, err)
	sourceDir := t.TempDir()
	_, err = evidence.ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{Receipts: []storage.OperationalReceiptSource{{TransactionID: receipt.GetTransactionId(), ExecutedAt: time.UnixMilli(receipt.GetExecutedAtUnixMs()), Body: receiptBody}}}, evidence.OperationalExportRequest{
		ScopeID:              "scope-1",
		AdmissionID:          "source-1",
		SourceKind:           "operator-audit",
		SourceVersion:        "1.0.0",
		SourceScopeID:        "operator-scope-1",
		OwnerRuntimeBoundary: "operator-1",
		AcquisitionBoundary:  "operator-local-export",
		RunID:                "run-1",
		VerifierID:           "operational-export",
		VerifierVersion:      "1.0.0",
		WindowStart:          assessmentAsOf.Add(-time.Hour),
		WindowEnd:            assessmentAsOf,
		MaxRows:              10,
		OutputDir:            sourceDir,
	})
	require.NoError(t, err)
	admission := &compliancev1.AssessmentSourceAdmission{AdmissionId: "source-1", SourceKind: "operator-audit", SourceVersion: "1.0.0", SourceScopeId: "operator-scope-1", OwnerRuntimeBoundary: "operator-1", AcquisitionBoundary: "operator-local-export", RunId: "run-1", VerifierRef: &compliancev1.VersionedReference{Id: "operational-export", Version: "1.0.0"}}
	scope := &compliancev1.AssessmentScope{ScopeId: "scope-1", SourceAdmissions: []*compliancev1.AssessmentSourceAdmission{admission}}
	trust := &assessedEvidenceTrust{keys: map[string]ed25519.PublicKey{signerKeyID: publicKey}}

	sources, artifacts, err := buildOperationalReportSources(context.Background(), []string{sourceDir}, scope, trust, assessmentAsOf)

	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "source-1", sources[0].AdmissionID)
	require.Len(t, artifacts, 2)
	for _, artifact := range artifacts {
		assert.True(t, strings.HasPrefix(artifact.BundlePath, path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1")))
	}
	nodes, err := sources[0].Importer.Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, evidence.VerificationStatusVerified, nodes[0].VerificationStatus)
	assert.True(t, strings.HasPrefix(nodes[0].BundlePath, path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1")))
}

type standaloneAttestationRecord struct {
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

func TestBuildStandaloneReportSources_ProtectsEveryPlatformSourceClass(t *testing.T) {
	root := t.TempDir()
	scopeID := "scope-1"
	verifiedAt := time.Date(2026, 9, 6, 10, 30, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	keyID := hex.EncodeToString(publicKey)
	trust := &assessedEvidenceTrust{keys: map[string]ed25519.PublicKey{keyID: publicKey}}

	binding := compliance.EvaluationBinding{ScopeID: scopeID, RunID: "ksi-run-1", WindowStartUnixMs: verifiedAt.Add(-time.Hour).UnixMilli(), WindowEndUnixMs: verifiedAt.UnixMilli(), EvaluatorID: constants.KSIEvaluatorID, EvaluatorVersion: constants.KSIEvaluatorVersion, MethodDefinitionID: constants.KSIMethodDefinitionVersion, AssertionAssessments: compliance.AssertionAssessmentScope{AssessmentIDs: []string{"assessment-1"}, AttemptIDs: []string{"attempt-1"}, ScenarioIDs: []string{"scenario-1"}, ActionIDs: []string{"action-1"}}}
	ksi := compliance.KSIResultSet{Class: compliance.ClassC, EvaluatedAtMs: verifiedAt.UnixMilli(), Binding: binding, Results: []compliance.KSIResult{{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied, Outcome: compliance.KSIOutcomeSatisfied, LastValidatedUnixMs: verifiedAt.Add(-time.Second).UnixMilli(), MethodCount: 1, Binding: binding}}}
	ksiResultsBody, err := json.Marshal(ksi)
	require.NoError(t, err)
	ksiHistoryBody := append(append([]byte(nil), ksiResultsBody...), '\n')
	ksiHistoryPath := filepath.Join(root, constants.ComplianceBundleKSIHistoryFilename)
	ksiResultsPath := filepath.Join(root, constants.ComplianceBundleKSIResultsFilename)
	require.NoError(t, os.WriteFile(ksiHistoryPath, ksiHistoryBody, constants.PermFileReadOnly))
	require.NoError(t, os.WriteFile(ksiResultsPath, ksiResultsBody, constants.PermFileReadOnly))

	commitment := &operatorv1.CommitmentAttestation{TransactionId: "transaction-1", TransactionHash: strings.Repeat("1", 64), PriorCommitmentHash: strings.Repeat("2", 64), StateRootAtCommit: strings.Repeat("3", 64), L2SignatureDigest: strings.Repeat("4", 64), WardenIntentSignatureDigest: strings.Repeat("5", 64), HumanSignatureDigest: strings.Repeat("6", 64), ActionType: "FILE_EDIT", TargetResource: constants.DemosTargetDataDir, CommittedAtUnixMs: verifiedAt.Add(-time.Second).UnixMilli(), AuditorKeyId: keyID}
	commitmentPayload, err := governance.CanonicalizeCommitmentAttestation(commitment)
	require.NoError(t, err)
	commitmentDigest := sha256.Sum256(commitmentPayload)
	commitment.Hash = hex.EncodeToString(commitmentDigest[:])
	commitment.Signature = hex.EncodeToString(ed25519.Sign(privateKey, commitmentPayload))
	commitmentBody, err := compliancev1.MarshalCanonical(commitment)
	require.NoError(t, err)
	commitmentPath := filepath.Join(root, constants.ComplianceBundleCommitmentsFilename)
	require.NoError(t, os.WriteFile(commitmentPath, commitmentBody, constants.PermFileReadOnly))

	attestations := []standaloneAttestationRecord{
		{SchemaVersion: constants.AttestationSchemaVersion, AttestationID: "customer-attestation-1", AttesterType: "customer", AttesterIdentity: "customer-1", SignerKeyID: keyID, IssuedAtUTC: "2026-09-06T10:00:00Z", ValidFromUTC: "2026-09-06T10:00:00Z", ValidUntilUTC: "2026-09-06T11:00:00Z", ScopeID: scopeID, RunID: "attestation-run-1", AssertionIDs: []string{"assertion-1"}, Statement: "Customer-operated control is in effect."},
		{SchemaVersion: constants.AttestationSchemaVersion, AttestationID: "assessor-attestation-1", AttesterType: "assessor", AttesterIdentity: "assessor-1", SignerKeyID: keyID, IssuedAtUTC: "2026-09-06T10:01:00Z", ValidFromUTC: "2026-09-06T10:01:00Z", ValidUntilUTC: "2026-09-06T11:00:00Z", ScopeID: scopeID, RunID: "attestation-run-1", AssertionIDs: []string{"assertion-2"}, Statement: "Assessor reviewed the declared evidence."},
	}
	attestationLines := make([][]byte, 0, len(attestations))
	for index := range attestations {
		payload, err := json.Marshal(attestations[index])
		require.NoError(t, err)
		attestations[index].Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
		line, err := json.Marshal(attestations[index])
		require.NoError(t, err)
		attestationLines = append(attestationLines, line)
	}
	attestationBody := append(bytes.Join(attestationLines, []byte{'\n'}), '\n')
	attestationPath := filepath.Join(root, constants.ComplianceBundleAttestationsFilename)
	require.NoError(t, os.WriteFile(attestationPath, attestationBody, constants.PermFileReadOnly))

	auditBody, err := compliancev1.MarshalCanonical(&operatorv1.AuditEvent{Id: 41, OperatorSessionId: "session-1", Timestamp: "2026-09-06T10:00:00Z", Type: string(constants.EventAppTaskCompleted)})
	require.NoError(t, err)
	auditPath := filepath.Join(root, constants.AuditRecordTestFilename)
	require.NoError(t, os.WriteFile(auditPath, auditBody, constants.PermFileReadOnly))

	commitsPath := filepath.Join(root, constants.LedgerCommitsFilename)
	statePath := filepath.Join(root, constants.LedgerStateFilename)
	buildPath := filepath.Join(root, constants.BuildConfigAttestationsFilename)
	commitsBody := []byte(`{"schema_version":"1.0.0","producer_identity":"gateway-1","commit_hash":"1111111111111111111111111111111111111111","parent_hash":"","timestamp_utc":"2026-09-06T10:00:00Z","message":"bootstrap","files_changed":1,"diff_stat":"1 file changed"}
{"schema_version":"1.0.0","producer_identity":"gateway-1","commit_hash":"2222222222222222222222222222222222222222","parent_hash":"1111111111111111111111111111111111111111","timestamp_utc":"2026-09-06T10:01:00Z","message":"mutation","files_changed":1,"diff_stat":"1 file changed"}
`)
	stateBody := []byte(`{"schema_version":"1.0.0","producer_identity":"gateway-1","merkle_root":"2222222222222222222222222222222222222222","captured_at_utc":"2026-09-06T10:02:00Z"}`)
	buildBody := []byte(`{"schema_version":"1.0.0","attestation_type":"build","producer_identity":"build-system-1","produced_at_utc":"2026-09-06T10:00:00Z","scope_id":"scope-1","run_id":"build-run-1","build_identity":"build-1","source_revision":"revision-1","image_digests":[{"name":"gateway","sha256":"1111111111111111111111111111111111111111111111111111111111111111"}],"component_inventory":[{"component_id":"gateway","component_type":"service","version":"2.1.7","digest":"2222222222222222222222222222222222222222222222222222222222222222"}]}
{"schema_version":"1.0.0","attestation_type":"configuration","producer_identity":"build-system-1","produced_at_utc":"2026-09-06T10:01:00Z","scope_id":"scope-1","run_id":"build-run-1","build_identity":"build-1","source_revision":"revision-1","configuration_hashes":[{"name":"gateway","sha256":"3333333333333333333333333333333333333333333333333333333333333333"}]}
`)
	require.NoError(t, os.WriteFile(commitsPath, commitsBody, constants.PermFileReadOnly))
	require.NoError(t, os.WriteFile(statePath, stateBody, constants.PermFileReadOnly))
	require.NoError(t, os.WriteFile(buildPath, buildBody, constants.PermFileReadOnly))

	importers, artifacts, err := buildStandaloneReportSources(context.Background(), standaloneReportSourceInput{scopeID: scopeID, verifiedAt: verifiedAt, evidenceTrust: trust, ksiRunID: "ksi-run-1", ksiHistory: ksiHistoryPath, ksiResults: ksiResultsPath, commitmentRunID: "commitment-run-1", commitment: commitmentPath, attestationRunID: "attestation-run-1", attestations: attestationPath, auditRunID: "audit-run-1", auditRecords: []string{auditPath}, ledgerRunID: "ledger-run-1", ledgerCommits: commitsPath, ledgerState: statePath, buildRunID: "build-run-1", buildConfig: buildPath})

	require.NoError(t, err)
	require.Len(t, importers, 6)
	assert.Len(t, artifacts, 8)
	types := make(map[evidence.ArtifactType]int)
	for _, importer := range importers {
		nodes, err := importer.Import(context.Background())
		require.NoError(t, err)
		for _, node := range nodes {
			types[node.ArtifactType]++
		}
	}
	assert.Equal(t, 1, types[evidence.ArtifactTypeKSIResult])
	assert.Equal(t, 1, types[evidence.ArtifactTypeCommitment])
	assert.Equal(t, 1, types[evidence.ArtifactTypeCustomerAttestation])
	assert.Equal(t, 1, types[evidence.ArtifactTypeAssessorAttestation])
	assert.Equal(t, 1, types[evidence.ArtifactTypeAuditRecord])
	assert.Equal(t, 2, types[evidence.ArtifactTypeLedgerCommit])
	assert.Equal(t, 1, types[evidence.ArtifactTypeLedgerState])
	assert.Equal(t, 1, types[evidence.ArtifactTypeBuildAttestation])
	assert.Equal(t, 1, types[evidence.ArtifactTypeConfigAttestation])

	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	reportPublicKey, reportPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	reportKeyDigest := sha256.Sum256(reportPublicKey)
	reportGeneratedAt := time.Now().UTC()
	reportMetadata := &compliancev1.ComplianceReportSigningKeyMetadata{KeyId: "complete-report-key-1", Algorithm: constants.ComplianceReportSignatureAlgorithm, Purpose: constants.ComplianceReportSigningPurpose, PublicKeySha256: hex.EncodeToString(reportKeyDigest[:]), CreatedAt: timestamppb.New(reportGeneratedAt.Add(-time.Hour)), ExpiresAt: timestamppb.New(reportGeneratedAt.Add(time.Hour))}
	reportIdentity, err := compliancereport.NewComplianceReportSigningIdentity(reportMetadata, reportPrivateKey)
	require.NoError(t, err)
	reportPolicy := &compliancev1.ComplianceReportTrustPolicy{PolicyId: "complete-report-policy-1", PolicyVersion: "1.0.0", TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{Metadata: reportMetadata, PublicKey: hex.EncodeToString(reportPublicKey), AssessmentId: "report-assessment-1", AssessorIdentity: "assessor-1", AssessedAt: timestamppb.New(reportGeneratedAt.Add(-time.Hour)), AllowedScopeRefs: []string{scopeID}}}}
	publicKeyDigest := sha256.Sum256(publicKey)
	evidencePolicy := &compliancev1.ComplianceEvidenceTrustPolicy{
		PolicyId:      "evidence-policy-1",
		PolicyVersion: "1.0.0",
		TrustedKeys: []*compliancev1.ComplianceEvidenceTrustedKey{{
			KeyId:            keyID,
			PublicKey:        hex.EncodeToString(publicKey),
			PublicKeySha256:  hex.EncodeToString(publicKeyDigest[:]),
			AssessmentId:     "evidence-assessment-1",
			AssessorIdentity: "assessor-1",
			AssessedAt:       timestamppb.New(verifiedAt.Add(-time.Hour)),
			ValidFrom:        timestamppb.New(verifiedAt.Add(-time.Hour)),
			ValidUntil:       timestamppb.New(verifiedAt.Add(time.Hour)),
			AllowedScopeRefs: []string{scopeID},
		}},
	}
	evidencePolicyBody, err := compliancev1.MarshalCanonical(evidencePolicy)
	require.NoError(t, err)
	evidencePolicyPath := filepath.Join(root, constants.ComplianceEvidenceTrustPolicyTestFilename)
	require.NoError(t, os.WriteFile(evidencePolicyPath, evidencePolicyBody, constants.PermFileReadOnly))
	generateCmd := complianceReportGenerateCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
		return reportIdentity, nil
	}, func() time.Time { return reportGeneratedAt })
	configureComplianceReportGenerateCommand(t, generateCmd)
	scopePath := writeComplianceAssessmentScopeForTest(t, scopeID, []string{"ksi-run-1", "commitment-run-1", "attestation-run-1", "audit-run-1", "ledger-run-1", "build-run-1"}, verifiedAt)
	require.NoError(t, generateCmd.Flags().Set("scope", scopePath))
	for name, value := range map[string]string{
		"evidence-trust": evidencePolicyPath, "ksi-run-id": "ksi-run-1", "ksi-history": ksiHistoryPath, "ksi-results": ksiResultsPath,
		"commitment-run-id": "commitment-run-1", "commitment": commitmentPath, "attestation-run-id": "attestation-run-1", "attestations": attestationPath,
		"audit-run-id": "audit-run-1", "audit-record": auditPath, "ledger-run-id": "ledger-run-1", "ledger-commits": commitsPath, "ledger-state": statePath,
		"build-run-id": "build-run-1", "build-config-attestations": buildPath,
	} {
		require.NoError(t, generateCmd.Flags().Set(name, value))
	}
	var generated bytes.Buffer
	generateCmd.SetOut(&generated)
	require.NoError(t, generateCmd.RunE(generateCmd, nil))
	descriptorPath := string(bytes.TrimSpace(generated.Bytes()))

	reportPolicyBody, err := compliancev1.MarshalCanonical(reportPolicy)
	require.NoError(t, err)
	reportPolicyPath := filepath.Join(root, constants.ComplianceReportTrustPolicyTestFilename)
	require.NoError(t, os.WriteFile(reportPolicyPath, reportPolicyBody, constants.PermFileReadOnly))
	verifyCmd := complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, func() time.Time { return reportGeneratedAt.Add(time.Minute) })
	require.NoError(t, verifyCmd.Flags().Set("trust-policy", reportPolicyPath))
	require.NoError(t, verifyCmd.Flags().Set("evidence-trust", evidencePolicyPath))
	var verified bytes.Buffer
	verifyCmd.SetOut(&verified)
	require.NoError(t, verifyCmd.RunE(verifyCmd, []string{descriptorPath}), verified.String())
	verificationReport := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(bytes.TrimSpace(verified.Bytes()), verificationReport))
	assert.True(t, verificationReport.GetValid(), verificationReport.GetFailures())
}

func TestComplianceReportGenerateCmdWithConfig_PersistedDemoSourceMutationsFailIndependentReplay(t *testing.T) {
	tests := []struct {
		name         string
		relativePath string
		body         []byte
	}{
		{name: "verification report", relativePath: constants.ComplianceBundleSourceVerificationFilename, body: []byte(`{}`)},
		{name: "run manifest", relativePath: path.Join(constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunManifestFilename), body: []byte(`{}`)},
		{name: "scenario results", relativePath: path.Join(constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunResultsFilename), body: []byte("{}\n")},
		{name: "provenance artifact", relativePath: path.Join(constants.ComplianceBundleSourceProvenanceDirname, constants.ComplianceBundleSourceArtifactsDirname, constants.DemosComposeFile), body: []byte("services: {tampered: true}")},
		{name: "scenario definitions", relativePath: path.Join(constants.ComplianceBundleSourceProvenanceDirname, constants.DemoRunDefinitionsFilename), body: []byte("{}\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fileSvc, _ := cmdtest.NewCmdTestEnv(t)
			projectRoot := writeDemoProvenanceTree(t)
			runID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)
			scopeID := constants.DemoScopeFedRAMP
			identity, policy, _ := complianceReportSigningFixtureForTest(t, scopeID)
			cmd := complianceReportGenerateCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), func(string) evidence.ProvenanceSource {
				return evidence.NewDemoDirectoryProvenanceSource(projectRoot)
			}, func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
				return identity, nil
			}, time.Now)
			configureComplianceReportGenerateCommand(t, cmd)
			assessmentAsOf := time.Unix(1_700_000_100, 0).UTC()
			scopePath := writeComplianceAssessmentScopeForTest(t, scopeID, []string{runID}, assessmentAsOf)
			require.NoError(t, cmd.Flags().Set("scope", scopePath))
			require.NoError(t, cmd.Flags().Set("demo-run", runID))
			var output bytes.Buffer
			cmd.SetOut(&output)
			require.NoError(t, cmd.RunE(cmd, nil))
			descriptorAbsolutePath := string(bytes.TrimSpace(output.Bytes()))
			trustBody, err := compliancev1.MarshalCanonical(policy)
			require.NoError(t, err)
			trustPath := filepath.Join(t.TempDir(), constants.ComplianceReportTrustPolicyTestFilename)
			require.NoError(t, os.WriteFile(trustPath, trustBody, constants.PermFilePublic))
			verificationTime := time.Now().UTC().Add(time.Minute)
			verifyCmd := complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, func() time.Time { return verificationTime })
			require.NoError(t, verifyCmd.Flags().Set("trust-policy", trustPath))
			verifyCmd.SetOut(io.Discard)
			require.NoError(t, verifyCmd.RunE(verifyCmd, []string{descriptorAbsolutePath}))
			descriptorPath, err := fileSvc.Rel(descriptorAbsolutePath)
			require.NoError(t, err)
			descriptorBody, err := fileSvc.ReadFile(context.Background(), descriptorPath)
			require.NoError(t, err)
			bundle := &compliancev1.ComplianceReportBundle{}
			require.NoError(t, compliancev1.UnmarshalCanonical(descriptorBody, bundle))
			bundleDir := path.Dir(descriptorPath)
			sourcePath := path.Join(bundleDir, constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, test.relativePath)
			require.NoError(t, fileSvc.WriteFile(context.Background(), sourcePath, test.body, constants.PermFileReadOnly))
			root, err := os.OpenRoot(fileSvc.Resolve(bundleDir))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, root.Close()) })

			report, err := compliancereport.VerifyComplianceReportBundle(context.Background(), compliancereport.BundleVerificationRequest{
				Bundle:      bundle,
				Reader:      &complianceBundleRootReader{root: root},
				TrustPolicy: policy,
				VerifiedAt:  verificationTime,
			})

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertComplianceVerificationFailure(t, report, constants.ErrDemoRunVerificationFailed, path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceVerificationFilename))
		})
	}
}

func TestBuildDemoVerificationArtifacts_EmbedsValidExistingVerifierResult(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	projectRoot := writeDemoProvenanceTree(t)
	runID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)
	verifiedAt := time.Unix(1_800_000_000, 0).UTC()

	artifacts, err := buildDemoVerificationArtifacts(context.Background(), fileSvc, evidence.NewDemoDirectoryProvenanceSource(projectRoot), []string{runID}, verifiedAt)

	require.NoError(t, err)
	artifactByPath := make(map[string]compliancereport.SourceArtifact, len(artifacts))
	for _, artifact := range artifacts {
		artifactByPath[artifact.BundlePath] = artifact
	}
	verificationPath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceVerificationFilename)
	report := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(artifactByPath[verificationPath].Body, report))
	assert.True(t, report.GetValid())
	assert.Equal(t, constants.DemoRunVerifierID, report.GetVerifierId())
	assert.NotEmpty(t, artifactByPath[path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunManifestFilename)].Body)
	assert.NotEmpty(t, artifactByPath[path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunResultsFilename)].Body)
	provenanceArtifacts, err := evidence.NewDemoDirectoryProvenanceSource(projectRoot).Artifacts(context.Background(), constants.DemosOrgFedRAMP)
	require.NoError(t, err)
	for _, expected := range provenanceArtifacts {
		bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceProvenanceDirname, constants.ComplianceBundleSourceArtifactsDirname, filepath.ToSlash(expected.Name))
		assert.Equal(t, expected.Body, artifactByPath[bundlePath].Body)
	}
	assert.NotEmpty(t, artifactByPath[path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceProvenanceDirname, constants.DemoRunDefinitionsFilename)].Body)
}

func TestComplianceReportGenerateCmdWithConfig_RejectsUnsupportedProfile(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t), time.Now)
	configureComplianceReportGenerateCommand(t, cmd)
	scopePath := writeComplianceAssessmentScopeForTest(t, "scope-1", []string{"run-1"}, time.Unix(1_700_000_001, 0).UTC())
	require.NoError(t, cmd.Flags().Set("scope", scopePath))
	require.NoError(t, cmd.Flags().Set("eval-run", "run-1"))
	require.NoError(t, cmd.Flags().Set("profile", "confidential"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrBundleProfileUnsupported)
}

func TestComplianceReportGenerateCmdWithConfig_RejectsMissingEvidenceRuns(t *testing.T) {
	body, err := runComplianceReportGenerateCommand(t, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Empty(t, body)
}

func TestComplianceReportGenerateCmdWithConfig_FailsClosedOnImporterFailure(t *testing.T) {
	body, err := runComplianceReportGenerateCommand(t, []string{"missing-eval-run"})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.Empty(t, body)
}

func TestComplianceReportGenerateCmdWithConfig_RejectsIncompleteCampaignSource(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	runID := "campaign-run"
	require.NoError(t, evaluation.NewStore(fileSvc).SaveRun(context.Background(), &evalv1.EvaluationRun{
		SchemaVersion: evaluation.CampaignSchemaVersion,
		RunId:         runID,
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId: "campaign-1",
		},
	}))
	cmd := complianceReportGenerateCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t), time.Now)
	configureComplianceReportGenerateCommand(t, cmd)
	scopePath := writeComplianceAssessmentScopeForTest(t, "scope-1", []string{runID}, time.Unix(1_700_000_001, 0).UTC())
	scopeBody, err := os.ReadFile(scopePath)
	require.NoError(t, err)
	scope := &compliancev1.AssessmentScope{}
	require.NoError(t, compliancev1.UnmarshalCanonical(scopeBody, scope))
	scope.SourceAdmissions[0].SourceKind = constants.EvaluationSourceKindCampaign
	scope.SourceAdmissions[0].SourceVersion = constants.EvaluationSourceVersion
	scope.SourceAdmissions[0].VerifierRef = &compliancev1.VersionedReference{Id: constants.CampaignVerifierID, Version: constants.CampaignVerifierVersion}
	scopeBody, err = compliancev1.MarshalCanonical(scope)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(scopePath, scopeBody, constants.PermFileReadOnly))
	require.NoError(t, cmd.Flags().Set("scope", scopePath))
	require.NoError(t, cmd.Flags().Set("eval-run", runID))

	err = cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, constants.ErrUnsupportedVerifier)
	assert.Contains(t, err.Error(), "campaign")
}

func TestComplianceReportGenerateCmdWithConfig_CampaignSourceVerifiesOffline(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	req := cmdtest.EvaluationTestCampaignInitRequest(t)
	controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	_, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	assessmentAsOf := time.Now().UTC()
	require.NoError(t, evaluation.NewStore(fileSvc).SaveReport(context.Background(), &evalv1.EvaluationReport{
		SchemaVersion: evaluation.RegistryVersion,
		Run: &evalv1.EvaluationRun{
			SchemaVersion: evaluation.RegistryVersion,
			RunId:         "unsupported-candidate",
			SuiteRef:      &compliancev1.VersionedReference{Id: "unsupported-suite", Version: "1.0.0"},
		},
	}))
	require.NoError(t, evaluation.NewStore(fileSvc).SaveReport(context.Background(), &evalv1.EvaluationReport{
		SchemaVersion: evaluation.RegistryVersion,
		Run: &evalv1.EvaluationRun{
			SchemaVersion: evaluation.RegistryVersion,
			RunId:         "outside-window-candidate",
			SuiteRef:      &compliancev1.VersionedReference{Id: evaluation.CoreExecutionBoundarySuiteID, Version: evaluation.CoreExecutionBoundarySuiteVersion},
			StartedAt:     timestamppb.New(time.Unix(1_600_000_000, 0).UTC()),
			CompletedAt:   timestamppb.New(time.Unix(1_600_000_100, 0).UTC()),
		},
	}))
	require.NoError(t, fileSvc.MkdirAll(context.Background(), path.Join(constants.DataDirname, constants.EvaluationDirname, constants.EvaluationRunsDirname, "incomplete-candidate"), constants.PermDirStandard))
	scopeID := constants.EvalScopePrefix + req.CampaignID
	identity, policy, _ := complianceReportSigningFixtureForTest(t, scopeID)
	cmd := complianceReportGenerateCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
		return identity, nil
	}, func() time.Time { return assessmentAsOf })
	configureComplianceReportGenerateCommand(t, cmd)
	scopePath := writeComplianceAssessmentScopeForTest(t, scopeID, []string{req.RunID}, assessmentAsOf)
	scopeBody, err := os.ReadFile(scopePath)
	require.NoError(t, err)
	scope := &compliancev1.AssessmentScope{}
	require.NoError(t, compliancev1.UnmarshalCanonical(scopeBody, scope))
	scope.SourceAdmissions[0].SourceKind = constants.EvaluationSourceKindCampaign
	scope.SourceAdmissions[0].VerifierRef = &compliancev1.VersionedReference{Id: constants.CampaignVerifierID, Version: constants.CampaignVerifierVersion}
	scope.SourceAdmissions[0].ProviderObservationPolicy = compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_STRICT
	scope.SourceAdmissions[0].ModelProvenancePolicy = compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_STRICT
	markAssessmentContextUnavailable(scope)
	scopeBody, err = compliancev1.MarshalCanonical(scope)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(scopePath, scopeBody, constants.PermFilePrivate))
	require.NoError(t, cmd.Flags().Set("scope", scopePath))
	require.NoError(t, cmd.Flags().Set("eval-run", req.RunID))
	require.NoError(t, cmd.Flags().Set("discover-eval-runs", "true"))
	var output bytes.Buffer
	cmd.SetOut(&output)
	require.NoError(t, cmd.RunE(cmd, nil))
	descriptorPath, err := fileSvc.Rel(strings.TrimSpace(output.String()))
	require.NoError(t, err)
	descriptorBody, err := fileSvc.ReadFile(context.Background(), descriptorPath)
	require.NoError(t, err)
	bundle := &compliancev1.ComplianceReportBundle{}
	require.NoError(t, compliancev1.UnmarshalCanonical(descriptorBody, bundle))
	selectionDiagnostics := make(map[string]*compliancev1.AssessmentDiagnostic)
	for _, diagnostic := range bundle.GetAnalysis().GetDiagnostics() {
		if strings.HasPrefix(diagnostic.GetCode(), "evaluation_candidate_") {
			selectionDiagnostics[diagnostic.GetSubject().GetRunId()] = diagnostic
		}
	}
	require.Contains(t, selectionDiagnostics, req.RunID)
	assert.Equal(t, scope.SourceAdmissions[0].GetAdmissionId(), selectionDiagnostics[req.RunID].GetSourceAdmissionId())
	require.Contains(t, selectionDiagnostics, "unsupported-candidate")
	assert.Equal(t, "evaluation_candidate_unsupported", selectionDiagnostics["unsupported-candidate"].GetCode())
	assert.Equal(t, "warning", selectionDiagnostics["unsupported-candidate"].GetSeverity())
	assert.Equal(t, "evaluation candidate is unsupported: native evaluation suite or schema is unsupported", selectionDiagnostics["unsupported-candidate"].GetMessage())
	require.Contains(t, selectionDiagnostics, "incomplete-candidate")
	assert.Equal(t, "evaluation_candidate_incomplete", selectionDiagnostics["incomplete-candidate"].GetCode())
	assert.Equal(t, "warning", selectionDiagnostics["incomplete-candidate"].GetSeverity())
	require.Contains(t, selectionDiagnostics, "outside-window-candidate")
	assert.Equal(t, "evaluation_candidate_outside_window", selectionDiagnostics["outside-window-candidate"].GetCode())
	assert.Equal(t, "info", selectionDiagnostics["outside-window-candidate"].GetSeverity())
	inventoryPath := path.Join(path.Dir(descriptorPath), constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, scope.SourceAdmissions[0].AdmissionId, constants.CampaignSourceInventoryFilename)
	inventoryBody, err := fileSvc.ReadFile(context.Background(), inventoryPath)
	require.NoError(t, err)
	inventory := &evalv1.CampaignComplianceSourceInventory{}
	require.NoError(t, evalv1.UnmarshalCanonical(inventoryBody, inventory))
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT, inventory.GetProviderObservationPolicy())
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT, inventory.GetModelProvenancePolicy())
	trustBody, err := compliancev1.MarshalCanonical(policy)
	require.NoError(t, err)
	trustPath := filepath.Join(t.TempDir(), constants.ComplianceReportTrustPolicyTestFilename)
	require.NoError(t, os.WriteFile(trustPath, trustBody, constants.PermFilePublic))
	verifyCmd := complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, func() time.Time { return assessmentAsOf.Add(time.Minute) })
	require.NoError(t, verifyCmd.Flags().Set("trust-policy", trustPath))
	var verificationOutput bytes.Buffer
	verifyCmd.SetOut(&verificationOutput)
	require.NoError(t, verifyCmd.RunE(verifyCmd, []string{strings.TrimSpace(output.String())}), verificationOutput.String())
}

type campaignComplianceWitnessExecutor struct {
	store *evaluation.Store
	now   time.Time
}

func (e *campaignComplianceWitnessExecutor) ExecuteAssignment(ctx context.Context, req evaluation.AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error) {
	role := "primary"
	trace := map[string]any{
		"schema_version":    "1",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      e.now.Format(time.RFC3339),
		"role_outcome":      "invoked",
		"evaluation_context": map[string]any{
			"campaign_id":                req.Assignment.GetCampaignId(),
			"run_id":                     req.Assignment.GetRunId(),
			"assignment_id":              req.Assignment.GetAssignmentId(),
			"evaluation_attempt_id":      req.AttemptID,
			"scenario_id":                req.Assignment.GetScenarioId(),
			"model_registry_digest":      req.Binding.ModelRegistryDigest,
			"target_operator_session_id": req.Binding.InferenceOperatorSessionID,
			"evaluation_lane":            "model_role",
			"designated_model_role":      role,
		},
		"controlled_role_assignment": map[string]any{"designated_model_role": role},
		"model_calls": []any{map[string]any{
			"agent_role":              "sage",
			"model_role":              role,
			"provider":                "G8EProvider",
			"governed_transaction_id": "tx-1",
			"governed_result_digest":  strings.Repeat("a", sha256.Size*2),
			"provider_attempt_id":     req.AttemptID,
			"normalized_request_hash": strings.Repeat("b", sha256.Size*2),
			"output_hash":             strings.Repeat("c", sha256.Size*2),
		}},
	}
	digest, err := evaluation.ComputeChatProbeTraceDigest(trace)
	if err != nil {
		return nil, err
	}
	trace["trace_digest"] = digest
	traceBody, err := json.Marshal(trace)
	if err != nil {
		return nil, err
	}
	if err := e.store.SaveAssignmentTrace(ctx, req.Assignment.GetRunId(), req.Assignment.GetAssignmentId(), traceBody); err != nil {
		return nil, err
	}
	evidenceRef, err := evaluation.BuildAssignmentTraceEvidenceReference(req.Assignment.GetRunId(), req.Assignment.GetAssignmentId(), req.AttemptID, trace, e.now)
	if err != nil {
		return nil, err
	}
	return evaluation.ImportAssignmentResultFromTrace(req, trace, evidenceRef, e.now, func(prefix string) string { return prefix + "-1" })
}

type campaignComplianceBundleFixture struct {
	fileSvc      fs.RuntimeFileService
	bundle       *compliancev1.ComplianceReportBundle
	bundleDir    string
	policy       *compliancev1.ComplianceReportTrustPolicy
	verifiedAt   time.Time
	admissionID  string
	attemptID    string
	verification *evalv1.EvaluationVerificationReport
}

func generateCampaignComplianceBundleFixture(t *testing.T, includeWitnesses bool) campaignComplianceBundleFixture {
	t.Helper()
	ctx := context.Background()
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	store := evaluation.NewStore(fileSvc)
	executedAt := time.Unix(1_700_000_000, 0).UTC()
	executor := &campaignComplianceWitnessExecutor{store: store, now: executedAt}
	controller := evaluation.NewCampaignController(store, executor, func() time.Time { return executedAt }, func(prefix string) string { return prefix + "-1" })
	req := cmdtest.EvaluationTestCampaignInitRequest(t)
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{SchemaVersion: catalog.GetSchemaVersion(), CatalogRef: catalog.GetCatalogRef(), Scenarios: catalog.GetScenarios()[:1]}
	catalogDigest, err := evaluation.ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = catalogDigest
	req.Catalog = truncated
	_, err = controller.InitializeCampaign(ctx, req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(ctx, req.RunID)
	require.NoError(t, err)
	result, executed, err := controller.ExecuteNextAssignment(ctx, req.RunID, evaluation.CampaignExecutionBinding{
		InferenceOperatorSessionID: req.InferenceOperatorSessionID,
		DataOperatorID:             "data-operator",
		DataOperatorSessionID:      req.DataOperatorSessionID,
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              evaluation.InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
	}, req.ScenarioArtifacts)
	require.NoError(t, err)
	require.True(t, executed)
	require.Len(t, result.GetModelInferences(), 1)
	attemptID := result.GetModelInferences()[0].GetProviderAttemptId()
	require.NotEmpty(t, attemptID)
	if includeWitnesses {
		persistCampaignComplianceWitnesses(t, fileSvc, result.GetModelInferences()[0], executedAt)
	}
	assessmentAsOf := time.Now().UTC()
	scopeID := constants.EvalScopePrefix + req.CampaignID
	identity, policy, _ := complianceReportSigningFixtureForTest(t, scopeID)
	cmd := complianceReportGenerateCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
		return identity, nil
	}, func() time.Time { return assessmentAsOf })
	configureComplianceReportGenerateCommand(t, cmd)
	scopePath := writeComplianceAssessmentScopeForTest(t, scopeID, []string{req.RunID}, assessmentAsOf)
	scopeBody, err := os.ReadFile(scopePath)
	require.NoError(t, err)
	scope := &compliancev1.AssessmentScope{}
	require.NoError(t, compliancev1.UnmarshalCanonical(scopeBody, scope))
	admission := scope.SourceAdmissions[0]
	admission.SourceKind = constants.EvaluationSourceKindCampaign
	admission.VerifierRef = &compliancev1.VersionedReference{Id: constants.CampaignVerifierID, Version: constants.CampaignVerifierVersion}
	admission.ProviderObservationPolicy = compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_STRICT
	admission.ModelProvenancePolicy = compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_STRICT
	scopeBody, err = compliancev1.MarshalCanonical(scope)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(scopePath, scopeBody, constants.PermFilePrivate))
	require.NoError(t, cmd.Flags().Set("scope", scopePath))
	require.NoError(t, cmd.Flags().Set("eval-run", req.RunID))
	var output bytes.Buffer
	cmd.SetOut(&output)
	require.NoError(t, cmd.RunE(cmd, nil))
	descriptor := strings.TrimSpace(output.String())
	descriptorPath, err := fileSvc.Rel(descriptor)
	require.NoError(t, err)
	descriptorBody, err := fileSvc.ReadFile(ctx, descriptorPath)
	require.NoError(t, err)
	bundle := &compliancev1.ComplianceReportBundle{}
	require.NoError(t, compliancev1.UnmarshalCanonical(descriptorBody, bundle))
	bundleDir := path.Dir(descriptorPath)
	verificationPath := path.Join(bundleDir, constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, admission.GetAdmissionId(), constants.ComplianceBundleSourceVerificationFilename)
	verificationBody, err := fileSvc.ReadFile(ctx, verificationPath)
	require.NoError(t, err)
	verification := &evalv1.EvaluationVerificationReport{}
	require.NoError(t, evalv1.UnmarshalCanonical(verificationBody, verification))
	return campaignComplianceBundleFixture{fileSvc: fileSvc, bundle: bundle, bundleDir: bundleDir, policy: policy, verifiedAt: assessmentAsOf.Add(time.Minute), admissionID: admission.GetAdmissionId(), attemptID: attemptID, verification: verification}
}

func persistCampaignComplianceWitnesses(t *testing.T, fileSvc fs.RuntimeFileService, inferenceRecord *evalv1.ModelInferenceRecord, observedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	attemptID := inferenceRecord.GetProviderAttemptId()
	attemptStore, err := inference.NewAttemptStore(fileSvc)
	require.NoError(t, err)
	startedAt := observedAt.UnixMilli()
	completedAt := observedAt.Add(time.Second).UnixMilli()
	require.NoError(t, attemptStore.Begin(ctx, &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: attemptID,
		TransactionId:     "tx-1",
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		StartedAtUnixMs:   startedAt,
		CompletedAtUnixMs: completedAt,
		ResultDigest:      strings.Repeat("a", sha256.Size*2),
	}))
	observation := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          attemptID,
		ObserverId:                 "observer-1",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(observedAt.UnixNano()),
		WindowCompletedAtUnixNanos: uint64(observedAt.Add(time.Second).UnixNano()),
		AttemptStartedAtUnixMs:     startedAt,
		AttemptCompletedAtUnixMs:   completedAt,
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			ObservedAtUnixNanos:        uint64(observedAt.Add(time.Millisecond).UnixNano()),
			GpuUtilizationAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			GpuUtilizationPercent:      10,
			HostRamAvailability:        evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			HostRamUsedBytes:           1024,
		}},
	}
	observationDigest, err := provider_observer.ComputeObservationDigest(observation)
	require.NoError(t, err)
	observation.ObservationDigest = observationDigest
	observationStore, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	require.NoError(t, observationStore.Save(ctx, observation))
	modelDigest := inferenceRecord.GetModelVariant().GetModelDigest()
	provenance := &evalv1.ModelProvenanceAttestationWindow{
		SchemaVersion:              model_provenance.SchemaVersion,
		ProviderAttemptId:          attemptID,
		ProvenanceOperatorId:       "provenance-1",
		ServedModelTag:             inferenceRecord.GetModelVariant().GetServedModelTag(),
		ExpectedModelDigest:        modelDigest,
		ObservedModelDigest:        modelDigest,
		ManifestDigest:             strings.Repeat("d", sha256.Size*2),
		ManifestVerificationStatus: evalv1.ModelManifestVerificationStatus_MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED,
		AttestedAtUnixMs:           completedAt,
		DigestMatch:                true,
	}
	provenanceDigest, err := model_provenance.ComputeAttestationDigest(provenance)
	require.NoError(t, err)
	provenance.AttestationDigest = provenanceDigest
	provenanceStore, err := model_provenance.NewWindowStore(fileSvc)
	require.NoError(t, err)
	require.NoError(t, provenanceStore.Save(ctx, provenance))
}

func verifyCampaignComplianceBundleFixture(t *testing.T, fixture campaignComplianceBundleFixture) *compliancev1.ComplianceVerificationReport {
	t.Helper()
	root, err := os.OpenRoot(fixture.fileSvc.Resolve(fixture.bundleDir))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	report, err := compliancereport.VerifyComplianceReportBundle(context.Background(), compliancereport.BundleVerificationRequest{
		Bundle: fixture.bundle, Reader: &complianceBundleRootReader{root: root}, TrustPolicy: fixture.policy, VerifiedAt: fixture.verifiedAt,
	})
	require.NoError(t, err)
	return report
}

func TestComplianceReportGenerateCmdWithConfig_StrictCampaignMissingWitnessesRemainFailedOffline(t *testing.T) {
	fixture := generateCampaignComplianceBundleFixture(t, false)

	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, fixture.verification.GetStatus())
	assert.Contains(t, strings.Join(fixture.verification.GetFailureReasons(), "\n"), "missing provider-boundary observation window")
	assert.Contains(t, strings.Join(fixture.verification.GetFailureReasons(), "\n"), "missing model provenance attestation window")
	assert.True(t, verifyCampaignComplianceBundleFixture(t, fixture).GetValid())
}

func TestComplianceReportGenerateCmdWithConfig_CampaignWitnessMutationsFailOfflineReplay(t *testing.T) {
	tests := []struct {
		name        string
		runtimePath func(string) string
		remove      bool
	}{
		{name: "tampered provider attempt", runtimePath: func(attemptID string) string {
			return path.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceAttemptsDirname, attemptID+constants.FileExtJSON)
		}},
		{name: "missing provider attempt", runtimePath: func(attemptID string) string {
			return path.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceAttemptsDirname, attemptID+constants.FileExtJSON)
		}, remove: true},
		{name: "tampered provider observation", runtimePath: func(attemptID string) string {
			return path.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceProviderObserverDirname, constants.InferenceProviderObserverWindowsDirname, attemptID+constants.FileExtJSON)
		}},
		{name: "missing provider observation", runtimePath: func(attemptID string) string {
			return path.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceProviderObserverDirname, constants.InferenceProviderObserverWindowsDirname, attemptID+constants.FileExtJSON)
		}, remove: true},
		{name: "tampered model provenance", runtimePath: func(attemptID string) string {
			return path.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceModelProvenanceDirname, constants.InferenceModelProvenanceWindowsDirname, attemptID+constants.FileExtJSON)
		}},
		{name: "missing model provenance", runtimePath: func(attemptID string) string {
			return path.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceModelProvenanceDirname, constants.InferenceModelProvenanceWindowsDirname, attemptID+constants.FileExtJSON)
		}, remove: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := generateCampaignComplianceBundleFixture(t, true)
			assert.True(t, verifyCampaignComplianceBundleFixture(t, fixture).GetValid())
			sourcePath := path.Join(fixture.bundleDir, constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, fixture.admissionID, constants.ComplianceBundleSourceRuntimeDirname, test.runtimePath(fixture.attemptID))
			if test.remove {
				require.NoError(t, fixture.fileSvc.Remove(context.Background(), sourcePath))
			} else {
				require.NoError(t, fixture.fileSvc.WriteFile(context.Background(), sourcePath, []byte(`{}`), constants.PermFilePrivate))
			}

			report := verifyCampaignComplianceBundleFixture(t, fixture)
			assert.False(t, report.GetValid())
			assert.NotEmpty(t, report.GetFailures())
		})
	}
}

func TestComplianceReportGenerateCmdWithConfig_RejectsInvalidProtectedAssessmentWindow(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t), time.Now)
	configureComplianceReportGenerateCommand(t, cmd)
	scopePath := writeComplianceAssessmentScopeForTest(t, "scope-1", []string{"run-1"}, time.Unix(1_700_000_001, 0).UTC())
	scopeBody, err := os.ReadFile(scopePath)
	require.NoError(t, err)
	scope := &compliancev1.AssessmentScope{}
	require.NoError(t, compliancev1.UnmarshalCanonical(scopeBody, scope))
	scope.AssessmentWindowStart = scope.AssessmentWindowEnd
	scopeBody, err = compliancev1.MarshalCanonical(scope)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(scopePath, scopeBody, constants.PermFileReadOnly))
	require.NoError(t, cmd.Flags().Set("scope", scopePath))
	require.NoError(t, cmd.Flags().Set("eval-run", "run-1"))

	err = cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestComplianceReportVerifyCmdWithConfig_PrintsTypedValidReport(t *testing.T) {
	verifiedAt := time.Unix(1_700_000_100, 0).UTC()
	loaderCalled := false
	expectedEvidenceTrust := &assessedEvidenceTrust{keys: map[string]ed25519.PublicKey{}}
	cmd := complianceReportVerifyCmdWithConfig(
		func(_ context.Context, bundlePath, trustPolicyPath, evidenceTrustPath string) (complianceReportBundleInput, error) {
			loaderCalled = true
			assert.Equal(t, "bundle.json", bundlePath)
			assert.Equal(t, "assessed-trust.json", trustPolicyPath)
			assert.Equal(t, "evidence-trust.json", evidenceTrustPath)
			return complianceReportBundleInput{evidenceTrust: expectedEvidenceTrust}, nil
		},
		func(_ context.Context, request compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
			assert.Equal(t, verifiedAt, request.VerifiedAt)
			assert.Same(t, expectedEvidenceTrust, request.EvidenceTrust)
			return &compliancev1.ComplianceVerificationReport{
				ReportId:               "report-1",
				Valid:                  true,
				VerifiedAt:             requestTimestamp(verifiedAt),
				VerifierId:             constants.ComplianceBundleVerifierID,
				VerifierVersion:        constants.ComplianceBundleVerifierVersion,
				ReproducedChecksumRoot: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}, nil
		},
		func() time.Time { return verifiedAt },
	)
	require.NoError(t, cmd.Flags().Set("trust-policy", "assessed-trust.json"))
	require.NoError(t, cmd.Flags().Set("evidence-trust", "evidence-trust.json"))
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.NoError(t, cmd.RunE(cmd, []string{"bundle.json"}))
	assert.True(t, loaderCalled)
	report := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(bytes.TrimSpace(output.Bytes()), report))
	assert.True(t, report.GetValid())
	assert.Equal(t, "report-1", report.GetReportId())
}

func TestComplianceReportVerifyCmdWithConfig_ReturnsFailureAfterPrintingInvalidReport(t *testing.T) {
	verifiedAt := time.Unix(1_700_000_100, 0).UTC()
	cmd := complianceReportVerifyCmdWithConfig(
		func(context.Context, string, string, string) (complianceReportBundleInput, error) {
			return complianceReportBundleInput{}, nil
		},
		func(context.Context, compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
			return &compliancev1.ComplianceVerificationReport{
				ReportId:               "report-1",
				Valid:                  false,
				VerifiedAt:             requestTimestamp(verifiedAt),
				VerifierId:             constants.ComplianceBundleVerifierID,
				VerifierVersion:        constants.ComplianceBundleVerifierVersion,
				ReproducedChecksumRoot: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Failures:               []*compliancev1.VerificationFailure{{Code: constants.ErrChecksumMismatch.Error(), SubjectRef: constants.ComplianceBundleAnalysisPath, Reason: "digest mismatch"}},
			}, nil
		},
		func() time.Time { return verifiedAt },
	)
	require.NoError(t, cmd.Flags().Set("trust-policy", "assessed-trust.json"))
	var output bytes.Buffer
	cmd.SetOut(&output)

	err := cmd.RunE(cmd, []string{"bundle.json"})

	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.Contains(t, output.String(), constants.ErrChecksumMismatch.Error())
}

func TestComplianceReportVerifyCmdWithConfig_PropagatesReaderCloseFailure(t *testing.T) {
	closeErr := fmt.Errorf("close failed")
	verifiedAt := time.Unix(1_700_000_100, 0).UTC()
	cmd := complianceReportVerifyCmdWithConfig(
		func(context.Context, string, string, string) (complianceReportBundleInput, error) {
			return complianceReportBundleInput{close: func() error { return closeErr }}, nil
		},
		func(context.Context, compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
			return &compliancev1.ComplianceVerificationReport{ReportId: "report-1", Valid: true, VerifiedAt: timestamppb.New(verifiedAt), VerifierId: constants.ComplianceBundleVerifierID, VerifierVersion: constants.ComplianceBundleVerifierVersion, ReproducedChecksumRoot: strings.Repeat("a", 64)}, nil
		},
		func() time.Time { return verifiedAt },
	)
	require.NoError(t, cmd.Flags().Set("trust-policy", "assessed-trust.json"))

	err := cmd.RunE(cmd, []string{"bundle.json"})

	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.ErrorIs(t, err, closeErr)
}

func TestComplianceReportVerifyCmdWithConfig_RequiresExternalTrustPolicy(t *testing.T) {
	cmd := complianceReportVerifyCmdWithConfig(
		func(context.Context, string, string, string) (complianceReportBundleInput, error) {
			t.Fatal("loader must not run without an explicit trust policy")
			return complianceReportBundleInput{}, nil
		},
		compliancereport.VerifyComplianceReportBundle,
		time.Now,
	)

	err := cmd.RunE(cmd, []string{"bundle.json"})

	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestNewAssessedEvidenceTrust_ValidatesEveryAssessedSigner(t *testing.T) {
	signedAt := time.Unix(1_700_000_000, 0).UTC()
	newPolicy := func() *compliancev1.ComplianceEvidenceTrustPolicy {
		publicKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		digest := sha256.Sum256(publicKey)
		return &compliancev1.ComplianceEvidenceTrustPolicy{
			PolicyId:      "evidence-policy-1",
			PolicyVersion: "1.0.0",
			TrustedKeys: []*compliancev1.ComplianceEvidenceTrustedKey{{
				KeyId:            "evidence-key-1",
				PublicKey:        hex.EncodeToString(publicKey),
				PublicKeySha256:  hex.EncodeToString(digest[:]),
				AssessmentId:     "assessment-1",
				AssessorIdentity: "assessor-1",
				AssessedAt:       timestamppb.New(signedAt.Add(-2 * time.Hour)),
				ValidFrom:        timestamppb.New(signedAt.Add(-time.Hour)),
				ValidUntil:       timestamppb.New(signedAt.Add(time.Hour)),
				AllowedScopeRefs: []string{"scope-1"},
			}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*compliancev1.ComplianceEvidenceTrustPolicy)
		wantErr bool
	}{
		{name: "valid assessed signer"},
		{name: "duplicate signer", mutate: func(policy *compliancev1.ComplianceEvidenceTrustPolicy) {
			policy.TrustedKeys = append(policy.TrustedKeys, policy.TrustedKeys[0])
		}, wantErr: true},
		{name: "nil signer", mutate: func(policy *compliancev1.ComplianceEvidenceTrustPolicy) {
			policy.TrustedKeys[0] = nil
		}, wantErr: true},
		{name: "invalid public key shape", mutate: func(policy *compliancev1.ComplianceEvidenceTrustPolicy) {
			policy.TrustedKeys[0].PublicKey = "00"
		}, wantErr: true},
		{name: "assessment after signing", mutate: func(policy *compliancev1.ComplianceEvidenceTrustPolicy) {
			policy.TrustedKeys[0].AssessedAt = timestamppb.New(signedAt.Add(time.Second))
		}, wantErr: true},
		{name: "not yet valid signer", mutate: func(policy *compliancev1.ComplianceEvidenceTrustPolicy) {
			policy.TrustedKeys[0].ValidFrom = timestamppb.New(signedAt.Add(time.Second))
		}, wantErr: true},
		{name: "digest mismatch", mutate: func(policy *compliancev1.ComplianceEvidenceTrustPolicy) {
			policy.TrustedKeys[0].PublicKeySha256 = strings.Repeat("0", 64)
		}, wantErr: true},
		{name: "scope ineligible", mutate: func(policy *compliancev1.ComplianceEvidenceTrustPolicy) {
			policy.TrustedKeys[0].AllowedScopeRefs = []string{"other-scope"}
		}, wantErr: true},
		{name: "expired signer", mutate: func(policy *compliancev1.ComplianceEvidenceTrustPolicy) {
			policy.TrustedKeys[0].ValidUntil = timestamppb.New(signedAt.Add(-time.Second))
		}, wantErr: true},
		{name: "revoked signer", mutate: func(policy *compliancev1.ComplianceEvidenceTrustPolicy) {
			policy.TrustedKeys[0].RevokedAt = timestamppb.New(signedAt)
		}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := newPolicy()
			if test.mutate != nil {
				test.mutate(policy)
			}
			trust, err := newAssessedEvidenceTrust(policy, "scope-1", timestamppb.New(signedAt))
			if test.wantErr {
				assert.ErrorIs(t, err, constants.ErrEvidenceTrustNotAssessed)
				assert.Nil(t, trust)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, trust)
		})
	}
}

func TestLoadComplianceReportBundleInput_RejectsMalformedAndOversizedInputs(t *testing.T) {
	canonicalEmpty := []byte(`{}`)
	noncanonicalEmpty := []byte("{\n}")
	oversized := bytes.Repeat([]byte("x"), int(constants.ComplianceBundleMaxArtifactBytes)+1)
	tests := []struct {
		name           string
		bundleBody     []byte
		trustBody      []byte
		expectedErrors []error
	}{
		{name: "noncanonical descriptor", bundleBody: noncanonicalEmpty, trustBody: canonicalEmpty, expectedErrors: []error{constants.ErrReportVerificationFailed}},
		{name: "noncanonical trust policy", bundleBody: canonicalEmpty, trustBody: noncanonicalEmpty, expectedErrors: []error{constants.ErrEvidenceTrustNotAssessed}},
		{name: "oversized descriptor", bundleBody: oversized, trustBody: canonicalEmpty, expectedErrors: []error{constants.ErrEvidenceArtifactTooLarge}},
		{name: "oversized trust policy", bundleBody: canonicalEmpty, trustBody: oversized, expectedErrors: []error{constants.ErrEvidenceTrustNotAssessed, constants.ErrEvidenceArtifactTooLarge}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundleRoot := t.TempDir()
			trustRoot := t.TempDir()
			bundlePath := filepath.Join(bundleRoot, constants.ComplianceBundleManifestPath)
			trustPath := filepath.Join(trustRoot, constants.ComplianceReportTrustPolicyTestFilename)
			require.NoError(t, os.WriteFile(bundlePath, test.bundleBody, constants.PermFilePublic))
			require.NoError(t, os.WriteFile(trustPath, test.trustBody, constants.PermFilePublic))

			input, err := loadComplianceReportBundleInput(context.Background(), bundlePath, trustPath, "")

			require.Error(t, err)
			for _, expectedErr := range test.expectedErrors {
				assert.ErrorIs(t, err, expectedErr)
			}
			assert.Nil(t, input.bundle)
			assert.Nil(t, input.reader)
		})
	}
}

func TestComplianceBundleRootReader_ReadFileRejectsOversizedArtifactAndPreservesCancellation(t *testing.T) {
	rootPath := t.TempDir()
	artifactPath := filepath.Join(rootPath, constants.ComplianceBundleAnalysisPath)
	require.NoError(t, os.WriteFile(artifactPath, bytes.Repeat([]byte("x"), int(constants.ComplianceBundleMaxArtifactBytes)+1), constants.PermFilePublic))
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	reader := &complianceBundleRootReader{root: root}

	_, err = reader.ReadFile(context.Background(), constants.ComplianceBundleAnalysisPath)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactTooLarge)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = reader.ReadFile(ctx, constants.ComplianceBundleAnalysisPath)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestComplianceBundleRootReader_ReadFileRejectsDirectory(t *testing.T) {
	rootPath := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(rootPath, constants.TestNestedDirname), constants.PermDirPrivate))
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	_, err = (&complianceBundleRootReader{root: root}).ReadFile(context.Background(), constants.TestNestedDirname)

	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
}

func TestComplianceBundleRootReader_ListsNestedUnexpectedArtifact(t *testing.T) {
	rootPath := t.TempDir()
	nestedPath := filepath.Join(rootPath, constants.TestNestedDirname)
	require.NoError(t, os.Mkdir(nestedPath, constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(filepath.Join(nestedPath, constants.ComplianceBundleUnexpectedTestPath), []byte(`{}`), constants.PermFilePublic))
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	paths, err := (&complianceBundleRootReader{root: root}).ListFiles(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{path.Join(constants.TestNestedDirname, constants.ComplianceBundleUnexpectedTestPath)}, paths)
}

func TestComplianceBundleRootReader_RejectsSymlinkEntries(t *testing.T) {
	rootPath := t.TempDir()
	targetPath := filepath.Join(rootPath, constants.ComplianceBundleAnalysisPath)
	require.NoError(t, os.WriteFile(targetPath, []byte(`{}`), constants.PermFilePublic))
	require.NoError(t, os.Symlink(constants.ComplianceBundleAnalysisPath, filepath.Join(rootPath, constants.ComplianceBundleUnexpectedTestPath)))
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	_, err = (&complianceBundleRootReader{root: root}).ListFiles(context.Background())

	assert.ErrorIs(t, err, constants.ErrUnexpectedEvidenceArtifact)
}

func TestLoadComplianceReportBundleInput_RejectsInBundleAndReusedTrustPaths(t *testing.T) {
	bundleRoot := t.TempDir()
	externalRoot := t.TempDir()
	bundlePath := filepath.Join(bundleRoot, constants.ComplianceBundleManifestPath)
	inBundleTrustPath := filepath.Join(bundleRoot, constants.ComplianceBundleUnexpectedTestPath)
	externalTrustPath := filepath.Join(externalRoot, constants.ComplianceReportTrustPolicyTestFilename)
	require.NoError(t, os.WriteFile(bundlePath, []byte(`{}`), constants.PermFilePublic))
	require.NoError(t, os.WriteFile(inBundleTrustPath, []byte(`{}`), constants.PermFilePublic))
	require.NoError(t, os.WriteFile(externalTrustPath, []byte(`{}`), constants.PermFilePublic))
	tests := []struct {
		name              string
		reportTrustPath   string
		evidenceTrustPath string
	}{
		{name: "report trust inside bundle", reportTrustPath: inBundleTrustPath},
		{name: "evidence trust inside bundle", reportTrustPath: externalTrustPath, evidenceTrustPath: inBundleTrustPath},
		{name: "report and evidence trust reuse one path", reportTrustPath: externalTrustPath, evidenceTrustPath: externalTrustPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, err := loadComplianceReportBundleInput(context.Background(), bundlePath, test.reportTrustPath, test.evidenceTrustPath)

			assert.ErrorIs(t, err, constants.ErrEvidenceTrustNotAssessed)
			assert.Nil(t, input.bundle)
		})
	}
}

func TestComplianceBundleRootReader_RejectsDirectoryDepthLimit(t *testing.T) {
	rootPath := t.TempDir()
	currentPath := rootPath
	for range constants.ComplianceBundleMaxDirectoryDepth + 1 {
		currentPath = filepath.Join(currentPath, constants.TestNestedDirname)
		require.NoError(t, os.Mkdir(currentPath, constants.PermDirPrivate))
	}
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	_, err = (&complianceBundleRootReader{root: root}).ListFiles(context.Background())

	assert.ErrorIs(t, err, constants.ErrEvidenceDirectoryLimitExceeded)
}

func TestCollectRuntimeSourceArtifacts_RejectsDirectoryDepthLimit(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	currentPath := constants.TestNestedDirname
	require.NoError(t, fileSvc.MkdirAll(context.Background(), currentPath, constants.PermDirPrivate))
	for range constants.ComplianceBundleMaxDirectoryDepth + 1 {
		currentPath = path.Join(currentPath, constants.TestNestedDirname)
		require.NoError(t, fileSvc.MkdirAll(context.Background(), currentPath, constants.PermDirPrivate))
	}

	_, err := collectRuntimeSourceArtifacts(context.Background(), fileSvc, constants.TestNestedDirname, constants.TestNestedDirname, constants.ComplianceBundleSourcesDirname, false)

	assert.ErrorIs(t, err, constants.ErrEvidenceDirectoryLimitExceeded)
}

func TestRecursiveDirectoryBudget_EnforcesDepthAndAggregateEntryLimits(t *testing.T) {
	tests := []struct {
		name             string
		depth            int
		currentEntries   int
		newEntries       int
		expectedExceeded bool
	}{
		{name: "maximum depth and entry count accepted", depth: constants.ComplianceBundleMaxDirectoryDepth, currentEntries: constants.ComplianceBundleMaxEnumeratedEntries - 1, newEntries: 1},
		{name: "depth above maximum rejected", depth: constants.ComplianceBundleMaxDirectoryDepth + 1, newEntries: 1, expectedExceeded: true},
		{name: "aggregate entries above maximum rejected", currentEntries: constants.ComplianceBundleMaxEnumeratedEntries, newEntries: 1, expectedExceeded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget := recursiveDirectoryBudget{entries: tt.currentEntries}

			err := budget.consume(tt.depth, tt.newEntries)

			if tt.expectedExceeded {
				assert.ErrorIs(t, err, constants.ErrEvidenceDirectoryLimitExceeded)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.currentEntries+tt.newEntries, budget.entries)
		})
	}
}

func assertComplianceVerificationFailure(t *testing.T, report *compliancev1.ComplianceVerificationReport, code error, subject string) {
	t.Helper()
	for _, failure := range report.GetFailures() {
		if failure.GetCode() == code.Error() && failure.GetSubjectRef() == subject {
			return
		}
	}
	assert.Fail(t, "expected compliance verification failure", "code=%q subject=%q failures=%v", code.Error(), subject, report.GetFailures())
}

func requestTimestamp(value time.Time) *timestamppb.Timestamp {
	return timestamppb.New(value)
}
