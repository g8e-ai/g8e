// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package dispatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// --- stubs ---

type stubCommandDispatcher struct {
	result  *CommandDispatchResult
	err     error
	calls   int
	lastReq CommandDispatchRequest
}

func (s *stubCommandDispatcher) Dispatch(_ context.Context, req CommandDispatchRequest) (*CommandDispatchResult, error) {
	s.calls++
	s.lastReq = req
	return s.result, s.err
}

type stubOperatorLister struct {
	ops []models.OperatorDocumentGo
	err error
}

func (s *stubOperatorLister) ListUserOperators(_ string) ([]models.OperatorDocumentGo, error) {
	return s.ops, s.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func capableOp(sessionID string) models.OperatorDocumentGo {
	return models.OperatorDocumentGo{
		ID:                "op-" + sessionID,
		OperatorSessionID: sessionID,
		Status:            constants.OperatorStatusActive,
		RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
	}
}

func successDispatchResult(t *testing.T) *CommandDispatchResult {
	t.Helper()
	return successDispatchResultFor(t, baseRequest())
}

func successDispatchResultFor(t *testing.T, req DispatchInferenceRequest) *CommandDispatchResult {
	t.Helper()
	payload, err := proto.Marshal(&operatorv1.InferenceResult{
		Parts:                 []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "ok"}}},
		Model:                 req.Model,
		RequestedModel:        req.Model,
		ProviderAttemptId:     req.ProviderAttemptID,
		RequestedModelDigest:  req.ModelDigest,
		ServedModelDigest:     req.ModelDigest,
		NormalizedRequestHash: "11aa22bb33cc44dd55ee66ff7788990011aa22bb33cc44dd55ee66ff77889900",
		OutputHash:            "00aa11bb22cc33dd44ee55ff6677889900aa11bb22cc33dd44ee55ff66778899",
		CampaignId:            req.CampaignID,
		RunId:                 req.RunID,
		AssignmentId:          req.AssignmentID,
		EvaluationAttemptId:   req.EvaluationAttemptID,
		ScenarioId:            req.ScenarioID,
		ModelRegistryDigest:   req.ModelRegistryDigest,
	})
	require.NoError(t, err)
	return &CommandDispatchResult{
		TransactionID: "tx-1",
		ResultPayload: payload,
		Receipt:       &operatorv1.ActionReceipt{TransactionId: "tx-1"},
	}
}

func baseMessages() []*operatorv1.InferenceMessage {
	return []*operatorv1.InferenceMessage{{
		Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
		Parts: []*operatorv1.InferenceMessagePart{{
			Part: &operatorv1.InferenceMessagePart_Text{Text: "hello"},
		}},
	}}
}

func baseRequest() DispatchInferenceRequest {
	return DispatchInferenceRequest{
		Role:                 models.InferenceModelRolePrimary,
		Model:                "gemma3:4b",
		Messages:             baseMessages(),
		ProviderAttemptID:    "provider-attempt-1",
		RequestorUserID:      "user-1",
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
	}
}

func configureCampaignRequest(t *testing.T, req *DispatchInferenceRequest) {
	t.Helper()
	req.ModelDigest = strings.Repeat("a", 64)
	req.CampaignID = "campaign-1"
	req.RunID = "run-1"
	req.AssignmentID = "assignment-1"
	req.EvaluationAttemptID = "evaluation-attempt-1"
	req.ScenarioID = "scenario-1"
	req.ModelRegistry = []*operatorv1.InferenceModelVariant{{Model: req.Model, Digest: req.ModelDigest}}
	registryDigest, err := models.ComputeInferenceModelRegistryDigest(req.CampaignID, req.ModelRegistry)
	require.NoError(t, err)
	req.ModelRegistryDigest = registryDigest
}

func TestValidateCampaignModelRegistry_FailsClosedOnInvalidAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		mutate    func(*DispatchInferenceRequest)
		wantError error
	}{
		{name: "valid authority", mutate: func(*DispatchInferenceRequest) {}},
		{name: "missing assignment binding", mutate: func(req *DispatchInferenceRequest) { req.AssignmentID = "" }, wantError: constants.ErrInferenceCampaignBindingInvalid},
		{name: "changed registry digest", mutate: func(req *DispatchInferenceRequest) { req.ModelRegistryDigest = strings.Repeat("b", 64) }, wantError: constants.ErrInferenceModelRegistryInvalid},
		{name: "model absent", mutate: func(req *DispatchInferenceRequest) { req.Model = "absent:1" }, wantError: constants.ErrInferenceModelOverrideDenied},
		{name: "duplicate model", mutate: func(req *DispatchInferenceRequest) {
			req.ModelRegistry = append(req.ModelRegistry, proto.Clone(req.ModelRegistry[0]).(*operatorv1.InferenceModelVariant))
		}, wantError: constants.ErrInferenceModelRegistryInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := baseRequest()
			configureCampaignRequest(t, &req)
			tt.mutate(&req)

			err := validateCampaignModelRegistry(req)

			if tt.wantError == nil {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantError)
		})
	}
}

// --- routing authority tests ---

func TestDispatchInference_ZeroCapableOperatorsFailsClosed(t *testing.T) {
	dispatcher := &stubCommandDispatcher{}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		{ID: "op-plain", OperatorSessionID: "sess-plain"},
	}}, testLogger())

	_, err := svc.DispatchInference(context.Background(), baseRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceOperatorNotFound)
	assert.Equal(t, 0, dispatcher.calls, "dispatcher must not be called when no capable operator exists")
}

func TestDispatchInference_SingleCapableOperatorSelected(t *testing.T) {
	dispatcher := &stubCommandDispatcher{result: successDispatchResult(t)}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		{ID: "op-plain", OperatorSessionID: "sess-plain"},
		capableOp("sess-inf"),
	}}, testLogger())

	result, err := svc.DispatchInference(context.Background(), baseRequest())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, dispatcher.calls)
	assert.Equal(t, "sess-inf", dispatcher.lastReq.TargetOperatorSessionID)
	assert.Equal(t, string(constants.ActionTypeInference), dispatcher.lastReq.ActionType)
	assert.Equal(t, RequestDeadline, dispatcher.lastReq.Timeout, "inference dispatch must carry the single explicit request deadline")
}

func TestDispatchInference_MultipleCapableOperatorsRejectedWithoutTarget(t *testing.T) {
	dispatcher := &stubCommandDispatcher{}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
		capableOp("sess-b"),
	}}, testLogger())

	_, err := svc.DispatchInference(context.Background(), baseRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceOperatorAmbiguous)
	assert.Equal(t, 0, dispatcher.calls, "ambiguous routing must not dispatch")
}

func TestDispatchInference_ExplicitTargetSelectsOperator(t *testing.T) {
	dispatcher := &stubCommandDispatcher{result: successDispatchResult(t)}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
		capableOp("sess-b"),
	}}, testLogger())

	req := baseRequest()
	req.TargetOperatorSessionID = "sess-b"
	_, err := svc.DispatchInference(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "sess-b", dispatcher.lastReq.TargetOperatorSessionID)
}

func TestDispatchInference_ExplicitTargetNotOwnedByRequestor(t *testing.T) {
	dispatcher := &stubCommandDispatcher{}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
	}}, testLogger())

	req := baseRequest()
	req.TargetOperatorSessionID = "sess-other-user"
	_, err := svc.DispatchInference(context.Background(), req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceOperatorNotFound)
	assert.Equal(t, 0, dispatcher.calls)
}

func TestDispatchInference_ExplicitTargetNotCapable(t *testing.T) {
	dispatcher := &stubCommandDispatcher{}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		{ID: "op-plain", OperatorSessionID: "sess-plain", Status: constants.OperatorStatusActive},
		capableOp("sess-a"),
	}}, testLogger())

	req := baseRequest()
	req.TargetOperatorSessionID = "sess-plain"
	_, err := svc.DispatchInference(context.Background(), req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceOperatorNotCapable)
	assert.Equal(t, 0, dispatcher.calls)
}

func TestDispatchInference_TerminatedOperatorNotSelectable(t *testing.T) {
	dispatcher := &stubCommandDispatcher{}
	terminated := capableOp("sess-dead")
	terminated.Status = constants.OperatorStatusTerminated
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		terminated,
	}}, testLogger())

	_, err := svc.DispatchInference(context.Background(), baseRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceOperatorNotFound)
	assert.Equal(t, 0, dispatcher.calls)
}

// --- request validation ---

func TestDispatchInference_UnspecifiedRoleRejected(t *testing.T) {
	dispatcher := &stubCommandDispatcher{}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
	}}, testLogger())

	req := baseRequest()
	req.Role = models.InferenceModelRoleUnspecified
	_, err := svc.DispatchInference(context.Background(), req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceRoleInvalid)
	assert.Equal(t, 0, dispatcher.calls)
}

func TestDispatchInference_MissingRequestorRejected(t *testing.T) {
	dispatcher := &stubCommandDispatcher{}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{}, testLogger())

	req := baseRequest()
	req.RequestorUserID = ""
	_, err := svc.DispatchInference(context.Background(), req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrRegistrationUserIDRequired)
	assert.Equal(t, 0, dispatcher.calls)
}

func TestDispatchInference_MissingProviderAttemptIDRejected(t *testing.T) {
	dispatcher := &stubCommandDispatcher{}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{capableOp("sess-a")}}, testLogger())

	req := baseRequest()
	req.ProviderAttemptID = ""
	_, err := svc.DispatchInference(context.Background(), req)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderAttemptRequired)
	assert.Equal(t, 0, dispatcher.calls)
}

func TestDispatchInference_OperatorListerErrorPropagates(t *testing.T) {
	dispatcher := &stubCommandDispatcher{}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{err: errors.New("doc store down")}, testLogger())

	_, err := svc.DispatchInference(context.Background(), baseRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "doc store down")
	assert.Equal(t, 0, dispatcher.calls)
}

// --- outcome classification ---

func TestDispatchInference_DispatchDeadlineMapsToUnknownOutcome(t *testing.T) {
	dispatcher := &stubCommandDispatcher{err: fmt.Errorf("dispatch: %w", constants.ErrDispatchResultTimeout)}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
	}}, testLogger())

	_, err := svc.DispatchInference(context.Background(), baseRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceOutcomeUnknown,
		"a dispatch deadline on inference means the remote provider call may still be running")
	assert.Equal(t, 1, dispatcher.calls, "unknown outcomes must not be retried automatically")
}

func TestDispatchInference_DispatchErrorPropagates(t *testing.T) {
	dispatcher := &stubCommandDispatcher{err: fmt.Errorf("dispatch: %w", constants.ErrDispatchNoDelivery)}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
	}}, testLogger())

	_, err := svc.DispatchInference(context.Background(), baseRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDispatchNoDelivery)
}

func TestDispatchInference_EmptyResultPayloadFailsClosed(t *testing.T) {
	dispatcher := &stubCommandDispatcher{result: &CommandDispatchResult{TransactionID: "tx-1"}}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
	}}, testLogger())

	_, err := svc.DispatchInference(context.Background(), baseRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceResultDecode)
}

func TestDispatchInference_MalformedResultPayloadFailsClosed(t *testing.T) {
	dispatcher := &stubCommandDispatcher{result: &CommandDispatchResult{
		TransactionID: "tx-1",
		ResultPayload: []byte{0xff, 0xff, 0xff},
	}}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
	}}, testLogger())

	_, err := svc.DispatchInference(context.Background(), baseRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceResultDecode)
}

func TestDispatchInference_ContradictoryResultMetadataFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		result *operatorv1.InferenceResult
	}{
		{
			name: "unreported nonzero usage",
			result: &operatorv1.InferenceResult{
				Parts:        []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "answer"}}},
				Model:        "test-model",
				PromptTokens: 1,
			},
		},
		{
			name: "reported usage arithmetic mismatch",
			result: &operatorv1.InferenceResult{
				Parts:            []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "answer"}}},
				Model:            "test-model",
				PromptTokens:     1,
				CompletionTokens: 1,
				TotalTokens:      3,
				UsageReported:    true,
			},
		},
		{
			name: "timing missing source",
			result: &operatorv1.InferenceResult{
				Parts:          []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "answer"}}},
				Model:          "test-model",
				LoadDurationNs: func() *int64 { value := int64(1); return &value }(),
			},
		},
		{
			name: "source missing timing",
			result: &operatorv1.InferenceResult{
				Parts:        []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "answer"}}},
				Model:        "test-model",
				TimingSource: operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_PROVIDER,
			},
		},
		{
			name: "unknown timing source",
			result: &operatorv1.InferenceResult{
				Parts:        []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "answer"}}},
				Model:        "test-model",
				TimingSource: operatorv1.InferenceTimingSource(99),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := proto.Marshal(tt.result)
			require.NoError(t, err)
			dispatcher := &stubCommandDispatcher{result: &CommandDispatchResult{TransactionID: "tx-1", ResultPayload: payload}}
			svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{capableOp("sess-a")}}, testLogger())

			result, err := svc.DispatchInference(context.Background(), baseRequest())

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
		})
	}
}

func TestDispatchInference_ResultIdentityMismatchFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*operatorv1.InferenceResult)
	}{
		{name: "provider attempt", mutate: func(result *operatorv1.InferenceResult) { result.ProviderAttemptId = "other-attempt" }},
		{name: "requested model", mutate: func(result *operatorv1.InferenceResult) { result.RequestedModel = "other-model" }},
		{name: "campaign", mutate: func(result *operatorv1.InferenceResult) { result.CampaignId = "other-campaign" }},
		{name: "model digest", mutate: func(result *operatorv1.InferenceResult) { result.RequestedModelDigest = "bb" + strings.Repeat("0", 62) }},
		{name: "model registry digest", mutate: func(result *operatorv1.InferenceResult) { result.ModelRegistryDigest = strings.Repeat("b", 64) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := baseRequest()
			configureCampaignRequest(t, &req)
			result := &operatorv1.InferenceResult{
				Parts:                 []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "answer"}}},
				Model:                 req.Model,
				RequestedModel:        req.Model,
				ProviderAttemptId:     req.ProviderAttemptID,
				NormalizedRequestHash: strings.Repeat("1", 64),
				OutputHash:            strings.Repeat("2", 64),
				CampaignId:            req.CampaignID,
				RunId:                 req.RunID,
				AssignmentId:          req.AssignmentID,
				EvaluationAttemptId:   req.EvaluationAttemptID,
				ScenarioId:            req.ScenarioID,
				RequestedModelDigest:  req.ModelDigest,
				ServedModelDigest:     req.ModelDigest,
				ModelRegistryDigest:   req.ModelRegistryDigest,
			}
			tt.mutate(result)
			payload, err := proto.Marshal(result)
			require.NoError(t, err)
			dispatcher := &stubCommandDispatcher{result: &CommandDispatchResult{TransactionID: "tx-1", ResultPayload: payload}}
			svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{capableOp("sess-a")}}, testLogger())

			out, err := svc.DispatchInference(context.Background(), req)

			require.Error(t, err)
			assert.Nil(t, out)
			assert.ErrorIs(t, err, constants.ErrInferenceIdentityMismatch)
		})
	}
}

func TestDispatchInference_SuccessReturnsResultAndReceipt(t *testing.T) {
	receipt := &operatorv1.ActionReceipt{TransactionId: "tx-9", Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED}
	resultPayload, err := proto.Marshal(&operatorv1.InferenceResult{
		Parts:                 []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "answer"}}},
		Model:                 "gemma3:4b",
		RequestedModel:        "gemma3:4b",
		ProviderAttemptId:     "provider-attempt-1",
		NormalizedRequestHash: strings.Repeat("1", 64),
		OutputHash:            strings.Repeat("2", 64),
	})
	require.NoError(t, err)
	dispatcher := &stubCommandDispatcher{result: &CommandDispatchResult{
		TransactionID: "tx-9",
		ResultPayload: resultPayload,
		Receipt:       receipt,
	}}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
	}}, testLogger())

	req := baseRequest()
	req.ActingAppID = "spiffe://g8e.local/app/g8ee"
	req.CaseID = "case-1"
	out, err := svc.DispatchInference(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "tx-9", out.TransactionID)
	require.NotNil(t, out.Result)
	require.Len(t, out.Result.GetParts(), 1)
	assert.Equal(t, "answer", out.Result.GetParts()[0].GetText())
	assert.Same(t, receipt, out.Receipt)
	assert.Equal(t, "case-1", dispatcher.lastReq.CaseID)
	assert.Equal(t, "spiffe://g8e.local/app/g8ee", dispatcher.lastReq.ActingAppID)
}

func TestDispatchInference_PayloadCarriesRequestFields(t *testing.T) {
	dispatcher := &stubCommandDispatcher{result: successDispatchResult(t)}
	svc := NewDispatchService(dispatcher, &stubOperatorLister{ops: []models.OperatorDocumentGo{
		capableOp("sess-a"),
	}}, testLogger())

	req := baseRequest()
	req.Role = models.InferenceModelRoleLite
	req.Model = "gemma3:4b"
	req.Temperature = 0.2
	req.MaxTokens = 64
	req.KeepAlive = "5m"
	topP := float32(0.8)
	topK := int32(40)
	req.TopP = &topP
	req.TopK = &topK
	req.StopSequences = []string{"END", "STOP"}
	req.ResponseFormat = &operatorv1.InferenceResponseFormat{MediaType: "application/json", JsonSchema: `{"properties":{"answer":{"type":"string"}},"type":"object"}`}
	req.RequestSchemaVersion = constants.InferenceRequestSchemaVersion
	parallelToolCalls := false
	contextLimit := int32(8192)
	req.ToolChoice = &operatorv1.InferenceToolChoice{Mode: operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO, AllowedToolNames: []string{"inspect"}}
	req.ParallelToolCalls = &parallelToolCalls
	req.Thinking = &operatorv1.InferenceThinkingControl{Mode: &operatorv1.InferenceThinkingControl_Enabled{Enabled: true}, IncludeThoughts: true}
	req.ContextLimit = &contextLimit
	configureCampaignRequest(t, &req)
	req.Tools = []*operatorv1.InferenceToolDeclaration{{
		Name:        "inspect",
		Description: "Inspect a target",
		JsonSchema:  `{"properties":{"path":{"type":"string"}},"type":"object"}`,
	}}
	dispatcher.result = successDispatchResultFor(t, req)
	_, err := svc.DispatchInference(context.Background(), req)
	require.NoError(t, err)

	infReq := &operatorv1.InferenceRequested{}
	require.NoError(t, proto.Unmarshal(dispatcher.lastReq.Payload, infReq))
	expected := &operatorv1.InferenceRequested{
		Role:                 operatorv1.ModelRole_MODEL_ROLE_LITE,
		Model:                "gemma3:4b",
		Temperature:          0.2,
		MaxTokens:            64,
		KeepAlive:            "5m",
		Messages:             baseMessages(),
		Tools:                req.Tools,
		TopP:                 &topP,
		TopK:                 &topK,
		StopSequences:        req.StopSequences,
		ResponseFormat:       req.ResponseFormat,
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		ToolChoice:           req.ToolChoice,
		ParallelToolCalls:    req.ParallelToolCalls,
		Thinking:             req.Thinking,
		ContextLimit:         req.ContextLimit,
		ProviderAttemptId:    req.ProviderAttemptID,
		ModelDigest:          req.ModelDigest,
		CampaignId:           req.CampaignID,
		RunId:                req.RunID,
		AssignmentId:         req.AssignmentID,
		EvaluationAttemptId:  req.EvaluationAttemptID,
		ScenarioId:           req.ScenarioID,
		ModelRegistry:        req.ModelRegistry,
		ModelRegistryDigest:  req.ModelRegistryDigest,
	}
	assert.True(t, proto.Equal(expected, infReq), "forwarded governed payload must preserve every ordered message and tool field")
}
