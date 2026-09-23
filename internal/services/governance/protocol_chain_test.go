// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func protocolChainStages(txID, txHash string, rejected bool) []*operatorv1.DeterministicStageEvidence {
	l4ID := txID + ":L4"
	stages := []*operatorv1.DeterministicStageEvidence{
		{StageId: txID + ":L1", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, TransactionId: txID, TransactionHash: txHash, ParentStageId: l4ID},
		{StageId: l4ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, TransactionId: txID, TransactionHash: txHash},
	}
	if rejected {
		stages[0].Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
		stages[1].Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
	}
	for _, stage := range stages {
		stage.ActionType = "FILE_EDIT"
	}
	return stages
}

func fullProtocolChainStages(txID, txHash string) []*operatorv1.DeterministicStageEvidence {
	l4ID := txID + ":L4"
	l5ID := txID + ":L5"
	stages := []*operatorv1.DeterministicStageEvidence{
		{StageId: txID + ":L1", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, TransactionId: txID, TransactionHash: txHash, ParentStageId: l4ID},
		{StageId: txID + ":L2", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, TransactionId: txID, TransactionHash: txHash, ParentStageId: l4ID},
		{StageId: txID + ":L3", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED, TransactionId: txID, TransactionHash: txHash, ParentStageId: l4ID},
		{StageId: l4ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, TransactionId: txID, TransactionHash: txHash, ParentStageId: l5ID},
		{StageId: txID + ":PERSIST", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, TransactionId: txID, TransactionHash: txHash, ParentStageId: l5ID},
		{StageId: txID + ":COMMIT", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, TransactionId: txID, TransactionHash: txHash, ParentStageId: l5ID},
		{StageId: l5ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, TransactionId: txID, TransactionHash: txHash, StateRootBefore: "before", StateRootAfter: "after"},
	}
	for _, stage := range stages {
		stage.ActionType = "FILE_EDIT"
	}
	return stages
}

func TestValidateDeterministicProtocolChainAcceptsVerifiedAndRejectedReceipts(t *testing.T) {
	tests := []struct {
		name    string
		receipt *operatorv1.ActionReceipt
		stages  int
	}{
		{
			name: "verified",
			receipt: &operatorv1.ActionReceipt{
				TransactionId: "tx-1", TransactionHash: "hash-1", Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
				StateRootBefore: "before", StateRootAfter: "after", L2Status: operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
				L3Status: operatorv1.L3Status_L3_STATUS_NOT_REQUIRED, DeterministicStageEvidence: fullProtocolChainStages("tx-1", "hash-1"),
			},
			stages: 7,
		},
		{
			name: "rejected",
			receipt: &operatorv1.ActionReceipt{
				TransactionId: "tx-1", TransactionHash: "hash-1", Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
				L2Status: operatorv1.L2Status_L2_STATUS_NOT_REQUIRED, L3Status: operatorv1.L3Status_L3_STATUS_NOT_REQUIRED,
				DeterministicStageEvidence: protocolChainStages("tx-1", "hash-1", true),
			},
			stages: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, err := ValidateDeterministicProtocolChain(tt.receipt)
			require.NoError(t, err)
			assert.Len(t, chain.Stages, tt.stages)
			assert.NotEmpty(t, chain.ContentReference)
		})
	}
}

func TestValidateDeterministicProtocolChainRejectsMalformedEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*operatorv1.ActionReceipt)
	}{
		{name: "nil receipt", mutate: func(receipt *operatorv1.ActionReceipt) {}},
		{name: "duplicate stage id", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.DeterministicStageEvidence[1].StageId = receipt.DeterministicStageEvidence[0].StageId
		}},
		{name: "transaction mismatch", mutate: func(receipt *operatorv1.ActionReceipt) { receipt.DeterministicStageEvidence[0].TransactionId = "other" }},
		{name: "invalid order", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.DeterministicStageEvidence[0], receipt.DeterministicStageEvidence[1] = receipt.DeterministicStageEvidence[1], receipt.DeterministicStageEvidence[0]
		}},
		{name: "state root mismatch", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.DeterministicStageEvidence[6].StateRootAfter = "tampered"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receipt *operatorv1.ActionReceipt
			if tt.name != "nil receipt" {
				receipt = &operatorv1.ActionReceipt{
					TransactionId: "tx-1", TransactionHash: "hash-1", Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
					StateRootBefore: "before", StateRootAfter: "after", L2Status: operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
					L3Status: operatorv1.L3Status_L3_STATUS_NOT_REQUIRED, DeterministicStageEvidence: fullProtocolChainStages("tx-1", "hash-1"),
				}
			}
			tt.mutate(receipt)
			_, err := ValidateDeterministicProtocolChain(receipt)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestDeterministicStagesContentAddressIsStable(t *testing.T) {
	stages := fullProtocolChainStages("tx-1", "hash-1")
	first, err := DeterministicStagesContentAddress(stages)
	require.NoError(t, err)
	second, err := DeterministicStagesContentAddress(stages)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Regexp(t, `^deterministic-stages:sha256:[0-9a-f]{64}$`, first)
}

func TestValidateDeterministicProtocolChainRejectsInconsistentStageOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*operatorv1.ActionReceipt)
	}{
		{name: "missing required stage", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.DeterministicStageEvidence = receipt.DeterministicStageEvidence[:4]
		}},
		{name: "l1 not verified", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.DeterministicStageEvidence[0].Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
		}},
		{name: "l2 status mismatch", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.DeterministicStageEvidence[1].Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
		}},
		{name: "identity mismatch", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.DeterministicStageEvidence[0].InvestigationId = "first"
			receipt.DeterministicStageEvidence[1].InvestigationId = "other"
		}},
		{name: "rejected receipt status", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
		}},
		{name: "rejected extra stage", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.DeterministicStageEvidence = append(receipt.DeterministicStageEvidence, &operatorv1.DeterministicStageEvidence{StageId: "tx-1:L5", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, TransactionId: "tx-1", TransactionHash: "hash-1"})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receipt := &operatorv1.ActionReceipt{
				TransactionId: "tx-1", TransactionHash: "hash-1", Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
				StateRootBefore: "before", StateRootAfter: "after", L2Status: operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
				L3Status: operatorv1.L3Status_L3_STATUS_NOT_REQUIRED, DeterministicStageEvidence: fullProtocolChainStages("tx-1", "hash-1"),
			}
			tt.mutate(receipt)
			_, err := ValidateDeterministicProtocolChain(receipt)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}
