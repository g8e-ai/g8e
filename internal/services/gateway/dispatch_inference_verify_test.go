// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// --- helpers for verifyInferenceCompletion tests ---

// signTestReceipt signs an ActionReceipt's canonical form and attaches a
// signed ReceiptPersistenceAttestation, mirroring L5Actuator.signReceipt and
// signReceiptPersistenceAttestation.
func signTestReceipt(t *testing.T, receipt *operatorv1.ActionReceipt, signerPriv ed25519.PrivateKey, keyID string) {
	t.Helper()
	receipt.SignerKeyId = keyID
	canonical, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(signerPriv, canonical))

	attestation := &operatorv1.ReceiptPersistenceAttestation{
		TransactionId:          receipt.TransactionId,
		ReceiptSignatureDigest: governance.SignatureDigest([]string{receipt.Signature}),
		PersistedAtUnixMs:      time.Now().UnixMilli(),
		AuditRecordId:          receipt.TransactionId,
		SignerKeyId:            keyID,
	}
	attestationPayload, err := governance.CanonicalizeReceiptPersistenceAttestation(attestation)
	require.NoError(t, err)
	attestation.Signature = hex.EncodeToString(ed25519.Sign(signerPriv, attestationPayload))
	receipt.FinalPersistenceAttestation = attestation
}

// inferenceTestResult returns a complete InferenceResult with its canonical
// digest stamped, mirroring handleInferenceRequestSync.
func inferenceTestResult(t *testing.T) *operatorv1.InferenceResult {
	t.Helper()
	result := &operatorv1.InferenceResult{
		Parts:                 []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "generated output"}}},
		PromptTokens:          7,
		CompletionTokens:      11,
		TotalTokens:           18,
		UsageReported:         true,
		FinishReason:          "stop",
		Model:                 "gemma3:4b",
		RequestedModel:        "gemma3:4b",
		ProviderAttemptId:     "provider-attempt-1",
		NormalizedRequestHash: strings.Repeat("1", 64),
		OutputHash:            strings.Repeat("2", 64),
		RetryClassification:   models.ClassifyRetry(0),
		LoadState:             models.ClassifyLoadState(nil),
	}
	digest, err := models.ComputeInferenceResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = digest
	return result
}

// newInferenceDispatchService builds a DispatchService whose signer store is
// the given store; other dependencies are stubs that are not exercised by
// verifyInferenceCompletion.
func newInferenceDispatchService(t *testing.T, signerStore governance.SignerStore) *DispatchService {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	return NewDispatchService(
		logger,
		NewGatewayWebSocketHandler(logger),
		&stubStateRootProvider{root: "root-abc"},
		&stubOperatorSessionValidator{op: &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}},
		"doctrine",
		governance.NewL1Doctrine(),
		nil,
		signerStore,
	)
}

// --- verifyInferenceCompletion table-driven tests ---

func TestVerifyInferenceCompletion(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"

	const txID = "tx-inference-001"
	const txHash = "hash-inference-001"

	requestPayload, err := proto.Marshal(&operatorv1.InferenceRequested{
		Model:             "gemma3:4b",
		ProviderAttemptId: "provider-attempt-1",
	})
	require.NoError(t, err)
	cmdEnv := &commonv1.GovernanceEnvelope{Id: txID, TransactionHash: txHash, Payload: requestPayload}

	// resultEnvelopeFor marshals a completion into a result envelope payload.
	resultEnvelopeFor := func(t *testing.T, completion *operatorv1.InferenceCompletion) *commonv1.GovernanceEnvelope {
		t.Helper()
		payload, err := proto.Marshal(completion)
		require.NoError(t, err)
		return &commonv1.GovernanceEnvelope{Id: txID, Payload: payload}
	}

	// completedReceipt returns a signed COMPLETED receipt whose
	// ResultSummary binds the given result's canonical digest.
	completedReceipt := func(t *testing.T, result *operatorv1.InferenceResult) *operatorv1.ActionReceipt {
		t.Helper()
		digest, err := models.ComputeInferenceResultDigest(result)
		require.NoError(t, err)
		receipt := &operatorv1.ActionReceipt{
			TransactionId:    txID,
			TransactionHash:  txHash,
			Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:    digest,
			StateRootBefore:  "root-before",
			StateRootAfter:   "root-after",
			ExecutedAtUnixMs: 1700000000000,
		}
		signTestReceipt(t, receipt, priv, keyID)
		return receipt
	}

	t.Run("success returns verified receipt and result", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		result := inferenceTestResult(t)
		receipt := completedReceipt(t, result)
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt, Result: result})

		out, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.NoError(t, err)
		assert.Equal(t, txID, out.TransactionID)
		assert.Same(t, resultEnv, out.ResultEnvelope)
		require.NotNil(t, out.Receipt)
		assert.Equal(t, receipt.TransactionId, out.Receipt.TransactionId)
		assert.Equal(t, receipt.Signature, out.Receipt.Signature)
		assert.Equal(t, receipt.ResultSummary, out.Receipt.ResultSummary)
		require.NotNil(t, out.InferenceResult)
		assert.True(t, proto.Equal(result, out.InferenceResult))
		assert.Equal(t, result.ResultDigest, out.InferenceResult.ResultDigest)
	})

	t.Run("malformed payload fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		resultEnv := &commonv1.GovernanceEnvelope{Id: txID, Payload: []byte{0xff, 0xff, 0xff, 0xff}}

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceResultDecode)
	})

	t.Run("missing receipt fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Result: inferenceTestResult(t)})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceCompletionNoReceipt)
	})

	t.Run("receipt transaction identity mismatch fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		result := inferenceTestResult(t)
		receipt := completedReceipt(t, result)
		receipt.TransactionId = "tx-other"
		signTestReceipt(t, receipt, priv, keyID)
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt, Result: result})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceReceiptVerify)
	})

	t.Run("nil signer store fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, nil)
		result := inferenceTestResult(t)
		receipt := completedReceipt(t, result)
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt, Result: result})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceReceiptVerify)
	})

	t.Run("unknown signer key id fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		result := inferenceTestResult(t)
		receipt := completedReceipt(t, result)
		receipt.SignerKeyId = "unregistered-key"
		signTestReceipt(t, receipt, priv, "unregistered-key")
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt, Result: result})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceReceiptVerify)
	})

	t.Run("signer store error fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{err: constants.ErrTrustedSignerKeyNotFound})
		result := inferenceTestResult(t)
		receipt := completedReceipt(t, result)
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt, Result: result})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceReceiptVerify)
	})

	t.Run("receipt signature from wrong key fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		_, otherPriv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		result := inferenceTestResult(t)
		receipt := completedReceipt(t, result)
		signTestReceipt(t, receipt, otherPriv, keyID)
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt, Result: result})

		_, err = svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceReceiptVerify)
	})

	t.Run("missing persistence attestation fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		result := inferenceTestResult(t)
		receipt := completedReceipt(t, result)
		receipt.FinalPersistenceAttestation = nil
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt, Result: result})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceReceiptVerify)
	})

	t.Run("failed receipt terminates as typed failure", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		receipt := &operatorv1.ActionReceipt{
			TransactionId:    txID,
			TransactionHash:  txHash,
			Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			ResultSummary:    "backend unavailable",
			ExecutedAtUnixMs: 1700000000000,
		}
		signTestReceipt(t, receipt, priv, keyID)
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceReceiptFailed)
	})

	t.Run("completed receipt without result fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		receipt := &operatorv1.ActionReceipt{
			TransactionId:    txID,
			TransactionHash:  txHash,
			Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:    "digest",
			ExecutedAtUnixMs: 1700000000000,
		}
		signTestReceipt(t, receipt, priv, keyID)
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceCompletionNoResult)
	})

	t.Run("result part substitution after digest fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		result := inferenceTestResult(t)
		receipt := completedReceipt(t, result)
		// Substitute the text after the digest was computed: the recomputed
		// digest no longer matches the stamped digest or the receipt summary.
		result.Parts[0].Part = &operatorv1.InferenceResponsePart_Text{Text: "substituted output"}
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt, Result: result})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceResultDigestMismatch)
	})

	t.Run("receipt summary substitution fails closed", func(t *testing.T) {
		svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
		result := inferenceTestResult(t)
		receipt := completedReceipt(t, result)
		receipt.ResultSummary = "truncated text summary"
		signTestReceipt(t, receipt, priv, keyID)
		resultEnv := resultEnvelopeFor(t, &operatorv1.InferenceCompletion{Receipt: receipt, Result: result})

		_, err := svc.verifyInferenceCompletion(cmdEnv, resultEnv)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInferenceResultDigestMismatch)
	})
}

// TestVerifyInferenceCompletion_FailureCodeMapping proves that a signed
// FAILED receipt's typed failure_code maps back to the corresponding
// typed sentinel on the gateway side, so the controller can distinguish
// governance rejections, client faults, and provider failures without
// string-matching the receipt summary.
func TestVerifyInferenceCompletion_FailureCodeMapping(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "actuator-key-1"

	const txID = "tx-inference-001"
	const txHash = "hash-inference-001"
	cmdEnv := &commonv1.GovernanceEnvelope{Id: txID, TransactionHash: txHash}

	failedReceiptWithCode := func(t *testing.T, code operatorv1.ReceiptFailureCode) *operatorv1.ActionReceipt {
		t.Helper()
		receipt := &operatorv1.ActionReceipt{
			TransactionId:    txID,
			TransactionHash:  txHash,
			Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			ResultSummary:    "rejected upstream",
			ExecutedAtUnixMs: 1700000000000,
			FailureCode:      code,
		}
		signTestReceipt(t, receipt, priv, keyID)
		return receipt
	}

	tests := []struct {
		name string
		code operatorv1.ReceiptFailureCode
		want error
	}{
		{name: "governance rejected", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_GOVERNANCE_REJECTED, want: constants.ErrInferenceGovernanceRejected},
		{name: "model override denied", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_OVERRIDE_DENIED, want: constants.ErrInferenceModelOverrideDenied},
		{name: "role invalid", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_ROLE_INVALID, want: constants.ErrInferenceRoleInvalid},
		{name: "model ref invalid", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_REF_INVALID, want: constants.ErrInferenceModelRefInvalid},
		{name: "backend unavailable", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_BACKEND_UNAVAILABLE, want: constants.ErrInferenceBackendUnavailable},
		{name: "backend timeout", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_BACKEND_TIMEOUT, want: constants.ErrInferenceBackendTimeout},
		{name: "generate failed", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_GENERATE_FAILED, want: constants.ErrInferenceGenerateFailed},
		{name: "model not found", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_NOT_FOUND, want: constants.ErrInferenceModelNotFound},
		{name: "provider response invalid", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_PROVIDER_RESPONSE_INVALID, want: constants.ErrInferenceProviderResponseInvalid},
		{name: "generation options invalid", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_GENERATION_OPTIONS_INVALID, want: constants.ErrInferenceGenerationOptionsInvalid},
		{name: "capability unsupported", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_CAPABILITY_UNSUPPORTED, want: constants.ErrInferenceCapabilityUnsupported},
		{name: "provider attempt required", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_PROVIDER_ATTEMPT_REQUIRED, want: constants.ErrInferenceProviderAttemptRequired},
		{name: "identity mismatch", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_IDENTITY_MISMATCH, want: constants.ErrInferenceIdentityMismatch},
		{name: "model digest mismatch", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_DIGEST_MISMATCH, want: constants.ErrInferenceModelDigestMismatch},
		{name: "evidence hash invalid", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_EVIDENCE_HASH_INVALID, want: constants.ErrInferenceEvidenceHashInvalid},
		{name: "model registry invalid", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_REGISTRY_INVALID, want: constants.ErrInferenceModelRegistryInvalid},
		{name: "campaign binding invalid", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_CAMPAIGN_BINDING_INVALID, want: constants.ErrInferenceCampaignBindingInvalid},
		{name: "generic execution failure", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_EXECUTION_FAILED, want: constants.ErrInferenceReceiptFailed},
		{name: "unspecified code falls back to generic failure", code: operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_UNSPECIFIED, want: constants.ErrInferenceReceiptFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newInferenceDispatchService(t, &stubSignerStore{keys: map[string]ed25519.PublicKey{keyID: pub}})
			receipt := failedReceiptWithCode(t, tt.code)
			payload, err := proto.Marshal(&operatorv1.InferenceCompletion{Receipt: receipt})
			require.NoError(t, err)
			resultEnv := &commonv1.GovernanceEnvelope{Id: txID, Payload: payload}

			_, err = svc.verifyInferenceCompletion(cmdEnv, resultEnv)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want, "failure_code %v must map to the typed sentinel", tt.code)
		})
	}
}
