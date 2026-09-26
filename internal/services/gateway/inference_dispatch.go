// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"
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
		EventType:               req.EventType,
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
		OnInferenceProgress:     req.OnInferenceProgress,
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
	Responder   *response.Writer
	Logger      *slog.Logger

	// MaxPayload is the bounded request-body limit in bytes, wired from
	// cfg.Gateway.MaxPayloadBytes.
	MaxPayload int64
}

// InferenceDispatchController handles POST /api/v1/inference/dispatch, the
// mTLS-protected platform-internal endpoint the ensemble chat pipeline calls
// to dispatch a governed inference request to the Inference Node. It is not
// AI-visible and is not an MCP tool.
type InferenceDispatchController struct {
	dispatchSvc *dispatch.DispatchService
	responder   *response.Writer
	logger      *slog.Logger
	maxPayload  int64
}

// newInferenceDispatchController creates an InferenceDispatchController from its deps.
func newInferenceDispatchController(d InferenceDispatchControllerDeps) *InferenceDispatchController {
	return &InferenceDispatchController{
		dispatchSvc: d.DispatchSvc,
		responder:   d.Responder,
		logger:      d.Logger,
		maxPayload:  d.MaxPayload,
	}
}

// HandleDispatch is the HTTP handler for POST /api/v1/inference/dispatch.
//
// The request and response are the protocol-owned
// operatorv1.InferenceDispatchRequest / InferenceDispatchResponse messages
// serialized as canonical protojson (proto field names). The caller must be
// an authenticated app workload with a delegated user identity; a body
// acting_app_id that contradicts the authenticated app identity is
// rejected. The success response is returned only after the gateway
// verifies the final receipt signature, persistence attestation,
// transaction identity, and result digest equality.
//
// @Summary		Dispatch a governed inference request
// @Description	Platform-internal endpoint (mTLS, app workload) that routes an inference request through the L1-L5 governance gauntlet to the Inference Node and returns the verified result plus final signed receipt.
// @Tags			inference
// @Accept			json
// @Produce		json
// @Param			request	body		operatorv1.InferenceDispatchRequest	true	"Governed inference dispatch request"
// @Success		200		{object}	operatorv1.InferenceDispatchResponse	"Verified inference result and final signed receipt"
// @Failure		400		{string}	string								"Bad Request — malformed body, unknown field, missing messages, invalid role, or acting_app_id identity mismatch"
// @Failure		401		{string}	string								"Unauthorized — missing delegated user identity"
// @Failure		403		{string}	string								"Forbidden — not an app workload, model override denied, or governance rejection"
// @Failure		404		{string}	string								"Not Found — no inference-capable operator session"
// @Failure		405		{string}	string								"Method Not Allowed"
// @Failure		409		{string}	string								"Conflict — multiple inference-capable operators; explicit target required"
// @Failure		413		{string}	string								"Request Entity Too Large — body exceeds the payload limit"
// @Failure		422		{string}	string								"Unprocessable Entity — target operator session is not inference-capable"
// @Failure		502		{string}	string								"Bad Gateway — provider or receipt verification failure"
// @Failure		503		{string}	string								"Service Unavailable — command delivered to no operator subscribers"
// @Failure		504		{string}	string								"Gateway Timeout — request deadline exceeded; remote provider outcome unknown"
// @Failure		500		{string}	string								"Internal Error"
// @Router			/api/v1/inference/dispatch [post]
func (c *InferenceDispatchController) HandleDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	appID, userID, ok := requireAppIdentity(c.responder, w, r)
	if !ok {
		return
	}

	body, err := readRequestBody(r, c.maxPayload)
	if err != nil {
		if errors.Is(err, constants.ErrPayloadExceedsLimit) {
			c.responder.Error(w, http.StatusRequestEntityTooLarge, constants.ErrPayloadExceedsLimit.Error())
			return
		}
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	req := &operatorv1.InferenceDispatchRequest{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(body, req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if len(req.GetMessages()) == 0 {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInferenceMessagesRequired.Error())
		return
	}
	if req.GetProviderAttemptId() == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInferenceProviderAttemptRequired.Error())
		return
	}
	role := models.InferenceModelRoleFromProto(req.GetRole())
	if role == models.InferenceModelRoleUnspecified {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInferenceRoleInvalid.Error())
		return
	}
	// acting_app_id derives from the authenticated app identity. A body
	// value that contradicts it is an identity binding violation; an absent
	// value is populated from the mTLS identity.
	if bodyAppID := req.GetActingAppId(); bodyAppID != "" && bodyAppID != appID {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrIdentityBindingFailed.Error())
		return
	}

	dispatchReq := dispatch.DispatchInferenceRequest{
		Role:                    role,
		Messages:                req.GetMessages(),
		Tools:                   req.GetTools(),
		Model:                   req.GetModel(),
		Temperature:             req.GetTemperature(),
		MaxTokens:               req.GetMaxTokens(),
		KeepAlive:               req.GetKeepAlive(),
		TopP:                    req.TopP,
		TopK:                    req.TopK,
		Seed:                    req.Seed,
		StopSequences:           req.GetStopSequences(),
		ResponseFormat:          req.GetResponseFormat(),
		RequestSchemaVersion:    req.GetRequestSchemaVersion(),
		ToolChoice:              req.GetToolChoice(),
		ParallelToolCalls:       req.ParallelToolCalls,
		Thinking:                req.GetThinking(),
		ContextLimit:            req.ContextLimit,
		ProviderAttemptID:       req.GetProviderAttemptId(),
		ModelDigest:             req.GetModelDigest(),
		CampaignID:              req.GetCampaignId(),
		RunID:                   req.GetRunId(),
		AssignmentID:            req.GetAssignmentId(),
		EvaluationAttemptID:     req.GetEvaluationAttemptId(),
		ScenarioID:              req.GetScenarioId(),
		ModelRegistry:           req.GetModelRegistry(),
		ModelRegistryDigest:     req.GetModelRegistryDigest(),
		TargetOperatorSessionID: req.GetTargetOperatorSessionId(),
		RequestorUserID:         userID,
		ActingAppID:             appID,
		CaseID:                  req.GetCaseId(),
		InvestigationID:         req.GetInvestigationId(),
		TaskID:                  req.GetTaskId(),
		WebSessionID:            req.GetWebSessionId(),
		CliSessionID:            req.GetCliSessionId(),
		Stream:                  req.GetStream(),
		RetryCount:              req.GetRetryCount(),
	}

	if req.GetStream() {
		flusher, ok := w.(http.Flusher)
		if !ok {
			c.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		dispatchReq.OnProgress = func(progress *operatorv1.InferenceProgressEvent) error {
			return writeInferenceDispatchStreamFrame(w, flusher, &operatorv1.InferenceDispatchStreamFrame{
				Frame: &operatorv1.InferenceDispatchStreamFrame_Progress{Progress: progress},
			})
		}

		result, err := c.dispatchSvc.DispatchInference(r.Context(), dispatchReq)
		if err != nil {
			writeInferenceDispatchStreamFailure(c, w, flusher, err)
			return
		}
		if err := writeInferenceDispatchStreamFrame(w, flusher, &operatorv1.InferenceDispatchStreamFrame{
			Frame: &operatorv1.InferenceDispatchStreamFrame_Completion{Completion: &operatorv1.InferenceDispatchResponse{
				TransactionId: result.TransactionID,
				Result:        result.Result,
				Receipt:       result.Receipt,
			}},
		}); err != nil {
			c.logger.Error("inference dispatch: failed to write completion frame", "error", err)
		}
		return
	}

	result, err := c.dispatchSvc.DispatchInference(r.Context(), dispatchReq)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, constants.ErrInferenceCanceled) {
			c.logger.Info("inference dispatch: caller canceled", "error", err)
			return
		}
		status, publicErr := classifyInferenceDispatchError(err)
		c.logger.Error("inference dispatch: dispatch failed", "status", status, "error", err)
		c.responder.Error(w, status, publicErr.Error())
		return
	}

	c.responder.ProtoJSONCanonical(w, http.StatusOK, &operatorv1.InferenceDispatchResponse{
		TransactionId: result.TransactionID,
		Result:        result.Result,
		Receipt:       result.Receipt,
	})
}

func writeInferenceDispatchStreamFailure(c *InferenceDispatchController, w http.ResponseWriter, flusher http.Flusher, err error) {
	if errors.Is(err, context.Canceled) {
		err = fmt.Errorf("%w: %w", constants.ErrInferenceCanceled, err)
	}
	if errors.Is(err, constants.ErrInferenceCanceled) || errors.Is(err, constants.ErrInferenceCallerDisconnected) {
		c.logger.Info("inference dispatch: streaming caller left", "error", err)
	} else {
		c.logger.Error("inference dispatch: streaming dispatch failed after response started", "error", err)
	}
	_, publicErr := classifyInferenceDispatchError(err)
	if writeErr := writeInferenceDispatchStreamFrame(w, flusher, &operatorv1.InferenceDispatchStreamFrame{
		Frame: &operatorv1.InferenceDispatchStreamFrame_Failure{
			Failure: &operatorv1.InferenceDispatchStreamFailure{Reason: publicErr.Error()},
		},
	}); writeErr != nil {
		c.logger.Info("inference dispatch: failed to write streaming failure frame", "error", writeErr)
	}
}

func writeInferenceDispatchStreamFrame(w http.ResponseWriter, flusher http.Flusher, frame *operatorv1.InferenceDispatchStreamFrame) error {
	payload, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal inference dispatch stream frame: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInferenceCallerDisconnected, err)
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInferenceCallerDisconnected, err)
	}
	flusher.Flush()
	return nil
}

// classifyInferenceDispatchError maps a dispatch failure to an HTTP status
// and a public-safe typed error. The returned error is always a centralized
// sentinel whose text is safe to expose; the internal error chain is logged
// by the caller, never returned to the client.
//
// Routing: missing target -> 404, ambiguous -> 409, not capable -> 422.
// Client faults: invalid role/model reference -> 400; model override denied
// and governance rejections (envelope construction failures, signed
// GOVERNANCE_REJECTED receipts) -> 403. Transport/provider: zero delivery ->
// 503; provider execution and receipt verification failures -> 502.
// Deadline/unknown outcome -> 504. Everything else -> 500 with the generic
// internal error.
func classifyInferenceDispatchError(err error) (int, error) {
	sentinels := []struct {
		sentinel error
		status   int
	}{
		{constants.ErrInferenceOutcomeUnknown, http.StatusGatewayTimeout},
		{constants.ErrDispatchResultTimeout, http.StatusGatewayTimeout},
		{constants.ErrInferenceOperatorNotFound, http.StatusNotFound},
		{constants.ErrInferenceOperatorAmbiguous, http.StatusConflict},
		{constants.ErrInferenceOperatorNotCapable, http.StatusUnprocessableEntity},
		{constants.ErrInferenceRoleInvalid, http.StatusBadRequest},
		{constants.ErrInferenceMessagesRequired, http.StatusBadRequest},
		{constants.ErrInferenceProviderAttemptRequired, http.StatusBadRequest},
		{constants.ErrInferenceMessageInvalid, http.StatusBadRequest},
		{constants.ErrInferenceJSONInvalid, http.StatusBadRequest},
		{constants.ErrInferenceJSONNonCanonical, http.StatusBadRequest},
		{constants.ErrInferenceToolSchemaInvalid, http.StatusBadRequest},
		{constants.ErrInferenceGenerationOptionsInvalid, http.StatusBadRequest},
		{constants.ErrInferenceCapabilityUnsupported, http.StatusUnprocessableEntity},
		{constants.ErrInferenceModelRefInvalid, http.StatusBadRequest},
		{constants.ErrInferenceModelOverrideDenied, http.StatusForbidden},
		{constants.ErrInferenceModelRegistryInvalid, http.StatusForbidden},
		{constants.ErrInferenceCampaignBindingInvalid, http.StatusForbidden},
		{constants.ErrInferenceGovernanceRejected, http.StatusForbidden},
		{constants.ErrDispatchNoDelivery, http.StatusServiceUnavailable},
		{constants.ErrInferenceReceiptFailed, http.StatusBadGateway},
		{constants.ErrInferenceReceiptVerify, http.StatusBadGateway},
		{constants.ErrInferenceResultDigestMismatch, http.StatusBadGateway},
		{constants.ErrInferenceResultDigest, http.StatusBadGateway},
		{constants.ErrInferenceIdentityMismatch, http.StatusBadGateway},
		{constants.ErrInferenceModelDigestMismatch, http.StatusBadGateway},
		{constants.ErrInferenceEvidenceHashInvalid, http.StatusBadGateway},
		{constants.ErrInferenceResultDecode, http.StatusBadGateway},
		{constants.ErrInferenceCompletionNoReceipt, http.StatusBadGateway},
		{constants.ErrInferenceCompletionNoResult, http.StatusBadGateway},
		{constants.ErrInferenceBackendUnavailable, http.StatusBadGateway},
		{constants.ErrInferenceBackendNotRegistered, http.StatusBadGateway},
		{constants.ErrInferenceBackendTimeout, http.StatusBadGateway},
		{constants.ErrInferenceGenerateFailed, http.StatusBadGateway},
		{constants.ErrInferenceModelNotFound, http.StatusBadGateway},
		{constants.ErrInferenceProviderResponseInvalid, http.StatusBadGateway},
		{constants.ErrInferenceCanceled, http.StatusRequestTimeout},
		{constants.ErrInferenceCallerDisconnected, http.StatusRequestTimeout},
		{constants.ErrInferenceProgressBackpressure, http.StatusServiceUnavailable},
		{constants.ErrInferenceProgressHashMismatch, http.StatusBadGateway},
	}
	for _, entry := range sentinels {
		if errors.Is(err, entry.sentinel) {
			return entry.status, entry.sentinel
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, constants.ErrInferenceOutcomeUnknown
	}
	// Governance envelope construction failures (L1 screening, L2/L3 posture
	// gates, hash/expiry checks) reject before publish; the shared envelope
	// classifier owns the status mapping for those sentinels.
	if status := classifyEnvelopeError(err); status != http.StatusInternalServerError {
		return status, constants.ErrInferenceGovernanceRejected
	}
	return http.StatusInternalServerError, constants.ErrInternal
}
