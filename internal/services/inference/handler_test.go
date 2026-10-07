// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// testCommandMessage is a test-only implementation of governance.CommandMessage.
type testCommandMessage struct {
	payload []byte
}

func (m *testCommandMessage) GetPayload() []byte  { return m.payload }
func (m *testCommandMessage) SetPayload(p []byte) { m.payload = p }

func mustNewScrubbingSvc(t *testing.T) *scrubbing.ScrubbingService {
	t.Helper()
	svc, err := scrubbing.NewScrubbingService(context.Background(), &scrubbing.Config{Enabled: true, StrictMode: false}, testutil.NewTestLogger(), nil)
	require.NoError(t, err)
	return svc
}

func mustNewHandler(t *testing.T, backend Backend, cfg *config.Config, scrubbingSvc *scrubbing.ScrubbingService, logger *slog.Logger) *InferenceExecutionHandler {
	t.Helper()
	handler, err := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)
	require.NoError(t, err)
	return handler
}

func mustMarshalInferenceRequested(t *testing.T, req *operatorv1.InferenceRequested) []byte {
	t.Helper()
	if req.ProviderAttemptId == "" {
		req.ProviderAttemptId = "provider-attempt-1"
	}
	data, err := proto.Marshal(req)
	require.NoError(t, err)
	return data
}

func textInferenceMessages(text string) []*operatorv1.InferenceMessage {
	return []*operatorv1.InferenceMessage{{
		Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
		Parts: []*operatorv1.InferenceMessagePart{{
			Part: &operatorv1.InferenceMessagePart_Text{Text: text},
		}},
	}}
}

func textInferenceResponseParts(text string) []*operatorv1.InferenceResponsePart {
	return []*operatorv1.InferenceResponsePart{{
		Part: &operatorv1.InferenceResponsePart_Text{Text: text},
	}}
}

// wantDigest returns the canonical result digest the handler must return as
// the receipt summary for the given backend response.
func wantDigest(t *testing.T, resp *models.GenerateResponse) string {
	t.Helper()
	digest, err := models.ComputeInferenceResultDigest(resp.ToProtoInferenceResult())
	require.NoError(t, err)
	return digest
}

func TestInferenceHandler_ExecuteVerifiedTransaction_ServesTheModelTheRequestNamesForEveryRole(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		role  operatorv1.ModelRole
		model string
	}{
		{operatorv1.ModelRole_MODEL_ROLE_PRIMARY, "gemma3:4b"},
		{operatorv1.ModelRole_MODEL_ROLE_ASSISTANT, "llama3.2:3b"},
		{operatorv1.ModelRole_MODEL_ROLE_LITE, "qwen3:1.5b"},
	} {
		t.Run(tc.role.String(), func(t *testing.T) {
			backend := &stubBackend{generateResp: &models.GenerateResponse{
				Parts:        textInferenceResponseParts("response"),
				FinishReason: "stop",
				Model:        tc.model,
			}}
			cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true, KeepAlive: "-1"}}
			handler := mustNewHandler(t, backend, cfg, mustNewScrubbingSvc(t), testutil.NewTestLogger())

			payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 tc.role,
				Model:                tc.model,
				Messages:             textInferenceMessages("What is 2+2?"),
			})

			summary, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, &testCommandMessage{payload: payload})

			require.NoError(t, err)
			assert.Equal(t, wantDigest(t, backend.generateResp), summary, "summary must be the canonical result digest binding the complete result")
			assert.Equal(t, tc.model, backend.lastReq.Model, "the Operator is a worker: it serves the model the user chose, whatever the role")
			require.Len(t, backend.lastReq.Messages, 1)
			assert.Equal(t, "What is 2+2?", backend.lastReq.Messages[0].Parts[0].GetText())
			assert.Equal(t, "-1", backend.lastReq.KeepAlive)
		})
	}
}

func TestInferenceHandler_ExecuteVerifiedTransaction_NoModelOnTheRequestFailsClosed(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true, KeepAlive: "-1"}}
	handler := mustNewHandler(t, backend, cfg, mustNewScrubbingSvc(t), testutil.NewTestLogger())

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages:             textInferenceMessages("test"),
	})

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, &testCommandMessage{payload: payload})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelRefInvalid, "the Operator holds no default model to fall back to")
	assert.Zero(t, backend.calls, "backend must not be called without a model")
}

func TestInferenceHandler_ExecuteVerifiedTransaction_MissingProviderAttemptIDRejected(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
	handler := mustNewHandler(t, backend, cfg, mustNewScrubbingSvc(t), testutil.NewTestLogger())
	payload, err := proto.Marshal(&operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model",
		Messages:             textInferenceMessages("test"),
	})
	require.NoError(t, err)

	_, err = handler.ExecuteInference(context.Background(), &testCommandMessage{payload: payload})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderAttemptRequired)
	assert.Zero(t, backend.calls)
}

func TestComputeInferenceModelRegistryDigest_IsOrderIndependentAndIdentitySensitive(t *testing.T) {
	t.Parallel()
	variants := []*operatorv1.InferenceModelVariant{
		{Model: "model-b:1", Digest: strings.Repeat("b", 64)},
		{Model: "model-a:1", Digest: strings.Repeat("a", 64)},
	}

	digest, err := models.ComputeInferenceModelRegistryDigest("campaign-1", variants)
	require.NoError(t, err)
	reversed, err := models.ComputeInferenceModelRegistryDigest("campaign-1", []*operatorv1.InferenceModelVariant{variants[1], variants[0]})
	require.NoError(t, err)
	changedCampaign, err := models.ComputeInferenceModelRegistryDigest("campaign-2", variants)
	require.NoError(t, err)
	changedVariant, err := models.ComputeInferenceModelRegistryDigest("campaign-1", []*operatorv1.InferenceModelVariant{
		{Model: "model-b:1", Digest: strings.Repeat("c", 64)},
		variants[1],
	})
	require.NoError(t, err)

	assert.Equal(t, digest, reversed)
	assert.NotEqual(t, digest, changedCampaign)
	assert.NotEqual(t, digest, changedVariant)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_CampaignRegistryAuthorizesEveryRole(t *testing.T) {
	t.Parallel()
	const model = "frozen-model:1"
	const campaignID = "campaign-1"
	modelDigest := strings.Repeat("a", 64)
	registry := []*operatorv1.InferenceModelVariant{{Model: model, Digest: modelDigest}}
	registryDigest, err := models.ComputeInferenceModelRegistryDigest(campaignID, registry)
	require.NoError(t, err)

	for _, role := range []operatorv1.ModelRole{
		operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		operatorv1.ModelRole_MODEL_ROLE_ASSISTANT,
		operatorv1.ModelRole_MODEL_ROLE_LITE,
	} {
		t.Run(role.String(), func(t *testing.T) {
			backend := &stubBackend{generateResp: &models.GenerateResponse{
				Parts:             textInferenceResponseParts("response"),
				FinishReason:      "stop",
				Model:             model,
				ServedModelDigest: modelDigest,
			}}
			handler := mustNewHandler(t, backend, &config.Config{Inference: config.InferenceConfig{
				Enabled: true,
			}}, mustNewScrubbingSvc(t), testutil.NewTestLogger())
			payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 role,
				Model:                model,
				ModelDigest:          modelDigest,
				Messages:             textInferenceMessages("test"),
				CampaignId:           campaignID,
				RunId:                "run-1",
				AssignmentId:         "assignment-1",
				EvaluationAttemptId:  "attempt-1",
				ScenarioId:           "scenario-1",
				ModelRegistry:        registry,
				ModelRegistryDigest:  registryDigest,
			})

			_, err := handler.ExecuteInference(context.Background(), &testCommandMessage{payload: payload})

			require.NoError(t, err)
			assert.Equal(t, model, backend.lastReq.Model)
			assert.Equal(t, registryDigest, backend.generateResp.ModelRegistryDigest)
		})
	}
}

func TestInferenceHandler_ExecuteVerifiedTransaction_CampaignRegistryRejectsInvalidAuthority(t *testing.T) {
	t.Parallel()
	const campaignID = "campaign-1"
	modelDigest := strings.Repeat("a", 64)
	registry := []*operatorv1.InferenceModelVariant{{Model: "frozen-model:1", Digest: modelDigest}}
	registryDigest, err := models.ComputeInferenceModelRegistryDigest(campaignID, registry)
	require.NoError(t, err)

	tests := []struct {
		name      string
		mutate    func(*operatorv1.InferenceRequested)
		wantError error
	}{
		{name: "missing assignment binding", mutate: func(req *operatorv1.InferenceRequested) { req.AssignmentId = "" }, wantError: constants.ErrInferenceCampaignBindingInvalid},
		{name: "changed campaign", mutate: func(req *operatorv1.InferenceRequested) { req.CampaignId = "campaign-2" }, wantError: constants.ErrInferenceModelRegistryInvalid},
		{name: "changed registry digest", mutate: func(req *operatorv1.InferenceRequested) { req.ModelRegistryDigest = strings.Repeat("b", 64) }, wantError: constants.ErrInferenceModelRegistryInvalid},
		{name: "model absent", mutate: func(req *operatorv1.InferenceRequested) { req.Model = "absent:1" }, wantError: constants.ErrInferenceModelOverrideDenied},
		{name: "model digest mismatch", mutate: func(req *operatorv1.InferenceRequested) { req.ModelDigest = strings.Repeat("b", 64) }, wantError: constants.ErrInferenceModelOverrideDenied},
		{name: "duplicate model", mutate: func(req *operatorv1.InferenceRequested) {
			req.ModelRegistry = append(req.ModelRegistry, proto.Clone(req.ModelRegistry[0]).(*operatorv1.InferenceModelVariant))
		}, wantError: constants.ErrInferenceModelRegistryInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &stubBackend{}
			handler := mustNewHandler(t, backend, &config.Config{Inference: config.InferenceConfig{
				Enabled: true,
			}}, mustNewScrubbingSvc(t), testutil.NewTestLogger())
			req := &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "frozen-model:1",
				ModelDigest:          modelDigest,
				Messages:             textInferenceMessages("test"),
				CampaignId:           campaignID,
				RunId:                "run-1",
				AssignmentId:         "assignment-1",
				EvaluationAttemptId:  "attempt-1",
				ScenarioId:           "scenario-1",
				ModelRegistry:        cloneInferenceModelVariants(registry),
				ModelRegistryDigest:  registryDigest,
			}
			tt.mutate(req)

			_, err := handler.ExecuteInference(context.Background(), &testCommandMessage{payload: mustMarshalInferenceRequested(t, req)})

			assert.ErrorIs(t, err, tt.wantError)
			assert.Zero(t, backend.calls)
		})
	}
}

func TestInferenceHandler_ExecuteVerifiedTransaction_AcceptsGovernedCampaignWithoutStartupBinding(t *testing.T) {
	t.Parallel()
	const model = "gemma2:9b"
	modelDigest := strings.Repeat("c", 64)
	registry := []*operatorv1.InferenceModelVariant{{Model: model, Digest: modelDigest}}

	for _, campaignID := range []string{"eval-init-gemma2-9b", "eval-init-deepseek-r1-7b"} {
		t.Run(campaignID, func(t *testing.T) {
			registryDigest, err := models.ComputeInferenceModelRegistryDigest(campaignID, registry)
			require.NoError(t, err)
			backend := &stubBackend{generateResp: &models.GenerateResponse{
				Parts:             textInferenceResponseParts("response"),
				FinishReason:      "stop",
				Model:             model,
				ServedModelDigest: modelDigest,
			}}
			handler := mustNewHandler(t, backend, &config.Config{Inference: config.InferenceConfig{
				Enabled: true,
			}}, mustNewScrubbingSvc(t), testutil.NewTestLogger())
			payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                model,
				ModelDigest:          modelDigest,
				Messages:             textInferenceMessages("test"),
				CampaignId:           campaignID,
				RunId:                "run-1",
				AssignmentId:         "assignment-1",
				EvaluationAttemptId:  "attempt-1",
				ScenarioId:           "scenario-1",
				ModelRegistry:        registry,
				ModelRegistryDigest:  registryDigest,
			})

			_, err = handler.ExecuteInference(context.Background(), &testCommandMessage{payload: payload})

			require.NoError(t, err)
			assert.Equal(t, model, backend.lastReq.Model)
		})
	}
}

// TestInferenceHandler_ExecuteVerifiedTransaction_ChatAndCampaignShareOneHandler
// pins that one Inference Operator serves console chat and evaluation
// campaigns concurrently: authority is decided per request, so neither kind
// of request changes how the other is authorized, in either order. A chat
// request is served the model it names; a campaign request is confined to its
// frozen registry.
func TestInferenceHandler_ExecuteVerifiedTransaction_ChatAndCampaignShareOneHandler(t *testing.T) {
	t.Parallel()
	const chatModel = "gemma4:e4b"
	const campaignModel = "frozen-model:1"
	const campaignID = "campaign-shared"
	modelDigest := strings.Repeat("d", 64)
	registry := []*operatorv1.InferenceModelVariant{{Model: campaignModel, Digest: modelDigest}}
	registryDigest, err := models.ComputeInferenceModelRegistryDigest(campaignID, registry)
	require.NoError(t, err)

	plain := func() []byte {
		return mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
			RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
			Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
			Model:                chatModel,
			Messages:             textInferenceMessages("chat"),
		})
	}
	campaign := func() []byte {
		return mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
			RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
			Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
			Model:                campaignModel,
			ModelDigest:          modelDigest,
			Messages:             textInferenceMessages("eval"),
			CampaignId:           campaignID,
			RunId:                "run-1",
			AssignmentId:         "assignment-1",
			EvaluationAttemptId:  "attempt-1",
			ScenarioId:           "scenario-1",
			ModelRegistry:        registry,
			ModelRegistryDigest:  registryDigest,
		})
	}
	campaignOutsideRegistry := func() []byte {
		return mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
			RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
			Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
			Model:                chatModel,
			ModelDigest:          modelDigest,
			Messages:             textInferenceMessages("eval"),
			CampaignId:           campaignID,
			RunId:                "run-1",
			AssignmentId:         "assignment-1",
			EvaluationAttemptId:  "attempt-1",
			ScenarioId:           "scenario-1",
			ModelRegistry:        registry,
			ModelRegistryDigest:  registryDigest,
		})
	}

	for _, order := range []string{"chat-first", "campaign-first"} {
		t.Run(order, func(t *testing.T) {
			backend := &stubBackend{}
			handler := mustNewHandler(t, backend, &config.Config{Inference: config.InferenceConfig{
				Enabled: true,
			}}, mustNewScrubbingSvc(t), testutil.NewTestLogger())
			runChat := func() {
				backend.generateResp = &models.GenerateResponse{Parts: textInferenceResponseParts("chat"), FinishReason: "stop", Model: chatModel}
				_, err := handler.ExecuteInference(context.Background(), &testCommandMessage{payload: plain()})
				require.NoError(t, err)
				assert.Equal(t, chatModel, backend.lastReq.Model)
			}
			runCampaign := func() {
				backend.generateResp = &models.GenerateResponse{Parts: textInferenceResponseParts("eval"), FinishReason: "stop", Model: campaignModel, ServedModelDigest: modelDigest}
				_, err := handler.ExecuteInference(context.Background(), &testCommandMessage{payload: campaign()})
				require.NoError(t, err)
				assert.Equal(t, campaignModel, backend.lastReq.Model)
			}
			if order == "chat-first" {
				runChat()
				runCampaign()
			} else {
				runCampaign()
				runChat()
			}
			_, err := handler.ExecuteInference(context.Background(), &testCommandMessage{payload: campaignOutsideRegistry()})
			require.ErrorIs(t, err, constants.ErrInferenceModelOverrideDenied, "campaign authority still confines a campaign request to its frozen registry")
		})
	}
}

func cloneInferenceModelVariants(variants []*operatorv1.InferenceModelVariant) []*operatorv1.InferenceModelVariant {
	clones := make([]*operatorv1.InferenceModelVariant, len(variants))
	for i, variant := range variants {
		clones[i] = proto.Clone(variant).(*operatorv1.InferenceModelVariant)
	}
	return clones
}

func TestInferenceHandler_ExecuteVerifiedTransaction_UnspecifiedRoleRejected(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := mustNewHandler(t, backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_UNSPECIFIED,
		Model:                "test-model",
		Messages:             textInferenceMessages("test"),
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceRoleInvalid)
	assert.Equal(t, 0, backend.calls)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_EmptyPayloadReturnsErrPubSubEmptyPayload(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := mustNewHandler(t, backend, cfg, scrubbingSvc, logger)

	cmdMsg := &testCommandMessage{payload: nil}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPubSubEmptyPayload)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_InvalidPayloadReturnsError(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := mustNewHandler(t, backend, cfg, scrubbingSvc, logger)

	cmdMsg := &testCommandMessage{payload: []byte("not a protobuf message")}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
}

func TestNewInferenceExecutionHandler_RejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	logger := testutil.NewTestLogger()
	tests := []struct {
		name         string
		backend      Backend
		cfg          *config.Config
		scrubbingSvc *scrubbing.ScrubbingService
		logger       *slog.Logger
	}{
		{name: "nil backend", cfg: cfg, scrubbingSvc: scrubbingSvc, logger: logger},
		{name: "nil config", backend: &stubBackend{}, scrubbingSvc: scrubbingSvc, logger: logger},
		{name: "nil scrubbing service", backend: &stubBackend{}, cfg: cfg, logger: logger},
		{name: "nil logger", backend: &stubBackend{}, cfg: cfg, scrubbingSvc: scrubbingSvc},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			handler, err := NewInferenceExecutionHandler(test.backend, test.cfg, test.scrubbingSvc, test.logger)
			assert.Nil(t, handler)
			assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
		})
	}
}

func TestInferenceHandler_ExecuteVerifiedTransaction_BackendErrorReturnsErrInferenceGenerateFailed(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateErr: constants.ErrInferenceGenerateFailed}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := mustNewHandler(t, backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model",
		Messages:             textInferenceMessages("test"),
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceGenerateFailed)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_ScrubsPromptBeforeBackend(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Parts:        textInferenceResponseParts("response"),
		FinishReason: "stop",
		Model:        "test-model",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:   true,
		KeepAlive: "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := mustNewHandler(t, backend, cfg, scrubbingSvc, logger)

	// Include an email-like pattern that scrubbing should redact
	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model",
		Messages:             textInferenceMessages("Contact me at user@example.com please"),
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.NotContains(t, backend.lastReq.Messages[0].Parts[0].GetText(), "user@example.com")
}

// TestInferenceHandler_ExecuteVerifiedTransaction_LongOutputStillBindsDigest
// proves that an output larger than ReceiptSummaryMaxBytes still produces a
// fixed-width canonical digest as the receipt summary — the digest binds the
// complete result rather than a truncated text prefix.
func TestInferenceHandler_ExecuteVerifiedTransaction_ScrubsTypedMessagesAndCanonicalJSONLeaves(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Parts:        textInferenceResponseParts("response"),
		FinishReason: "stop",
		Model:        "test-model",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
	handler := mustNewHandler(t, backend, cfg, mustNewScrubbingSvc(t), testutil.NewTestLogger())
	schema := `{"properties":{"email":{"type":"string"}},"type":"object"}`
	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model",
		Messages: []*operatorv1.InferenceMessage{
			{
				Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
				Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: "contact user@example.com"}}},
			},
			{
				Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT,
				Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_ToolCall{ToolCall: &operatorv1.InferenceToolCall{
					CallId:        "call-1",
					Name:          "inspect",
					ArgumentsJson: `{"count":2,"nested":{"email":"tool@example.com"}}`,
				}}}},
			},
			{
				Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL,
				Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_ToolResult{ToolResult: &operatorv1.InferenceToolResult{
					CallId:     "call-1",
					Name:       "inspect",
					ResultJson: `{"items":["result@example.com",3],"ok":true}`,
				}}}},
			},
		},
		Tools: []*operatorv1.InferenceToolDeclaration{{Name: "inspect", Description: "Inspect", JsonSchema: schema}},
	})

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, &testCommandMessage{payload: payload})

	require.NoError(t, err)
	require.Len(t, backend.lastReq.Messages, 3)
	assert.NotContains(t, backend.lastReq.Messages[0].Parts[0].GetText(), "user@example.com")
	arguments := backend.lastReq.Messages[1].Parts[0].GetToolCall().GetArgumentsJson()
	assert.Equal(t, `{"count":2,"nested":{"email":"[EMAIL]"}}`, arguments)
	result := backend.lastReq.Messages[2].Parts[0].GetToolResult().GetResultJson()
	assert.Equal(t, `{"items":["[EMAIL]",3],"ok":true}`, result)
	require.Len(t, backend.lastReq.Tools, 1)
	assert.Equal(t, schema, backend.lastReq.Tools[0].GetJsonSchema())
}

func TestInferenceHandler_ExecuteVerifiedTransaction_PreservesValidatedGenerationControls(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
	handler := mustNewHandler(t, backend, cfg, mustNewScrubbingSvc(t), testutil.NewTestLogger())
	topP := float32(0.75)
	topK := int32(42)
	seed := int32(424242)
	responseFormat := &operatorv1.InferenceResponseFormat{
		MediaType:  "application/json",
		JsonSchema: `{"properties":{"answer":{"type":"string"}},"type":"object"}`,
	}
	toolChoice := &operatorv1.InferenceToolChoice{
		Mode:             operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO,
		AllowedToolNames: []string{"inspect"},
	}
	parallelToolCalls := false
	thinking := &operatorv1.InferenceThinkingControl{
		Mode:            &operatorv1.InferenceThinkingControl_Enabled{Enabled: true},
		IncludeThoughts: true,
	}
	contextLimit := int32(8192)
	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model",
		Messages:             textInferenceMessages("test"),
		TopP:                 &topP,
		TopK:                 &topK,
		Seed:                 &seed,
		StopSequences:        []string{"END", "STOP"},
		ResponseFormat:       responseFormat,
		Tools:                []*operatorv1.InferenceToolDeclaration{{Name: "inspect", JsonSchema: `{"type":"object"}`}},
		ToolChoice:           toolChoice,
		ParallelToolCalls:    &parallelToolCalls,
		Thinking:             thinking,
		ContextLimit:         &contextLimit,
	})

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, &testCommandMessage{payload: payload})

	require.NoError(t, err)
	require.NotNil(t, backend.lastReq.TopP)
	assert.Equal(t, topP, *backend.lastReq.TopP)
	require.NotNil(t, backend.lastReq.TopK)
	assert.Equal(t, topK, *backend.lastReq.TopK)
	require.NotNil(t, backend.lastReq.Seed)
	assert.Equal(t, seed, *backend.lastReq.Seed)
	assert.Equal(t, []string{"END", "STOP"}, backend.lastReq.StopSequences)
	assert.True(t, proto.Equal(responseFormat, backend.lastReq.ResponseFormat))
	assert.True(t, proto.Equal(toolChoice, backend.lastReq.ToolChoice))
	require.NotNil(t, backend.lastReq.ParallelToolCalls)
	assert.False(t, *backend.lastReq.ParallelToolCalls)
	assert.True(t, proto.Equal(thinking, backend.lastReq.Thinking))
	require.NotNil(t, backend.lastReq.ContextLimit)
	assert.Equal(t, contextLimit, *backend.lastReq.ContextLimit)
	assert.Equal(t, constants.InferenceRequestSchemaVersion, backend.lastReq.RequestSchemaVersion)
}

func TestFromProtoInferenceRequested_ClonesRequestControls(t *testing.T) {
	t.Parallel()
	parallelToolCalls := false
	contextLimit := int32(8192)
	seed := int32(424242)
	source := &operatorv1.InferenceRequested{
		Messages:          textInferenceMessages("test"),
		Tools:             []*operatorv1.InferenceToolDeclaration{{Name: "inspect", JsonSchema: `{"type":"object"}`}},
		ToolChoice:        &operatorv1.InferenceToolChoice{Mode: operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO, AllowedToolNames: []string{"inspect"}},
		ParallelToolCalls: &parallelToolCalls,
		Thinking:          &operatorv1.InferenceThinkingControl{Mode: &operatorv1.InferenceThinkingControl_Enabled{Enabled: false}},
		ContextLimit:      &contextLimit,
		Seed:              &seed,
		ModelRegistry:     []*operatorv1.InferenceModelVariant{{Model: "model:1", Digest: strings.Repeat("a", 64)}},
	}

	cloned := models.FromProtoInferenceRequested(source)
	source.Messages[0].Parts[0].Part = &operatorv1.InferenceMessagePart_Text{Text: "changed"}
	source.Tools[0].Name = "changed"
	source.ToolChoice.AllowedToolNames[0] = "changed"
	source.ParallelToolCalls = nil
	source.Thinking.Mode = &operatorv1.InferenceThinkingControl_Enabled{Enabled: true}
	source.ContextLimit = nil
	source.Seed = nil
	source.ModelRegistry[0].Model = "changed"

	assert.Equal(t, "test", cloned.Messages[0].Parts[0].GetText())
	assert.Equal(t, "inspect", cloned.Tools[0].GetName())
	assert.Equal(t, []string{"inspect"}, cloned.ToolChoice.GetAllowedToolNames())
	require.NotNil(t, cloned.ParallelToolCalls)
	assert.False(t, *cloned.ParallelToolCalls)
	assert.False(t, cloned.Thinking.GetEnabled())
	require.NotNil(t, cloned.ContextLimit)
	assert.Equal(t, int32(8192), *cloned.ContextLimit)
	require.NotNil(t, cloned.Seed)
	assert.Equal(t, int32(424242), *cloned.Seed)
	assert.Equal(t, "model:1", cloned.ModelRegistry[0].GetModel())
}

func TestInferenceHandler_ExecuteVerifiedTransaction_AcceptsDisabledThinkingAndParallelTools(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{}
	handler := mustNewHandler(t, backend, &config.Config{Inference: config.InferenceConfig{Enabled: true}}, mustNewScrubbingSvc(t), testutil.NewTestLogger())
	parallelToolCalls := true
	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model",
		Messages:             textInferenceMessages("test"),
		ParallelToolCalls:    &parallelToolCalls,
		Thinking:             &operatorv1.InferenceThinkingControl{Mode: &operatorv1.InferenceThinkingControl_Enabled{Enabled: false}},
	})

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, &testCommandMessage{payload: payload})

	require.NoError(t, err)
	require.NotNil(t, backend.lastReq.ParallelToolCalls)
	assert.True(t, *backend.lastReq.ParallelToolCalls)
	require.NotNil(t, backend.lastReq.Thinking)
	assert.False(t, backend.lastReq.Thinking.GetEnabled())
}

func TestInferenceHandler_ExecuteVerifiedTransaction_InvalidTypedInputRejectedBeforeBackend(t *testing.T) {
	t.Parallel()
	invalidTopP := float32(1.1)
	invalidTopK := int32(maxInferenceTopK + 1)
	invalidSeed := int32(-1)
	invalidContextLow := int32(0)
	invalidContextHigh := int32(maxInferenceContextLimit + 1)
	declaredTool := []*operatorv1.InferenceToolDeclaration{{Name: "inspect", JsonSchema: `{"type":"object"}`}}
	tests := []struct {
		name string
		req  *operatorv1.InferenceRequested
		err  error
	}{
		{
			name: "missing request schema version",
			req: &operatorv1.InferenceRequested{
				Role:     operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:    "test-model",
				Messages: textInferenceMessages("test"),
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "top p exceeds supported bound",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				TopP:                 &invalidTopP,
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "top k exceeds supported bound",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				TopK:                 &invalidTopK,
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "negative seed rejected",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				Seed:                 &invalidSeed,
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "max tokens above the platform ceiling",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				MaxTokens:            constants.InferenceMaxOutputTokens + 1,
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "negative max tokens",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				MaxTokens:            -1,
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "unsupported response media type",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				ResponseFormat:       &operatorv1.InferenceResponseFormat{MediaType: "text/plain", JsonSchema: `{"type":"object"}`},
			},
			err: constants.ErrInferenceCapabilityUnsupported,
		},
		{
			name: "invalid response schema",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				ResponseFormat:       &operatorv1.InferenceResponseFormat{MediaType: "application/json", JsonSchema: `{"type":7}`},
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "unsupported none tool choice",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				Tools:                declaredTool,
				ToolChoice:           &operatorv1.InferenceToolChoice{Mode: operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_NONE},
			},
			err: constants.ErrInferenceCapabilityUnsupported,
		},
		{
			name: "unsupported required tool choice",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				Tools:                declaredTool,
				ToolChoice:           &operatorv1.InferenceToolChoice{Mode: operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_REQUIRED},
			},
			err: constants.ErrInferenceCapabilityUnsupported,
		},
		{
			name: "duplicate allowed tool name",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				Tools:                declaredTool,
				ToolChoice: &operatorv1.InferenceToolChoice{
					Mode:             operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO,
					AllowedToolNames: []string{"inspect", "inspect"},
				},
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "unknown allowed tool name",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				Tools:                declaredTool,
				ToolChoice: &operatorv1.InferenceToolChoice{
					Mode:             operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO,
					AllowedToolNames: []string{"search"},
				},
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "disabled thinking includes thoughts",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				Thinking: &operatorv1.InferenceThinkingControl{
					Mode:            &operatorv1.InferenceThinkingControl_Enabled{Enabled: false},
					IncludeThoughts: true,
				},
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "unsupported thinking level",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				Thinking: &operatorv1.InferenceThinkingControl{
					Mode: &operatorv1.InferenceThinkingControl_Level{Level: "high"},
				},
			},
			err: constants.ErrInferenceCapabilityUnsupported,
		},
		{
			name: "context limit is zero",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				ContextLimit:         &invalidContextLow,
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "context limit exceeds supported bound",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				ContextLimit:         &invalidContextHigh,
			},
			err: constants.ErrInferenceGenerationOptionsInvalid,
		},
		{
			name: "malformed tool call arguments",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages: []*operatorv1.InferenceMessage{{
					Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT,
					Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_ToolCall{ToolCall: &operatorv1.InferenceToolCall{Name: "inspect", ArgumentsJson: "{"}}}},
				}},
			},
			err: constants.ErrInferenceJSONInvalid,
		},
		{
			name: "noncanonical tool result",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages: []*operatorv1.InferenceMessage{{
					Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL,
					Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_ToolResult{ToolResult: &operatorv1.InferenceToolResult{Name: "inspect", ResultJson: `{ "ok": true }`}}}},
				}},
			},
			err: constants.ErrInferenceJSONNonCanonical,
		},
		{
			name: "unspecified message role",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages: []*operatorv1.InferenceMessage{{
					Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_UNSPECIFIED,
					Parts: textInferenceMessages("test")[0].Parts,
				}},
			},
			err: constants.ErrInferenceMessageInvalid,
		},
		{
			name: "message without parts",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             []*operatorv1.InferenceMessage{{Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER}},
			},
			err: constants.ErrInferenceMessageInvalid,
		},
		{
			name: "tool role with text part",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages: []*operatorv1.InferenceMessage{{
					Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL,
					Parts: textInferenceMessages("test")[0].Parts,
				}},
			},
			err: constants.ErrInferenceMessageInvalid,
		},
		{
			name: "duplicate tool argument key",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages: []*operatorv1.InferenceMessage{{
					Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT,
					Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_ToolCall{ToolCall: &operatorv1.InferenceToolCall{
						Name: "inspect", ArgumentsJson: `{"path":"a","path":"b"}`,
					}}}},
				}},
			},
			err: constants.ErrInferenceJSONInvalid,
		},
		{
			name: "semantically invalid tool schema",
			req: &operatorv1.InferenceRequested{
				RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
				Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Model:                "test-model",
				Messages:             textInferenceMessages("test"),
				Tools:                []*operatorv1.InferenceToolDeclaration{{Name: "inspect", JsonSchema: `{"type":7}`}},
			},
			err: constants.ErrInferenceToolSchemaInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &stubBackend{}
			cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
			handler := mustNewHandler(t, backend, cfg, mustNewScrubbingSvc(t), testutil.NewTestLogger())
			payload := mustMarshalInferenceRequested(t, tt.req)

			_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, &testCommandMessage{payload: payload})

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.err)
			assert.Equal(t, 0, backend.calls)
		})
	}
}

func TestComputeInferenceResultDigest_CoversUsageAndTimingEvidence(t *testing.T) {
	t.Parallel()
	loadDuration := int64(2_000_000)
	base := (&models.GenerateResponse{
		Parts:          textInferenceResponseParts("response"),
		UsageReported:  true,
		LoadDurationNS: &loadDuration,
		TimingSource:   operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_PROVIDER,
		Model:          "test-model",
	}).ToProtoInferenceResult()
	baseDigest, err := models.ComputeInferenceResultDigest(base)
	require.NoError(t, err)
	changed := proto.Clone(base).(*operatorv1.InferenceResult)
	changed.LoadDurationNs = func() *int64 { value := int64(3_000_000); return &value }()

	changedDigest, err := models.ComputeInferenceResultDigest(changed)

	require.NoError(t, err)
	assert.NotEqual(t, baseDigest, changedDigest)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_LongOutputStillBindsDigest(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	longText := make([]byte, constants.ReceiptSummaryMaxBytes+100)
	for i := range longText {
		longText[i] = 'a'
	}
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Parts:        textInferenceResponseParts(string(longText)),
		FinishReason: "stop",
		Model:        "test-model",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:   true,
		KeepAlive: "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := mustNewHandler(t, backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model",
		Messages:             textInferenceMessages("test"),
	})
	cmdMsg := &testCommandMessage{payload: payload}

	summary, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, wantDigest(t, backend.generateResp), summary, "summary must be the digest of the complete untruncated result")
	assert.Len(t, summary, 64, "digest summary is a fixed-width lowercase hex SHA-256")
}

func TestInferenceHandler_ExecuteVerifiedTransaction_AppliesConfigKeepAliveDefault(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Parts:        textInferenceResponseParts("response"),
		FinishReason: "stop",
		Model:        "test-model",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:   true,
		KeepAlive: "30m",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := mustNewHandler(t, backend, cfg, scrubbingSvc, logger)

	// Request does not set keep_alive, so config default should be used
	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model",
		Messages:             textInferenceMessages("test"),
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, "30m", backend.lastReq.KeepAlive, "config keep_alive default should be applied")
}

func TestInferenceHandler_ExecuteVerifiedTransaction_RequestKeepAliveOverridesConfig(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Parts:        textInferenceResponseParts("response"),
		FinishReason: "stop",
		Model:        "test-model",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:   true,
		KeepAlive: "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := mustNewHandler(t, backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model",
		Messages:             textInferenceMessages("test"),
		KeepAlive:            "5m",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, "5m", backend.lastReq.KeepAlive, "request keep_alive should override config default")
}

// The ensemble provider (ensemble/app/llm/providers/g8e.py) must emit exactly
// this escaped form; a raw '<', '>' or '&' round-trips to a different string
// and is rejected as non-canonical.
func TestNormalizeCanonicalJSON_EscapesHTMLCharacters(t *testing.T) {
	escaped := `{"pattern":":(){ :|:\u0026 };:","substring":"\u003e /dev/sd","tag":"\u003cb\u003e\u2028\u2029\u003c/b\u003e"}`
	got, err := normalizeCanonicalJSON(escaped, true, nil)
	require.NoError(t, err)
	assert.Equal(t, escaped, got)

	_, err = normalizeCanonicalJSON(`{"substring":"> /dev/sd"}`, true, nil)
	assert.ErrorIs(t, err, constants.ErrInferenceJSONNonCanonical)
}

// Verify the handler implements governance.ExecutionHandler at compile time.
var _ governance.ExecutionHandler = (*InferenceExecutionHandler)(nil)

func TestInferenceHandler_ProviderOutageFailsOnlyCurrentRequest(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{generateErr: constants.ErrInferenceBackendUnavailable}
	handler := mustNewHandler(t, backend, &config.Config{Inference: config.InferenceConfig{Enabled: true}}, mustNewScrubbingSvc(t), testutil.NewTestLogger())
	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:                "test-model", Messages: textInferenceMessages("test"),
	})
	command := &testCommandMessage{payload: payload}
	_, err := handler.ExecuteVerifiedTransaction(t.Context(), constants.Event.Operator.Inference.Requested, command)
	require.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)

	backend.generateErr = nil
	backend.generateResp = &models.GenerateResponse{Parts: textInferenceResponseParts("recovered"), FinishReason: "stop", Model: "test-model"}
	_, err = handler.ExecuteVerifiedTransaction(t.Context(), constants.Event.Operator.Inference.Requested, command)
	require.NoError(t, err, "the same handler must serve requests after provider recovery")
}
