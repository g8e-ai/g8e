// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// mockAuditReader is a mock implementation of AuditEvidenceReader for unit testing.
type mockAuditReader struct {
	receipts     []*models.ActionReceiptRecord
	receiptsErr  error
	events       []*storage.Event
	eventsErr    error
	mutations    []*storage.FileMutationLog
	mutationsErr error
}

func (m *mockAuditReader) ListActionReceipts(_ string, _, _ int) ([]*models.ActionReceiptRecord, error) {
	if m.receiptsErr != nil {
		return nil, m.receiptsErr
	}
	return m.receipts, nil
}

func (m *mockAuditReader) ListEvents(_ string, _, _ int) ([]*storage.Event, error) {
	if m.eventsErr != nil {
		return nil, m.eventsErr
	}
	return m.events, nil
}

func (m *mockAuditReader) ListFileMutations(_, _ int) ([]*storage.FileMutationLog, error) {
	if m.mutationsErr != nil {
		return nil, m.mutationsErr
	}
	return m.mutations, nil
}

// mockLedgerReader is a mock implementation of LedgerEvidenceReader for unit testing.
type mockLedgerReader struct {
	merkleRoot    string
	merkleRootErr error
	commits       []storage.LedgerCommit
	commitsErr    error
}

func (m *mockLedgerReader) GetStateMerkleRoot() (string, error) {
	if m.merkleRootErr != nil {
		return "", m.merkleRootErr
	}
	return m.merkleRoot, nil
}

func (m *mockLedgerReader) ListCommits(_ string, _ int) ([]storage.LedgerCommit, error) {
	if m.commitsErr != nil {
		return nil, m.commitsErr
	}
	return m.commits, nil
}

// mockCommitmentReader is a mock implementation of CommitmentEvidenceReader for unit testing.
type mockCommitmentReader struct {
	commitments    []*storage.CommitmentRow
	commitmentsErr error
}

func (m *mockCommitmentReader) ListCommitments() ([]*storage.CommitmentRow, error) {
	if m.commitmentsErr != nil {
		return nil, m.commitmentsErr
	}
	return m.commitments, nil
}

type mockKSIHistoryReader struct {
	snapshots []KSIResultSet
	err       error
}

func (m *mockKSIHistoryReader) ListSnapshots(context.Context) ([]KSIResultSet, error) {
	return m.snapshots, m.err
}

type mockEvalGraderReader struct {
	results []GraderResult
	err     error
}

func (m *mockEvalGraderReader) ListGraderResults(context.Context) ([]GraderResult, error) {
	return m.results, m.err
}

// testCatalog returns a minimal catalog for evaluator tests with 4 KSIs
// across different categories, all applicable to Class C. LastValidatedUnixMs
// is set to now so KSIs are not stale by default; the stale test overrides this.
func testCatalog() *KSICatalog {
	now := time.Now().UnixMilli()
	return &KSICatalog{
		Version: "test-1.0",
		Source:  "test",
		KSIs: []KSI{
			{
				ID:                  "KSI-CMT-01",
				Title:               "Logging Changes",
				Category:            KSICategoryCMT,
				ControlRefs:         []string{"AU-2", "CM-3"},
				ApplicableClasses:   []CertificationClass{ClassB, ClassC},
				ValidationCycle:     ValidationCycleMachine,
				LastValidatedUnixMs: now,
			},
			{
				ID:                  "KSI-MLA-07",
				Title:               "Audit Trail Protection",
				Category:            KSICategoryMLA,
				ControlRefs:         []string{"AU-2", "AU-6"},
				ApplicableClasses:   []CertificationClass{ClassB, ClassC},
				ValidationCycle:     ValidationCycleMachine,
				LastValidatedUnixMs: now,
			},
			{
				ID:                  "KSI-SVC-05",
				Title:               "Validating Resource Integrity",
				Category:            KSICategorySVC,
				ControlRefs:         []string{"CM-2", "SI-7"},
				ApplicableClasses:   []CertificationClass{ClassB, ClassC},
				ValidationCycle:     ValidationCycleMachine,
				LastValidatedUnixMs: now,
			},
			{
				ID:                  "KSI-CED-01",
				Title:               "Reviewing All Training",
				Category:            KSICategoryCED,
				ControlRefs:         []string{"AT-2"},
				ApplicableClasses:   []CertificationClass{ClassB, ClassC},
				ValidationCycle:     ValidationCycleNonMachine,
				LastValidatedUnixMs: now,
			},
		},
	}
}

// fullDeps returns EvaluatorDeps with all mocks populated with valid evidence.
func fullDeps(t *testing.T) EvaluatorDeps {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	signerKeyID := hex.EncodeToString(privateKey.Public().(ed25519.PublicKey))
	receipt := &operatorv1.ActionReceipt{TransactionId: "tx-1", TransactionHash: "hash-1", StateRootBefore: "state-before", StateRootAfter: "state-after", ExecutedAtUnixMs: 1_700_000_001_000, SignerKeyId: signerKeyID}
	payload, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	attestation := &operatorv1.ReceiptPersistenceAttestation{TransactionId: receipt.TransactionId, ReceiptSignatureDigest: governance.SignatureDigest([]string{receipt.Signature}), PersistedAtUnixMs: 1_700_000_002_000, AuditRecordId: receipt.TransactionId, SignerKeyId: signerKeyID}
	payload, err = governance.CanonicalizeReceiptPersistenceAttestation(attestation)
	require.NoError(t, err)
	attestation.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	receipt.FinalPersistenceAttestation = attestation
	commitment := &operatorv1.CommitmentAttestation{TransactionId: "tx-1", TransactionHash: "hash-1", StateRootAtCommit: "state-before", CommittedAtUnixMs: 1_700_000_003_000, AuditorKeyId: signerKeyID}
	payload, err = governance.CanonicalizeCommitmentAttestation(commitment)
	require.NoError(t, err)
	commitmentHash := sha256.Sum256(payload)
	commitment.Hash = hex.EncodeToString(commitmentHash[:])
	commitment.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	commitmentJSON, err := json.Marshal(commitment)
	require.NoError(t, err)
	committedAt := time.UnixMilli(commitment.CommittedAtUnixMs).UTC()
	now := time.Now().UTC()
	return EvaluatorDeps{
		Audit: &mockAuditReader{
			receipts: []*models.ActionReceiptRecord{{
				TransactionID: receipt.TransactionId, TransactionHash: receipt.TransactionHash,
				StateRootBefore: receipt.StateRootBefore, StateRootAfter: receipt.StateRootAfter,
				ExecutedAt: now, Signature: receipt.Signature, SignerKeyID: receipt.SignerKeyId, ActionReceipt: receipt,
			}},
			events:    []*storage.Event{{ID: 1, OperatorSessionID: "sess-1"}},
			mutations: []*storage.FileMutationLog{{ID: 1, Filepath: "/etc/config"}},
		},
		Ledger: &mockLedgerReader{
			merkleRoot: "commit-1",
			commits:    []storage.LedgerCommit{{CommitHash: "commit-1", ParentHash: "bootstrap-commit", TimestampUTC: now}},
		},
		Commitments: &mockCommitmentReader{commitments: []*storage.CommitmentRow{{
			Seq: 1, TransactionID: commitment.TransactionId, TransactionHash: commitment.TransactionHash,
			Hash: commitment.Hash, StateRootAtCommit: commitment.StateRootAtCommit,
			CommittedAt: committedAt, AuditorKeyID: commitment.AuditorKeyId, Signature: commitment.Signature,
			AttestationJSON: commitmentJSON,
		}}},
		History: &mockKSIHistoryReader{snapshots: []KSIResultSet{{
			Class: ClassC, EvaluatedAtMs: now.UnixMilli(), Results: []KSIResult{
				{ID: "KSI-CMT-01", Status: KSIStatusSatisfied},
				{ID: "KSI-MLA-08", Status: KSIStatusSatisfied},
			},
		}}},
		Graders: &mockEvalGraderReader{results: []GraderResult{{
			ArtifactID: "metric:protocol-chain", GraderID: "protocol_chain", GraderVersion: "1.0.0",
			SHA256: strings.Repeat("a", 64), Verified: true, ProducedAt: now,
			Evidence: []*compliancev1.ComplianceEvidenceReference{{ArtifactId: "receipt:tx-1", Sha256: strings.Repeat("b", 64)}},
		}}},
	}
}

func testKSIMethod(name string, property KSIMeasuredProperty, evaluate ksiMethodEvaluator) KSIMethod {
	return newKSIMethod(name, KSIArtifactActionReceipts, KSICollectionAuditStore, KSIVerifierStructural, property, evaluate)
}

// testBinding returns a valid EvaluationBinding for evaluator tests.
func testBinding(t *testing.T) EvaluationBinding {
	t.Helper()
	now := time.Now()
	return EvaluationBinding{
		ScopeID:            "test-scope",
		RunID:              "test-run",
		WindowStartUnixMs:  now.UnixMilli(),
		WindowEndUnixMs:    now.Add(time.Second).UnixMilli(),
		EvaluatorID:        constants.KSIEvaluatorID,
		EvaluatorVersion:   constants.KSIEvaluatorVersion,
		MethodDefinitionID: constants.KSIMethodDefinitionVersion,
	}
}

// TestKSIEvaluator_RegisterMethods binds methods to a KSI and verifies MethodCount.
func TestKSIEvaluator_RegisterMethods(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	assert.Equal(t, 0, eval.MethodCount("KSI-CMT-01"))

	method := testKSIMethod("testMethod", KSIPropertyPresence, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, nil, nil
	})
	require.NoError(t, eval.RegisterMethods("KSI-CMT-01", method))
	assert.Equal(t, 1, eval.MethodCount("KSI-CMT-01"))

	require.ErrorIs(t, eval.RegisterMethods("KSI-CMT-01", method), constants.ErrKSIMethodNotIndependent)
	assert.Equal(t, 1, eval.MethodCount("KSI-CMT-01"))

	sameIdentity := method
	sameIdentity.MeasuredProperty = KSIPropertySignatureValidity
	require.ErrorIs(t, eval.RegisterMethods("KSI-CMT-01", sameIdentity), constants.ErrKSIMethodInvalid)
	assert.Equal(t, 1, eval.MethodCount("KSI-CMT-01"))
}

func TestKSIEvaluator_RegisterMethods_AllowsEachIndependentMetadataDimension(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*KSIMethod)
	}{
		{name: "artifact identity", mutate: func(method *KSIMethod) { method.ArtifactIdentity = KSIArtifactFileMutations }},
		{name: "collection boundary", mutate: func(method *KSIMethod) { method.CollectionBoundary = KSICollectionEvalResults }},
		{name: "verifier family", mutate: func(method *KSIMethod) { method.VerifierFamily = KSIVerifierCryptographic }},
		{name: "measured property", mutate: func(method *KSIMethod) { method.MeasuredProperty = KSIPropertySignatureValidity }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewKSIEvaluator(testCatalog())
			first := testKSIMethod("first", KSIPropertyPresence, func(context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
				return true, nil, nil
			})
			second := first
			second.Name = "second"
			tt.mutate(&second)

			require.NoError(t, eval.RegisterMethods("KSI-CMT-01", first, second))
			assert.Equal(t, 2, eval.MethodCount("KSI-CMT-01"))
		})
	}
}

func TestKSIEvaluator_RegisterMethods_RejectsInvalidMetadataAtomically(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*KSIMethod)
	}{
		{name: "name", mutate: func(method *KSIMethod) { method.Name = "" }},
		{name: "version", mutate: func(method *KSIMethod) { method.Version = "" }},
		{name: "artifact identity", mutate: func(method *KSIMethod) { method.ArtifactIdentity = "" }},
		{name: "unknown artifact identity", mutate: func(method *KSIMethod) { method.ArtifactIdentity = "unknown" }},
		{name: "collection boundary", mutate: func(method *KSIMethod) { method.CollectionBoundary = "" }},
		{name: "unknown collection boundary", mutate: func(method *KSIMethod) { method.CollectionBoundary = "unknown" }},
		{name: "verifier family", mutate: func(method *KSIMethod) { method.VerifierFamily = "" }},
		{name: "unknown verifier family", mutate: func(method *KSIMethod) { method.VerifierFamily = "unknown" }},
		{name: "measured property", mutate: func(method *KSIMethod) { method.MeasuredProperty = "" }},
		{name: "unknown measured property", mutate: func(method *KSIMethod) { method.MeasuredProperty = "unknown" }},
		{name: "satisfied outcome for failure", mutate: func(method *KSIMethod) { method.UnsatisfiedOutcome = KSIOutcomeSatisfied }},
		{name: "method failure outcome for false result", mutate: func(method *KSIMethod) { method.UnsatisfiedOutcome = KSIOutcomeMethodFailure }},
		{name: "not applicable outcome for false result", mutate: func(method *KSIMethod) { method.UnsatisfiedOutcome = KSIOutcomeNotApplicable }},
		{name: "unknown unsatisfied outcome", mutate: func(method *KSIMethod) { method.UnsatisfiedOutcome = "unknown" }},
		{name: "evaluator", mutate: func(method *KSIMethod) { method.evaluate = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewKSIEvaluator(testCatalog())
			valid := testKSIMethod("valid", KSIPropertyPresence, func(context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
				return true, nil, nil
			})
			invalid := valid
			invalid.Name = "invalid"
			invalid.MeasuredProperty = KSIPropertySignatureValidity
			tt.mutate(&invalid)

			require.ErrorIs(t, eval.RegisterMethods("KSI-CMT-01", valid, invalid), constants.ErrKSIMethodInvalid)
			assert.Equal(t, 0, eval.MethodCount("KSI-CMT-01"))
		})
	}
}

// TestKSIEvaluator_RegisterMethods_UnknownKSI_ReturnsError verifies that registering
// methods for an unknown KSI ID returns an error.
func TestKSIEvaluator_RegisterMethods_UnknownKSI_ReturnsError(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	err := eval.RegisterMethods("KSI-FAKE-99", testKSIMethod("testMethod", KSIPropertyPresence, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, nil, nil
	}))
	require.ErrorIs(t, err, constants.ErrKSICatalogInvalid)
}

// TestKSIEvaluator_RegisterDefaultMethods registers default methods and verifies
// that automatable KSIs get >=2 methods while non-automatable KSIs get 0.
func TestKSIEvaluator_RegisterDefaultMethods(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)
	require.NoError(t, eval.RegisterDefaultMethods(fullDeps(t)))

	assert.GreaterOrEqual(t, eval.MethodCount("KSI-CMT-01"), 2)
	assert.GreaterOrEqual(t, eval.MethodCount("KSI-MLA-07"), 2)
	assert.GreaterOrEqual(t, eval.MethodCount("KSI-SVC-05"), 2)
	assert.Equal(t, 0, eval.MethodCount("KSI-CED-01"), "CED KSIs are not automatable by g8e")
}

// TestKSIEvaluator_Evaluate_AllSatisfied verifies that a fully-evidenced
// evaluation produces all satisfied results for automatable KSIs.
func TestKSIEvaluator_Evaluate_AllSatisfied(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)
	require.NoError(t, eval.RegisterDefaultMethods(fullDeps(t)))

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, ClassC, result.Class)
	assert.NotEmpty(t, result.Results)

	for _, res := range result.Results {
		if res.MethodCount >= 2 {
			assert.Equal(t, KSIStatusSatisfied, res.Status,
				"KSI %s should be satisfied with %d methods", res.ID, res.MethodCount)
			assert.Equal(t, KSIOutcomeSatisfied, res.Outcome)
			assert.NotEmpty(t, res.Evidence, "KSI %s should have evidence", res.ID)
		}
	}
	assert.Equal(t, 3, result.SatisfiedCount(), "3 automatable KSIs should be satisfied")
}

// TestKSIEvaluator_Evaluate_InsufficientMethods_FailClosed verifies that KSIs
// with fewer than the minimum required methods for Class C are marked not_satisfied.
func TestKSIEvaluator_Evaluate_InsufficientMethods_FailClosed(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)
	require.NoError(t, eval.RegisterDefaultMethods(fullDeps(t)))

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)

	// KSI-CED-01 has 0 methods (not automatable by g8e), should fail-closed
	for _, res := range result.Results {
		if res.ID == "KSI-CED-01" {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"KSI-CED-01 should fail-closed with 0 methods for Class C")
			assert.Equal(t, 0, res.MethodCount)
		}
	}
	assert.True(t, result.HasFailures(), "Result set should have failures due to KSI-CED-01")
}

// TestKSIEvaluator_Evaluate_StaleKSI_FailClosed verifies that a stale KSI
// (LastValidatedUnixMs beyond validation cycle) is marked not_satisfied.
func TestKSIEvaluator_Evaluate_StaleKSI_FailClosed(t *testing.T) {
	catalog := testCatalog()

	// Make KSI-CMT-01 stale by setting LastValidatedUnixMs to 8 days ago
	// (7-day cycle for machine-based resources)
	for i := range catalog.KSIs {
		if catalog.KSIs[i].ID == "KSI-CMT-01" {
			catalog.KSIs[i].LastValidatedUnixMs = time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
		}
	}

	eval := NewKSIEvaluator(catalog)
	require.NoError(t, eval.RegisterDefaultMethods(fullDeps(t)))

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)

	for _, res := range result.Results {
		if res.ID == "KSI-CMT-01" {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"Stale KSI-CMT-01 should be not_satisfied despite having methods")
			assert.Equal(t, KSIOutcomeStaleEvidence, res.Outcome)
		}
	}
}

// TestKSIEvaluator_Evaluate_MethodError_FailClosed verifies that a method
// returning an error causes the KSI to be marked not_satisfied.
func TestKSIEvaluator_Evaluate_MethodError_FailClosed(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	errMethod := testKSIMethod("errorMethod", KSIPropertyPresence, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return false, nil, errors.New("simulated storage failure")
	})
	okMethod := testKSIMethod("okMethod", KSIPropertySignatureValidity, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeReceiptID, "tx-1")}, nil
	})

	require.NoError(t, eval.RegisterMethods("KSI-CMT-01", errMethod, okMethod))

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)

	for _, res := range result.Results {
		if res.ID == "KSI-CMT-01" {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"KSI with a method error should be not_satisfied")
		}
	}
}

// TestKSIEvaluator_Evaluate_MethodReturnsFalse_FailClosed verifies that a
// method returning false (but no error) causes the KSI to be not_satisfied.
func TestKSIEvaluator_Evaluate_MethodReturnsFalse_FailClosed(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	falseMethod := testKSIMethod("falseMethod", KSIPropertyPresence, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return false, nil, nil
	})
	trueMethod := testKSIMethod("trueMethod", KSIPropertyStateRootMatchesHead, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeMerkleRoot, "root")}, nil
	})

	require.NoError(t, eval.RegisterMethods("KSI-CMT-01", falseMethod, trueMethod))

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)

	for _, res := range result.Results {
		if res.ID == "KSI-CMT-01" {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"KSI with a false method result should be not_satisfied")
		}
	}
}

func TestKSIEvaluator_Evaluate_SeparatesDetailedOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		configure   func(*KSICatalog, *KSIEvaluator)
		wantOutcome KSIOutcome
	}{
		{
			name: "method execution failure",
			configure: func(_ *KSICatalog, evaluator *KSIEvaluator) {
				method := testKSIMethod("methodFailure", KSIPropertyPresence, func(context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
					return false, nil, errors.New("method failed")
				})
				require.NoError(t, evaluator.RegisterMethods("KSI-CMT-01", method))
			},
			wantOutcome: KSIOutcomeMethodFailure,
		},
		{
			name: "invalid evidence",
			configure: func(_ *KSICatalog, evaluator *KSIEvaluator) {
				method := testKSIMethod("invalidEvidence", KSIPropertyPresence, func(context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
					return false, nil, nil
				})
				require.NoError(t, evaluator.RegisterMethods("KSI-CMT-01", method))
			},
			wantOutcome: KSIOutcomeInvalidEvidence,
		},
		{
			name: "stale evidence",
			configure: func(_ *KSICatalog, evaluator *KSIEvaluator) {
				method := testKSIMethod("staleEvidence", KSIPropertyPresence, func(context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
					return false, nil, nil
				})
				method.UnsatisfiedOutcome = KSIOutcomeStaleEvidence
				require.NoError(t, evaluator.RegisterMethods("KSI-CMT-01", method))
			},
			wantOutcome: KSIOutcomeStaleEvidence,
		},
		{
			name:        "unsupported automation",
			configure:   func(_ *KSICatalog, _ *KSIEvaluator) {},
			wantOutcome: KSIOutcomeUnsupportedAutomation,
		},
		{
			name: "customer attestation required",
			configure: func(_ *KSICatalog, evaluator *KSIEvaluator) {
				method := testKSIMethod("customerAttestation", KSIPropertyPresence, func(context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
					return false, nil, nil
				})
				method.UnsatisfiedOutcome = KSIOutcomeCustomerAttestationRequired
				require.NoError(t, evaluator.RegisterMethods("KSI-CMT-01", method))
			},
			wantOutcome: KSIOutcomeCustomerAttestationRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := testCatalog()
			evaluator := NewKSIEvaluator(catalog)
			tt.configure(catalog, evaluator)

			resultSet, err := evaluator.Evaluate(context.Background(), ClassB, testBinding(t))
			require.NoError(t, err)
			result := findKSIResult(t, resultSet, "KSI-CMT-01")
			assert.Equal(t, KSIStatusNotSatisfied, result.Status)
			assert.Equal(t, tt.wantOutcome, result.Outcome)
		})
	}
}

func findKSIResult(t *testing.T, resultSet *KSIResultSet, ksiID string) KSIResult {
	t.Helper()
	for _, result := range resultSet.Results {
		if result.ID == ksiID {
			return result
		}
	}
	t.Fatalf("missing KSI result %s", ksiID)
	return KSIResult{}
}

// TestKSIEvaluator_Evaluate_NilDeps_FailClosed verifies that nil dependency
// fields cause methods to return false (fail-closed).
func TestKSIEvaluator_Evaluate_NilDeps_FailClosed(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)
	require.NoError(t, eval.RegisterDefaultMethods(EvaluatorDeps{}))

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)

	for _, res := range result.Results {
		if res.MethodCount >= 2 {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"KSI %s should fail-closed with nil deps", res.ID)
		}
	}
}

// TestKSIEvaluator_Evaluate_EmptyStores_FailClosed verifies that empty audit
// stores and ledgers cause methods to return false.
func TestKSIEvaluator_Evaluate_EmptyStores_FailClosed(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	deps := EvaluatorDeps{
		Audit:       &mockAuditReader{},
		Ledger:      &mockLedgerReader{},
		Commitments: &mockCommitmentReader{},
	}
	require.NoError(t, eval.RegisterDefaultMethods(deps))

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)

	for _, res := range result.Results {
		if res.MethodCount >= 2 {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"KSI %s should be not_satisfied with empty stores", res.ID)
		}
	}
	assert.True(t, result.HasFailures())
}

// TestKSIEvaluator_Evaluate_ClassB_LowerThreshold verifies that Class B
// requires only 1 method (not 2), so KSIs with 1 method are satisfied.
func TestKSIEvaluator_Evaluate_ClassB_LowerThreshold(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	// Register only 1 method for KSI-CMT-01
	okMethod := testKSIMethod("okMethod", KSIPropertyPresence, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeReceiptID, "tx-1")}, nil
	})
	require.NoError(t, eval.RegisterMethods("KSI-CMT-01", okMethod))

	result, err := eval.Evaluate(context.Background(), ClassB, testBinding(t))
	require.NoError(t, err)

	for _, res := range result.Results {
		if res.ID == "KSI-CMT-01" {
			assert.Equal(t, KSIStatusSatisfied, res.Status,
				"KSI-CMT-01 should be satisfied with 1 method for Class B")
			assert.Equal(t, 1, res.MethodCount)
		}
	}
}

// TestKSIEvaluator_Evaluate_ClassA_NoMinimum verifies that Class A has no
// minimum method requirement (MAY automate), so KSIs with 0 methods are
// still evaluated without method-count failure.
func TestKSIEvaluator_Evaluate_ClassA_NoMinimum(t *testing.T) {
	catalog := testCatalog()

	// Add a Class A KSI
	catalog.KSIs = append(catalog.KSIs, KSI{
		ID:                  "KSI-TEST-A1",
		Title:               "Class A Test",
		Category:            KSICategoryCMT,
		ControlRefs:         []string{"CM-3"},
		ApplicableClasses:   []CertificationClass{ClassA},
		ValidationCycle:     ValidationCycleMachine,
		LastValidatedUnixMs: time.Now().UnixMilli(),
	})

	eval := NewKSIEvaluator(catalog)

	result, err := eval.Evaluate(context.Background(), ClassA, testBinding(t))
	require.NoError(t, err)

	assert.Len(t, result.Results, 1)
	assert.Equal(t, "KSI-TEST-A1", result.Results[0].ID)
	// Class A has 0 minimum methods, so 0 methods is not a failure
	// But with 0 methods, allSatisfied stays true (vacuous truth)
	assert.Equal(t, KSIStatusSatisfied, result.Results[0].Status,
		"Class A KSI with 0 methods should be satisfied (no minimum)")
}

// TestKSIEvaluator_Evaluate_ContextCancellation verifies that context
// cancellation aborts evaluation.
func TestKSIEvaluator_Evaluate_ContextCancellation(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)
	require.NoError(t, eval.RegisterDefaultMethods(fullDeps(t)))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := eval.Evaluate(ctx, ClassC, testBinding(t))
	assert.ErrorIs(t, err, context.Canceled)
}

// TestKSIResultSet_HasFailures verifies HasFailures detects not_satisfied results.
func TestKSIResultSet_HasFailures(t *testing.T) {
	rs := &KSIResultSet{
		Results: []KSIResult{
			{ID: "KSI-1", Status: KSIStatusSatisfied},
			{ID: "KSI-2", Status: KSIStatusNotSatisfied},
		},
	}
	assert.True(t, rs.HasFailures())

	rs2 := &KSIResultSet{
		Results: []KSIResult{
			{ID: "KSI-1", Status: KSIStatusSatisfied},
		},
	}
	assert.False(t, rs2.HasFailures())
}

// TestKSIResultSet_SatisfiedCount verifies SatisfiedCount counts correctly.
func TestKSIResultSet_SatisfiedCount(t *testing.T) {
	rs := &KSIResultSet{
		Results: []KSIResult{
			{ID: "KSI-1", Status: KSIStatusSatisfied},
			{ID: "KSI-2", Status: KSIStatusSatisfied},
			{ID: "KSI-3", Status: KSIStatusNotSatisfied},
		},
	}
	assert.Equal(t, 2, rs.SatisfiedCount())
	assert.Equal(t, 1, rs.NotSatisfiedCount())
}

// TestKSIResultSet_Validate verifies result set validation against a catalog.
func TestKSIResultSet_Validate(t *testing.T) {
	catalog := testCatalog()
	binding := testBinding(t)

	t.Run("valid result set", func(t *testing.T) {
		rs := &KSIResultSet{
			Binding: binding,
			Results: []KSIResult{
				{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, Outcome: KSIOutcomeSatisfied, Binding: binding},
				{ID: "KSI-MLA-07", Status: KSIStatusSatisfied, Outcome: KSIOutcomeSatisfied, Binding: binding},
			},
		}
		assert.NoError(t, rs.Validate(catalog))
	})

	t.Run("empty result set", func(t *testing.T) {
		rs := &KSIResultSet{Results: []KSIResult{}}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrValidationFailed)
		assert.Contains(t, err.Error(), "empty result set")
	})

	t.Run("missing result set binding", func(t *testing.T) {
		rs := &KSIResultSet{
			Results: []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, Outcome: KSIOutcomeSatisfied, Binding: binding}},
		}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrValidationFailed)
		assert.ErrorIs(t, err, constants.ErrKSIBindingIncomplete)
	})

	t.Run("result binding mismatch", func(t *testing.T) {
		mismatched := binding
		mismatched.RunID = "other-run"
		rs := &KSIResultSet{
			Binding: binding,
			Results: []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, Outcome: KSIOutcomeSatisfied, Binding: mismatched}},
		}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrKSIBindingMismatch)
	})

	t.Run("evidence scope mismatch", func(t *testing.T) {
		rs := &KSIResultSet{
			Binding: binding,
			Results: []KSIResult{{
				ID: "KSI-CMT-01", Status: KSIStatusSatisfied, Outcome: KSIOutcomeSatisfied, Binding: binding,
				Evidence: []*compliancev1.ComplianceEvidenceReference{{ArtifactId: "tx-1", ArtifactType: string(EvidenceTypeReceiptID), ScopeId: "other-scope"}},
			}},
		}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrKSIBindingMismatch)
	})

	t.Run("unknown KSI ID", func(t *testing.T) {
		rs := &KSIResultSet{
			Binding: binding,
			Results: []KSIResult{
				{ID: "KSI-FAKE-99", Status: KSIStatusSatisfied, Outcome: KSIOutcomeSatisfied, Binding: binding},
			},
		}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrValidationFailed)
		assert.Contains(t, err.Error(), "unknown KSI")
	})

	t.Run("empty KSI ID in result", func(t *testing.T) {
		rs := &KSIResultSet{
			Binding: binding,
			Results: []KSIResult{
				{ID: "", Status: KSIStatusSatisfied, Outcome: KSIOutcomeSatisfied, Binding: binding},
			},
		}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrValidationFailed)
		assert.Contains(t, err.Error(), "empty ID")
	})

	t.Run("missing detailed outcome", func(t *testing.T) {
		rs := &KSIResultSet{Binding: binding, Results: []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusNotSatisfied, Binding: binding}}}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrValidationFailed)
	})

	t.Run("status contradicts detailed outcome", func(t *testing.T) {
		rs := &KSIResultSet{Binding: binding, Results: []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, Outcome: KSIOutcomeInvalidEvidence, Binding: binding}}}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrValidationFailed)
	})
}

// TestDefaultMethods_CommitmentChainIntact verifies that the commitment chain
// integrity method correctly detects broken chains.
func TestDefaultMethods_CommitmentChainIntact(t *testing.T) {
	deps := fullDeps(t)
	methods := DefaultMethods(deps)

	// KSI-SVC-05 uses commitmentChainIntact as one of its methods
	svcMethods := methods["KSI-SVC-05"]
	require.Len(t, svcMethods, 2)

	// With intact chain (fullDeps), at least one method should return true
	anyTrue := false
	for _, m := range svcMethods {
		ok, _, err := m.evaluate(context.Background())
		require.NoError(t, err)
		if ok {
			anyTrue = true
		}
	}
	assert.True(t, anyTrue, "At least one method should return true with intact chain")

	// Now break the chain
	deps.Commitments = &mockCommitmentReader{
		commitments: []*storage.CommitmentRow{
			{Seq: 1, Hash: "hash-1", PriorCommitmentHash: ""},
			{Seq: 2, Hash: "hash-2", PriorCommitmentHash: "wrong-prior"},
		},
	}
	methods = DefaultMethods(deps)
	svcMethods = methods["KSI-SVC-05"]

	// commitmentChainIntact should now return false
	allTrue := true
	for _, m := range svcMethods {
		ok, _, err := m.evaluate(context.Background())
		require.NoError(t, err)
		if !ok {
			allTrue = false
		}
	}
	assert.False(t, allTrue, "At least one method should return false with broken chain")
}

// TestDefaultMethods_ReceiptAndPersistenceCryptographicVerification verifies
// canonical receipt and final-persistence evidence fail closed under mutation.
func TestDefaultMethods_ReceiptAndPersistenceCryptographicVerification(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*mockAuditReader)
		want   bool
	}{
		{name: "valid receipt and persistence", want: true},
		{name: "missing canonical receipt", mutate: func(audit *mockAuditReader) { audit.receipts[0].ActionReceipt = nil }},
		{name: "record signature binding mismatch", mutate: func(audit *mockAuditReader) { audit.receipts[0].Signature = "different" }},
		{name: "malformed receipt signature", mutate: func(audit *mockAuditReader) {
			audit.receipts[0].ActionReceipt.Signature = "not-hex"
			audit.receipts[0].Signature = "not-hex"
		}},
		{name: "missing final persistence attestation", mutate: func(audit *mockAuditReader) {
			audit.receipts[0].ActionReceipt.FinalPersistenceAttestation = nil
		}},
		{name: "mutated persistence signature", mutate: func(audit *mockAuditReader) {
			audit.receipts[0].ActionReceipt.FinalPersistenceAttestation.Signature = "not-hex"
		}},
		{name: "mutated receipt signature digest", mutate: func(audit *mockAuditReader) {
			audit.receipts[0].ActionReceipt.FinalPersistenceAttestation.ReceiptSignatureDigest = "wrong-digest"
		}},
		{name: "one invalid receipt among valid receipts", mutate: func(audit *mockAuditReader) {
			invalid := *audit.receipts[0]
			invalid.TransactionID = "tx-2"
			invalid.ActionReceipt = nil
			audit.receipts = append(audit.receipts, &invalid)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := fullDeps(t)
			audit := deps.Audit.(*mockAuditReader)
			if tt.mutate != nil {
				tt.mutate(audit)
			}
			methods := DefaultMethods(deps)["KSI-MLA-08"]
			require.Len(t, methods, 2)
			satisfied, evidence, err := methods[0].evaluate(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, satisfied)
			assert.NotEmpty(t, evidence)
		})
	}
}

// TestKSIEvaluator_Evaluate_FullIntegration verifies end-to-end evaluation
// with all deps populated, checking evidence anchors are present.
func TestKSIEvaluator_Evaluate_FullIntegration(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)
	require.NoError(t, eval.RegisterDefaultMethods(fullDeps(t)))

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)

	// Verify result set structure
	assert.Equal(t, ClassC, result.Class)
	assert.Greater(t, result.EvaluatedAtMs, int64(0))
	assert.Len(t, result.Results, 4) // 4 KSIs in testCatalog applicable to Class C

	// Verify each automatable KSI has evidence
	for _, res := range result.Results {
		if res.Status == KSIStatusSatisfied {
			assert.NotEmpty(t, res.Evidence, "Satisfied KSI %s must have evidence", res.ID)
			assert.GreaterOrEqual(t, res.MethodCount, 2, "Satisfied KSI %s must have >=2 methods", res.ID)
		}
	}

	// Validate against catalog
	assert.NoError(t, result.Validate(catalog))
}

// TestKSIEvaluator_Evaluate_NoMethodsRegistered verifies that evaluating
// without registering any methods produces all not_satisfied results for
// Class C (fail-closed on method count).
func TestKSIEvaluator_Evaluate_NoMethodsRegistered(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)

	for _, res := range result.Results {
		assert.Equal(t, KSIStatusNotSatisfied, res.Status,
			"KSI %s should be not_satisfied with 0 methods for Class C", res.ID)
		assert.Equal(t, 0, res.MethodCount)
	}
	assert.Equal(t, 4, result.NotSatisfiedCount())
	assert.Equal(t, 0, result.SatisfiedCount())
}

// TestKSIEvaluator_Evaluate_StorageError_FailClosed verifies that storage
// errors cause methods to return errors, which fail-closed the KSI.
func TestKSIEvaluator_Evaluate_StorageError_FailClosed(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	deps := EvaluatorDeps{
		Audit: &mockAuditReader{
			receiptsErr: fmt.Errorf("database unavailable"),
		},
		Ledger:      &mockLedgerReader{merkleRoot: "abc123", commits: []storage.LedgerCommit{{CommitHash: "c1"}}},
		Commitments: &mockCommitmentReader{commitments: []*storage.CommitmentRow{{Hash: "h1"}}},
	}
	require.NoError(t, eval.RegisterDefaultMethods(deps))

	result, err := eval.Evaluate(context.Background(), ClassC, testBinding(t))
	require.NoError(t, err)

	// KSIs that depend on audit store should be not_satisfied due to error
	for _, res := range result.Results {
		if res.ID == "KSI-CMT-01" || res.ID == "KSI-MLA-03" || res.ID == "KSI-IAM-05" {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"KSI %s should be not_satisfied due to audit store error", res.ID)
		}
	}
}

func TestDefaultMethods_CommitmentCryptographicVerification(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvaluatorDeps)
		want   bool
	}{
		{name: "valid signed commitment", want: true},
		{name: "missing commitment", mutate: func(deps *EvaluatorDeps) { deps.Commitments = &mockCommitmentReader{} }},
		{name: "malformed attestation", mutate: func(deps *EvaluatorDeps) {
			deps.Commitments.(*mockCommitmentReader).commitments[0].AttestationJSON = []byte("{")
		}},
		{name: "mutated hash", mutate: func(deps *EvaluatorDeps) {
			deps.Commitments.(*mockCommitmentReader).commitments[0].Hash = strings.Repeat("0", 64)
		}},
		{name: "mutated signature", mutate: func(deps *EvaluatorDeps) {
			deps.Commitments.(*mockCommitmentReader).commitments[0].Signature = "not-hex"
		}},
		{name: "nil dependency", mutate: func(deps *EvaluatorDeps) { deps.Commitments = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := fullDeps(t)
			if tt.mutate != nil {
				tt.mutate(&deps)
			}
			method := DefaultMethods(deps)["KSI-MLA-07"][1]
			got, evidence, err := method.evaluate(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			if tt.want {
				require.NotEmpty(t, evidence)
				assert.Equal(t, string(EvidenceTypeCommitmentSignature), evidence[0].ArtifactType)
				assert.Equal(t, constants.KSIMethodVerifierID, evidence[0].VerifierId)
				assert.Equal(t, "verified", evidence[0].VerificationStatus)
			} else if len(evidence) > 0 {
				assert.Equal(t, "failed", evidence[0].VerificationStatus)
			}
		})
	}
}

func TestDefaultMethods_LedgerBootstrapCommitIsNotOperationalEvidence(t *testing.T) {
	deps := fullDeps(t)
	deps.Ledger.(*mockLedgerReader).commits[0].ParentHash = ""
	methods := DefaultMethods(deps)["KSI-CMT-03"]
	require.Len(t, methods, 2)

	for _, method := range methods {
		satisfied, _, err := method.evaluate(context.Background())
		require.NoError(t, err)
		assert.False(t, satisfied)
	}
}

func TestDefaultMethods_LedgerMerkleRootMatchesHead(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvaluatorDeps)
		want   bool
	}{
		{name: "root matches head", want: true},
		{name: "root differs from head", mutate: func(deps *EvaluatorDeps) { deps.Ledger.(*mockLedgerReader).merkleRoot = "different" }},
		{name: "missing head commit", mutate: func(deps *EvaluatorDeps) { deps.Ledger.(*mockLedgerReader).commits = nil }},
		{name: "nil dependency", mutate: func(deps *EvaluatorDeps) { deps.Ledger = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := fullDeps(t)
			if tt.mutate != nil {
				tt.mutate(&deps)
			}
			got, _, err := DefaultMethods(deps)["KSI-SVC-05"][1].evaluate(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultMethods_IndependentStateObservation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvaluatorDeps)
		want   bool
	}{
		{name: "distinct state roots", want: true},
		{name: "missing state root", mutate: func(deps *EvaluatorDeps) { deps.Audit.(*mockAuditReader).receipts[0].StateRootAfter = "" }},
		{name: "unchanged state root", mutate: func(deps *EvaluatorDeps) { deps.Audit.(*mockAuditReader).receipts[0].StateRootAfter = "state-before" }},
		{name: "missing receipt", mutate: func(deps *EvaluatorDeps) { deps.Audit.(*mockAuditReader).receipts = nil }},
		{name: "nil dependency", mutate: func(deps *EvaluatorDeps) { deps.Audit = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := fullDeps(t)
			if tt.mutate != nil {
				tt.mutate(&deps)
			}
			got, _, err := DefaultMethods(deps)["KSI-CNA-01"][1].evaluate(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultMethods_DeterministicGraderResultsVerified(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvaluatorDeps)
		want   bool
	}{
		{name: "verified content-addressed result", want: true},
		{name: "missing result", mutate: func(deps *EvaluatorDeps) { deps.Graders = &mockEvalGraderReader{} }},
		{name: "unverified result", mutate: func(deps *EvaluatorDeps) { deps.Graders.(*mockEvalGraderReader).results[0].Verified = false }},
		{name: "malformed result digest", mutate: func(deps *EvaluatorDeps) { deps.Graders.(*mockEvalGraderReader).results[0].SHA256 = "bad" }},
		{name: "missing source evidence", mutate: func(deps *EvaluatorDeps) { deps.Graders.(*mockEvalGraderReader).results[0].Evidence = nil }},
		{name: "malformed source digest", mutate: func(deps *EvaluatorDeps) { deps.Graders.(*mockEvalGraderReader).results[0].Evidence[0].Sha256 = "bad" }},
		{name: "nil dependency", mutate: func(deps *EvaluatorDeps) { deps.Graders = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := fullDeps(t)
			if tt.mutate != nil {
				tt.mutate(&deps)
			}
			got, _, err := DefaultMethods(deps)["KSI-IAM-05"][1].evaluate(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultMethods_KSIHistoryFreshness(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvaluatorDeps)
		want   bool
	}{
		{name: "fresh KSI snapshot", want: true},
		{name: "stale KSI snapshot", mutate: func(deps *EvaluatorDeps) {
			deps.History.(*mockKSIHistoryReader).snapshots[0].EvaluatedAtMs = time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
		}},
		{name: "missing snapshots", mutate: func(deps *EvaluatorDeps) { deps.History = &mockKSIHistoryReader{} }},
		{name: "missing KSI result", mutate: func(deps *EvaluatorDeps) { deps.History.(*mockKSIHistoryReader).snapshots[0].Results = nil }},
		{name: "nil dependency", mutate: func(deps *EvaluatorDeps) { deps.History = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := fullDeps(t)
			if tt.mutate != nil {
				tt.mutate(&deps)
			}
			got, _, err := DefaultMethods(deps)["KSI-CMT-01"][1].evaluate(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
