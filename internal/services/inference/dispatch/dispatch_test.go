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
	payload, err := proto.Marshal(&operatorv1.InferenceResult{
		Parts: []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "ok"}}},
		Model: "gemma3:4b",
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
		Role:            models.InferenceModelRolePrimary,
		Messages:        baseMessages(),
		RequestorUserID: "user-1",
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

func TestDispatchInference_SuccessReturnsResultAndReceipt(t *testing.T) {
	receipt := &operatorv1.ActionReceipt{TransactionId: "tx-9", Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED}
	resultPayload, err := proto.Marshal(&operatorv1.InferenceResult{
		Parts: []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "answer"}}},
		Model: "gemma3:4b",
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
	req.Model = "qwen3:1.5b"
	req.Temperature = 0.2
	req.MaxTokens = 64
	req.KeepAlive = "5m"
	req.Tools = []*operatorv1.InferenceToolDeclaration{{
		Name:        "inspect",
		Description: "Inspect a target",
		JsonSchema:  `{"properties":{"path":{"type":"string"}},"type":"object"}`,
	}}
	_, err := svc.DispatchInference(context.Background(), req)
	require.NoError(t, err)

	infReq := &operatorv1.InferenceRequested{}
	require.NoError(t, proto.Unmarshal(dispatcher.lastReq.Payload, infReq))
	expected := &operatorv1.InferenceRequested{
		Role:        operatorv1.ModelRole_MODEL_ROLE_LITE,
		Model:       "qwen3:1.5b",
		Temperature: 0.2,
		MaxTokens:   64,
		KeepAlive:   "5m",
		Messages:    baseMessages(),
		Tools:       req.Tools,
	}
	assert.True(t, proto.Equal(expected, infReq), "forwarded governed payload must preserve every ordered message and tool field")
}
