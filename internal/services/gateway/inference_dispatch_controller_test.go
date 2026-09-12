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
	result  *dispatch.CommandDispatchResult
	err     error
	lastReq dispatch.CommandDispatchRequest
	called  bool
}

func (s *stubInferenceCommandDispatcher) Dispatch(_ context.Context, req dispatch.CommandDispatchRequest) (*dispatch.CommandDispatchResult, error) {
	s.called = true
	s.lastReq = req
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
	body, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(req)
	require.NoError(t, err)
	return body
}

func successDispatchResult(t *testing.T) *dispatch.CommandDispatchResult {
	t.Helper()
	result := &operatorv1.InferenceResult{
		Text:             "generated output",
		PromptTokens:     7,
		CompletionTokens: 11,
		TotalTokens:      18,
		FinishReason:     "stop",
		Model:            "gemma3:4b",
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
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "hello",
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
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "hello",
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

	req := inferenceDispatchHTTPRequest(t, []byte(`{"role":"MODEL_ROLE_PRIMARY","prompt":"hi","not_a_field":1}`), "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "protojson must reject unknown fields")
}

func TestInferenceDispatchController_TrailingJSONRejected(t *testing.T) {
	ctrl := newInferenceDispatchControllerForTest(t, &stubInferenceCommandDispatcher{}, &stubInferenceOperatorLister{}, 1024)

	req := inferenceDispatchHTTPRequest(t, []byte(`{"role":"MODEL_ROLE_PRIMARY","prompt":"hi"}{"extra":true}`), "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "trailing content after the request object must be rejected")
}

func TestInferenceDispatchController_OversizedBodyRejected(t *testing.T) {
	ctrl := newInferenceDispatchControllerForTest(t, &stubInferenceCommandDispatcher{}, &stubInferenceOperatorLister{}, 64)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: strings.Repeat("x", 256),
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code, "oversized bodies map to 413 like the auth middleware")
}

func TestInferenceDispatchController_EmptyPromptRejected(t *testing.T) {
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
		marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{Prompt: "hi"}),
		[]byte(`{"role":99,"prompt":"hi"}`),
		[]byte(`{"role":"MODEL_ROLE_UNSPECIFIED","prompt":"hi"}`),
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
		Prompt:      "hi",
		ActingAppId: "other-app",
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "a body acting_app_id that differs from the authenticated app identity must be rejected")
	assert.False(t, dispatcher.called)
}

func TestInferenceDispatchController_SuccessReturnsVerifiedProtoContract(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{result: successDispatchResult(t)}
	lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:                  operatorv1.ModelRole_MODEL_ROLE_ASSISTANT,
		Prompt:                "summarize this",
		Model:                 "gemma3:4b",
		CaseId:                "case-1",
		InvestigationId:       "inv-1",
		TaskId:                "task-1",
		WebSessionId:          "web-1",
		TargetOperatorSessionId: "sess-inf-1",
	})
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
	assert.Equal(t, "generated output", resp.Result.Text)
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
	assert.Equal(t, operatorv1.ModelRole_MODEL_ROLE_ASSISTANT, forwarded.Role)
	assert.Equal(t, "summarize this", forwarded.Prompt)
	assert.Equal(t, "gemma3:4b", forwarded.Model)
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
			Prompt:      "hi",
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
		name       string
		dispatchErr error
		wantStatus int
	}{
		{name: "no inference operator", dispatchErr: constants.ErrInferenceOperatorNotFound, wantStatus: http.StatusNotFound},
		{name: "ambiguous inference operators", dispatchErr: constants.ErrInferenceOperatorAmbiguous, wantStatus: http.StatusConflict},
		{name: "target not inference capable", dispatchErr: constants.ErrInferenceOperatorNotCapable, wantStatus: http.StatusUnprocessableEntity},
		{name: "model override denied", dispatchErr: constants.ErrInferenceModelOverrideDenied, wantStatus: http.StatusForbidden},
		{name: "governance rejected receipt", dispatchErr: constants.ErrInferenceGovernanceRejected, wantStatus: http.StatusForbidden},
		{name: "l1 doctrine rejection", dispatchErr: constants.ErrTxL1ValidationFailed, wantStatus: http.StatusForbidden},
		{name: "l3 proof unmintable under posture", dispatchErr: constants.ErrTxL3ProofUnmintable, wantStatus: http.StatusForbidden},
		{name: "zero delivery", dispatchErr: constants.ErrDispatchNoDelivery, wantStatus: http.StatusServiceUnavailable},
		{name: "receipt execution failed", dispatchErr: constants.ErrInferenceReceiptFailed, wantStatus: http.StatusBadGateway},
		{name: "receipt verification failed", dispatchErr: constants.ErrInferenceReceiptVerify, wantStatus: http.StatusBadGateway},
		{name: "result digest mismatch", dispatchErr: constants.ErrInferenceResultDigestMismatch, wantStatus: http.StatusBadGateway},
		{name: "backend timeout", dispatchErr: constants.ErrInferenceBackendTimeout, wantStatus: http.StatusBadGateway},
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
				Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
				Prompt: "hi",
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
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "hi",
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NotContains(t, rr.Body.String(), "password authentication",
		"the client-visible error must be a generic code, not the internal error chain")
	assert.Contains(t, rr.Body.String(), constants.ErrInternal.Error())
}

func TestInferenceDispatchController_ContextCanceledWritesNothing(t *testing.T) {
	dispatcher := &stubInferenceCommandDispatcher{err: context.Canceled}
	lister := &stubInferenceOperatorLister{ops: []models.OperatorDocumentGo{inferenceCapableOperator("sess-inf-1")}}
	ctrl := newInferenceDispatchControllerForTest(t, dispatcher, lister, 4096)

	body := marshalInferenceDispatchRequest(t, &operatorv1.InferenceDispatchRequest{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "hi",
	})
	req := inferenceDispatchHTTPRequest(t, body, "ensemble-app", "user-001")
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "recorder default; the handler must not write an error for a canceled client")
	assert.Empty(t, rr.Body.String(), "no response is written when the caller is gone")
}
