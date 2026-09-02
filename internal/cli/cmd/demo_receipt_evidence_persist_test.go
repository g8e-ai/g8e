// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/scenarios"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const receiptEvidencePersistIssue = "PHASE2: ISSUE: canonical receipt and persistence bodies not persisted as resolvable runtime evidence artifacts"

func buildTestReceiptEvidence(t *testing.T) scenarios.ReceiptEvidence {
	t.Helper()
	result := &scenarios.Result{
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
	evidence, err := scenarios.BuildReceiptEvidence(result, projection, receipt)
	require.NoError(t, err)
	return *evidence
}

// receiptDigestHex extracts the hex digest from a content address of the form
// "prefix:sha256:<hex>".
func receiptDigestHex(contentAddress string) string {
	parts := strings.SplitN(contentAddress, ":", 3)
	if len(parts) != 3 {
		return ""
	}
	return parts[2]
}

// TestPersistReceiptEvidenceBodies_WritesCanonicalReceiptAndPersistenceArtifacts
// verifies that the canonical ActionReceipt body and its final-persistence
// attestation body are persisted as resolvable runtime evidence artifacts under
// the per-run demo evidence tree, named by their SHA-256 content-address digests.
func TestPersistReceiptEvidenceBodies_WritesCanonicalReceiptAndPersistenceArtifacts(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()
	evidence := buildTestReceiptEvidence(t)

	require.NoError(t, persistReceiptEvidenceBodies(ctx, fileSvc, "run-1", []scenarios.ReceiptEvidence{evidence}))

	receiptHex := receiptDigestHex(evidence.ReceiptRef)
	persistenceHex := receiptDigestHex(evidence.PersistenceRef)
	require.NotEmpty(t, receiptHex)
	require.NotEmpty(t, persistenceHex)

	receiptPath := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, "run-1", constants.DemoRunReceiptsDirname, receiptHex+".json")
	persistencePath := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, "run-1", constants.DemoRunPersistenceDirname, persistenceHex+".json")

	receiptExists, err := fileSvc.FileExists(ctx, receiptPath)
	require.NoError(t, err)
	assert.True(t, receiptExists, receiptEvidencePersistIssue)

	persistenceExists, err := fileSvc.FileExists(ctx, persistencePath)
	require.NoError(t, err)
	assert.True(t, persistenceExists, receiptEvidencePersistIssue)

	receiptBytes, err := fileSvc.ReadFile(ctx, receiptPath)
	require.NoError(t, err)
	expectedReceipt, err := compliancev1.MarshalCanonical(evidence.Receipt)
	require.NoError(t, err)
	assert.Equal(t, string(expectedReceipt), string(receiptBytes), "persisted receipt body must match canonical protojson")

	persistenceBytes, err := fileSvc.ReadFile(ctx, persistencePath)
	require.NoError(t, err)
	expectedPersistence, err := compliancev1.MarshalCanonical(evidence.Receipt.GetFinalPersistenceAttestation())
	require.NoError(t, err)
	assert.Equal(t, string(expectedPersistence), string(persistenceBytes), "persisted persistence attestation body must match canonical protojson")
}

// TestPersistReceiptEvidenceBodies_PersistedBodiesResolveFromContentAddresses
// verifies that the persisted receipt and persistence bodies can be resolved
// from the content addresses carried on the DemoScenarioResult, proving the
// artifacts are resolvable rather than dangling references.
func TestPersistReceiptEvidenceBodies_PersistedBodiesResolveFromContentAddresses(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()
	evidence := buildTestReceiptEvidence(t)

	require.NoError(t, persistReceiptEvidenceBodies(ctx, fileSvc, "run-1", []scenarios.ReceiptEvidence{evidence}))

	receiptHex := receiptDigestHex(evidence.ReceiptRef)
	persistenceHex := receiptDigestHex(evidence.PersistenceRef)

	receiptPath := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, "run-1", constants.DemoRunReceiptsDirname, receiptHex+".json")
	persistencePath := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, "run-1", constants.DemoRunPersistenceDirname, persistenceHex+".json")

	receiptBytes, err := fileSvc.ReadFile(ctx, receiptPath)
	require.NoError(t, err)
	persistenceBytes, err := fileSvc.ReadFile(ctx, persistencePath)
	require.NoError(t, err)

	// The persisted receipt body must deserialize back to a proto-equal ActionReceipt.
	var decoded operatorv1.ActionReceipt
	require.NoError(t, protojson.Unmarshal(receiptBytes, &decoded))
	assert.True(t, proto.Equal(evidence.Receipt, &decoded), "persisted receipt body must round-trip to the canonical receipt")

	// The persisted persistence body must deserialize back to a proto-equal attestation.
	var decodedPersistence operatorv1.ReceiptPersistenceAttestation
	require.NoError(t, protojson.Unmarshal(persistenceBytes, &decodedPersistence))
	assert.True(t, proto.Equal(evidence.Receipt.GetFinalPersistenceAttestation(), &decodedPersistence), "persisted persistence body must round-trip to the canonical attestation")
}

// TestPersistReceiptEvidenceBodies_SkipsNilAndEmptyRunID verifies the
// persistence function is a no-op for nil evidence or an empty run ID.
func TestPersistReceiptEvidenceBodies_SkipsNilAndEmptyRunID(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()

	assert.NoError(t, persistReceiptEvidenceBodies(ctx, fileSvc, "", []scenarios.ReceiptEvidence{buildTestReceiptEvidence(t)}))
	assert.NoError(t, persistReceiptEvidenceBodies(ctx, fileSvc, "run-1", nil))
}

// TestPersistReceiptEvidenceBodies_FailsClosedOnNilReceiptBody verifies that
// a ReceiptEvidence carrying a nil receipt body is rejected rather than
// persisting an empty or incomplete artifact.
func TestPersistReceiptEvidenceBodies_FailsClosedOnNilReceiptBody(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()
	evidence := buildTestReceiptEvidence(t)
	evidence.Receipt = nil

	err := persistReceiptEvidenceBodies(ctx, fileSvc, "run-1", []scenarios.ReceiptEvidence{evidence})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDemoEvidencePersistFailed)
}

// TestPersistReceiptEvidenceBodies_FailsClosedOnMissingPersistenceAttestation
// verifies that a receipt without a final-persistence attestation is rejected.
func TestPersistReceiptEvidenceBodies_FailsClosedOnMissingPersistenceAttestation(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()
	evidence := buildTestReceiptEvidence(t)
	evidence.Receipt.FinalPersistenceAttestation = nil

	err := persistReceiptEvidenceBodies(ctx, fileSvc, "run-1", []scenarios.ReceiptEvidence{evidence})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDemoEvidencePersistFailed)
}
