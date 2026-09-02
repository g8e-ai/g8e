// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/scenarios"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestApplyHarnessAuthoritativeIdentityRetainsContentAddressedReceiptAndPersistenceEvidence(t *testing.T) {
	projection := clientpkg.Receipt{
		ExecutionID: "execution-1", TransactionID: "transaction-1", TransactionHash: "transaction-hash-1",
		InvestigationID: "investigation-1", SignerKeyID: "signer-1", Signature: "receipt-signature-1",
	}
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "transaction-1", TransactionHash: "transaction-hash-1", SignerKeyId: "signer-1", Signature: "receipt-signature-1",
		DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{{TransactionId: "transaction-1", TransactionHash: "transaction-hash-1", InvestigationId: "investigation-1"}},
		FinalPersistenceAttestation: &operatorv1.ReceiptPersistenceAttestation{
			TransactionId: "transaction-1", ReceiptSignatureDigest: "receipt-signature-digest-1", PersistedAtUnixMs: 1,
			AuditRecordId: "transaction-1", SignerKeyId: "signer-1", Signature: "persistence-signature-1",
		},
	}
	harnessScenarioResult := &scenarios.Result{
		RunID: "run-1", ScenarioID: "fedramp-deny", AttemptIDs: []string{"attempt-1"}, ExecutionIDs: []string{"execution-1"},
		TransactionIDs: []string{"transaction-1"}, InvestigationIDs: []string{"investigation-1"},
	}
	evidence, err := scenarios.BuildReceiptEvidence(harnessScenarioResult, projection, receipt)
	require.NoError(t, err)
	harness := &harnessResult{
		RunID: "run-1", ScenarioID: "fedramp-deny", AttemptIDs: []string{"attempt-1"}, ExecutionIDs: []string{"execution-1"},
		TransactionIDs: []string{"transaction-1"}, InvestigationIDs: []string{"investigation-1"},
		Receipts: []harnessReceipt{{
			ExecutionID: projection.ExecutionID, TransactionID: projection.TransactionID, TransactionHash: projection.TransactionHash,
			InvestigationID: projection.InvestigationID, SignerKeyID: projection.SignerKeyID, Signature: projection.Signature,
		}},
		ReceiptEvidence: []scenarios.ReceiptEvidence{*evidence},
	}
	result := &compliancev1.DemoScenarioResult{}

	applied := applyHarnessAuthoritativeIdentity(result, harness)

	assert.True(t, applied)
	assert.Equal(t, []string{"attempt-1"}, result.GetAttemptIds())
	assert.Equal(t, []string{"execution-1"}, result.GetExecutionIds())
	assert.Equal(t, []string{"transaction-1"}, result.GetTransactionIds())
	assert.Equal(t, []string{"investigation-1"}, result.GetInvestigationIds())
	assert.Equal(t, []string{evidence.ReceiptRef, evidence.PersistenceRef}, result.GetReceiptRefs())
}

func TestApplyHarnessAuthoritativeIdentityRejectsTamperedReceiptEvidenceWithoutPartialMutation(t *testing.T) {
	projection := clientpkg.Receipt{
		ExecutionID: "execution-1", TransactionID: "transaction-1", TransactionHash: "transaction-hash-1",
		InvestigationID: "investigation-1", SignerKeyID: "signer-1", Signature: "receipt-signature-1",
	}
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "transaction-1", TransactionHash: "transaction-hash-1", SignerKeyId: "signer-1", Signature: "receipt-signature-1",
		DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{{TransactionId: "transaction-1", TransactionHash: "transaction-hash-1", InvestigationId: "investigation-1"}},
		FinalPersistenceAttestation: &operatorv1.ReceiptPersistenceAttestation{
			TransactionId: "transaction-1", ReceiptSignatureDigest: "receipt-signature-digest-1", PersistedAtUnixMs: 1,
			AuditRecordId: "transaction-1", SignerKeyId: "signer-1", Signature: "persistence-signature-1",
		},
	}
	identity := &scenarios.Result{
		RunID: "run-1", ScenarioID: "fedramp-deny", AttemptIDs: []string{"attempt-1"}, ExecutionIDs: []string{"execution-1"},
		TransactionIDs: []string{"transaction-1"}, InvestigationIDs: []string{"investigation-1"},
	}
	evidence, err := scenarios.BuildReceiptEvidence(identity, projection, receipt)
	require.NoError(t, err)
	evidence.ReceiptRef = "action-receipt:sha256:tampered"
	harness := &harnessResult{
		RunID: "run-1", ScenarioID: "fedramp-deny", AttemptIDs: identity.AttemptIDs, ExecutionIDs: identity.ExecutionIDs,
		TransactionIDs: identity.TransactionIDs, InvestigationIDs: identity.InvestigationIDs,
		Receipts: []harnessReceipt{{
			ExecutionID: projection.ExecutionID, TransactionID: projection.TransactionID, TransactionHash: projection.TransactionHash,
			InvestigationID: projection.InvestigationID, SignerKeyID: projection.SignerKeyID, Signature: projection.Signature,
		}},
		ReceiptEvidence: []scenarios.ReceiptEvidence{*evidence},
	}
	result := &compliancev1.DemoScenarioResult{}

	applied := applyHarnessAuthoritativeIdentity(result, harness)

	assert.False(t, applied)
	assert.Empty(t, result.GetAttemptIds())
	assert.Empty(t, result.GetExecutionIds())
	assert.Empty(t, result.GetTransactionIds())
	assert.Empty(t, result.GetInvestigationIds())
	assert.Empty(t, result.GetReceiptRefs())
}
