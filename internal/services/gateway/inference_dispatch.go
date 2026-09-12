// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/dispatch"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// gatewayDispatcherAdapter adapts *DispatchService to the
// dispatch.CommandDispatcher interface. The adapter lives in the gateway
// package to break the import cycle: dispatch defines the interface,
// gateway imports dispatch and provides the concrete adapter.
type gatewayDispatcherAdapter struct {
	svc *DispatchService
}

// Dispatch implements dispatch.CommandDispatcher by delegating to the
// gateway's DispatchService and converting between the gateway-agnostic
// and gateway-specific request/result types.
func (a *gatewayDispatcherAdapter) Dispatch(ctx context.Context, req dispatch.CommandDispatchRequest) (*dispatch.CommandDispatchResult, error) {
	result, err := a.svc.Dispatch(ctx, DispatchRequest{
		TargetOperatorSessionID: req.TargetOperatorSessionID,
		ActionType:              req.ActionType,
		Payload:                 req.Payload,
		TargetResource:          req.TargetResource,
		RequestorUserID:         req.RequestorUserID,
		ActingAppID:             req.ActingAppID,
		CaseID:                  req.CaseID,
		InvestigationID:         req.InvestigationID,
		TaskID:                  req.TaskID,
		WebSessionID:            req.WebSessionID,
		CliSessionID:            req.CliSessionID,
		Timeout:                 req.Timeout,
	})
	if err != nil {
		return nil, err
	}
	var payload []byte
	if result.InferenceResult != nil {
		// The verified InferenceResult is the result payload for inference
		// dispatches; the containing envelope payload is the
		// InferenceCompletion (result plus signed receipt).
		payload, err = proto.Marshal(result.InferenceResult)
		if err != nil {
			return nil, fmt.Errorf("dispatch: marshal inference result: %w", err)
		}
	} else if result.ResultEnvelope != nil {
		payload = result.ResultEnvelope.Payload
	}
	return &dispatch.CommandDispatchResult{
		TransactionID: result.TransactionID,
		ResultPayload: payload,
		Receipt:       result.Receipt,
	}, nil
}

// gatewayOperatorListerAdapter adapts *RegistrationService to the
// dispatch.OperatorLister interface.
type gatewayOperatorListerAdapter struct {
	svc *RegistrationService
}

// ListUserOperators implements dispatch.OperatorLister by delegating to
// the gateway's RegistrationService.
func (a *gatewayOperatorListerAdapter) ListUserOperators(userID string) ([]models.OperatorDocumentGo, error) {
	return a.svc.ListUserOperators(userID)
}

// InferenceDispatchControllerDeps groups all dependencies for
// InferenceDispatchController.
type InferenceDispatchControllerDeps struct {
	DispatchSvc *dispatch.DispatchService
	Responder    *response.Writer
	Logger       *slog.Logger
}

// InferenceDispatchController handles POST /api/v1/inference/dispatch, the
// mTLS-protected platform-internal endpoint the ensemble chat pipeline calls
// to dispatch a governed inference request to the Inference Node. It is not
// AI-visible and is not an MCP tool.
type InferenceDispatchController struct {
	dispatchSvc *dispatch.DispatchService
	responder   *response.Writer
	logger      *slog.Logger
}

// newInferenceDispatchController creates an InferenceDispatchController from its deps.
func newInferenceDispatchController(d InferenceDispatchControllerDeps) *InferenceDispatchController {
	return &InferenceDispatchController{
		dispatchSvc: d.DispatchSvc,
		responder:   d.Responder,
		logger:      d.Logger,
	}
}

// InferenceDispatchRequest is the typed JSON request for POST
// /api/v1/inference/dispatch.
type InferenceDispatchRequest struct {
	Role                    int32   `json:"role"`
	Prompt                  string  `json:"prompt"`
	Model                   string  `json:"model,omitempty"`
	Temperature             float32 `json:"temperature,omitempty"`
	MaxTokens               int32   `json:"max_tokens,omitempty"`
	KeepAlive               string  `json:"keep_alive,omitempty"`
	TargetOperatorSessionID string  `json:"target_operator_session_id,omitempty"`
	ActingAppID             string  `json:"acting_app_id,omitempty"`
	CaseID                  string  `json:"case_id,omitempty"`
	InvestigationID         string  `json:"investigation_id,omitempty"`
	TaskID                  string  `json:"task_id,omitempty"`
	WebSessionID            string  `json:"web_session_id,omitempty"`
	CliSessionID            string  `json:"cli_session_id,omitempty"`
}

// Validate returns an error if the request is missing required fields.
func (r *InferenceDispatchRequest) Validate() error {
	if r.Prompt == "" {
		return constants.ErrPubSubEmptyPayload
	}
	if r.Role < int32(operatorv1.ModelRole_MODEL_ROLE_PRIMARY) || r.Role > int32(operatorv1.ModelRole_MODEL_ROLE_LITE) {
		return constants.ErrInferenceRoleInvalid
	}
	return nil
}

// InferenceDispatchResponse is the typed JSON response for POST
// /api/v1/inference/dispatch.
type InferenceDispatchResponse struct {
	Success       bool                       `json:"success"`
	TransactionID string                     `json:"transaction_id"`
	Result        *operatorv1.InferenceResult `json:"result,omitempty"`
	Error         string                     `json:"error,omitempty"`
}

// HandleDispatch is the HTTP handler for POST /api/v1/inference/dispatch.
func (c *InferenceDispatchController) HandleDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req InferenceDispatchRequest
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

	result, err := c.dispatchSvc.DispatchInference(r.Context(), dispatch.DispatchInferenceRequest{
		Role:                    models.InferenceModelRole(req.Role),
		Prompt:                  req.Prompt,
		Model:                   req.Model,
		Temperature:             req.Temperature,
		MaxTokens:               req.MaxTokens,
		KeepAlive:               req.KeepAlive,
		TargetOperatorSessionID: req.TargetOperatorSessionID,
		RequestorUserID:         requestorUserID,
		ActingAppID:             req.ActingAppID,
		CaseID:                  req.CaseID,
		InvestigationID:         req.InvestigationID,
		TaskID:                  req.TaskID,
		WebSessionID:            req.WebSessionID,
		CliSessionID:            req.CliSessionID,
	})
	if err != nil {
		c.logger.Error("inference dispatch: dispatch failed", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, InferenceDispatchResponse{
		Success:       true,
		TransactionID: result.TransactionID,
		Result:        result.Result,
	})
}
