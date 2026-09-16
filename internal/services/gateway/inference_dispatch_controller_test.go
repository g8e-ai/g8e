// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/dispatch"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// --- stubs for inference dispatch controller tests ---

// stubInferenceCommandDispatcher implements dispatch.CommandDispatcher for
// controller unit tests. It returns a canned result or sentinel error.
type stubInferenceCommandDispatcher struct {
	result      *dispatch.CommandDispatchResult
	err         error
	dispatchErr error
	lastReq     dispatch.CommandDispatchRequest
	called      bool
}

func (s *stubInferenceCommandDispatcher) Dispatch(_ context.Context, req dispatch.CommandDispatchRequest) (*dispatch.CommandDispatchResult, error) {
	s.called = true
	s.lastReq = req
	if req.OnInferenceProgress != nil {
		if err := req.OnInferenceProgress(&operatorv1.InferenceProgressEvent{
			ProviderAttemptId: "provider-attempt-1",
			Sequence:          1,
			Parts: []*operatorv1.InferenceResponsePart{
				{Part: &operatorv1.InferenceResponsePart_Text{Text: "partial"}},
			},
		}); err != nil {
			s.dispatchErr = err
			return nil, err
		}
	}
	s.dispatchErr = s.err
	return s.result, s.err
}

// stubInferenceOperatorLister implements dispatch.OperatorLister for
// controller unit tests.
type stubInferenceOperatorLister struct {
	ops []models.OperatorDocumentGo
	err error
}

func (s *stubInferenceOperatorLister) ListUserOperators(_ string) ([]models.OperatorDocumentGo, error) {
	return s.ops, s.err
}

func inferenceCapableOperator(sessionID string) models.OperatorDocumentGo {
	return models.OperatorDocumentGo{
		ID:                "op-inf-001",
		OperatorSessionID: sessionID,
		Status:            constants.OperatorStatusActive,
		RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
	}
}

// newInferenceDispatchControllerForTest builds the controller wired to a real
// inference dispatch.DispatchService backed by the given stubs, so request
// validation and error classification are exercised end to end.
func newInferenceDispatchControllerForTest(t *testing.T, dispatcher dispatch.CommandDispatcher, lister dispatch.OperatorLister, maxPayload int64) *InferenceDispatchController {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	return newInferenceDispatchController(InferenceDispatchControllerDeps{
		DispatchSvc: dispatch.NewDispatchService(dispatcher, lister, logger),
		Responder:   response.NewWriter(logger),
		Logger:      logger,
		MaxPayload:  maxPayload,
	})
}

// inferenceDispatchHTTPRequest builds a POST request carrying the protojson
// request body with the app workload and delegated user identity stamped the
// way the unified auth middleware does.
func inferenceDispatchHTTPRequest(t *testing.T, body []byte, appID, userID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.InferenceDispatch, bytes.NewReader(body))
	ctx := req.Context()
	if appID != "" {
		ctx = context.WithValue(ctx, constants.ContextKeyAppID, appID)
	}
	if userID != "" {
		ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	}
	return req.WithContext(ctx)
}

func marshalInferenceDispatchRequest(t *testing.T, req *operatorv1.InferenceDispatchRequest) []byte {
	t.Helper()
	if req.ProviderAttemptId == "" {
		req.ProviderAttemptId = "provider-attempt-1"
	}
	body, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(req)
	require.NoError(t, err)
	return body
}

func inferenceTextMessages(text string) []*operatorv1.InferenceMessage {
	return []*operatorv1.InferenceMessage{{
		Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
		Parts: []*operatorv1.InferenceMessagePart{{
			Part: &operatorv1.InferenceMessagePart_Text{Text: text},
		}},
	}}
}

func successDispatchResult(t *testing.T) *dispatch.CommandDispatchResult {
	t.Helper()
	return successDispatchResultForRequest(t, &operatorv1.InferenceDispatchRequest{Model: "gemma3:4b", ProviderAttemptId: "provider-attempt-1"})
}

func successDispatchResultForRequest(t *testing.T, req *operatorv1.InferenceDispatchRequest) *dispatch.CommandDispatchResult {
	t.Helper()
	result := &operatorv1.InferenceResult{
		Parts:                 []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "generated output"}}},
		PromptTokens:          7,
		CompletionTokens:      11,
		TotalTokens:           18,
		UsageReported:         true,
		FinishReason:          "stop",
		Model:                 req.GetModel(),
		RequestedModel:        req.GetModel(),
		ProviderAttemptId:     req.GetProviderAttemptId(),
		RequestedModelDigest:  req.GetModelDigest(),
		ServedModelDigest:     req.GetModelDigest(),
		NormalizedRequestHash: strings.Repeat("1", 64),
		OutputHash:            strings.Repeat("2", 64),
		CampaignId:            req.GetCampaignId(),
		RunId:                 req.GetRunId(),
		AssignmentId:          req.GetAssignmentId(),
		EvaluationAttemptId:   req.GetEvaluationAttemptId(),
		ScenarioId:            req.GetScenarioId(),
		ModelRegistryDigest:   req.GetModelRegistryDigest(),
		RetryCount:            req.GetRetryCount(),
		RetryClassification:   models.ClassifyRetry(req.GetRetryCount()),
		LoadState:             models.ClassifyLoadState(nil),
	}
	digest, err := models.ComputeInferenceResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = digest
	payload, err := proto.Marshal(result)
	require.NoError(t, err)
	return &dispatch.CommandDispatchResult{
		TransactionID: "tx-inference-001",
		ResultPayload: payload,
		Receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-inference-001",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: digest,
		},
	}
}

// --- InferenceDispatchController.HandleDispatch tests ---

func TestInferenceDispatchController_MethodNotAllowed(t *testing.T) {
	ctrl := newInferenceDispatchControllerForTest(t, &stubInferenceCommandDispatcher{}, &stubInferenceOperatorLister{}, 1024)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.InferenceDispatch, nil)
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestInferenceDispatchController_MissingAppIdentity(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, &stubInferenceOperatorLister{}, 1024)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:     operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages: inferenceTextMessages("hello"),
	})
	req := inferenceDispatchHTTPRequest(t, body, "", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code, "non-app callers must be rejected")
	assert.False(t, dispatcher.called, "dispatch must not run without an app workload identity")
}

func TestInferenceDispatchController_MissingUserIdentity(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, &stubInferenceOperatorLister{}, 1024)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:     operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages: inferenceTextMessages("hello"),
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code, "missing delegated user must be rejected")
	assert.False(t, dispatcher.called, "dispatch must not run without a user identity")
}

func TestInferenceDispatchController_MalformedJSON(t *testing.T) {
	ctrl := newInferenceDispatchControllerForTest(t, &stubInferenceCommandDispatcher{}, &stubInferenceOperatorLister{}, 1024)

	req := inferenceDispatchHTTPRequest(t, []byte("{invalid"), "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestInferenceDispatchController_UnknownFieldRejected(t *testing.T) {
	ctrl := newInferenceDispatchControllerForTest(t, &stubInferenceCommandDispatcher{}, &stubInferenceOperatorLister{}, 1024)

	req := inferenceDispatchHTTPRequest(t, []byte(`{"role":"MODEL_ROLE_PRIMARY","messages":[{"role":"INFERENCE_MESSAGE_ROLE_USER","parts":[{"text":"hi"}]}],"not_a_field":1}`), "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "protojson must reject unknown fields")
}

func TestInferenceDispatchController_TrailingJSONRejected(t *testing.T) {
	ctrl := newInferenceDispatchControllerForTest(t, &stubInferenceCommandDispatcher{}, &stubInferenceOperatorLister{}, 1024)

	req := inferenceDispatchHTTPRequest(t, []byte(`{"role":"MODEL_ROLE_PRIMARY","messages":[{"role":"INFERENCE_MESSAGE_ROLE_USER","parts":[{"text":"hi"}]}]}{"extra":true}`), "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "trailing content after the request object must be rejected")
}

func TestInferenceDispatchController_OversizedBodyRejected(t *testing.T) {
	ctrl := newInferenceDispatchControllerForTest(t, &stubInferenceCommandDispatcher{}, &stubInferenceOperatorLister{}, 64)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:     operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages: inferenceTextMessages(strings.Repeat("x", 256)),
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code, "oversized bodies map to 413 like the auth middleware")
}

func TestInferenceDispatchController_EmptyMessagesRejected(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}, 1024)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role: operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.False(t, dispatcher.called)
}

func TestInferenceDispatchController_InvalidRoleRejected(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}, 1024)

	for _, body := range [][]byte{
		marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{Messages: inferenceTextMessages("hi")}),
		[]byte(`{"role":99,"messages":[{"role":"INFERENCE_MESSAGE_ROLE_USER","parts":[{"text":"hi"}]}]}`),
		[]byte(`{"role":"MODEL_ROLE_UNSPECIFIED","messages":[{"role":"INFERENCE_MESSAGE_ROLE_USER","parts":[{"text":"hi"}]}]}`),
	} {
		req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
		rr := httptest.NewRecorder()
		ctrl.HandleDispatch(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "body %q must be rejected", string(body))
	}
	assert.False(t, dispatcher.called)
}

func TestInferenceDispatchController_ActingAppIDMismatchRejected(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}, 1024)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:        operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages:    inferenceTextMessages("hi"),
		ActingAppId: "other-app",
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "a body acting_app_id that differs from the authenticated app identity must be rejected")
	assert.False(t, dispatcher.called)
}

func TestInferenceDispatchController_SuccessReturnsVerifiedProtoContract(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{}
	lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)
	topP := float32(0.8)
	topK := int32(40)
	seed := int32(424242)
	parallelToolCalls := false
	contextLimit := int32(8192)
	modelDigest := strings.Repeat("a", 64)
	modelRegistry := []*operatorv1.InferenceModelVariant{{Model: "gemma3:4b", Digest: modelDigest}}
	modelRegistryDigest, err := models.ComputeInferenceModelRegistryDigest("campaign-1", modelRegistry)
	require.NoError(t, err)

	dispatchRequest := &operatorv1.InferenceDispatchRequest{
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
		Role:                 operatorv1.ModelRole_MODEL_ROLE_ASSISTANT,
		Model:                "gemma3:4b",
		Messages: []*operatorv1.InferenceMessage{
			{
				Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_SYSTEM,
				Parts: []*operatorv1.InferenceMessagePart{{
					Part: &operatorv1.InferenceMessagePart_Text{Text: "use tools precisely"},
				}},
			},
			{
				Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT,
				Parts: []*operatorv1.InferenceMessagePart{{
					Part: &operatorv1.InferenceMessagePart_ToolCall{ToolCall: &operatorv1.InferenceToolCall{
						CallId:        "call-1",
						Name:          "inspect",
						ArgumentsJson: `{"path":"target.txt"}`,
					}},
				}},
			},
			{
				Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL,
				Parts: []*operatorv1.InferenceMessagePart{{
					Part: &operatorv1.InferenceMessagePart_ToolResult{ToolResult: &operatorv1.InferenceToolResult{
						CallId:     "call-1",
						Name:       "inspect",
						ResultJson: `{"ok":true}`,
					}},
				}},
			},
		},
		Tools: []*operatorv1.InferenceToolDeclaration{{
			Name:        "inspect",
			Description: "Inspect a target",
			JsonSchema:  `{"properties":{"path":{"type":"string"}},"type":"object"}`,
		}},
		TopP:                    &topP,
		TopK:                    &topK,
		Seed:                    &seed,
		StopSequences:           []string{"END"},
		ResponseFormat:          &operatorv1.InferenceResponseFormat{MediaType: "application/json", JsonSchema: `{"type":"object"}`},
		ToolChoice:              &operatorv1.InferenceToolChoice{Mode: operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO, AllowedToolNames: []string{"inspect"}},
		ParallelToolCalls:       &parallelToolCalls,
		Thinking:                &operatorv1.InferenceThinkingControl{Mode: &operatorv1.InferenceThinkingControl_Enabled{Enabled: true}, IncludeThoughts: true},
		ContextLimit:            &contextLimit,
		ProviderAttemptId:       "provider-attempt-1",
		ModelDigest:             modelDigest,
		CampaignId:              "campaign-1",
		RunId:                   "run-1",
		AssignmentId:            "assignment-1",
		EvaluationAttemptId:     "evaluation-attempt-1",
		ScenarioId:              "scenario-1",
		ModelRegistry:           modelRegistry,
		ModelRegistryDigest:     modelRegistryDigest,
		CaseId:                  "case-1",
		InvestigationId:         "inv-1",
		TaskId:                  "task-1",
		WebSessionId:            "web-1",
		TargetOperatorSessionId: "sess-inf-1",
	}
	dispatcher.result = successDispatchResultForRequest(t, dispatchRequest)
	body := marshalInferenceDispatchRequest(t, dispatchRequest)
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	// The response must be the protocol-owned InferenceDispatchResponse in
	// proto field-name JSON form, carrying the verified receipt and result.
	var resp operatorv1.InferenceDispatchResponse
	require.NoError(t, protojson.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "tx-inference-001", resp.TransactionId)
	require.NotNil(t, resp.Result)
	require.Len(t, resp.Result.Parts, 1)
	assert.Equal(t, "generated output", resp.Result.Parts[0].GetText())
	require.NotNil(t, resp.Receipt, "the verified final signed receipt must be returned to the caller")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, resp.Receipt.Status)

	// The response must not carry a second success/error envelope shape.
	assert.NotContains(t, rr.Body.String(), "\"success\"", "the protocol contract has no ad-hoc success field")

	// Request fields propagate through the dispatch boundary.
	require.True(t, dispatcher.called)
	assert.Equal(t, "sess-inf-1", dispatcher.lastReq.TargetOperatorSessionID)
	assert.Equal(t, "user-001", dispatcher.lastReq.RequestorUserID)
	assert.Equal(t, "ensemble-app", dispatcher.lastReq.ActingAppID)
	assert.Equal(t, "case-1", dispatcher.lastReq.CaseID)
	assert.Equal(t, "inv-1", dispatcher.lastReq.InvestigationID)
	assert.Equal(t, "task-1", dispatcher.lastReq.TaskID)
	assert.Equal(t, "web-1", dispatcher.lastReq.WebSessionID)
	assert.Equal(t, dispatch.RequestDeadline, dispatcher.lastReq.Timeout, "inference dispatches carry the provider-aware request deadline")

	var forwarded operatorv1.InferenceRequested
	require.NoError(t, proto.Unmarshal(dispatcher.lastReq.Payload, &forwarded))
	expected := &operatorv1.InferenceRequested{
		Role:                 dispatchRequest.Role,
		Model:                dispatchRequest.Model,
		Messages:             dispatchRequest.Messages,
		Tools:                dispatchRequest.Tools,
		TopP:                 dispatchRequest.TopP,
		TopK:                 dispatchRequest.TopK,
		Seed:                 dispatchRequest.Seed,
		StopSequences:        dispatchRequest.StopSequences,
		ResponseFormat:       dispatchRequest.ResponseFormat,
		RequestSchemaVersion: dispatchRequest.RequestSchemaVersion,
		ToolChoice:           dispatchRequest.ToolChoice,
		ParallelToolCalls:    dispatchRequest.ParallelToolCalls,
		Thinking:             dispatchRequest.Thinking,
		ContextLimit:         dispatchRequest.ContextLimit,
		ProviderAttemptId:    dispatchRequest.ProviderAttemptId,
		ModelDigest:          dispatchRequest.ModelDigest,
		CampaignId:           dispatchRequest.CampaignId,
		RunId:                dispatchRequest.RunId,
		AssignmentId:         dispatchRequest.AssignmentId,
		EvaluationAttemptId:  dispatchRequest.EvaluationAttemptId,
		ScenarioId:           dispatchRequest.ScenarioId,
		ModelRegistry:        dispatchRequest.ModelRegistry,
		ModelRegistryDigest:  dispatchRequest.ModelRegistryDigest,
	}
	assert.True(t, proto.Equal(expected, &forwarded), "canonical protojson and governed protobuf forwarding must preserve the typed request exactly")
}

func TestInferenceDispatchController_ActingAppIDDerivedFromIdentity(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{result: successDispatchResult(t)}
	lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)

	// A body acting_app_id equal to the authenticated identity is accepted;
	// an absent one is populated from the mTLS app identity.
	for _, actingAppID := range []string{"", "ensemble-app"} {
		body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
			Role:        operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
			Messages:    inferenceTextMessages("hi"),
			ActingAppId: actingAppID,
		})
		req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
		rr := httptest.NewRecorder()
		ctrl.HandleDispatch(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, "acting_app_id %q: body: %s", actingAppID, rr.Body.String())
		assert.Equal(t, "ensemble-app", dispatcher.lastReq.ActingAppID)
	}
}

func TestInferenceDispatchController_ErrorStatusMapping(t *testing.T) {
	tests := []struct {
		name        string
		dispatchErr error
		wantStatus  int
	}{
		{name: "no inference operator", dispatchErr: constants.ErrInferenceOperatorNotFound, wantStatus: http.StatusNotFound},
		{name: "ambiguous inference operators", dispatchErr: constants.ErrInferenceOperatorAmbiguous, wantStatus: http.StatusConflict},
		{name: "target not inference capable", dispatchErr: constants.ErrInferenceOperatorNotCapable, wantStatus: http.StatusUnprocessableEntity},
		{name: "model override denied", dispatchErr: constants.ErrInferenceModelOverrideDenied, wantStatus: http.StatusForbidden},
		{name: "model registry invalid", dispatchErr: constants.ErrInferenceModelRegistryInvalid, wantStatus: http.StatusForbidden},
		{name: "campaign binding invalid", dispatchErr: constants.ErrInferenceCampaignBindingInvalid, wantStatus: http.StatusForbidden},
		{name: "governance rejected receipt", dispatchErr: constants.ErrInferenceGovernanceRejected, wantStatus: http.StatusForbidden},
		{name: "l1 doctrine rejection", dispatchErr: constants.ErrTxL1ValidationFailed, wantStatus: http.StatusForbidden},
		{name: "l3 proof unmintable under posture", dispatchErr: constants.ErrTxL3ProofUnmintable, wantStatus: http.StatusForbidden},
		{name: "zero delivery", dispatchErr: constants.ErrDispatchNoDelivery, wantStatus: http.StatusServiceUnavailable},
		{name: "receipt execution failed", dispatchErr: constants.ErrInferenceReceiptFailed, wantStatus: http.StatusBadGateway},
		{name: "receipt verification failed", dispatchErr: constants.ErrInferenceReceiptVerify, wantStatus: http.StatusBadGateway},
		{name: "result digest mismatch", dispatchErr: constants.ErrInferenceResultDigestMismatch, wantStatus: http.StatusBadGateway},
		{name: "backend timeout", dispatchErr: constants.ErrInferenceBackendTimeout, wantStatus: http.StatusBadGateway},
		{name: "model not found", dispatchErr: constants.ErrInferenceModelNotFound, wantStatus: http.StatusBadGateway},
		{name: "provider response invalid", dispatchErr: constants.ErrInferenceProviderResponseInvalid, wantStatus: http.StatusBadGateway},
		{name: "unknown remote outcome", dispatchErr: constants.ErrInferenceOutcomeUnknown, wantStatus: http.StatusGatewayTimeout},
		{name: "caller deadline exceeded", dispatchErr: context.DeadlineExceeded, wantStatus: http.StatusGatewayTimeout},
		{name: "unexpected internal error", dispatchErr: errors.New("sql: connection refused at internal/db/store.go:123"), wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher := &stubInferenceCommandDispatcher{err: tt.dispatchErr}
			lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
			ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)

			body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
				Role:     operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Messages: inferenceTextMessages("hi"),
			})
			req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
			rr := httptest.NewRecorder()
			ctrl.HandleDispatch(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code, "body: %s", rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "internal/db/store.go",
				"internal error chains must never reach the client")
		})
	}
}

func TestInferenceDispatchController_InternalErrorIsPublicSafe(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{err: errors.New("pq: password authentication failed for user \"gateway\"")}
	lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:     operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages: inferenceTextMessages("hi"),
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NotContains(t, rr.Body.String(), "password authentication",
		"the client-visible error must be a generic code, not the internal error chain")
	assert.Contains(t, rr.Body.String(), constants.ErrInternal.Error())
}

func TestInferenceDispatchController_StreamingReturnsNDJSONProgressAndCompletion(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{result: successDispatchResult(t)}
	lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:              operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages:          inferenceTextMessages("hi"),
		ProviderAttemptId: "provider-attempt-1",
		Stream:            true,
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-ndjson", rr.Header().Get("Content-Type"))

	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	require.Len(t, lines, 2)

	var progressFrame operatorv1.InferenceDispatchStreamFrame
	require.NoError(t, protojson.Unmarshal([]byte(lines[0]), &progressFrame))
	require.NotNil(t, progressFrame.GetProgress())
	assert.Equal(t, uint32(1), progressFrame.GetProgress().GetSequence())
	assert.Equal(t, "partial", progressFrame.GetProgress().GetParts()[0].GetText())

	var completionFrame operatorv1.InferenceDispatchStreamFrame
	require.NoError(t, protojson.Unmarshal([]byte(lines[1]), &completionFrame))
	require.NotNil(t, completionFrame.GetCompletion())
	assert.Equal(t, "tx-inference-001", completionFrame.GetCompletion().GetTransactionId())
	assert.True(t, dispatcher.lastReq.OnInferenceProgress != nil)

	var forwarded operatorv1.InferenceRequested
	require.NoError(t, proto.Unmarshal(dispatcher.lastReq.Payload, &forwarded))
	assert.True(t, forwarded.GetStream())
}

func TestInferenceDispatchController_StreamingDispatchFailureWritesFailureFrame(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{err: constants.ErrInferenceProgressBackpressure}
	lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:              operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages:          inferenceTextMessages("hi"),
		ProviderAttemptId: "provider-attempt-1",
		Stream:            true,
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-ndjson", rr.Header().Get("Content-Type"))

	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 2)

	var progressFrame operatorv1.InferenceDispatchStreamFrame
	require.NoError(t, protojson.Unmarshal([]byte(lines[0]), &progressFrame))
	require.NotNil(t, progressFrame.GetProgress())

	var failureFrame operatorv1.InferenceDispatchStreamFrame
	require.NoError(t, protojson.Unmarshal([]byte(lines[len(lines)-1]), &failureFrame))
	require.NotNil(t, failureFrame.GetFailure())
	assert.Equal(t, constants.ErrInferenceProgressBackpressure.Error(), failureFrame.GetFailure().GetReason())
}

type failingFlushWriter struct {
	http.ResponseWriter
	failOnWrite bool
}

func (w *failingFlushWriter) Write(p []byte) (int, error) {
	if w.failOnWrite {
		return 0, errors.New("client disconnected")
	}
	return w.ResponseWriter.Write(p)
}

func (w *failingFlushWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func TestInferenceDispatchController_StreamingWriteFailureIsCallerDisconnect(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{result: successDispatchResult(t)}
	lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:              operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages:          inferenceTextMessages("hi"),
		ProviderAttemptId: "provider-attempt-1",
		Stream:            true,
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(&failingFlushWriter{ResponseWriter: rr, failOnWrite: true}, req)

	require.True(t, dispatcher.called)
	assert.ErrorIs(t, dispatcher.dispatchErr, constants.ErrInferenceCallerDisconnected)
}

func TestInferenceDispatchController_ContextCanceledWritesNothing(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{err: context.Canceled}
	lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:     operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Messages: inferenceTextMessages("hi"),
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "recorder default; the handler must not write an error for a canceled client")
	assert.Empty(t, rr.Body.String(), "no response is written when the caller is gone")
}
