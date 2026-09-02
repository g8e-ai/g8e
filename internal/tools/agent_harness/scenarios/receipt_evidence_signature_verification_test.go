// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// buildSignedReceiptEvidence constructs a ReceiptEvidence whose receipt and
// persistence attestation carry real Ed25519 signatures from the provided
// signing key. The receipt carries a complete verified deterministic stage
// chain so ResolveReceiptEvidence's protocol-chain validation passes. This is
// the canonical fixture for signature-verification tests.
func buildSignedReceiptEvidence(t *testing.T, result *Result, projection clientpkg.Receipt, signerPriv ed25519.PrivateKey, signerKeyID string) *ReceiptEvidence {
	t.Helper()

	receipt := &operatorv1.ActionReceipt{
		TransactionId:    projection.TransactionID,
		TransactionHash:  projection.TransactionHash,
		SignerKeyId:      signerKeyID,
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		StateRootBefore:  "root-before",
		StateRootAfter:   "root-after",
		ResultSummary:    "completed",
		ExecutedAtUnixMs: time.Now().UnixMilli(),
		L2Status:         operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
		L3Status:         operatorv1.L3Status_L3_STATUS_NOT_REQUIRED,
		DeterministicStageEvidence: buildVerifiedChainStages(
			projection.TransactionID, projection.TransactionHash, projection.InvestigationID,
		),
	}

	payload, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(signerPriv, payload))
	projection.Signature = receipt.Signature

	attestation := &operatorv1.ReceiptPersistenceAttestation{
		TransactionId:          receipt.TransactionId,
		ReceiptSignatureDigest: governance.SignatureDigest([]string{receipt.Signature}),
		PersistedAtUnixMs:      time.Now().UnixMilli(),
		AuditRecordId:          receipt.TransactionId,
		SignerKeyId:            signerKeyID,
	}
	attestationPayload, err := governance.CanonicalizeReceiptPersistenceAttestation(attestation)
	require.NoError(t, err)
	attestation.Signature = hex.EncodeToString(ed25519.Sign(signerPriv, attestationPayload))
	receipt.FinalPersistenceAttestation = attestation

	evidence, err := buildReceiptEvidence(result, projection, receipt)
	require.NoError(t, err)
	return evidence
}

func baseSignedEvidenceResult() *Result {
	return &Result{
		RunID:            "run-1",
		ScenarioID:       "fedramp-deny",
		AttemptIDs:       []string{"attempt-1"},
		ExecutionIDs:     []string{"execution-1"},
		TransactionIDs:   []string{"transaction-1"},
		InvestigationIDs: []string{"investigation-1"},
	}
}

func baseSignedEvidenceProjection() clientpkg.Receipt {
	return clientpkg.Receipt{
		ExecutionID:     "execution-1",
		TransactionID:   "transaction-1",
		TransactionHash: "transaction-hash-1",
		InvestigationID: "investigation-1",
		SignerKeyID:     "signer-1",
		Signature:       "", // set by buildSignedReceiptEvidence after signing
	}
}

// TestVerifyReceiptEvidenceSignatures_AcceptsValidSignatures proves that
// independently verifying the receipt and persistence attestation signatures
// against the assessed signer's public key succeeds when both signatures are
// valid. This test fails before VerifyReceiptEvidenceSignatures exists.
func TestVerifyReceiptEvidenceSignatures_AcceptsValidSignatures(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	projection := baseSignedEvidenceProjection()
	result := baseSignedEvidenceResult()
	evidence := buildSignedReceiptEvidence(t, result, projection, privKey, "signer-1")

	err = VerifyReceiptEvidenceSignatures(evidence, pubKey)

	require.NoError(t, err)
}

// TestVerifyReceiptEvidenceSignatures_RejectsTamperedReceiptSignature proves
// that mutating the receipt signature after signing causes independent
// verification to fail closed. This prevents a tampered receipt body from
// being persisted as evidence.
func TestVerifyReceiptEvidenceSignatures_RejectsTamperedReceiptSignature(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	projection := baseSignedEvidenceProjection()
	result := baseSignedEvidenceResult()
	evidence := buildSignedReceiptEvidence(t, result, projection, privKey, "signer-1")

	// Tamper: flip one hex character in the receipt signature.
	sigBytes, decErr := hex.DecodeString(evidence.Receipt.Signature)
	require.NoError(t, decErr)
	sigBytes[0] ^= 0x01
	evidence.Receipt.Signature = hex.EncodeToString(sigBytes)

	err = VerifyReceiptEvidenceSignatures(evidence, pubKey)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrActionReceiptSignatureInvalid)
}

// TestVerifyReceiptEvidenceSignatures_RejectsTamperedPersistenceSignature
// proves that mutating the persistence attestation signature after signing
// causes independent verification to fail closed.
func TestVerifyReceiptEvidenceSignatures_RejectsTamperedPersistenceSignature(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	projection := baseSignedEvidenceProjection()
	result := baseSignedEvidenceResult()
	evidence := buildSignedReceiptEvidence(t, result, projection, privKey, "signer-1")

	// Tamper: flip one byte in the persistence attestation signature.
	sigBytes, decErr := hex.DecodeString(evidence.Receipt.FinalPersistenceAttestation.Signature)
	require.NoError(t, decErr)
	sigBytes[0] ^= 0x01
	evidence.Receipt.FinalPersistenceAttestation.Signature = hex.EncodeToString(sigBytes)

	err = VerifyReceiptEvidenceSignatures(evidence, pubKey)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReceiptPersistenceAttestationInvalid)
}

// TestVerifyReceiptEvidenceSignatures_RejectsTamperedReceiptBody proves that
// mutating a receipt field after signing causes verification to fail closed,
// preventing a tampered body from passing with the original signature.
func TestVerifyReceiptEvidenceSignatures_RejectsTamperedReceiptBody(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	projection := baseSignedEvidenceProjection()
	result := baseSignedEvidenceResult()
	evidence := buildSignedReceiptEvidence(t, result, projection, privKey, "signer-1")

	// Tamper: change the result summary after signing.
	evidence.Receipt.ResultSummary = "tampered"

	err = VerifyReceiptEvidenceSignatures(evidence, pubKey)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrActionReceiptSignatureInvalid)
}

// TestVerifyReceiptEvidenceSignatures_FailsClosedOnNilPublicKey proves that
// a missing signer public key is rejected rather than skipping verification.
func TestVerifyReceiptEvidenceSignatures_FailsClosedOnNilPublicKey(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	projection := baseSignedEvidenceProjection()
	result := baseSignedEvidenceResult()
	evidence := buildSignedReceiptEvidence(t, result, projection, privKey, "signer-1")

	err = VerifyReceiptEvidenceSignatures(evidence, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrActionReceiptSignatureInvalid)
}

// TestVerifyReceiptEvidenceSignatures_FailsClosedOnNilEvidence proves that a
// nil evidence record is rejected rather than panicking.
func TestVerifyReceiptEvidenceSignatures_FailsClosedOnNilEvidence(t *testing.T) {
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	err = VerifyReceiptEvidenceSignatures(nil, pubKey)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrActionReceiptMissing)
}

// TestVerifyReceiptEvidenceSignatures_RejectsWrongSignerKey proves that a
// signature verified against the wrong public key fails closed, preventing
// cross-key substitution.
func TestVerifyReceiptEvidenceSignatures_RejectsWrongSignerKey(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	wrongPubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	projection := baseSignedEvidenceProjection()
	result := baseSignedEvidenceResult()
	evidence := buildSignedReceiptEvidence(t, result, projection, privKey, "signer-1")

	err = VerifyReceiptEvidenceSignatures(evidence, wrongPubKey)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrActionReceiptSignatureInvalid)
}

// --- ResolveReceiptEvidence integration with signature verification ---

// stubReceiptEvidenceResolver is a Tier 1 stub implementing
// ReceiptEvidenceResolver for testing ResolveReceiptEvidence without a real
// gateway. It returns pre-built receipts and signer keys.
type stubReceiptEvidenceResolver struct {
	receipts   map[string]*operatorv1.ActionReceipt
	signerKeys map[string]ed25519.PublicKey
}

func (s *stubReceiptEvidenceResolver) GetActionReceipt(_ context.Context, transactionID string, _ ...clientpkg.Persona) (*operatorv1.ActionReceipt, []byte, error) {
	r, ok := s.receipts[transactionID]
	if !ok {
		return nil, nil, nil
	}
	return r, nil, nil
}

func (s *stubReceiptEvidenceResolver) GetTrustedSignerPublicKey(_ context.Context, keyID string) (ed25519.PublicKey, error) {
	k, ok := s.signerKeys[keyID]
	if !ok {
		return nil, constants.ErrTrustedSignerKeyNotFound
	}
	return k, nil
}

// TestResolveReceiptEvidence_VerifiesSignaturesAgainstAssessedSignerKey proves
// that ResolveReceiptEvidence independently verifies the receipt and
// persistence signatures against the signer's trusted public key fetched from
// the gateway. A valid signature passes; a tampered signature fails closed.
func TestResolveReceiptEvidence_VerifiesSignaturesAgainstAssessedSignerKey(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	projection := baseSignedEvidenceProjection()
	result := baseSignedEvidenceResult()
	evidence := buildSignedReceiptEvidence(t, result, projection, privKey, "signer-1")
	// Sync projection signature to the signed value so the result's receipt
	// projection matches the canonical receipt.
	projection.Signature = evidence.Receipt.Signature
	result.Receipts = []clientpkg.Receipt{projection}

	stub := &stubReceiptEvidenceResolver{
		receipts:   map[string]*operatorv1.ActionReceipt{projection.TransactionID: evidence.Receipt},
		signerKeys: map[string]ed25519.PublicKey{"signer-1": pubKey},
	}

	err = ResolveReceiptEvidence(context.Background(), stub, result)

	require.NoError(t, err)
	require.Len(t, result.ReceiptEvidence, 1)
}

// TestResolveReceiptEvidence_FailsClosedOnTamperedReceiptSignature proves
// that a tampered receipt signature causes ResolveReceiptEvidence to reject
// the evidence rather than persisting unverified identity.
func TestResolveReceiptEvidence_FailsClosedOnTamperedReceiptSignature(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	projection := baseSignedEvidenceProjection()
	result := baseSignedEvidenceResult()
	evidence := buildSignedReceiptEvidence(t, result, projection, privKey, "signer-1")
	projection.Signature = evidence.Receipt.Signature
	result.Receipts = []clientpkg.Receipt{projection}

	// Tamper the receipt signature after signing.
	sigBytes, decErr := hex.DecodeString(evidence.Receipt.Signature)
	require.NoError(t, decErr)
	sigBytes[0] ^= 0x01
	evidence.Receipt.Signature = hex.EncodeToString(sigBytes)

	stub := &stubReceiptEvidenceResolver{
		receipts:   map[string]*operatorv1.ActionReceipt{projection.TransactionID: evidence.Receipt},
		signerKeys: map[string]ed25519.PublicKey{"signer-1": pubKey},
	}

	err = ResolveReceiptEvidence(context.Background(), stub, result)

	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrActionReceiptSignatureInvalid) || errors.Is(err, constants.ErrInvalidEvidenceGraph),
		"expected signature invalid or evidence graph error, got: %v", err)
	assert.Empty(t, result.ReceiptEvidence, "tampered evidence must not be appended to the result")
}

// TestResolveReceiptEvidence_FailsClosedOnMissingSignerKey proves that when
// the gateway cannot resolve the signer's public key, ResolveReceiptEvidence
// fails closed rather than skipping signature verification.
func TestResolveReceiptEvidence_FailsClosedOnMissingSignerKey(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	projection := baseSignedEvidenceProjection()
	result := baseSignedEvidenceResult()
	evidence := buildSignedReceiptEvidence(t, result, projection, privKey, "signer-1")
	projection.Signature = evidence.Receipt.Signature
	result.Receipts = []clientpkg.Receipt{projection}

	stub := &stubReceiptEvidenceResolver{
		receipts:   map[string]*operatorv1.ActionReceipt{projection.TransactionID: evidence.Receipt},
		signerKeys: map[string]ed25519.PublicKey{}, // no signer key registered
	}

	err = ResolveReceiptEvidence(context.Background(), stub, result)

	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrTrustedSignerKeyNotFound) || errors.Is(err, constants.ErrInvalidEvidenceGraph),
		"expected signer key not found or evidence graph error, got: %v", err)
	assert.Empty(t, result.ReceiptEvidence, "unverifiable evidence must not be appended to the result")
}
