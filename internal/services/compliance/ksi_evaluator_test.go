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
	"encoding/hex"
	"errors"
	"fmt"
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
	receipt := &operatorv1.ActionReceipt{TransactionId: "tx-1", TransactionHash: "hash-1", ExecutedAtUnixMs: 1_700_000_001_000, SignerKeyId: signerKeyID}
	payload, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	attestation := &operatorv1.ReceiptPersistenceAttestation{TransactionId: receipt.TransactionId, ReceiptSignatureDigest: governance.SignatureDigest([]string{receipt.Signature}), PersistedAtUnixMs: 1_700_000_002_000, AuditRecordId: receipt.TransactionId, SignerKeyId: signerKeyID}
	payload, err = governance.CanonicalizeReceiptPersistenceAttestation(attestation)
	require.NoError(t, err)
	attestation.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	receipt.FinalPersistenceAttestation = attestation
	return EvaluatorDeps{
		Audit: &mockAuditReader{
			receipts:  []*models.ActionReceiptRecord{{TransactionID: receipt.TransactionId, TransactionHash: receipt.TransactionHash, Signature: receipt.Signature, SignerKeyID: receipt.SignerKeyId, ActionReceipt: receipt}},
			events:    []*storage.Event{{ID: 1, OperatorSessionID: "sess-1"}},
			mutations: []*storage.FileMutationLog{{ID: 1, Filepath: "/etc/config"}},
		},
		Ledger: &mockLedgerReader{
			merkleRoot: "abc123def456",
			commits:    []storage.LedgerCommit{{CommitHash: "commit-1"}},
		},
		Commitments: &mockCommitmentReader{
			commitments: []*storage.CommitmentRow{
				{Seq: 1, TransactionID: "tx-1", Hash: "hash-1", PriorCommitmentHash: ""},
				{Seq: 2, TransactionID: "tx-2", Hash: "hash-2", PriorCommitmentHash: "hash-1"},
			},
		},
	}
}

// TestKSIEvaluator_RegisterMethods binds methods to a KSI and verifies MethodCount.
func TestKSIEvaluator_RegisterMethods(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	assert.Equal(t, 0, eval.MethodCount("KSI-CMT-01"))

	method := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, nil, nil
	}
	require.NoError(t, eval.RegisterMethods("KSI-CMT-01", method))
	assert.Equal(t, 1, eval.MethodCount("KSI-CMT-01"))

	require.NoError(t, eval.RegisterMethods("KSI-CMT-01", method))
	assert.Equal(t, 2, eval.MethodCount("KSI-CMT-01"))
}

// TestKSIEvaluator_RegisterMethods_UnknownKSI_ReturnsError verifies that registering
// methods for an unknown KSI ID returns an error.
func TestKSIEvaluator_RegisterMethods_UnknownKSI_ReturnsError(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	err := eval.RegisterMethods("KSI-FAKE-99", func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, nil, nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown KSI ID")
}

// TestKSIEvaluator_RegisterDefaultMethods registers default methods and verifies
// that automatable KSIs get >=2 methods while non-automatable KSIs get 0.
func TestKSIEvaluator_RegisterDefaultMethods(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)
	eval.RegisterDefaultMethods(fullDeps(t))

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
	eval.RegisterDefaultMethods(fullDeps(t))

	result, err := eval.Evaluate(context.Background(), ClassC)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, ClassC, result.Class)
	assert.NotEmpty(t, result.Results)

	for _, res := range result.Results {
		if res.MethodCount >= 2 {
			assert.Equal(t, KSIStatusSatisfied, res.Status,
				"KSI %s should be satisfied with %d methods", res.ID, res.MethodCount)
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
	eval.RegisterDefaultMethods(fullDeps(t))

	result, err := eval.Evaluate(context.Background(), ClassC)
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
	eval.RegisterDefaultMethods(fullDeps(t))

	result, err := eval.Evaluate(context.Background(), ClassC)
	require.NoError(t, err)

	for _, res := range result.Results {
		if res.ID == "KSI-CMT-01" {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"Stale KSI-CMT-01 should be not_satisfied despite having methods")
		}
	}
}

// TestKSIEvaluator_Evaluate_MethodError_FailClosed verifies that a method
// returning an error causes the KSI to be marked not_satisfied.
func TestKSIEvaluator_Evaluate_MethodError_FailClosed(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)

	errMethod := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return false, nil, errors.New("simulated storage failure")
	}
	okMethod := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeReceiptID, "tx-1")}, nil
	}

	require.NoError(t, eval.RegisterMethods("KSI-CMT-01", errMethod, okMethod))

	result, err := eval.Evaluate(context.Background(), ClassC)
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

	falseMethod := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return false, nil, nil
	}
	trueMethod := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeMerkleRoot, "root")}, nil
	}

	require.NoError(t, eval.RegisterMethods("KSI-CMT-01", falseMethod, trueMethod))

	result, err := eval.Evaluate(context.Background(), ClassC)
	require.NoError(t, err)

	for _, res := range result.Results {
		if res.ID == "KSI-CMT-01" {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"KSI with a false method result should be not_satisfied")
		}
	}
}

// TestKSIEvaluator_Evaluate_NilDeps_FailClosed verifies that nil dependency
// fields cause methods to return false (fail-closed).
func TestKSIEvaluator_Evaluate_NilDeps_FailClosed(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)
	eval.RegisterDefaultMethods(EvaluatorDeps{})

	result, err := eval.Evaluate(context.Background(), ClassC)
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
	eval.RegisterDefaultMethods(deps)

	result, err := eval.Evaluate(context.Background(), ClassC)
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
	okMethod := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeReceiptID, "tx-1")}, nil
	}
	require.NoError(t, eval.RegisterMethods("KSI-CMT-01", okMethod))

	result, err := eval.Evaluate(context.Background(), ClassB)
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

	result, err := eval.Evaluate(context.Background(), ClassA)
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
	eval.RegisterDefaultMethods(fullDeps(t))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := eval.Evaluate(ctx, ClassC)
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

	t.Run("valid result set", func(t *testing.T) {
		rs := &KSIResultSet{
			Results: []KSIResult{
				{ID: "KSI-CMT-01", Status: KSIStatusSatisfied},
				{ID: "KSI-MLA-07", Status: KSIStatusSatisfied},
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

	t.Run("unknown KSI ID", func(t *testing.T) {
		rs := &KSIResultSet{
			Results: []KSIResult{
				{ID: "KSI-FAKE-99", Status: KSIStatusSatisfied},
			},
		}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrValidationFailed)
		assert.Contains(t, err.Error(), "unknown KSI")
	})

	t.Run("empty KSI ID in result", func(t *testing.T) {
		rs := &KSIResultSet{
			Results: []KSIResult{
				{ID: "", Status: KSIStatusSatisfied},
			},
		}
		err := rs.Validate(catalog)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrValidationFailed)
		assert.Contains(t, err.Error(), "empty ID")
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
		ok, _, err := m(context.Background())
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
		ok, _, err := m(context.Background())
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
			satisfied, evidence, err := methods[0](context.Background())
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
	eval.RegisterDefaultMethods(fullDeps(t))

	result, err := eval.Evaluate(context.Background(), ClassC)
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

	result, err := eval.Evaluate(context.Background(), ClassC)
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
	eval.RegisterDefaultMethods(deps)

	result, err := eval.Evaluate(context.Background(), ClassC)
	require.NoError(t, err)

	// KSIs that depend on audit store should be not_satisfied due to error
	for _, res := range result.Results {
		if res.ID == "KSI-CMT-01" || res.ID == "KSI-MLA-03" || res.ID == "KSI-IAM-05" {
			assert.Equal(t, KSIStatusNotSatisfied, res.Status,
				"KSI %s should be not_satisfied due to audit store error", res.ID)
		}
	}
}
