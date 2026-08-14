// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
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
				ActionType:              string(constants.ActionTypeFsRead),
				Payload:                 []byte("payload"),
			},
			wantErr: nil,
		},
		{
			name: "missing operator session id",
			req: OperatorCommandRequest{
				ActionType: string(constants.ActionTypeFsRead),
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
			wantErr: constants.ErrTxUnknownActionType,
		},
		{
			name: "missing payload",
			req: OperatorCommandRequest{
				TargetOperatorSessionID: "session-123",
				ActionType:              string(constants.ActionTypeFsRead),
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
			EventType:  "FS_READ",
			ActionType:  string(constants.ActionTypeFsRead),
			Payload:     []byte("result-payload-bytes"),
		}
		result := &DispatchResult{
			TransactionID:  "tx-abc-123",
			ResultEnvelope: env,
		}
		resp := result.ToResponse()
		assert.True(t, resp.Success)
		assert.Equal(t, "tx-abc-123", resp.TransactionID)
		assert.Equal(t, "FS_READ", resp.EventType)
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

// --- dispatchResultTracker tests ---

func TestDispatchResultTracker_RegisterRouteUnregister(t *testing.T) {
	tracker := newDispatchResultTracker()
	txID := "tx-001"

	ch := tracker.register(txID)
	require.NotNil(t, ch)

	env := &commonv1.GovernanceEnvelope{Id: txID}
	routed := tracker.route(txID, env)
	assert.True(t, routed)

	select {
	case got := <-ch:
		assert.Equal(t, txID, got.Id)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected result on channel")
	}

	tracker.unregister(txID)

	// After unregister, route returns false.
	routed = tracker.route(txID, env)
	assert.False(t, routed)
}

func TestDispatchResultTracker_RouteUnknownTxID(t *testing.T) {
	tracker := newDispatchResultTracker()
	env := &commonv1.GovernanceEnvelope{Id: "unknown"}
	routed := tracker.route("unknown", env)
	assert.False(t, routed)
}

func TestDispatchResultTracker_RouteFullChannel(t *testing.T) {
	tracker := newDispatchResultTracker()
	txID := "tx-002"
	ch := tracker.register(txID)

	// Fill the buffered channel (capacity 1).
	env := &commonv1.GovernanceEnvelope{Id: txID}
	require.True(t, tracker.route(txID, env))

	// Second route should return false (channel full, default case).
	routed := tracker.route(txID, env)
	assert.False(t, routed)

	// Drain to verify the first one is still there.
	<-ch
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
			ActionType:  cmdEnv.ActionType,
			Timestamp:  timestamppb.Now(),
		}
		resultWire, err := protojson.Marshal(resultEnv)
		require.NoError(t, err)
		broker.Publish(resultsChannel, resultWire)
	})
	defer unregisterOperator()

	result, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		ActionType:              string(constants.ActionTypeFsRead),
		Payload:                 []byte("read-payload"),
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
	svc.auth = &stubOperatorSessionValidator{err: errors.New("session not found")}

	_, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: "invalid-session",
		ActionType:              string(constants.ActionTypeFsRead),
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
		&stubStateRootProvider{err: errors.New("state root unavailable")},
		&stubOperatorSessionValidator{op: op},
	)

	_, err := svc.Dispatch(context.Background(), DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		ActionType:              string(constants.ActionTypeFsRead),
		Payload:                 []byte("payload"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dispatch: get state root")
}

func TestDispatchService_Dispatch_TimeoutNoResult(t *testing.T) {
	op := &models.OperatorDocumentGo{ID: "op-001", OperatorSessionID: "sess-001"}
	svc, _ := newTestDispatchService(t, "root-abc", op)

	// No operator handler registered — no result will be published.
	// Use a context with a short timeout so the test doesn't wait 30s.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := svc.Dispatch(ctx, DispatchRequest{
		TargetOperatorSessionID: "sess-001",
		ActionType:              string(constants.ActionTypeFsRead),
		Payload:                 []byte("payload"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
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

	// Missing target_operator_session_id.
	body := `{"action_type":"FS_READ","payload":"dGVzdA=="}`
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsCommands, bytes.NewReader([]byte(body)))
	rr := httptest.NewRecorder()
	ctrl.HandleDispatch(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
