// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package pubsub

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	govpkg "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// stubInferenceBackend is a test-only inference.Backend that returns a
// canned GenerateResponse without contacting any real Ollama daemon. It
// records the last request so the test can assert role/model routing.
type stubInferenceBackend struct {
	resp    *models.GenerateResponse
	err     error
	lastReq models.GenerateRequest
	called  bool
}

func (s *stubInferenceBackend) Generate(_ context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
	s.called = true
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	response := s.resp
	if response == nil {
		response = &models.GenerateResponse{
			Parts: []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "stub inference response"}}},
			Model: req.Model, FinishReason: "stop",
		}
	}
	response.NormalizedRequestHash = models.SHA256Hex([]byte("integration request"))
	outputHash, err := models.ComputeInferenceOutputHash(response.Parts, response.FinishReason)
	if err != nil {
		return nil, err
	}
	response.OutputHash = outputHash
	return response, nil
}

func (s *stubInferenceBackend) Status(_ context.Context) (*models.BackendStatus, error) {
	return &models.BackendStatus{Available: true, Models: []string{"test-model"}}, nil
}

// newInferenceTestAuditStore builds a real SQLAuditStore (real SQLite, real
// vault, foreign_keys enforced) rooted in a fresh isolated runtime tree. The
// vault lives inside the managed .g8e tree via fileSvc.Resolve; the audit DB
// uses the canonical DbFilename under the data directory.
func newInferenceTestAuditStore(t *testing.T) *storage.SQLAuditStore {
	t.Helper()
	logger := testutil.NewTestLogger()

	fileSvc := storagetest.NewTestFileSvc(t, testutil.TempDir(t))

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := storagetest.CreateTestVault(t, fileSvc.Resolve(constants.VaultDirname), privKey)

	auditStore, err := storage.NewSQLAuditStore(&storage.AuditStoreConfig{
		DBPath:          constants.DbFilename,
		MaxDBSizeMB:     100,
		RetentionDays:   1,
		EncryptionVault: testVault,
	}, logger, fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auditStore.Close()) })

	return auditStore
}

// newInferenceIntegrationFixture constructs an OperatorPubSubService in
// outbound mode with a real SQLAuditStore (real SQLite, real vault,
// foreign_keys enforced) and a stub inference backend wired through the
// governed execution handler. Returns the service, the stub backend, the
// audit store, and the capturing results publisher so the test can assert
// on persisted receipts and published InferenceCompletion envelopes.
func newInferenceIntegrationFixture(t *testing.T) (*OperatorPubSubService, *stubInferenceBackend, *storage.SQLAuditStore, *mockResultsPublisher) {
	t.Helper()

	cfg := testutil.NewTestConfig(t)
	cfg.Inference = config.InferenceConfig{
		Enabled:        true,
		Backend:        "ollama",
		PrimaryModel:   "gemma3:4b",
		AssistantModel: "llama3.2:3b",
		LiteModel:      "qwen3:1.5b",
		KeepAlive:      "-1",
	}
	logger := testutil.NewTestLogger()

	auditStore := newInferenceTestAuditStore(t)

	backend := &stubInferenceBackend{
		resp: &models.GenerateResponse{
			Parts:            []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "governed inference output"}}},
			PromptTokens:     12,
			CompletionTokens: 8,
			TotalTokens:      20,
			UsageReported:    true,
			FinishReason:     "stop",
			Model:            "gemma3:4b",
		},
	}
	scrubbingSvc := mustNewScrubbingSvc(t, logger)
	inferenceHandler := inference.NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signerStore := &governance.FailClosedSignerStore{
		Signers: map[string]ed25519.PublicKey{"test-key": pubKey},
	}

	results := &mockResultsPublisher{}

	svc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:             cfg,
		Logger:             logger,
		PubSubClient:       NewInProcessPubSubClient(nil),
		ResultsService:     results,
		ActuatorSigningKey: privKey,
		ActuatorKeyID:      "test-key",
		AuditorSigningKey:  privKey,
		AuditorKeyID:       hex.EncodeToString(pubKey),
		Scrubbing:          scrubbingSvc,
		Inference:          inferenceHandler,
		AuditStore:         auditStore,
	}, OutboundModeDeps{
		GovernanceCoreDeps: GovernanceCoreDeps{
			ReplayStore:       testutil.NewStatefulMockReplayStore(),
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.ConfigurableMockL3Notary{ShouldPass: true},
			SignerStore:       signerStore,
			Doctrine:          governance.NewL1Doctrine(),
		},
	})
	require.NoError(t, err)

	return svc, backend, auditStore, results
}

// buildInferenceEnvelope constructs a governed GovernanceEnvelope carrying
// an InferenceRequested payload under the doctrine posture and returns the
// protojson-marshaled wire bytes ready for ProcessEnvelope. Inference is a
// mutation, so it cannot traverse postures that require L3 human proofs:
// the gateway dispatch path cannot mint L3 proofs and rejects such envelopes
// at construction. Doctrine is the release posture for governed inference.
var inferenceEnvelopeNonce atomic.Int64

func buildInferenceEnvelope(t *testing.T, role operatorv1.ModelRole, model, prompt string) []byte {
	t.Helper()

	infReq := &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 role,
		Model:                model,
		ProviderAttemptId:    fmt.Sprintf("provider-attempt-%d", inferenceEnvelopeNonce.Load()+1),
		Messages: []*operatorv1.InferenceMessage{{
			Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
			Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: prompt}}},
		}},
	}
	payloadBytes, err := proto.Marshal(infReq)
	require.NoError(t, err)

	envelope := &govpkg.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "operator-inference-int",
		OperatorSessionId: "session-inference-int",
		ActionType:        string(constants.ActionTypeInference),
		TargetResource:    "ollama",
		Payload:           payloadBytes,
		StateMerkleRoot:   "test-state-root",
		Nonce:             fmt.Sprintf("nonce-inference-int-%d", inferenceEnvelopeNonce.Add(1)),
		Posture:           constants.PostureDoctrine,
	}

	txHash, err := govpkg.GenerateMessageID(envelope)
	require.NoError(t, err)
	envelope.Id = txHash
	envelope.TransactionHash = txHash

	envelope.Governance = &commonv1.GovernanceMetadata{
		L1: &commonv1.L1Metadata{Validated: true},
	}

	wire, err := (protojson.MarshalOptions{}).Marshal(envelope)
	require.NoError(t, err)
	return wire
}

// TestInferenceDispatch_ProcessEnvelope_PrimaryRole_PersistsReceiptAndAudit
// verifies the full governed inference path: a GovernanceEnvelope carrying an
// InferenceRequested payload traverses L1–L5 verification, the
// InferenceExecutionHandler calls the stub backend, the L5 actuator stamps a
// signed ActionReceipt whose result_summary is the canonical result digest,
// the receipt is persisted in the real SQLAuditStore with the INFERENCE
// action type, and an InferenceCompletion carrying the final receipt plus
// the complete digest-bound result is published to the results channel.
// This exercises real SQLite, real vault, foreign_keys enforcement, and the
// real L4 warden — no mocks for governance internals.
func TestInferenceDispatch_ProcessEnvelope_PrimaryRole_PersistsReceiptAndAudit(t *testing.T) {
	svc, backend, auditStore, results := newInferenceIntegrationFixture(t)

	wire := buildInferenceEnvelope(t, operatorv1.ModelRole_MODEL_ROLE_PRIMARY, "gemma3:4b", "What is 2+2?")

	receipt, err := svc.ProcessEnvelope(context.Background(), wire)

	require.NoError(t, err, "governed inference envelope must execute successfully")
	require.NotNil(t, receipt, "actuator must return a signed receipt")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status, "receipt status must be COMPLETED")
	assert.NotEmpty(t, receipt.Signature, "receipt must carry a non-empty actuator signature")

	require.True(t, backend.called, "stub backend must be invoked by the inference handler")
	assert.Equal(t, "gemma3:4b", backend.lastReq.Model, "backend must receive the primary model name")
	require.Len(t, backend.lastReq.Messages, 1)
	assert.Equal(t, "What is 2+2?", backend.lastReq.Messages[0].GetParts()[0].GetText(), "backend must receive the user message text")

	// The signed receipt's result_summary is the canonical digest of the
	// complete InferenceResult — not the (possibly truncated) text.
	require.Len(t, results.inferenceCompletions, 1, "exactly one inference completion must be published")
	completion := results.inferenceCompletions[0]
	require.NotNil(t, completion.Receipt, "completion must carry the final signed receipt")
	require.NotNil(t, completion.Result, "completed receipt must carry the full result")
	require.Len(t, completion.Result.GetParts(), 1)
	assert.Equal(t, "governed inference output", completion.Result.GetParts()[0].GetText())
	assert.Equal(t, receipt.TransactionId, completion.Receipt.TransactionId, "completion receipt must correlate to the executed transaction")
	assert.Equal(t, receipt.Signature, completion.Receipt.Signature, "completion receipt must be the signed final receipt")

	wantDigest, err := models.ComputeInferenceResultDigest(completion.Result)
	require.NoError(t, err)
	assert.Equal(t, wantDigest, completion.Result.ResultDigest, "result must carry its canonical digest")
	assert.Equal(t, wantDigest, receipt.ResultSummary, "receipt result_summary must be the result digest")
	assert.Equal(t, wantDigest, completion.Receipt.ResultSummary, "completion receipt must bind the result digest")

	receipts, err := auditStore.ListActionReceipts("", 10, 0)
	require.NoError(t, err)
	require.Len(t, receipts, 1, "exactly one inference receipt must be persisted in the audit store")
	persisted := receipts[0]
	assert.Equal(t, constants.ActionTypeInference, persisted.ActionType, "persisted receipt action type must be INFERENCE")
	assert.Equal(t, wantDigest, persisted.ResultSummary, "persisted receipt summary must be the result digest")
}

// TestInferenceDispatch_ProcessEnvelope_AllThreeRoles_RoutesByConfigDefault
// verifies that the inference handler resolves the correct Ollama model name
// for each chat-tier role (Primary, Assistant, Lite) from the config defaults,
// proving multi-role support through a single handler without per-role
// backend instances.
func TestInferenceDispatch_ProcessEnvelope_AllThreeRoles_RoutesByConfigDefault(t *testing.T) {
	svc, backend, _, _ := newInferenceIntegrationFixture(t)

	roles := []struct {
		name      string
		protoRole operatorv1.ModelRole
		wantModel string
	}{
		{"primary", operatorv1.ModelRole_MODEL_ROLE_PRIMARY, "gemma3:4b"},
		{"assistant", operatorv1.ModelRole_MODEL_ROLE_ASSISTANT, "llama3.2:3b"},
		{"lite", operatorv1.ModelRole_MODEL_ROLE_LITE, "qwen3:1.5b"},
	}

	for _, tc := range roles {
		t.Run(tc.name, func(t *testing.T) {
			backend.called = false
			backend.lastReq = models.GenerateRequest{}

			wire := buildInferenceEnvelope(t, tc.protoRole, "", "test prompt")
			receipt, err := svc.ProcessEnvelope(context.Background(), wire)

			require.NoError(t, err)
			require.NotNil(t, receipt)
			assert.True(t, backend.called, "backend must be invoked for role %s", tc.name)
			assert.Equal(t, tc.wantModel, backend.lastReq.Model, "backend must receive the %s model", tc.name)
		})
	}
}

// TestInferenceDispatch_ProcessEnvelope_NilInferenceHandler_FailsClosed
// verifies that when the inference handler is not wired (cfg.Inference.Enabled
// is false), a governed inference envelope is rejected at execution time with
// ErrInferenceBackendNotRegistered rather than silently succeeding.
func TestInferenceDispatch_ProcessEnvelope_NilInferenceHandler_FailsClosed(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	auditStore := newInferenceTestAuditStore(t)

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signerStore := &governance.FailClosedSignerStore{
		Signers: map[string]ed25519.PublicKey{"test-key": pubKey},
	}

	svc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:             cfg,
		Logger:             logger,
		PubSubClient:       NewInProcessPubSubClient(nil),
		ActuatorSigningKey: privKey,
		ActuatorKeyID:      "test-key",
		AuditorSigningKey:  privKey,
		AuditorKeyID:       hex.EncodeToString(pubKey),
		Scrubbing:          mustNewScrubbingSvc(t, logger),
		AuditStore:         auditStore,
	}, OutboundModeDeps{
		GovernanceCoreDeps: GovernanceCoreDeps{
			ReplayStore:       testutil.NewStatefulMockReplayStore(),
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.ConfigurableMockL3Notary{ShouldPass: true},
			SignerStore:       signerStore,
			Doctrine:          governance.NewL1Doctrine(),
		},
	})
	require.NoError(t, err)

	wire := buildInferenceEnvelope(t, operatorv1.ModelRole_MODEL_ROLE_PRIMARY, "gemma3:4b", "test")
	receipt, err := svc.ProcessEnvelope(context.Background(), wire)

	require.Error(t, err, "nil inference handler must fail closed")
	require.NotNil(t, receipt, "actuator must return a signed failed receipt")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, receipt.Status, "receipt status must be FAILED")
	assert.ErrorIs(t, err, constants.ErrInferenceBackendNotRegistered, "error must wrap ErrInferenceBackendNotRegistered")
}

// TestInferenceDispatch_ProcessEnvelope_BackendFailure_PublishesFailedCompletion
// verifies that a backend failure produces a FAILED signed receipt and an
// InferenceCompletion carrying that receipt with no result, so the waiting
// Gateway dispatch terminates immediately with a typed failure instead of
// the generic dispatch timeout.
func TestInferenceDispatch_ProcessEnvelope_BackendFailure_PublishesFailedCompletion(t *testing.T) {
	svc, backend, _, results := newInferenceIntegrationFixture(t)
	backend.err = constants.ErrInferenceGenerateFailed

	wire := buildInferenceEnvelope(t, operatorv1.ModelRole_MODEL_ROLE_PRIMARY, "gemma3:4b", "test")

	receipt, err := svc.ProcessEnvelope(context.Background(), wire)

	require.Error(t, err, "backend failure must fail the transaction")
	assert.ErrorIs(t, err, constants.ErrInferenceGenerateFailed)
	require.NotNil(t, receipt, "actuator must return a signed failed receipt")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, receipt.Status)

	require.Len(t, results.inferenceCompletions, 1, "a failed execution must still publish a completion")
	completion := results.inferenceCompletions[0]
	require.NotNil(t, completion.Receipt, "failed completion must carry the FAILED receipt")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, completion.Receipt.Status)
	assert.Equal(t, receipt.Signature, completion.Receipt.Signature, "completion must carry the final signed receipt")
	assert.Nil(t, completion.Result, "failed completion must not carry a result")
}
