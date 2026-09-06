// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MethodClassification is the Phase 0 classification of every KSI automated
// method registered in DefaultMethods. The plan requires each method to be
// classified as existence, structural, cryptographic, state-observation,
// historical, customer-attestation, or unsupported evidence.
type MethodClassification string

const (
	MethodClassExistence           MethodClassification = "existence"
	MethodClassStructural          MethodClassification = "structural"
	MethodClassCryptographic       MethodClassification = "cryptographic"
	MethodClassStateObservation    MethodClassification = "state_observation"
	MethodClassHistorical          MethodClassification = "historical"
	MethodClassCustomerAttestation MethodClassification = "customer_attestation"
	MethodClassUnsupported         MethodClassification = "unsupported"
)

// MethodInventoryEntry classifies one KSI method or one unregistered KSI.
type MethodInventoryEntry struct {
	KSIID          string
	MethodName     string // empty for unregistered KSIs
	Classification MethodClassification
	Reason         string
}

// phase0MethodInventory classifies every method registered in DefaultMethods
// and every KSI in the catalog that has no registered method.
//
// Key finding: receipt and final-persistence verification is cryptographic.
// The remaining registered methods are existence or structural checks and do
// not perform independent state observation, historical freshness analysis,
// or customer-attestation import.
var phase0MethodInventory = []MethodInventoryEntry{
	// KSI-CMT-01: auditEventsExist + ledgerCommitsExist
	{KSIID: "KSI-CMT-01", MethodName: "auditEventsExist", Classification: MethodClassExistence, Reason: "checks ListEvents returns a non-empty slice"},
	{KSIID: "KSI-CMT-01", MethodName: "ledgerCommitsExist", Classification: MethodClassExistence, Reason: "checks ListCommits returns a non-empty slice"},

	// KSI-CMT-03: ledgerCommitsExist + receiptsExist
	{KSIID: "KSI-CMT-03", MethodName: "ledgerCommitsExist", Classification: MethodClassExistence, Reason: "checks ListCommits returns a non-empty slice"},
	{KSIID: "KSI-CMT-03", MethodName: "receiptsExist", Classification: MethodClassExistence, Reason: "checks ListActionReceipts returns a non-empty slice"},

	// KSI-CNA-01: receiptsExist + auditEventsExist
	{KSIID: "KSI-CNA-01", MethodName: "receiptsExist", Classification: MethodClassExistence, Reason: "checks ListActionReceipts returns a non-empty slice"},
	{KSIID: "KSI-CNA-01", MethodName: "auditEventsExist", Classification: MethodClassExistence, Reason: "checks ListEvents returns a non-empty slice"},

	// KSI-IAM-05: receiptsExist + auditEventsExist
	{KSIID: "KSI-IAM-05", MethodName: "receiptsExist", Classification: MethodClassExistence, Reason: "checks ListActionReceipts returns a non-empty slice"},
	{KSIID: "KSI-IAM-05", MethodName: "auditEventsExist", Classification: MethodClassExistence, Reason: "checks ListEvents returns a non-empty slice"},

	// KSI-IAM-07: fileMutationsTracked + receiptsExist
	{KSIID: "KSI-IAM-07", MethodName: "fileMutationsTracked", Classification: MethodClassExistence, Reason: "checks ListFileMutations returns a non-empty slice"},
	{KSIID: "KSI-IAM-07", MethodName: "receiptsExist", Classification: MethodClassExistence, Reason: "checks ListActionReceipts returns a non-empty slice"},

	// KSI-MLA-03: auditEventsExist + receiptsExist
	{KSIID: "KSI-MLA-03", MethodName: "auditEventsExist", Classification: MethodClassExistence, Reason: "checks ListEvents returns a non-empty slice"},
	{KSIID: "KSI-MLA-03", MethodName: "receiptsExist", Classification: MethodClassExistence, Reason: "checks ListActionReceipts returns a non-empty slice"},

	// KSI-MLA-07: commitmentChainExists + merkleRootExists
	{KSIID: "KSI-MLA-07", MethodName: "commitmentChainExists", Classification: MethodClassExistence, Reason: "checks ListCommitments returns a non-empty slice"},
	{KSIID: "KSI-MLA-07", MethodName: "merkleRootExists", Classification: MethodClassExistence, Reason: "checks GetStateMerkleRoot returns a non-empty string"},

	// KSI-MLA-08: receiptsCryptographicallyVerified + commitmentChainExists
	{KSIID: "KSI-MLA-08", MethodName: "receiptsCryptographicallyVerified", Classification: MethodClassCryptographic, Reason: "verifies canonical receipt and final-persistence Ed25519 signatures against the signer public key"},
	{KSIID: "KSI-MLA-08", MethodName: "commitmentChainExists", Classification: MethodClassExistence, Reason: "checks ListCommitments returns a non-empty slice"},

	// KSI-SVC-04: fileMutationsTracked + ledgerCommitsExist
	{KSIID: "KSI-SVC-04", MethodName: "fileMutationsTracked", Classification: MethodClassExistence, Reason: "checks ListFileMutations returns a non-empty slice"},
	{KSIID: "KSI-SVC-04", MethodName: "ledgerCommitsExist", Classification: MethodClassExistence, Reason: "checks ListCommits returns a non-empty slice"},

	// KSI-SVC-05: merkleRootExists + commitmentChainIntact
	{KSIID: "KSI-SVC-05", MethodName: "merkleRootExists", Classification: MethodClassExistence, Reason: "checks GetStateMerkleRoot returns a non-empty string"},
	{KSIID: "KSI-SVC-05", MethodName: "commitmentChainIntact", Classification: MethodClassStructural, Reason: "checks prior-hash linking between consecutive commitments; does not verify commitment signatures or cross-links to receipts"},
}

// TestPhase0MethodInventory_EveryRegisteredMethodClassified validates that
// every method bound in DefaultMethods has a corresponding inventory entry
// with a valid classification.
func TestPhase0MethodInventory_EveryRegisteredMethodClassified(t *testing.T) {
	deps := EvaluatorDeps{
		Audit:       &mockAuditReader{},
		Ledger:      &mockLedgerReader{},
		Commitments: &mockCommitmentReader{},
	}
	registered := DefaultMethods(deps)

	valid := map[MethodClassification]bool{
		MethodClassExistence:           true,
		MethodClassStructural:          true,
		MethodClassCryptographic:       true,
		MethodClassStateObservation:    true,
		MethodClassHistorical:          true,
		MethodClassCustomerAttestation: true,
		MethodClassUnsupported:         true,
	}

	// Build a lookup of (KSIID, MethodName) -> entry from the inventory.
	lookup := make(map[string]MethodInventoryEntry)
	for _, e := range phase0MethodInventory {
		lookup[e.KSIID+"|"+e.MethodName] = e
		assert.True(t, valid[e.Classification],
			"inventory entry %s/%s has invalid classification %q",
			e.KSIID, e.MethodName, e.Classification)
	}

	// Every registered method must appear in the inventory. We cannot match
	// closures by name, so we verify by count: the inventory must have exactly
	// as many method entries as DefaultMethods registers.
	inventoryMethodCount := 0
	for _, e := range phase0MethodInventory {
		if e.MethodName != "" {
			inventoryMethodCount++
		}
	}

	totalRegistered := 0
	for _, methods := range registered {
		totalRegistered += len(methods)
	}

	assert.Equal(t, totalRegistered, inventoryMethodCount,
		phase0RegressionBeforeFix+
			": inventory method count must match DefaultMethods registered count; "+
			"inventory=%d registered=%d", inventoryMethodCount, totalRegistered)

	// Verify the lookup keys are unique (no duplicate KSI|method pairs).
	assert.Equal(t, len(lookup), inventoryMethodCount,
		"inventory contains duplicate KSI|method entries")
}

// TestMethodInventory_CryptographicReceiptVerificationIsClassified verifies
// that cryptographic receipt verification is present while later method
// classes remain outstanding.
func TestMethodInventory_CryptographicReceiptVerificationIsClassified(t *testing.T) {
	counts := make(map[MethodClassification]int)
	for _, e := range phase0MethodInventory {
		if e.MethodName != "" {
			counts[e.Classification]++
		}
	}

	assert.Equal(t, 1, counts[MethodClassCryptographic],
		phase0RegressionAfterFix+": receipt and final-persistence verification is cryptographic")
	assert.Equal(t, 0, counts[MethodClassStateObservation],
		phase0RegressionBeforeFix+
			": no registered KSI method performs independent state observation; "+
			"Phase 3 adds boundary-specific state collectors")
	assert.Equal(t, 0, counts[MethodClassHistorical],
		phase0RegressionBeforeFix+
			": no registered KSI method performs historical freshness analysis; "+
			"Phase 6 adds evidence-window completeness and missingness")
	assert.Equal(t, 0, counts[MethodClassCustomerAttestation],
		phase0RegressionBeforeFix+
			": no registered KSI method imports customer attestations; "+
			"Phase 7 adds signed scoped attestation import")

	assert.Greater(t, counts[MethodClassExistence], 0,
		"inventory must contain existence-check methods (the current baseline)")
	assert.Greater(t, counts[MethodClassStructural], 0,
		"inventory must contain structural-check methods (the current baseline)")
}

// TestPhase0MethodInventory_UnregisteredKSIsAreUnsupported documents that the
// 21 KSIs in the catalog without registered methods are classified as
// unsupported. These KSIs fail-closed during Class C evaluation because the
// method count is below the minimum. Phase 3 and Phase 7 add methods for the
// automatable subset.
func TestPhase0MethodInventory_UnregisteredKSIsAreUnsupported(t *testing.T) {
	catalog := testCatalog()
	deps := EvaluatorDeps{
		Audit:       &mockAuditReader{},
		Ledger:      &mockLedgerReader{},
		Commitments: &mockCommitmentReader{},
	}
	registered := DefaultMethods(deps)

	// The test catalog has 4 KSIs; the full catalog has 31. Count unregistered
	// KSIs in the test catalog and verify they have no methods.
	unregistered := 0
	for _, ksi := range catalog.KSIs {
		if len(registered[ksi.ID]) == 0 {
			unregistered++
		}
	}

	assert.Greater(t, unregistered, 0,
		phase0RegressionBeforeFix+
			": the catalog must contain KSIs with no registered methods (unsupported); "+
			"these fail-closed during Class C evaluation")
}

// TestPhase0MethodInventory_ClassCIndependenceIsNotEnforced documents that the
// current evaluator does not verify that Class C's two required methods are
// independent. Two methods that inspect the same unchecked field (e.g. two
// existence checks against the same store) count as two methods even though
// they derive the same fact from the same artifact. Phase 3 adds independence
// validation that rejects methods restating the same unchecked fact.
func TestPhase0MethodInventory_ClassCIndependenceIsNotEnforced(t *testing.T) {
	catalog := testCatalog()
	eval := NewKSIEvaluator(catalog)
	eval.RegisterDefaultMethods(EvaluatorDeps{
		Audit:       &mockAuditReader{},
		Ledger:      &mockLedgerReader{},
		Commitments: &mockCommitmentReader{},
	})

	// KSI-CMT-01 has two existence methods (auditEventsExist + ledgerCommitsExist).
	// The evaluator reports MethodCount=2, satisfying the Class C minimum, but
	// neither method independence nor method classification is checked.
	assert.Equal(t, 2, eval.MethodCount("KSI-CMT-01"),
		phase0RegressionBeforeFix+
			": Class C minimum is 2 methods; KSI-CMT-01 has exactly 2 existence checks "+
			"with no independence verification")
}
