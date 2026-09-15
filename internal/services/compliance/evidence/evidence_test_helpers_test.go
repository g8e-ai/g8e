package evidence

import operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

func nodesByType(nodes []EvidenceNode, artifactType ArtifactType) []EvidenceNode {
	result := make([]EvidenceNode, 0)
	for _, node := range nodes {
		if node.ArtifactType == artifactType {
			result = append(result, node)
		}
	}
	return result
}

func newEvalVerifiedChainReceipt(signerKeyID string) *operatorv1.ActionReceipt {
	receipt := &operatorv1.ActionReceipt{TransactionId: "tx-1", TransactionHash: "tx-hash", Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, StateRootBefore: "root-before", StateRootAfter: "root-after", SignerKeyId: signerKeyID, ExecutedAtUnixMs: 1_700_000_001_000, L2Status: operatorv1.L2Status_L2_STATUS_REQUIRED_VALID, L3Status: operatorv1.L3Status_L3_STATUS_NOT_REQUIRED}
	l4ID := receipt.TransactionId + ":L4"
	l5ID := receipt.TransactionId + ":L5"
	receipt.DeterministicStageEvidence = []*operatorv1.DeterministicStageEvidence{
		{StageId: receipt.TransactionId + ":L1", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, ParentStageId: l4ID},
		{StageId: receipt.TransactionId + ":L2", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, ParentStageId: l4ID},
		{StageId: receipt.TransactionId + ":L3", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED, ParentStageId: l4ID},
		{StageId: l4ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, ParentStageId: l5ID},
		{StageId: receipt.TransactionId + ":PERSIST", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, ParentStageId: l5ID},
		{StageId: receipt.TransactionId + ":COMMIT", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, ParentStageId: l5ID},
		{StageId: l5ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, StateRootBefore: receipt.StateRootBefore, StateRootAfter: receipt.StateRootAfter},
	}
	for _, stage := range receipt.DeterministicStageEvidence {
		stage.TransactionId = receipt.TransactionId
		stage.TransactionHash = receipt.TransactionHash
		stage.ActionType = "GOVERNANCE_ACTION"
		stage.OperatorId = "operator-1"
		stage.OperatorSessionId = "session-1"
		stage.CaseId = "run-1"
		stage.InvestigationId = "scenario-1"
		stage.TaskId = "attempt-1"
	}
	return receipt
}
