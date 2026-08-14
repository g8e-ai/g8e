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
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	govpkg "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// DispatchTimeout is the maximum time to wait for an operator result after
// publishing a command. The round-trip includes in-process publish, WS delivery,
// operator L4/L5 verification and execution, WS publish back, and in-process
// handler delivery.
const DispatchTimeout = 30 * time.Second

// DispatchRequest is the input to the command dispatch service.
type DispatchRequest struct {
	TargetOperatorSessionID string
	ActionType              string
	Payload                 []byte
	TargetResource          string
	RequestorUserID         string
	ActingAppID             string
}

// DispatchResult is the output of a successful command dispatch.
type DispatchResult struct {
	TransactionID string
	ResultEnvelope *commonv1.GovernanceEnvelope
}

// operatorSessionValidator resolves an operator session ID to the operator
// document. AuthService implements this; the interface makes the dispatch
// service's dependency on auth explicit and testable.
type operatorSessionValidator interface {
	ValidateOperatorSession(operatorSessionID string) (*models.OperatorDocumentGo, error)
}

// DispatchService constructs a GovernanceEnvelope, publishes it to an operator's
// cmd channel via the in-process WS broker, and correlates the operator's result
// published on the results channel back to the originating request.
//
// The gateway owns the state Merkle root and the governance posture. The
// dispatch service sets StateMerkleRoot to the gateway's current state root.
// Under DoctrinePosture (the docker-compose default), no L2 votes or L3 proofs
// are required for read-only commands.
type DispatchService struct {
	logger          *slog.Logger
	pubsub          *GatewayWebSocketHandler
	stateRootProvider governance.StateRootProvider
	auth            operatorSessionValidator
}

// NewDispatchService creates a DispatchService wired to the gateway's in-process
// pub/sub broker, state root provider, and auth service.
func NewDispatchService(logger *slog.Logger, pubsubHandler *GatewayWebSocketHandler, stateRootProvider governance.StateRootProvider, auth operatorSessionValidator) *DispatchService {
	return &DispatchService{
		logger:            logger,
		pubsub:            pubsubHandler,
		stateRootProvider: stateRootProvider,
		auth:              auth,
	}
}

// Dispatch sends a signed command to the target operator and waits for the result.
// The envelope is constructed with the gateway's current state root, a unique
// nonce, and a near-future expiry. The result is correlated by transaction ID.
// Returns an error if the operator session is invalid, envelope construction
// fails, or the result does not arrive within DispatchTimeout.
func (d *DispatchService) Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResult, error) {
	// 1. Resolve the target operator session.
	op, err := d.auth.ValidateOperatorSession(req.TargetOperatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("dispatch: validate operator session: %w", err)
	}

	operatorID := op.ID
	operatorSessionID := op.OperatorSessionID

	// 2. Fetch the gateway's current state root.
	stateRoot, err := d.stateRootProvider.GetCurrentStateRoot()
	if err != nil {
		return nil, fmt.Errorf("dispatch: get state root: %w", err)
	}

	// 3. Build the GovernanceEnvelope.
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("dispatch: generate nonce: %w", err)
	}

	env := &commonv1.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        operatorID,
		OperatorSessionId: operatorSessionID,
		ActionType:        req.ActionType,
		TargetResource:    req.TargetResource,
		EventType:         string(constants.MapActionTypeToEventType(constants.ActionType(req.ActionType))),
		Payload:           req.Payload,
		StateMerkleRoot:   stateRoot,
		Nonce:             hex.EncodeToString(nonce),
		RequestorUserId:   req.RequestorUserID,
		ActingAppId:       req.ActingAppID,
		Governance: &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{Validated: true},
		},
	}

	// 4. Compute the transaction hash and set Id == TransactionHash.
	txHash, err := govpkg.GenerateMessageID(env)
	if err != nil {
		return nil, fmt.Errorf("dispatch: generate message ID: %w", err)
	}
	env.Id = txHash
	env.TransactionHash = txHash

	// 5. Marshal as protojson (the canonical wire format).
	wire, err := protojson.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("dispatch: marshal envelope: %w", err)
	}

	// 6. Register an in-process handler on the operator's results channel to
	//    correlate the result by transaction ID.
	resultsChannel := pubsub.ResultsChannel(operatorID, operatorSessionID)
	resultCh := make(chan *commonv1.GovernanceEnvelope, 1)

	handler := func(channel string, data []byte) {
		resultEnv := &commonv1.GovernanceEnvelope{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, resultEnv); err != nil {
			d.logger.Warn("dispatch: failed to unmarshal result envelope", "error", err)
			return
		}
		if resultEnv.Id == txHash {
			select {
			case resultCh <- resultEnv:
			default:
			}
		}
	}
	unregister := d.pubsub.RegisterHandler(resultsChannel, handler)
	defer unregister()

	// 7. Publish the envelope to the operator's cmd channel.
	cmdChannel := pubsub.CmdChannel(operatorID, operatorSessionID)
	delivered := d.pubsub.Publish(cmdChannel, wire)
	d.logger.Info("dispatch: published command",
		"transaction_id", txHash,
		"cmd_channel", cmdChannel,
		"results_channel", resultsChannel,
		"delivered", delivered)

	// 8. Wait for the result with a timeout.
	timeoutCtx, cancel := context.WithTimeout(ctx, DispatchTimeout)
	defer cancel()

	select {
	case resultEnv := <-resultCh:
		return &DispatchResult{
			TransactionID:  txHash,
			ResultEnvelope: resultEnv,
		}, nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("dispatch: timed out waiting for operator result after %s", DispatchTimeout)
	}
}

// dispatchResultTracker is a concurrency-safe tracker for in-flight dispatches.
// Currently unused but reserved for the production long-lived results listener.
type dispatchResultTracker struct {
	mu       sync.Mutex
	pending  map[string]chan *commonv1.GovernanceEnvelope
}

func newDispatchResultTracker() *dispatchResultTracker {
	return &dispatchResultTracker{pending: make(map[string]chan *commonv1.GovernanceEnvelope)}
}

func (t *dispatchResultTracker) register(txID string) chan *commonv1.GovernanceEnvelope {
	t.mu.Lock()
	defer t.mu.Unlock()
	ch := make(chan *commonv1.GovernanceEnvelope, 1)
	t.pending[txID] = ch
	return ch
}

func (t *dispatchResultTracker) unregister(txID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, txID)
}

func (t *dispatchResultTracker) route(txID string, env *commonv1.GovernanceEnvelope) bool {
	t.mu.Lock()
	ch, ok := t.pending[txID]
	t.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- env:
		return true
	default:
		return false
	}
}

// DispatchResponse is the typed JSON response for POST /api/v1/operators/commands.
// ResultPayload carries the operator's proto-marshaled result payload (e.g.
// FsReadResult) so the caller can inspect the execution outcome. It is
// base64-encoded in JSON per protojson/encoding/json conventions for []byte.
type DispatchResponse struct {
	Success       bool   `json:"success"`
	TransactionID string `json:"transaction_id"`
	EventType     string `json:"event_type,omitempty"`
	ActionType    string `json:"action_type,omitempty"`
	ResultPayload []byte `json:"result_payload,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ToResponse converts a DispatchResult to a DispatchResponse.
func (r *DispatchResult) ToResponse() DispatchResponse {
	resp := DispatchResponse{
		Success:       true,
		TransactionID: r.TransactionID,
	}
	if r.ResultEnvelope != nil {
		resp.EventType = string(r.ResultEnvelope.EventType)
		resp.ActionType = r.ResultEnvelope.ActionType
		resp.ResultPayload = r.ResultEnvelope.Payload
	}
	return resp
}

// OperatorCommandRequest is the typed JSON request for POST /api/v1/operators/commands.
type OperatorCommandRequest struct {
	TargetOperatorSessionID string `json:"target_operator_session_id"`
	ActionType              string `json:"action_type"`
	Payload                 []byte `json:"payload"`
	TargetResource          string `json:"target_resource,omitempty"`
}

// Validate returns an error if the request is missing required fields.
func (r *OperatorCommandRequest) Validate() error {
	if r.TargetOperatorSessionID == "" {
		return constants.ErrGatewayOperatorSessionIDRequired
	}
	if r.ActionType == "" {
		return constants.ErrTxUnknownActionType
	}
	if len(r.Payload) == 0 {
		return constants.ErrTxPayloadMissing
	}
	return nil
}

// DispatchControllerDeps groups all dependencies for DispatchController.
type DispatchControllerDeps struct {
	DispatchSvc *DispatchService
	Responder   *response.Writer
	Logger      *slog.Logger
}

// DispatchController handles POST /api/v1/operators/commands, the mTLS-protected
// entry point for dispatching signed commands to operators.
type DispatchController struct {
	dispatchSvc *DispatchService
	responder   *response.Writer
	logger      *slog.Logger
}

// newDispatchController creates a DispatchController from its deps.
func newDispatchController(d DispatchControllerDeps) *DispatchController {
	return &DispatchController{
		dispatchSvc: d.DispatchSvc,
		responder:   d.Responder,
		logger:      d.Logger,
	}
}

// HandleDispatch is the HTTP handler for POST /api/v1/operators/commands.
func (c *DispatchController) HandleDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req OperatorCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := req.Validate(); err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	// Extract the requestor's user ID from the mTLS identity context.
	requestorUserID, _ := r.Context().Value(constants.ContextKeyUserID).(string)

	result, err := c.dispatchSvc.Dispatch(r.Context(), DispatchRequest{
		TargetOperatorSessionID: req.TargetOperatorSessionID,
		ActionType:              req.ActionType,
		Payload:                 req.Payload,
		TargetResource:          req.TargetResource,
		RequestorUserID:         requestorUserID,
	})
	if err != nil {
		c.logger.Error("dispatch: command dispatch failed", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, result.ToResponse())
}
