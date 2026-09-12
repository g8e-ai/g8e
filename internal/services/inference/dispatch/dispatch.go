// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package dispatch owns the platform-internal inference dispatch service on
// the User Gateway. It constructs a governed InferenceRequested envelope,
// resolves the Inference Node's operator session by querying enrolled
// operators with runtime_config.inference_enabled set to true, and
// dispatches through the gateway's CommandDispatcher (the same
// DispatchService used for every other governed command). It never calls
// the Ollama backend directly; inference is a governed mutation that
// traverses the full L1–L5 gauntlet on the Inference Node.
package dispatch

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/proto"
)

// CommandDispatcher dispatches a governed envelope to a target operator
// session and correlates the result by transaction ID. Implemented by
// *gateway.DispatchService; the interface breaks the import cycle between
// the gateway package (which imports this package for wiring) and this
// package (which needs the dispatch primitive).
type CommandDispatcher interface {
	// Dispatch publishes a signed command to the target operator's cmd
	// channel and waits for the result envelope on the results channel.
	// Returns the transaction ID and the result envelope's payload
	// (proto-marshaled). Returns an error if the operator session is
	// invalid, envelope construction fails, or the result does not arrive
	// within the dispatch timeout.
	Dispatch(ctx context.Context, req CommandDispatchRequest) (*CommandDispatchResult, error)
}

// CommandDispatchRequest is the gateway-agnostic dispatch request carried
// by the CommandDispatcher interface. The gateway adapter converts this to
// gateway.DispatchRequest before calling DispatchService.Dispatch.
type CommandDispatchRequest struct {
	TargetOperatorSessionID string
	ActionType              string
	Payload                 []byte
	TargetResource          string
	RequestorUserID         string
	ActingAppID             string
	CaseID                  string
	InvestigationID         string
	TaskID                  string
	WebSessionID            string
	CliSessionID            string
}

// CommandDispatchResult is the gateway-agnostic dispatch result. The
// TransactionID correlates the dispatch with the signed ActionReceipt
// recorded in the audit chain. The ResultPayload carries the
// proto-marshaled result message (e.g., InferenceResult). For inference
// dispatches, Receipt carries the verified final signed ActionReceipt whose
// result_summary binds the result digest.
type CommandDispatchResult struct {
	TransactionID string
	ResultPayload []byte
	Receipt       *operatorv1.ActionReceipt
}

// OperatorLister lists enrolled operators for a user. Implemented by
// *gateway.RegistrationService; the interface breaks the import cycle.
type OperatorLister interface {
	// ListUserOperators returns every operator document owned by userID,
	// including platform-enrolled operators. The dispatch service filters
	// these by RuntimeConfig.InferenceEnabled to resolve the Inference
	// Node's operator session.
	ListUserOperators(userID string) ([]models.OperatorDocumentGo, error)
}

// DispatchService is the platform-internal inference dispatch service on
// the User Gateway. It is not a NativeTool and is not registered in the MCP
// ToolRegistry. The ensemble chat pipeline calls DispatchInference to route
// a governed inference request to the Inference Node through the full
// L1–L5 gauntlet.
type DispatchService struct {
	dispatcher   CommandDispatcher
	operatorList OperatorLister
	logger       *slog.Logger
}

// NewDispatchService constructs a DispatchService wired to the gateway's
// command dispatcher and operator lister.
func NewDispatchService(dispatcher CommandDispatcher, operatorList OperatorLister, logger *slog.Logger) *DispatchService {
	return &DispatchService{
		dispatcher:   dispatcher,
		operatorList: operatorList,
		logger:       logger,
	}
}

// DispatchInferenceRequest is the input to DispatchInference. The Role and
// Prompt are required; the optional fields override the Inference Node's
// config defaults when non-zero/non-empty.
type DispatchInferenceRequest struct {
	// Role is the chat-tier role for this request (primary, assistant, lite).
	Role models.InferenceModelRole

	// Prompt is the scrubbed prompt text sent to the Inference Node. The
	// Inference Node re-scrubs through its own ScrubbingService before
	// crossing the execution boundary.
	Prompt string

	// Model overrides the Inference Node's configured default model for the
	// role. Empty means use the config default.
	Model string

	// Temperature overrides the backend's default temperature. Zero means
	// use the backend default.
	Temperature float32

	// MaxTokens overrides the backend's default max tokens. Zero means use
	// the backend default.
	MaxTokens int32

	// KeepAlive overrides the config default keep-alive duration. Empty
	// means use the config default.
	KeepAlive string

	// RequestorUserID is the user who initiated the inference request.
	// Used to resolve the Inference Node's operator session from the user's
	// enrolled operators.
	RequestorUserID string

	// ActingAppID is the application that initiated the inference request.
	ActingAppID string

	// Application context fields propagated from the chat turn.
	CaseID          string
	InvestigationID string
	TaskID          string
	WebSessionID    string
	CliSessionID    string
}

// DispatchInferenceResult is the output of a successful inference dispatch.
// The InferenceResult carries the generated text, usage metadata, and
// finish reason. The TransactionID correlates the dispatch with the signed
// ActionReceipt recorded in the User Gateway's audit chain. Receipt carries
// the verified final signed ActionReceipt whose result_summary binds the
// result digest.
type DispatchInferenceResult struct {
	TransactionID string
	Result        *operatorv1.InferenceResult
	Receipt       *operatorv1.ActionReceipt
}

// DispatchInference constructs a governed InferenceRequested envelope,
// resolves the Inference Node's operator session from the requestor's
// enrolled operators (filtering by runtime_config.inference_enabled),
// dispatches through the gateway's CommandDispatcher, and decodes the
// InferenceResult from the result envelope's payload. It never calls the
// Ollama backend directly; the Inference Node executes the request through
// the full L1–L5 gauntlet.
func (s *DispatchService) DispatchInference(ctx context.Context, req DispatchInferenceRequest) (*DispatchInferenceResult, error) {
	if req.RequestorUserID == "" {
		return nil, fmt.Errorf("inference dispatch: %w", constants.ErrRegistrationUserIDRequired)
	}

	// Resolve the Inference Node's operator session from the requestor's
	// enrolled operators. The Inference Node stamps
	// runtime_config.inference_enabled at enrollment time; the dispatch
	// service filters by this flag.
	operatorSessionID, err := s.resolveInferenceOperator(req.RequestorUserID)
	if err != nil {
		return nil, fmt.Errorf("inference dispatch: %w", err)
	}

	// Construct the InferenceRequested proto payload.
	infReq := &operatorv1.InferenceRequested{
		Role:        req.Role.ToProto(),
		Model:       req.Model,
		Prompt:      req.Prompt,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		KeepAlive:   req.KeepAlive,
	}
	payload, err := proto.Marshal(infReq)
	if err != nil {
		return nil, fmt.Errorf("inference dispatch: marshal payload: %w", err)
	}

	// Dispatch through the gateway's CommandDispatcher. The dispatcher
	// constructs the GovernanceEnvelope with the gateway's state root and
	// posture, publishes to the Inference Node's cmd channel, and
	// correlates the result by transaction ID.
	result, err := s.dispatcher.Dispatch(ctx, CommandDispatchRequest{
		TargetOperatorSessionID: operatorSessionID,
		ActionType:               string(constants.ActionTypeInference),
		Payload:                  payload,
		RequestorUserID:          req.RequestorUserID,
		ActingAppID:              req.ActingAppID,
		CaseID:                   req.CaseID,
		InvestigationID:          req.InvestigationID,
		TaskID:                   req.TaskID,
		WebSessionID:             req.WebSessionID,
		CliSessionID:             req.CliSessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("inference dispatch: %w", err)
	}

	// Decode the InferenceResult from the result envelope's payload.
	if len(result.ResultPayload) == 0 {
		return nil, fmt.Errorf("inference dispatch: %w", constants.ErrInferenceResultDecode)
	}
	infResult := &operatorv1.InferenceResult{}
	if err := proto.Unmarshal(result.ResultPayload, infResult); err != nil {
		return nil, fmt.Errorf("inference dispatch: %w: %v", constants.ErrInferenceResultDecode, err)
	}

	s.logger.Info("Governed inference dispatch completed",
		"transaction_id", result.TransactionID,
		"role", req.Role,
		"model", infResult.GetModel(),
		"completion_tokens", infResult.GetCompletionTokens())

	return &DispatchInferenceResult{
		TransactionID: result.TransactionID,
		Result:        infResult,
		Receipt:       result.Receipt,
	}, nil
}

// resolveInferenceOperator resolves the Inference Node's operator session
// from the requestor's enrolled operators by filtering for
// runtime_config.inference_enabled == true. Returns
// constants.ErrInferenceOperatorNotFound when no inference-capable operator
// is enrolled.
func (s *DispatchService) resolveInferenceOperator(userID string) (string, error) {
	operators, err := s.operatorList.ListUserOperators(userID)
	if err != nil {
		return "", err
	}
	for _, op := range operators {
		if op.RuntimeConfig != nil && op.RuntimeConfig.InferenceEnabled && op.OperatorSessionID != "" {
			return op.OperatorSessionID, nil
		}
	}
	return "", constants.ErrInferenceOperatorNotFound
}
