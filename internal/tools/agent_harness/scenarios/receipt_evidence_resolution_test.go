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
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestBuildReceiptEvidenceBindsCanonicalReceiptToDemoIdentity(t *testing.T) {
	result := &Result{
		RunID:            "run-1",
		ScenarioID:       "fedramp-deny",
		AttemptIDs:       []string{"attempt-1"},
		ExecutionIDs:     []string{"execution-1"},
		TransactionIDs:   []string{"transaction-1"},
		InvestigationIDs: []string{"investigation-1"},
	}
	projection := clientpkg.Receipt{
		ExecutionID:     "execution-1",
		TransactionID:   "transaction-1",
		TransactionHash: "transaction-hash-1",
		InvestigationID: "investigation-1",
		SignerKeyID:     "signer-1",
		Signature:       "receipt-signature-1",
	}
	receipt := &operatorv1.ActionReceipt{
		TransactionId:   "transaction-1",
		TransactionHash: "transaction-hash-1",
		SignerKeyId:     "signer-1",
		Signature:       "receipt-signature-1",
		DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{{
			TransactionId:   "transaction-1",
			TransactionHash: "transaction-hash-1",
			InvestigationId: "investigation-1",
		}},
		FinalPersistenceAttestation: &operatorv1.ReceiptPersistenceAttestation{
			TransactionId:          "transaction-1",
			ReceiptSignatureDigest: "receipt-signature-digest-1",
			PersistedAtUnixMs:      1,
			AuditRecordId:          "transaction-1",
			SignerKeyId:            "signer-1",
			Signature:              "persistence-signature-1",
		},
	}

	evidence, err := buildReceiptEvidence(result, projection, receipt)

	require.NoError(t, err)
	assert.Equal(t, "run-1", evidence.RunID)
	assert.Equal(t, "fedramp-deny", evidence.ScenarioID)
	assert.Equal(t, "attempt-1", evidence.AttemptID)
	assert.Equal(t, "execution-1", evidence.ExecutionID)
	assert.Equal(t, "investigation-1", evidence.InvestigationID)
	assert.Equal(t, "transaction-1", evidence.TransactionID)
	assert.Regexp(t, `^action-receipt:sha256:[0-9a-f]{64}$`, evidence.ReceiptRef)
	assert.Regexp(t, `^receipt-persistence:sha256:[0-9a-f]{64}$`, evidence.PersistenceRef)
	assert.Same(t, receipt, evidence.Receipt)
}

func TestBuildReceiptEvidenceFailsClosedOnIdentityAndPersistenceMismatch(t *testing.T) {
	baseResult := func() *Result {
		return &Result{
			RunID:            "run-1",
			ScenarioID:       "fedramp-deny",
			AttemptIDs:       []string{"attempt-1"},
			ExecutionIDs:     []string{"execution-1"},
			TransactionIDs:   []string{"transaction-1"},
			InvestigationIDs: []string{"investigation-1"},
		}
	}
	baseProjection := func() clientpkg.Receipt {
		return clientpkg.Receipt{
			ExecutionID: "execution-1", TransactionID: "transaction-1", TransactionHash: "transaction-hash-1",
			InvestigationID: "investigation-1", SignerKeyID: "signer-1", Signature: "receipt-signature-1",
		}
	}
	baseReceipt := func() *operatorv1.ActionReceipt {
		return &operatorv1.ActionReceipt{
			TransactionId: "transaction-1", TransactionHash: "transaction-hash-1", SignerKeyId: "signer-1", Signature: "receipt-signature-1",
			DeterministicStageEvidence:  []*operatorv1.DeterministicStageEvidence{{TransactionId: "transaction-1", TransactionHash: "transaction-hash-1", InvestigationId: "investigation-1"}},
			FinalPersistenceAttestation: &operatorv1.ReceiptPersistenceAttestation{TransactionId: "transaction-1", ReceiptSignatureDigest: "receipt-signature-digest-1", PersistedAtUnixMs: 1, AuditRecordId: "transaction-1", SignerKeyId: "signer-1", Signature: "persistence-signature-1"},
		}
	}

	tests := []struct {
		name   string
		mutate func(*Result, *clientpkg.Receipt, *operatorv1.ActionReceipt)
	}{
		{name: "missing run identity", mutate: func(r *Result, _ *clientpkg.Receipt, _ *operatorv1.ActionReceipt) { r.RunID = "" }},
		{name: "multiple attempts", mutate: func(r *Result, _ *clientpkg.Receipt, _ *operatorv1.ActionReceipt) {
			r.AttemptIDs = append(r.AttemptIDs, "attempt-2")
		}},
		{name: "execution outside result", mutate: func(_ *Result, p *clientpkg.Receipt, _ *operatorv1.ActionReceipt) { p.ExecutionID = "execution-2" }},
		{name: "transaction outside result", mutate: func(_ *Result, p *clientpkg.Receipt, _ *operatorv1.ActionReceipt) { p.TransactionID = "transaction-2" }},
		{name: "investigation outside result", mutate: func(_ *Result, p *clientpkg.Receipt, _ *operatorv1.ActionReceipt) {
			p.InvestigationID = "investigation-2"
		}},
		{name: "receipt transaction mismatch", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.TransactionId = "transaction-2"
		}},
		{name: "receipt hash mismatch", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.TransactionHash = "transaction-hash-2"
		}},
		{name: "receipt signer mismatch", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.SignerKeyId = "signer-2"
		}},
		{name: "receipt signature mismatch", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.Signature = "receipt-signature-2"
		}},
		{name: "missing investigation stage", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.DeterministicStageEvidence = nil
		}},
		{name: "missing persistence attestation", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.FinalPersistenceAttestation = nil
		}},
		{name: "persistence transaction mismatch", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.FinalPersistenceAttestation.TransactionId = "transaction-2"
		}},
		{name: "persistence audit record mismatch", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.FinalPersistenceAttestation.AuditRecordId = "transaction-2"
		}},
		{name: "persistence signer mismatch", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.FinalPersistenceAttestation.SignerKeyId = "signer-2"
		}},
		{name: "missing persistence signature", mutate: func(_ *Result, _ *clientpkg.Receipt, receipt *operatorv1.ActionReceipt) {
			receipt.FinalPersistenceAttestation.Signature = ""
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, projection, receipt := baseResult(), baseProjection(), baseReceipt()
			tt.mutate(result, &projection, receipt)

			_, err := buildReceiptEvidence(result, projection, receipt)

			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}
