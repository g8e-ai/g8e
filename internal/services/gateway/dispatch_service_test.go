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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// --- stubs for Tier 1 unit tests ---

// stubStateRootProvider implements governance.StateRootProvider for unit tests.
type stubStateRootProvider struct {
	root string
	err  error
}

func (s *stubStateRootProvider) GetCurrentStateRoot() (string, error) {
	return s.root, s.err
}

// stubOperatorSessionValidator implements operatorSessionValidator for unit tests.
type stubOperatorSessionValidator struct {
	op  *models.OperatorDocumentGo
	err error
}

func (s *stubOperatorSessionValidator) ValidateOperatorSession(_ string) (*models.OperatorDocumentGo, error) {
	return s.op, s.err
}

// --- OperatorCommandRequest.Validate tests ---

func TestOperatorCommandRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     OperatorCommandRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: OperatorCommandRequest{
				TargetOperatorSessionID: "session-123",
				EventType: string(constants.Event.Operator.FsRead.Requested),
				Payload:                 []byte("payload"),
			},
			wantErr: nil,
		},
		{
			name: "missing operator session id",
			req: OperatorCommandRequest{
				EventType: string(constants.Event.Operator.FsRead.Requested),
				Payload:    []byte("payload"),
			},
			wantErr: constants.ErrGatewayOperatorSessionIDRequired,
		},
		{
			name: "missing action type",
			req: OperatorCommandRequest{
				TargetOperatorSessionID: "session-123",
				Payload:                 []byte("payload"),
			},
			wantErr: constants.ErrTxUnknownEventType,
		},
		{
			name: "missing payload",
			req: OperatorCommandRequest{
				TargetOperatorSessionID: "session-123",
				EventType: string(constants.Event.Operator.FsRead.Requested),
			},
			wantErr: constants.ErrTxPayloadMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr), "expected %v, got %v", tt.wantErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- DispatchResult.ToResponse tests ---

func TestDispatchResult_ToResponse(t *testing.T) {
	t.Run("with result envelope", func(t *testing.T) {
		env := &commonv1.GovernanceEnvelope{
			EventType:  string(constants.Event.Operator.FsRead.Completed),
			ActionType: string(constants.ActionTypeFsRead),
			Payload:    []byte("result-payload-bytes"),
		}
		result := &DispatchResult{
			TransactionID:  "tx-abc-123",
			ResultEnvelope: env,
		}
		resp := result.ToResponse()
		assert.True(t, resp.Success)
		assert.Equal(t, "tx-abc-123", resp.TransactionID)
		assert.Equal(t, string(constants.Event.Operator.FsRead.Completed), resp.EventType)
		assert.Equal(t, string(constants.ActionTypeFsRead), resp.ActionType)
		assert.Equal(t, []byte("result-payload-bytes"), resp.ResultPayload)
		assert.Empty(t, resp.Error)
	})

	t.Run("nil result envelope", func(t *testing.T) {
		result := &DispatchResult{
			TransactionID: "tx-abc-123",
		}
		resp := result.ToResponse()
		assert.True(t, resp.Success)
		assert.Equal(t, "tx-abc-123", resp.TransactionID)
		assert.Empty(t, resp.EventType)
		assert.Empty(t, resp.ActionType)
		assert.Empty(t, resp.ResultPayload)
	})
}

// fsReadPayloadBytes builds a valid proto-marshaled FsReadRequested payload
// for dispatch tests that need a typed payload the builder can decode.
func fsReadPayloadBytes(t *testing.T) []byte {
	t.Helper()
	b, err := proto.Marshal(&operatorv1.FsReadRequested{Path: "/etc/hostname", ExecutionId: "exec-1"})
	require.NoError(t, err)
	return b
}

// --- DispatchService.Dispatch tests ---
//
// These tests use a real GatewayWebSocketHandler (in-process broker, no DB)
// with stub StateRootProvider and operatorSessionValidator. The "operator" is
// simulated by registering an in-process handler on the cmd channel that
// publishes a result envelope on the results channel.

func newTestDispatchService(t *testing.T, stateRoot string, op *models.OperatorDocumentGo) (*DispatchService, *GatewayWebSocketHandler) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)
	svc := NewDispatchService(
		logger,
		broker,
		&stubStateRootProvider{root: stateRoot},
		&stubOperatorSessionValidator{op: op},
		"doctrine",
		governance.NewL1Doctrine(),
		nil, // no L2 deliberator under doctrine posture
		nil, // no receipt signer store; inference dispatch tests wire their own
	)
	return svc, broker
}

func TestDispatchService_Dispatch_Success(t *testing.T) {
	op := &models.OperatorDocumentGo{
		ID:                "op-001",
		OperatorSessionID: "sess-001",
	}
	svc, broker := newTestDispatchService(t, "root-abc", op)

	// Simulate the operator: register a handler on the cmd channel that
	// unmarshals the command, builds a result envelope with the same Id,
	// and publishes it on the results channel.
	cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)
	resultsChannel := pubsub.ResultsChannel(op.ID, op.OperatorSessionID)

	unregisterOperator := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		cmdEnv := &commonv1.GovernanceEnvelope{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, cmdEnv); err != nil {
			t.Errorf("operator: unmarshal command: %v", err)
			return
		}
		// Verify the command envelope is well-formed.
		assert.Equal(t, "root-abc", cmdEnv.StateMerkleRoot, "envelope must carry gateway state root")
		assert.Equal(t, op.ID, cmdEnv.OperatorId, "envelope must target the correct operator")
		assert.Equal(t, op.OperatorSessionID, cmdEnv.OperatorSessionId, "envelope must target the correct session")
		assert.NotEmpty(t, cmdEnv.Nonce, "envelope must have a nonce")
		assert.NotEmpty(t, cmdEnv.Id, "envelope must have an Id (transaction hash)")
		assert.Equal(t, cmdEnv.Id, cmdEnv.TransactionHash, "Id must equal TransactionHash")

		// Build and publish the result envelope.
		resultEnv := &commonv1.GovernanceEnvelope{
			Id:         cmdEnv.Id,
			EventType:  cmdEnv.EventType,
			ActionType: cmdEnv.ActionType,
			Timestamp:  timestamppb.Now(),
		}
		resultWire, err := protojson.Marshal(resultEnv)
		require.NoError(t, err)
		broker.Publish(resultsChannel, resultWire)
	})
	defer unregisterOperator()

	result, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		EventType: string(constants.Event.Operator.FsRead.Requested),
		Payload:                 fsReadPayloadBytes(t),
		TargetResource:          "/etc/hostname",
		RequestorUserID:         "user-001",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.TransactionID)
	require.NotNil(t, result.ResultEnvelope)
	assert.Equal(t, result.TransactionID, result.ResultEnvelope.Id)
}

func TestDispatchService_Dispatch_InvalidOperatorSession(t *testing.T) {
	svc, _ := newTestDispatchService(t, "root-abc", nil)

	// Override the validator to return an error.
	svc.auth = &stubOperatorSessionValidator{err: fmt.Errorf("session not found")}

	_, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: "invalid-session",
		EventType: string(constants.Event.Operator.FsRead.Requested),
		Payload:                 []byte("payload"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dispatch: validate operator session")
}

func TestDispatchService_Dispatch_StateRootError(t *testing.T) {
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)
	svc := NewDispatchService(
		logger,
		broker,
		&stubStateRootProvider{err: fmt.Errorf("state root unavailable")},
		&stubOperatorSessionValidator{op: op},
		"doctrine",
		governance.NewL1Doctrine(),
		nil,
		nil,
	)

	_, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		EventType: string(constants.Event.Operator.FsRead.Requested),
		Payload:                 []byte("payload"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dispatch: get state root")
}

func TestDispatchService_Dispatch_ZeroDeliveryFailsClosed(t *testing.T) {
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	svc, _ := newTestDispatchService(t, "root-abc", op)

	// No operator handler registered — the publish delivers to zero
	// subscribers, which is a terminal transport failure.
	_, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		EventType: string(constants.Event.Operator.FsRead.Requested),
		Payload:                 fsReadPayloadBytes(t),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDispatchNoDelivery)
}

func TestDispatchService_Dispatch_TimeoutNoResult(t *testing.T) {
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	svc, broker := newTestDispatchService(t, "root-abc", op)

	// Operator handler is registered (so delivery succeeds) but never
	// publishes a result.
	unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, _ []byte) {})
	defer unreg()

	// Use a context with a short timeout so the test doesn't wait 30s.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := svc.Dispatch(ctx, DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		EventType: string(constants.Event.Operator.FsRead.Requested),
		Payload:                 fsReadPayloadBytes(t),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestDispatchService_Dispatch_RequestTimeoutOverridesDefault(t *testing.T) {
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	svc, broker := newTestDispatchService(t, "root-abc", op)

	unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, _ []byte) {})
	defer unreg()

	start := time.Now()
	_, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		EventType: string(constants.Event.Operator.FsRead.Requested),
		Payload:                 fsReadPayloadBytes(t),
		Timeout:                 100 * time.Millisecond,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDispatchResultTimeout)
	assert.Less(t, time.Since(start), 5*time.Second, "request timeout must override the default dispatch timeout")
}

func TestDispatchService_Dispatch_CallerCancelReturnsCtxErr(t *testing.T) {
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	svc, broker := newTestDispatchService(t, "root-abc", op)

	unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, _ []byte) {})
	defer unreg()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := svc.Dispatch(ctx, DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		EventType: string(constants.Event.Operator.FsRead.Requested),
		Payload:                 fsReadPayloadBytes(t),
		Timeout:                 30 * time.Second,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceCanceled)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, constants.ErrDispatchResultTimeout,
		"caller cancellation must not be reported as a dispatch timeout")
}

func TestDispatchService_Dispatch_StreamingProgressOverflowFailsClosed(t *testing.T) {
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	svc, broker := newTestDispatchService(t, "root-abc", op)

	unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, data []byte) {
		cmdEnv := &commonv1.GovernanceEnvelope{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, cmdEnv); err != nil {
			return
		}
		resultsChannel := pubsub.ResultsChannel(op.ID, op.OperatorSessionID)
		for i := 0; i < InferenceProgressResultBuffer+1; i++ {
			progress, err := proto.Marshal(&operatorv1.InferenceProgressEvent{
				ProviderAttemptId: "attempt-1",
				Sequence:          uint32(i + 1),
				Parts: []*operatorv1.InferenceResponsePart{
					{Part: &operatorv1.InferenceResponsePart_Text{Text: "x"}},
				},
			})
			if err != nil {
				return
			}
			wire, err := protojson.Marshal(&commonv1.GovernanceEnvelope{
				Id:        cmdEnv.Id,
				EventType: string(constants.Event.Operator.Inference.ProgressUpdated),
				Payload:   progress,
			})
			if err != nil {
				return
			}
			broker.Publish(resultsChannel, wire)
		}
	})
	defer unreg()

	_, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: op.OperatorSessionID,
		EventType: string(constants.Event.Operator.Inference.Requested),
		Payload:                 inferencePayload(t),
		Timeout:                 2 * time.Second,
		OnInferenceProgress:     func(*operatorv1.InferenceProgressEvent) error { return nil },
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProgressBackpressure)
	assert.NotErrorIs(t, err, constants.ErrDispatchResultTimeout)
}

// resultHandlerCount returns the number of registered in-process handlers on
// the operator's results channel, for leak assertions.
func resultHandlerCount(b *GatewayWebSocketHandler, op *models.OperatorDocumentGo) int {
	b.handlersMu.RLock()
	defer b.handlersMu.RUnlock()
	return len(b.handlers[pubsub.ResultsChannel(op.ID, op.OperatorSessionID)])
}

func TestDispatchService_Dispatch_ResultHandlerRemovedAfterReturn(t *testing.T) {
	newOp := func() *models.OperatorDocumentGo {
		return &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	}

	publishResult := func(broker *GatewayWebSocketHandler, op *models.OperatorDocumentGo) func(string, []byte) {
		return func(_ string, data []byte) {
			cmdEnv := &commonv1.GovernanceEnvelope{}
			if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, cmdEnv); err != nil {
				return
			}
			resultEnv := &commonv1.GovernanceEnvelope{Id: cmdEnv.Id, EventType: cmdEnv.EventType, ActionType: cmdEnv.ActionType}
			wire, err := protojson.Marshal(resultEnv)
			if err != nil {
				return
			}
			broker.Publish(pubsub.ResultsChannel(op.ID, op.OperatorSessionID), wire)
		}
	}

	t.Run("after success", func(t *testing.T) {
		op := newOp()
		svc, broker := newTestDispatchService(t, "root-abc", op)
		unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), publishResult(broker, op))
		defer unreg()

		_, err := svc.Dispatch(context.Background(), DispatchRequest{
			TargetOperatorSessionID: op.OperatorSessionID,
			EventType: string(constants.Event.Operator.FsRead.Requested),
			Payload:                 fsReadPayloadBytes(t),
		})
		require.NoError(t, err)
		assert.Equal(t, 0, resultHandlerCount(broker, op), "result handler must be unregistered after success")
	})

	t.Run("after timeout", func(t *testing.T) {
		op := newOp()
		svc, broker := newTestDispatchService(t, "root-abc", op)
		unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, _ []byte) {})
		defer unreg()

		_, err := svc.Dispatch(context.Background(), DispatchRequest{
			TargetOperatorSessionID: op.OperatorSessionID,
			EventType: string(constants.Event.Operator.FsRead.Requested),
			Payload:                 fsReadPayloadBytes(t),
			Timeout:                 50 * time.Millisecond,
		})
		require.Error(t, err)
		assert.Equal(t, 0, resultHandlerCount(broker, op), "result handler must be unregistered after timeout")
	})

	t.Run("after caller cancellation", func(t *testing.T) {
		op := newOp()
		svc, broker := newTestDispatchService(t, "root-abc", op)
		unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, _ []byte) {})
		defer unreg()

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		_, err := svc.Dispatch(ctx, DispatchRequest{
			TargetOperatorSessionID: op.OperatorSessionID,
			EventType: string(constants.Event.Operator.FsRead.Requested),
			Payload:                 fsReadPayloadBytes(t),
		})
		require.Error(t, err)
		assert.Equal(t, 0, resultHandlerCount(broker, op), "result handler must be unregistered after cancellation")
	})

	t.Run("after zero delivery", func(t *testing.T) {
		op := newOp()
		svc, broker := newTestDispatchService(t, "root-abc", op)

		_, err := svc.Dispatch(context.Background(), DispatchRequest{
			TargetOperatorSessionID: op.OperatorSessionID,
			EventType: string(constants.Event.Operator.FsRead.Requested),
			Payload:                 fsReadPayloadBytes(t),
		})
		require.Error(t, err)
		assert.Equal(t, 0, resultHandlerCount(broker, op), "result handler must be unregistered after zero delivery")
	})
}

func TestDispatchService_Dispatch_LateResultDiscarded(t *testing.T) {
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	svc, broker := newTestDispatchService(t, "root-abc", op)

	var cmdEnvID string
	unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, data []byte) {
		cmdEnv := &commonv1.GovernanceEnvelope{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, cmdEnv); err == nil {
			cmdEnvID = cmdEnv.Id
		}
	})
	defer unreg()

	_, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: op.OperatorSessionID,
		EventType: string(constants.Event.Operator.FsRead.Requested),
		Payload:                 fsReadPayloadBytes(t),
		Timeout:                 50 * time.Millisecond,
	})
	require.Error(t, err)
	require.NotEmpty(t, cmdEnvID)

	// The result arrives after the dispatch deadline: the handler is gone
	// and the publication is a no-op.
	resultEnv := &commonv1.GovernanceEnvelope{Id: cmdEnvID}
	wire, merr := protojson.Marshal(resultEnv)
	require.NoError(t, merr)
	broker.Publish(pubsub.ResultsChannel(op.ID, op.OperatorSessionID), wire)
	assert.Equal(t, 0, resultHandlerCount(broker, op))
}

func TestDispatchService_Dispatch_DuplicateResultDropped(t *testing.T) {
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	svc, broker := newTestDispatchService(t, "root-abc", op)

	unreg := broker.RegisterHandler(pubsub.CmdChannel(op.ID, op.OperatorSessionID), func(_ string, data []byte) {
		cmdEnv := &commonv1.GovernanceEnvelope{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, cmdEnv); err != nil {
			return
		}
		// Publish two result envelopes for the same transaction in one
		// handler invocation: the first fills the buffered result channel,
		// the second must be dropped, not block or corrupt the outcome.
		for i := 0; i < 2; i++ {
			resultEnv := &commonv1.GovernanceEnvelope{
				Id:         cmdEnv.Id,
				EventType:  cmdEnv.EventType,
				ActionType: cmdEnv.ActionType,
				Payload:    []byte{byte('a' + i)},
			}
			wire, err := protojson.Marshal(resultEnv)
			if err != nil {
				return
			}
			broker.Publish(pubsub.ResultsChannel(op.ID, op.OperatorSessionID), wire)
		}
	})
	defer unreg()

	result, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: op.OperatorSessionID,
		EventType: string(constants.Event.Operator.FsRead.Requested),
		Payload:                 fsReadPayloadBytes(t),
		Timeout:                 5 * time.Second,
	})
	require.NoError(t, err)
	require.NotNil(t, result.ResultEnvelope)
	assert.Equal(t, []byte("a"), result.ResultEnvelope.Payload, "the first correlated result wins; the duplicate is dropped")
	assert.Equal(t, 0, resultHandlerCount(broker, op))
}

// --- DispatchController.HandleDispatch tests ---

func TestDispatchController_HandleDispatch_MethodNotAllowed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	resp := response.NewWriter(logger)
	ctrl := newDispatchController(DispatchControllerDeps{
		DispatchSvc: nil, // not reached
		Responder:   resp,
		Logger:      logger,
	})

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.OperatorsCommands, nil)
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestDispatchController_HandleDispatch_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	resp := response.NewWriter(logger)
	ctrl := newDispatchController(DispatchControllerDeps{
		DispatchSvc: nil,
		Responder:   resp,
		Logger:      logger,
	})

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsCommands, bytes.NewReader([]byte("{invalid")))
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestDispatchController_HandleDispatch_ValidationFails(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	resp := response.NewWriter(logger)
	ctrl := newDispatchController(DispatchControllerDeps{
		DispatchSvc: nil,
		Responder:   resp,
		Logger:      logger,
	})

	// Missing target_operator_session_id and event_type.
	body := `{"payload":"dGVzdA=="}`
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsCommands, bytes.NewReader([]byte(body)))
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestDispatchController_HandleDispatch_WitnessCommandRejectedAsUnprocessableEntity(t *testing.T) {
	observer := &models.OperatorDocumentGo{
		ID:                "observer-op",
		OperatorSessionID: "observer-session",
		RuntimeConfig:     &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true},
	}
	dispatchSvc, _ := newTestDispatchService(t, "root-abc", observer)
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctrl := newDispatchController(DispatchControllerDeps{
		DispatchSvc: dispatchSvc,
		Responder:   response.NewWriter(logger),
		Logger:      logger,
	})

	payload, err := proto.Marshal(&operatorv1.CommandRequested{
		Command:     "ollama stop qwen3:0.6b",
		ExecutionId: "exec-1",
	})
	require.NoError(t, err)
	body, err := json.Marshal(OperatorCommandRequest{
		TargetOperatorSessionID: observer.OperatorSessionID,
		EventType: string(constants.Event.Operator.Command.Requested),
		Payload:                 payload,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsCommands, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrWitnessCommandNotCapable.Error())
}

func TestOperatorCommandResultHelpers(t *testing.T) {
	t.Run("terminal command events", func(t *testing.T) {
		assert.True(t, isOperatorCommandTerminalResult(&commonv1.GovernanceEnvelope{
			EventType: string(constants.Event.Operator.Command.Completed),
		}))
		assert.False(t, isOperatorCommandTerminalResult(&commonv1.GovernanceEnvelope{
			EventType: string(constants.Event.Operator.Command.StatusUpdated.Running),
		}))
	})

	t.Run("payload from envelope bytes", func(t *testing.T) {
		cmdResult := &operatorv1.CommandResult{Stdout: "hello\n", ReturnCode: 0}
		wire, err := proto.Marshal(cmdResult)
		require.NoError(t, err)
		assert.Equal(t, wire, operatorCommandResultPayload(&commonv1.GovernanceEnvelope{Payload: wire}))
	})

	t.Run("ToResponse uses command result payload for completed events", func(t *testing.T) {
		cmdResult := &operatorv1.CommandResult{Stdout: "hello\n", ReturnCode: 0}
		wire, err := proto.Marshal(cmdResult)
		require.NoError(t, err)
		resp := (&DispatchResult{
			TransactionID: "tx-1",
			ResultEnvelope: &commonv1.GovernanceEnvelope{
				EventType:  string(constants.Event.Operator.Command.Completed),
				ActionType: string(constants.ActionTypeExecuteBash),
				Payload:    wire,
			},
		}).ToResponse()
		assert.Equal(t, wire, resp.ResultPayload)
	})
}

func TestDispatchService_Dispatch_ExecuteBash_WaitsForTerminalResult(t *testing.T) {
	op := &models.OperatorDocumentGo{
		ID:                "op-001",
		OperatorSessionID: "sess-001",
	}
	svc, broker := newTestDispatchService(t, "root-abc", op)

	cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)
	resultsChannel := pubsub.ResultsChannel(op.ID, op.OperatorSessionID)

	unregisterOperator := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		cmdEnv := &commonv1.GovernanceEnvelope{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, cmdEnv); err != nil {
			t.Errorf("operator: unmarshal command: %v", err)
			return
		}

		statusEnv := &commonv1.GovernanceEnvelope{
			Id:         cmdEnv.Id,
			EventType:  string(constants.Event.Operator.Command.StatusUpdated.Running),
			ActionType: string(constants.ActionTypeExecuteBash),
			Timestamp:  timestamppb.Now(),
		}
		statusWire, err := protojson.Marshal(statusEnv)
		require.NoError(t, err)
		broker.Publish(resultsChannel, statusWire)

		resultPayload, err := proto.Marshal(&operatorv1.CommandResult{
			Stdout:     "hello\n",
			ReturnCode: 0,
		})
		require.NoError(t, err)
		completedEnv := &commonv1.GovernanceEnvelope{
			Id:         cmdEnv.Id,
			EventType:  string(constants.Event.Operator.Command.Completed),
			ActionType: "EXECUTE_BASH_RESULT",
			Payload:    resultPayload,
			Timestamp:  timestamppb.Now(),
		}
		completedWire, err := protojson.Marshal(completedEnv)
		require.NoError(t, err)
		broker.Publish(resultsChannel, completedWire)
	})
	defer unregisterOperator()

	execPayload, err := proto.Marshal(&operatorv1.CommandRequested{
		Command:     "echo hello",
		ExecutionId: "exec-1",
	})
	require.NoError(t, err)

	result, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		EventType: string(constants.Event.Operator.Command.Requested),
		Payload:                 execPayload,
		TargetResource:          "cli",
		RequestorUserID:         "user-001",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.ResultEnvelope)
	assert.Equal(t, string(constants.Event.Operator.Command.Completed), result.ResultEnvelope.EventType)

	decoded := &operatorv1.CommandResult{}
	require.NoError(t, proto.Unmarshal(result.ToResponse().ResultPayload, decoded))
	assert.Equal(t, "hello\n", decoded.Stdout)
}

func TestDecodeInferenceProgressEnvelope(t *testing.T) {
	t.Parallel()
	progress := &operatorv1.InferenceProgressEvent{
		ProviderAttemptId: "attempt-1",
		Sequence:          2,
	}
	payload, err := proto.Marshal(progress)
	require.NoError(t, err)

	decoded, ok := decodeInferenceProgressEnvelope(&commonv1.GovernanceEnvelope{
		EventType: string(constants.Event.Operator.Inference.ProgressUpdated),
		Payload:   payload,
	})
	require.True(t, ok)
	assert.Equal(t, "attempt-1", decoded.GetProviderAttemptId())
	assert.Equal(t, uint32(2), decoded.GetSequence())

	_, ok = decodeInferenceProgressEnvelope(&commonv1.GovernanceEnvelope{
		EventType: string(constants.Event.Operator.Inference.Completed),
		Payload:   payload,
	})
	assert.False(t, ok)
}
