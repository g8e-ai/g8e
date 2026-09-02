// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// protocolChainGraderIssue documents the Phase 2 gap this slice closes: the
// harness resolved canonical receipts and independently verified receipt and
// persistence signatures, but no Go-side deterministic-stage normalization or
// protocol-chain grader validated the stage chain extracted from the receipt.
// The Python eval ProtocolChainGrader existed, but the demo path had no
// equivalent, so demo compliance decisions could not trace to a verified
// protocol chain.
const protocolChainGraderIssue = "PHASE2: ISSUE: no Go-side deterministic-stage normalization or protocol-chain grader for demo receipt evidence"

// buildVerifiedChainStages constructs the full deterministic stage chain for a
// completed (allowed) L1+L2 transaction. The stages carry correct parent/child
// links, outcomes, and transaction bindings matching the Python eval
// ProtocolChainGrader's verified-chain expectations.
func buildVerifiedChainStages(txID, txHash, investigationID string) []*operatorv1.DeterministicStageEvidence {
	l4ID := txID + ":L4"
	l5ID := txID + ":L5"
	return []*operatorv1.DeterministicStageEvidence{
		{
			StageId: txID + ":L1", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
			Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
			TransactionId: txID, TransactionHash: txHash, InvestigationId: investigationID,
			ParentStageId: l4ID,
		},
		{
			StageId: txID + ":L2", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
			Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
			TransactionId: txID, TransactionHash: txHash, InvestigationId: investigationID,
			ParentStageId: l4ID,
		},
		{
			StageId: txID + ":L3", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY,
			Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
			TransactionId: txID, TransactionHash: txHash, InvestigationId: investigationID,
			ParentStageId: l4ID,
		},
		{
			StageId: l4ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
			Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
			TransactionId: txID, TransactionHash: txHash, InvestigationId: investigationID,
			ParentStageId: l5ID,
		},
		{
			StageId: txID + ":PERSIST", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
			Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
			TransactionId: txID, TransactionHash: txHash, InvestigationId: investigationID,
			ParentStageId: l5ID,
		},
		{
			StageId: txID + ":COMMIT", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
			Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
			TransactionId: txID, TransactionHash: txHash, InvestigationId: investigationID,
			ParentStageId: l5ID,
		},
		{
			StageId: l5ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
			Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
			TransactionId: txID, TransactionHash: txHash, InvestigationId: investigationID,
			StateRootBefore: "root-before", StateRootAfter: "root-after",
			ParentStageId: "",
		},
	}
}

// buildRejectedChainStages constructs the deterministic stage chain for a
// transaction rejected at L1: L1 failed, L4 failed, receipt status FAILED.
func buildRejectedChainStages(txID, txHash, investigationID string) []*operatorv1.DeterministicStageEvidence {
	l4ID := txID + ":L4"
	return []*operatorv1.DeterministicStageEvidence{
		{
			StageId: txID + ":L1", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
			Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED,
			TransactionId: txID, TransactionHash: txHash, InvestigationId: investigationID,
			ParentStageId: l4ID,
		},
		{
			StageId: l4ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
			Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED,
			TransactionId: txID, TransactionHash: txHash, InvestigationId: investigationID,
			ParentStageId: "",
		},
	}
}

func buildVerifiedChainReceipt() *operatorv1.ActionReceipt {
	return &operatorv1.ActionReceipt{
		TransactionId:   "transaction-1",
		TransactionHash: "transaction-hash-1",
		Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		StateRootBefore: "root-before",
		StateRootAfter:  "root-after",
		SignerKeyId:     "signer-1",
		Signature:       "receipt-signature-1",
		L2Status:        operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
		L3Status:        operatorv1.L3Status_L3_STATUS_NOT_REQUIRED,
		DeterministicStageEvidence: buildVerifiedChainStages(
			"transaction-1", "transaction-hash-1", "investigation-1",
		),
	}
}

func buildRejectedChainReceipt() *operatorv1.ActionReceipt {
	return &operatorv1.ActionReceipt{
		TransactionId:   "transaction-1",
		TransactionHash: "transaction-hash-1",
		Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
		SignerKeyId:     "signer-1",
		Signature:       "receipt-signature-1",
		L2Status:        operatorv1.L2Status_L2_STATUS_NOT_REQUIRED,
		L3Status:        operatorv1.L3Status_L3_STATUS_NOT_REQUIRED,
		DeterministicStageEvidence: buildRejectedChainStages(
			"transaction-1", "transaction-hash-1", "investigation-1",
		),
	}
}

// --- NormalizeDeterministicStages ---

// TestNormalizeDeterministicStages_ExtractsValidVerifiedChain proves that
// normalization extracts the deterministic stages from a completed receipt,
// validates transaction binding, unique IDs, and correct ordering, and returns
// the stages in canonical kind order. This test fails before
// NormalizeDeterministicStages exists.
func TestNormalizeDeterministicStages_ExtractsValidVerifiedChain(t *testing.T) {
	receipt := buildVerifiedChainReceipt()

	stages, err := NormalizeDeterministicStages(receipt)

	require.NoError(t, err)
	require.Len(t, stages, 7)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, stages[0].Kind)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2, stages[1].Kind)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY, stages[2].Kind)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, stages[3].Kind)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE, stages[4].Kind)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND, stages[5].Kind)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION, stages[6].Kind)
}

// TestNormalizeDeterministicStages_ExtractsValidRejectedChain proves that
// normalization accepts the short rejected chain (L1 failed, L4 failed) and
// returns the stages in canonical order.
func TestNormalizeDeterministicStages_ExtractsValidRejectedChain(t *testing.T) {
	receipt := buildRejectedChainReceipt()

	stages, err := NormalizeDeterministicStages(receipt)

	require.NoError(t, err)
	require.Len(t, stages, 2)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, stages[0].Kind)
	assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, stages[1].Kind)
}

// TestNormalizeDeterministicStages_FailsClosedOnEmptyStages proves that a
// receipt with no deterministic stage evidence is rejected.
func TestNormalizeDeterministicStages_FailsClosedOnEmptyStages(t *testing.T) {
	receipt := &operatorv1.ActionReceipt{TransactionId: "transaction-1"}

	_, err := NormalizeDeterministicStages(receipt)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

// TestNormalizeDeterministicStages_FailsClosedOnDuplicateStageIDs proves that
// two stages with the same stage_id are rejected.
func TestNormalizeDeterministicStages_FailsClosedOnDuplicateStageIDs(t *testing.T) {
	receipt := buildRejectedChainReceipt()
	receipt.DeterministicStageEvidence[0].StageId = receipt.DeterministicStageEvidence[1].StageId

	_, err := NormalizeDeterministicStages(receipt)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

// TestNormalizeDeterministicStages_FailsClosedOnTransactionMismatch proves
// that a stage whose transaction_id does not match the receipt is rejected.
func TestNormalizeDeterministicStages_FailsClosedOnTransactionMismatch(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	receipt.DeterministicStageEvidence[0].TransactionId = "wrong-transaction"

	_, err := NormalizeDeterministicStages(receipt)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

// TestNormalizeDeterministicStages_FailsClosedOnEmptyStageID proves that a
// stage with an empty stage_id is rejected.
func TestNormalizeDeterministicStages_FailsClosedOnEmptyStageID(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	receipt.DeterministicStageEvidence[0].StageId = ""

	_, err := NormalizeDeterministicStages(receipt)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

// TestNormalizeDeterministicStages_FailsClosedOnDuplicateKinds proves that
// two stages of the same kind are rejected.
func TestNormalizeDeterministicStages_FailsClosedOnDuplicateKinds(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	receipt.DeterministicStageEvidence[1].Kind = receipt.DeterministicStageEvidence[0].Kind

	_, err := NormalizeDeterministicStages(receipt)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

// TestNormalizeDeterministicStages_FailsClosedOnInvalidKindOrder proves that
// stages not in canonical kind order are rejected.
func TestNormalizeDeterministicStages_FailsClosedOnInvalidKindOrder(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	receipt.DeterministicStageEvidence[0], receipt.DeterministicStageEvidence[1] =
		receipt.DeterministicStageEvidence[1], receipt.DeterministicStageEvidence[0]

	_, err := NormalizeDeterministicStages(receipt)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

// --- GradeProtocolChain ---

// TestGradeProtocolChain_AcceptsVerifiedChain proves that the grader accepts a
// valid verified chain for a completed receipt with required stages L1, L2,
// L5. The grade is verified with value 1.0 and a content-addressed stage
// evidence reference. This test fails before GradeProtocolChain exists.
func TestGradeProtocolChain_AcceptsVerifiedChain(t *testing.T) {
	receipt := buildVerifiedChainReceipt()

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.NoError(t, err)
	assert.True(t, grade.Verified)
	assert.InDelta(t, 1.0, grade.Value, 0.001)
	assert.Empty(t, grade.Failure)
	assert.Regexp(t, `^deterministic-stages:sha256:[0-9a-f]{64}$`, grade.StageEvidenceRef)
}

// TestGradeProtocolChain_AcceptsRejectedChain proves that the grader accepts a
// valid rejected chain for a failed receipt blocked at L1 with required stage
// L1. The grade is verified because the rejection is correctly attributed.
func TestGradeProtocolChain_AcceptsRejectedChain(t *testing.T) {
	receipt := buildRejectedChainReceipt()

	grade, err := GradeProtocolChain(receipt, []string{"L1"})

	require.NoError(t, err)
	assert.True(t, grade.Verified)
	assert.InDelta(t, 1.0, grade.Value, 0.001)
	assert.Empty(t, grade.Failure)
	assert.Regexp(t, `^deterministic-stages:sha256:[0-9a-f]{64}$`, grade.StageEvidenceRef)
}

// TestGradeProtocolChain_FailsClosedOnMissingRequiredStage proves that a
// verified chain missing a required stage (L5 absent) fails closed.
func TestGradeProtocolChain_FailsClosedOnMissingRequiredStage(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	// Remove L5 and the stages that depend on it (persistence, commitment, L5).
	receipt.DeterministicStageEvidence = receipt.DeterministicStageEvidence[:4]

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnL5OutcomeMismatch proves that an L5
// stage outcome that does not match the receipt status is rejected.
func TestGradeProtocolChain_FailsClosedOnL5OutcomeMismatch(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	// Flip L5 outcome to FAILED while receipt status is COMPLETED.
	for _, s := range receipt.DeterministicStageEvidence {
		if s.Kind == operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION {
			s.Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
		}
	}

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnInvalidParentLinks proves that a
// verified chain with wrong parent_stage_id values is rejected.
func TestGradeProtocolChain_FailsClosedOnInvalidParentLinks(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	// Corrupt L1's parent to point at L5 instead of L4.
	receipt.DeterministicStageEvidence[0].ParentStageId = receipt.DeterministicStageEvidence[5].StageId

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnL1NotVerifiedInVerifiedChain proves
// that a verified chain where L1 is not VERIFIED is rejected.
func TestGradeProtocolChain_FailsClosedOnL1NotVerifiedInVerifiedChain(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	receipt.DeterministicStageEvidence[0].Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnL2OutcomeMismatch proves that an L2
// stage outcome inconsistent with the receipt's L2 status is rejected.
func TestGradeProtocolChain_FailsClosedOnL2OutcomeMismatch(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	// L2 status is REQUIRED_VALID but flip L2 stage outcome to FAILED.
	receipt.DeterministicStageEvidence[1].Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnL5StateRootMismatch proves that L5
// state roots not matching the receipt are rejected.
func TestGradeProtocolChain_FailsClosedOnL5StateRootMismatch(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	for _, s := range receipt.DeterministicStageEvidence {
		if s.Kind == operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION {
			s.StateRootAfter = "tampered-root"
		}
	}

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnRejectedChainWithWrongReceiptStatus
// proves that a rejected chain with a COMPLETED receipt status is rejected.
func TestGradeProtocolChain_FailsClosedOnRejectedChainWithWrongReceiptStatus(t *testing.T) {
	receipt := buildRejectedChainReceipt()
	receipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED

	grade, err := GradeProtocolChain(receipt, []string{"L1"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnRejectedChainWithExtraStages proves
// that a rejected chain carrying extra post-L4 stages is rejected.
func TestGradeProtocolChain_FailsClosedOnRejectedChainWithExtraStages(t *testing.T) {
	receipt := buildRejectedChainReceipt()
	// Add an L5 stage that should not be present in an L1-rejected chain.
	receipt.DeterministicStageEvidence = append(receipt.DeterministicStageEvidence, &operatorv1.DeterministicStageEvidence{
		StageId: "transaction-1:L5", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
		Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
		TransactionId: "transaction-1", TransactionHash: "transaction-hash-1", InvestigationId: "investigation-1",
	})

	grade, err := GradeProtocolChain(receipt, []string{"L1"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnNilReceipt proves that a nil receipt is
// rejected rather than panicking.
func TestGradeProtocolChain_FailsClosedOnNilReceipt(t *testing.T) {
	grade, err := GradeProtocolChain(nil, []string{"L1"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

// TestGradeProtocolChain_FailsClosedOnEmptyRequiredStages proves that an
// empty required-stages list is rejected — every scenario must declare at
// least L1.
func TestGradeProtocolChain_FailsClosedOnEmptyRequiredStages(t *testing.T) {
	receipt := buildVerifiedChainReceipt()

	grade, err := GradeProtocolChain(receipt, nil)

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

// TestGradeProtocolChain_FailsClosedOnUnknownRequiredStageLabel proves that
// an unrecognized stage label in the required list is rejected.
func TestGradeProtocolChain_FailsClosedOnUnknownRequiredStageLabel(t *testing.T) {
	receipt := buildVerifiedChainReceipt()

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L9"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

// TestGradeProtocolChain_FailsClosedOnL4InvalidOutcome proves that an L4
// stage with an outcome other than VERIFIED or FAILED is rejected.
func TestGradeProtocolChain_FailsClosedOnL4InvalidOutcome(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	for _, s := range receipt.DeterministicStageEvidence {
		if s.Kind == operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION {
			s.Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED
		}
	}

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnPersistenceNotCompleted proves that a
// receipt-persistence stage with an outcome other than COMPLETED is rejected
// in a verified chain.
func TestGradeProtocolChain_FailsClosedOnPersistenceNotCompleted(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	for _, s := range receipt.DeterministicStageEvidence {
		if s.Kind == operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE {
			s.Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
		}
	}

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnCommitmentNotCompleted proves that a
// commitment-append stage with an outcome other than COMPLETED is rejected in
// a verified chain.
func TestGradeProtocolChain_FailsClosedOnCommitmentNotCompleted(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	for _, s := range receipt.DeterministicStageEvidence {
		if s.Kind == operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND {
			s.Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
		}
	}

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}

// TestGradeProtocolChain_FailsClosedOnInconsistentIdentityFields proves that
// stages with inconsistent investigation_id values are rejected.
func TestGradeProtocolChain_FailsClosedOnInconsistentIdentityFields(t *testing.T) {
	receipt := buildVerifiedChainReceipt()
	receipt.DeterministicStageEvidence[0].InvestigationId = "different-investigation"

	grade, err := GradeProtocolChain(receipt, []string{"L1", "L2", "L5"})

	require.Error(t, err)
	require.NotNil(t, grade)
	assert.False(t, grade.Verified)
	assert.NotEmpty(t, grade.Failure)
}
