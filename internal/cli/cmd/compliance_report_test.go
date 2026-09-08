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
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func complianceReportSigningFixtureForTest(t *testing.T, scopeID string) (*compliancereport.ComplianceReportSigningIdentity, *compliancev1.ComplianceReportTrustPolicy, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	digest := sha256.Sum256(publicKey)
	createdAt := time.Unix(1_699_999_900, 0).UTC()
	metadata := &compliancev1.ComplianceReportSigningKeyMetadata{
		KeyId:           "report-key-1",
		Algorithm:       constants.ComplianceReportSignatureAlgorithm,
		Purpose:         constants.ComplianceReportSigningPurpose,
		PublicKeySha256: hex.EncodeToString(digest[:]),
		CreatedAt:       timestamppb.New(createdAt),
		ExpiresAt:       timestamppb.New(createdAt.Add(time.Hour)),
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

func configureComplianceReportGenerateCommand(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	require.NoError(t, cmd.Flags().Set("report-id", "report-1"))
	require.NoError(t, cmd.Flags().Set("signing-metadata", constants.ComplianceReportSigningMetadataTestFilename))
	require.NoError(t, cmd.Flags().Set("signing-private-key", constants.ComplianceReportSigningPrivateKeyTestFilename))
}

func runComplianceReportGenerateCommand(t *testing.T, evalRuns []string) ([]byte, error) {
	t.Helper()
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t))
	configureComplianceReportGenerateCommand(t, cmd)
	windowStart := time.Unix(1_699_999_999, 0).UTC()
	windowEnd := time.Unix(1_700_000_100, 0).UTC()
	require.NoError(t, cmd.Flags().Set("scope-id", evidence.EvalScopeID("evidence-graph-suite")))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(windowStart.UnixMilli(), 10)))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(windowEnd.UnixMilli(), 10)))
	for _, runID := range evalRuns {
		require.NoError(t, cmd.Flags().Set("eval-run", runID))
	}
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	return bytes.TrimSpace(output.Bytes()), cmd.RunE(cmd, nil)
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

	binding := compliance.EvaluationBinding{ScopeID: scopeID, RunID: "ksi-run-1", WindowStartUnixMs: 1_699_999_000_000, WindowEndUnixMs: 1_700_001_000_000, EvaluatorID: constants.KSIEvaluatorID, EvaluatorVersion: constants.KSIEvaluatorVersion, MethodDefinitionID: constants.KSIMethodDefinitionVersion, AssertionAssessments: compliance.AssertionAssessmentScope{AssessmentIDs: []string{"assessment-1"}, AttemptIDs: []string{"attempt-1"}, ScenarioIDs: []string{"scenario-1"}, ActionIDs: []string{"action-1"}}}
	ksi := compliance.KSIResultSet{Class: compliance.ClassC, EvaluatedAtMs: 1_700_000_000_000, Binding: binding, Results: []compliance.KSIResult{{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied, Outcome: compliance.KSIOutcomeSatisfied, LastValidatedUnixMs: 1_699_999_999_000, MethodCount: 1, Binding: binding}}}
	ksiResultsBody, err := json.Marshal(ksi)
	require.NoError(t, err)
	ksiHistoryBody := append(append([]byte(nil), ksiResultsBody...), '\n')
	ksiHistoryPath := filepath.Join(root, constants.ComplianceBundleKSIHistoryFilename)
	ksiResultsPath := filepath.Join(root, constants.ComplianceBundleKSIResultsFilename)
	require.NoError(t, os.WriteFile(ksiHistoryPath, ksiHistoryBody, constants.PermFileReadOnly))
	require.NoError(t, os.WriteFile(ksiResultsPath, ksiResultsBody, constants.PermFileReadOnly))

	commitment := &operatorv1.CommitmentAttestation{TransactionId: "transaction-1", TransactionHash: strings.Repeat("1", 64), PriorCommitmentHash: strings.Repeat("2", 64), StateRootAtCommit: strings.Repeat("3", 64), L2SignatureDigest: strings.Repeat("4", 64), WardenIntentSignatureDigest: strings.Repeat("5", 64), HumanSignatureDigest: strings.Repeat("6", 64), ActionType: "FILE_EDIT", TargetResource: constants.DemosTargetDataDir, CommittedAtUnixMs: 1_700_000_000_000, AuditorKeyId: keyID}
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

	fileSvc, _ := newCmdTestEnv(t)
	reportPublicKey, reportPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	reportKeyDigest := sha256.Sum256(reportPublicKey)
	reportMetadata := &compliancev1.ComplianceReportSigningKeyMetadata{KeyId: "complete-report-key-1", Algorithm: constants.ComplianceReportSignatureAlgorithm, Purpose: constants.ComplianceReportSigningPurpose, PublicKeySha256: hex.EncodeToString(reportKeyDigest[:]), CreatedAt: timestamppb.New(verifiedAt.Add(-time.Hour)), ExpiresAt: timestamppb.New(verifiedAt.Add(time.Hour))}
	reportIdentity, err := compliancereport.NewComplianceReportSigningIdentity(reportMetadata, reportPrivateKey)
	require.NoError(t, err)
	reportPolicy := &compliancev1.ComplianceReportTrustPolicy{PolicyId: "complete-report-policy-1", PolicyVersion: "1.0.0", TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{Metadata: reportMetadata, PublicKey: hex.EncodeToString(reportPublicKey), AssessmentId: "report-assessment-1", AssessorIdentity: "assessor-1", AssessedAt: timestamppb.New(verifiedAt.Add(-time.Hour)), AllowedScopeRefs: []string{scopeID}}}}
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
	generateCmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
		return reportIdentity, nil
	})
	configureComplianceReportGenerateCommand(t, generateCmd)
	require.NoError(t, generateCmd.Flags().Set("scope-id", scopeID))
	require.NoError(t, generateCmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(time.UnixMilli(1_699_999_000_000).UnixMilli(), 10)))
	require.NoError(t, generateCmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(verifiedAt.UnixMilli(), 10)))
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
	verifyCmd := complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, func() time.Time { return verifiedAt })
	require.NoError(t, verifyCmd.Flags().Set("trust-policy", reportPolicyPath))
	require.NoError(t, verifyCmd.Flags().Set("evidence-trust", evidencePolicyPath))
	var verified bytes.Buffer
	verifyCmd.SetOut(&verified)
	require.NoError(t, verifyCmd.RunE(verifyCmd, []string{descriptorPath}))
	verificationReport := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(bytes.TrimSpace(verified.Bytes()), verificationReport))
	assert.True(t, verificationReport.GetValid(), verificationReport.GetFailures())
}

func TestComplianceReportGenerateCmdWithConfig_PersistsSignedBundleAndRootedVerifierRejectsInventoryMutations(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	runID := persistMinimalEvidenceGraphEvalFixture(t, fileSvc)
	scopeID := evidence.EvalScopeID("evidence-graph-suite")
	_, policy, privateKey := complianceReportSigningFixtureForTest(t, scopeID)
	inputDir := t.TempDir()
	metadataPath := filepath.Join(inputDir, constants.ComplianceReportSigningMetadataTestFilename)
	metadataBody, err := compliancev1.MarshalCanonical(policy.GetTrustedKeys()[0].GetMetadata())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metadataPath, metadataBody, constants.PermFileReadOnly))
	privateKeyPath := filepath.Join(inputDir, constants.ComplianceReportSigningPrivateKeyTestFilename)
	require.NoError(t, os.WriteFile(privateKeyPath, []byte(hex.EncodeToString(privateKey)), constants.PermFileReadOnly))
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), loadComplianceReportSigningIdentity)
	configureComplianceReportGenerateCommand(t, cmd)
	require.NoError(t, cmd.Flags().Set("signing-metadata", metadataPath))
	require.NoError(t, cmd.Flags().Set("signing-private-key", privateKeyPath))
	windowStart := time.Unix(1_699_999_999, 0).UTC()
	windowEnd := time.Unix(1_700_000_100, 0).UTC()
	require.NoError(t, cmd.Flags().Set("scope-id", evidence.EvalScopeID("evidence-graph-suite")))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(windowStart.UnixMilli(), 10)))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(windowEnd.UnixMilli(), 10)))
	require.NoError(t, cmd.Flags().Set("eval-run", runID))
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.NoError(t, cmd.RunE(cmd, nil))
	descriptorPath, err := fileSvc.Rel(string(bytes.TrimSpace(output.Bytes())))
	require.NoError(t, err)
	descriptorBody, err := fileSvc.ReadFile(context.Background(), descriptorPath)
	require.NoError(t, err)
	bundle := &compliancev1.ComplianceReportBundle{}
	require.NoError(t, compliancev1.UnmarshalCanonical(descriptorBody, bundle))
	assert.Equal(t, evidence.EvalScopeID("evidence-graph-suite"), bundle.GetAnalysis().GetScopeRef())
	assert.NotNil(t, bundle.GetManifest().GetSignature())
	assert.NotNil(t, bundle.GetChecksumRootSignature())
	artifactPaths := make(map[string]struct{}, len(bundle.GetArtifacts()))
	for _, artifact := range bundle.GetArtifacts() {
		artifactPaths[artifact.GetBundlePath()] = struct{}{}
	}
	evalSourceBase := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, runID)
	for _, filename := range []string{
		constants.EvalRunManifestFilename,
		constants.EvalRunTasksFilename,
		constants.EvalRunAttemptsFilename,
		constants.EvalRunReceiptsFilename,
		constants.EvalRunStagesFilename,
		constants.EvalRunMetricsFilename,
		constants.EvalRunEvidenceIndexFilename,
	} {
		assert.Contains(t, artifactPaths, path.Join(evalSourceBase, constants.ComplianceBundleSourceRuntimeDirname, filename))
	}
	evalVerificationPath := path.Join(evalSourceBase, constants.ComplianceBundleSourceVerificationFilename)
	require.Contains(t, artifactPaths, evalVerificationPath)
	evalVerificationBody, err := fileSvc.ReadFile(context.Background(), path.Join(constants.ComplianceBundlesDirname, bundle.GetManifest().GetReportId(), evalVerificationPath))
	require.NoError(t, err)
	evalVerificationReport := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(evalVerificationBody, evalVerificationReport))
	assert.True(t, evalVerificationReport.GetValid())
	assert.Equal(t, constants.EvalRunVerifierID, evalVerificationReport.GetVerifierId())
	require.Len(t, bundle.GetRenderedFormats(), len(compliancereport.SupportedFormats()))
	for _, rendered := range bundle.GetRenderedFormats() {
		artifactPath := path.Join(constants.ComplianceBundlesDirname, bundle.GetManifest().GetReportId(), rendered.GetBundlePath())
		body, err := fileSvc.ReadFile(context.Background(), artifactPath)
		require.NoError(t, err)
		assert.NotEmpty(t, body)
	}
	trustBody, err := compliancev1.MarshalCanonical(policy)
	require.NoError(t, err)
	trustPath := filepath.Join(t.TempDir(), constants.ComplianceReportTrustPolicyTestFilename)
	require.NoError(t, os.WriteFile(trustPath, trustBody, constants.PermFilePublic))
	verifyCmd := complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, func() time.Time { return windowEnd })
	require.NoError(t, verifyCmd.Flags().Set("trust-policy", trustPath))
	var verificationOutput bytes.Buffer
	verifyCmd.SetOut(&verificationOutput)

	require.NoError(t, verifyCmd.RunE(verifyCmd, []string{fileSvc.Resolve(descriptorPath)}))
	verificationReport := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(bytes.TrimSpace(verificationOutput.Bytes()), verificationReport))
	assert.True(t, verificationReport.GetValid())
	assert.Empty(t, verificationReport.GetFailures())

	bundleDir := path.Dir(descriptorPath)
	nestedDir := path.Join(bundleDir, constants.TestNestedDirname)
	require.NoError(t, fileSvc.MkdirAll(context.Background(), nestedDir, constants.PermDirPrivate))
	nestedUnexpectedPath := path.Join(constants.TestNestedDirname, constants.ComplianceBundleUnexpectedTestPath)
	require.NoError(t, fileSvc.WriteFile(context.Background(), path.Join(bundleDir, nestedUnexpectedPath), []byte(`{}`), constants.PermFilePublic))
	input, err := loadComplianceReportBundleInput(context.Background(), fileSvc.Resolve(descriptorPath), trustPath, "")
	require.NoError(t, err)
	unexpectedReport, err := compliancereport.VerifyComplianceReportBundle(context.Background(), compliancereport.BundleVerificationRequest{Bundle: input.bundle, Reader: input.reader, TrustPolicy: input.trustPolicy, VerifiedAt: windowEnd})
	require.NoError(t, err)
	require.NoError(t, input.close())
	assert.False(t, unexpectedReport.GetValid())
	assertComplianceVerificationFailure(t, unexpectedReport, constants.ErrUnexpectedEvidenceArtifact, nestedUnexpectedPath)

	bundle.Artifacts = append(bundle.Artifacts, &compliancev1.BundleArtifact{
		BundlePath: constants.ComplianceBundleUnexpectedTestPath,
		Sha256:     strings.Repeat("0", sha256.Size*2),
		MediaType:  constants.MediaTypeJSON,
		Profile:    constants.ComplianceBundleProfilePublic,
	})
	mutatedDescriptorBody, err := compliancev1.MarshalCanonical(bundle)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), descriptorPath, mutatedDescriptorBody, constants.PermFilePublic))
	input, err = loadComplianceReportBundleInput(context.Background(), fileSvc.Resolve(descriptorPath), trustPath, "")
	require.NoError(t, err)
	missingReport, err := compliancereport.VerifyComplianceReportBundle(context.Background(), compliancereport.BundleVerificationRequest{Bundle: input.bundle, Reader: input.reader, TrustPolicy: input.trustPolicy, VerifiedAt: windowEnd})
	require.NoError(t, err)
	require.NoError(t, input.close())
	assert.False(t, missingReport.GetValid())
	assertComplianceVerificationFailure(t, missingReport, constants.ErrBundleArtifactMissing, constants.ComplianceBundleUnexpectedTestPath)
}

func TestComplianceReportGenerateCmdWithConfig_PersistedEvalSourceMutationsFailIndependentReplay(t *testing.T) {
	tests := []struct {
		name         string
		relativePath string
		body         []byte
	}{
		{name: "verification report", relativePath: constants.ComplianceBundleSourceVerificationFilename, body: []byte(`{}`)},
		{name: "run manifest", relativePath: path.Join(constants.ComplianceBundleSourceRuntimeDirname, constants.EvalRunManifestFilename), body: []byte(`{}`)},
		{name: "tasks", relativePath: path.Join(constants.ComplianceBundleSourceRuntimeDirname, constants.EvalRunTasksFilename), body: []byte("{}\n")},
		{name: "attempts", relativePath: path.Join(constants.ComplianceBundleSourceRuntimeDirname, constants.EvalRunAttemptsFilename), body: []byte("{}\n")},
		{name: "receipts", relativePath: path.Join(constants.ComplianceBundleSourceRuntimeDirname, constants.EvalRunReceiptsFilename), body: []byte("{}\n")},
		{name: "stages", relativePath: path.Join(constants.ComplianceBundleSourceRuntimeDirname, constants.EvalRunStagesFilename), body: []byte("{}\n")},
		{name: "metrics", relativePath: path.Join(constants.ComplianceBundleSourceRuntimeDirname, constants.EvalRunMetricsFilename), body: []byte("{}\n")},
		{name: "evidence index", relativePath: path.Join(constants.ComplianceBundleSourceRuntimeDirname, constants.EvalRunEvidenceIndexFilename), body: []byte("{}\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fileSvc, _ := newCmdTestEnv(t)
			runID := persistMinimalEvidenceGraphEvalFixture(t, fileSvc)
			scopeID := evidence.EvalScopeID("evidence-graph-suite")
			identity, policy, _ := complianceReportSigningFixtureForTest(t, scopeID)
			cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
				return identity, nil
			})
			configureComplianceReportGenerateCommand(t, cmd)
			windowStart := time.Unix(1_699_999_999, 0).UTC()
			windowEnd := time.Unix(1_700_000_100, 0).UTC()
			require.NoError(t, cmd.Flags().Set("scope-id", scopeID))
			require.NoError(t, cmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(windowStart.UnixMilli(), 10)))
			require.NoError(t, cmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(windowEnd.UnixMilli(), 10)))
			require.NoError(t, cmd.Flags().Set("eval-run", runID))
			var output bytes.Buffer
			cmd.SetOut(&output)
			require.NoError(t, cmd.RunE(cmd, nil))
			descriptorPath, err := fileSvc.Rel(string(bytes.TrimSpace(output.Bytes())))
			require.NoError(t, err)
			descriptorBody, err := fileSvc.ReadFile(context.Background(), descriptorPath)
			require.NoError(t, err)
			bundle := &compliancev1.ComplianceReportBundle{}
			require.NoError(t, compliancev1.UnmarshalCanonical(descriptorBody, bundle))
			bundleDir := path.Dir(descriptorPath)
			sourcePath := path.Join(bundleDir, constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, runID, test.relativePath)
			require.NoError(t, fileSvc.WriteFile(context.Background(), sourcePath, test.body, constants.PermFileReadOnly))
			root, err := os.OpenRoot(fileSvc.Resolve(bundleDir))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, root.Close()) })

			report, err := compliancereport.VerifyComplianceReportBundle(context.Background(), compliancereport.BundleVerificationRequest{
				Bundle:      bundle,
				Reader:      &complianceBundleRootReader{root: root},
				TrustPolicy: policy,
				VerifiedAt:  windowEnd,
			})

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertComplianceVerificationFailure(t, report, constants.ErrEvalRunVerificationFailed, path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceEvalsDirname, runID, constants.ComplianceBundleSourceVerificationFilename))
		})
	}
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
			fileSvc, _ := newCmdTestEnv(t)
			projectRoot := writeDemoProvenanceTree(t)
			runID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)
			scopeID := constants.DemoScopeFedRAMP
			identity, policy, _ := complianceReportSigningFixtureForTest(t, scopeID)
			cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), func(string) evidence.ProvenanceSource {
				return evidence.NewDemoDirectoryProvenanceSource(projectRoot)
			}, func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
				return identity, nil
			})
			configureComplianceReportGenerateCommand(t, cmd)
			windowStart := time.Unix(1_699_999_999, 0).UTC()
			windowEnd := time.Unix(1_700_000_100, 0).UTC()
			require.NoError(t, cmd.Flags().Set("scope-id", scopeID))
			require.NoError(t, cmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(windowStart.UnixMilli(), 10)))
			require.NoError(t, cmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(windowEnd.UnixMilli(), 10)))
			require.NoError(t, cmd.Flags().Set("demo-run", runID))
			var output bytes.Buffer
			cmd.SetOut(&output)
			require.NoError(t, cmd.RunE(cmd, nil))
			descriptorAbsolutePath := string(bytes.TrimSpace(output.Bytes()))
			trustBody, err := compliancev1.MarshalCanonical(policy)
			require.NoError(t, err)
			trustPath := filepath.Join(t.TempDir(), constants.ComplianceReportTrustPolicyTestFilename)
			require.NoError(t, os.WriteFile(trustPath, trustBody, constants.PermFilePublic))
			verifyCmd := complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, func() time.Time { return windowEnd })
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
				VerifiedAt:  windowEnd,
			})

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertComplianceVerificationFailure(t, report, constants.ErrDemoRunVerificationFailed, path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceVerificationFilename))
		})
	}
}

func TestBuildDemoVerificationArtifacts_EmbedsValidExistingVerifierResult(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
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
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t))
	configureComplianceReportGenerateCommand(t, cmd)
	require.NoError(t, cmd.Flags().Set("scope-id", "scope-1"))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", "1700000000000"))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", "1700000001000"))
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

func TestComplianceReportGenerateCmdWithConfig_RejectsInvalidEvidenceWindow(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t))
	require.NoError(t, cmd.Flags().Set("scope-id", "scope-1"))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", "1700000001000"))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", "1700000000000"))
	require.NoError(t, cmd.Flags().Set("eval-run", "run-1"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
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
	closeErr := errors.New("close failed")
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
	fileSvc, _ := newCmdTestEnv(t)
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
