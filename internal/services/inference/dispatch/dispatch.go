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
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// governanceRoundTripMargin covers the governed round trip around a provider
// call: envelope construction, L1 screening, L2 deliberation where required,
// pub/sub delivery, L4/L5 verification and receipt signing, and the result
// publication back to the gateway.
const governanceRoundTripMargin = 30 * time.Second

// RequestDeadline is the single explicit request deadline for a governed
// inference dispatch. It derives from the provider request timeout so the
// dispatch wait always outlives an in-flight provider call. When it expires
// the remote provider call may still be running; the outcome is classified
// unknown (constants.ErrInferenceOutcomeUnknown) and is never retried
// automatically.
const RequestDeadline = inference.ProviderRequestTimeout + governanceRoundTripMargin

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

	// Timeout is the explicit request deadline for this dispatch. Zero means
	// the dispatcher's default applies. Inference dispatches always set
	// RequestDeadline so the wait outlives an in-flight provider call.
	Timeout time.Duration

	// OnInferenceProgress receives bounded provider progress telemetry while
	// waiting for the authoritative terminal completion. Nil disables live
	// progress forwarding.
	OnInferenceProgress func(*operatorv1.InferenceProgressEvent) error
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

// ProviderObservationNotifier coordinates remote provider-boundary observation
// for scored inference attempts. Implementations use the gateway command
// transport and must fail closed when observation commands cannot be delivered.
type ProviderObservationNotifier interface {
	NotifyAttemptBegin(ctx context.Context, requestorUserID, providerAttemptID string, startedAtUnixMs int64, retryCount uint32) error
	NotifyAttemptFinalize(ctx context.Context, requestorUserID, providerAttemptID, inferenceTransactionID string, startedAtUnixMs, completedAtUnixMs int64, failed bool, retryCount uint32) error
}

// DispatchService is the platform-internal inference dispatch service on
// the User Gateway. It is not a NativeTool and is not registered in the MCP
// ToolRegistry. The ensemble chat pipeline calls DispatchInference to route
// a governed inference request to the Inference Node through the full
// L1–L5 gauntlet.
type DispatchService struct {
	dispatcher          CommandDispatcher
	operatorList        OperatorLister
	observationNotifier ProviderObservationNotifier
	logger              *slog.Logger
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

// SetProviderObservationNotifier wires the optional remote provider-boundary
// observation coordinator for scored inference attempts.
func (s *DispatchService) SetProviderObservationNotifier(notifier ProviderObservationNotifier) {
	if s == nil {
		return
	}
	s.observationNotifier = notifier
}

// DispatchInferenceRequest is the input to DispatchInference. Role and ordered messages are required; optional fields override the Inference Node's config defaults when non-zero/non-empty.
type DispatchInferenceRequest struct {
	// Role is the chat-tier role for this request (primary, assistant, lite).
	Role models.InferenceModelRole

	// Messages and Tools preserve the typed request sent to the Inference Node. The node validates and re-scrubs every data-bearing part before crossing the provider boundary.
	Messages []*operatorv1.InferenceMessage
	Tools    []*operatorv1.InferenceToolDeclaration

	// Model selects an exact frozen registry entry in campaign mode. Standard
	// inference accepts only the configured model for the role.
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

	TopP                 *float32
	TopK                 *int32
	Seed                 *int32
	StopSequences        []string
	ResponseFormat       *operatorv1.InferenceResponseFormat
	RequestSchemaVersion string
	ToolChoice           *operatorv1.InferenceToolChoice
	ParallelToolCalls    *bool
	Thinking             *operatorv1.InferenceThinkingControl
	ContextLimit         *int32
	ProviderAttemptID    string
	ModelDigest          string
	CampaignID           string
	RunID                string
	AssignmentID         string
	EvaluationAttemptID  string
	ScenarioID           string
	ModelRegistry        []*operatorv1.InferenceModelVariant
	ModelRegistryDigest  string

	// TargetOperatorSessionID pins the dispatch to a specific Inference Node
	// session. When empty, exactly one inference-capable operator session
	// must be enrolled for the requestor; zero or multiple matches are
	// rejected rather than resolved by query order.
	TargetOperatorSessionID string

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

	// Stream requests live InferenceProgressEvent telemetry while waiting for
	// the authoritative terminal completion.
	Stream bool

	// OnProgress receives progress telemetry when Stream is true.
	OnProgress func(*operatorv1.InferenceProgressEvent) error

	// RetryCount is the zero-based retry ordinal for this provider attempt.
	RetryCount uint32
}

// DispatchInferenceResult is the output of a successful inference dispatch.
// The InferenceResult carries ordered response parts, usage metadata, and
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
	if req.Role == models.InferenceModelRoleUnspecified {
		return nil, fmt.Errorf("inference dispatch: %w", constants.ErrInferenceRoleInvalid)
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("inference dispatch: %w", constants.ErrInferenceMessagesRequired)
	}
	if req.ProviderAttemptID == "" {
		return nil, fmt.Errorf("inference dispatch: %w", constants.ErrInferenceProviderAttemptRequired)
	}
	if req.ModelDigest != "" && !models.IsSHA256Hex(req.ModelDigest) {
		return nil, fmt.Errorf("inference dispatch: %w", constants.ErrInferenceEvidenceHashInvalid)
	}
	if err := validateCampaignModelRegistry(req); err != nil {
		return nil, fmt.Errorf("inference dispatch: %w", err)
	}

	// Resolve the Inference Node's operator session from the requestor's
	// enrolled operators. The Inference Node stamps
	// runtime_config.inference_enabled at enrollment time; the dispatch
	// service filters by this flag.
	operatorSessionID, err := s.resolveInferenceOperator(req)
	if err != nil {
		return nil, fmt.Errorf("inference dispatch: %w", err)
	}

	// Construct the InferenceRequested proto payload.
	infReq := &operatorv1.InferenceRequested{
		Role:                 req.Role.ToProto(),
		Model:                req.Model,
		Messages:             req.Messages,
		Tools:                req.Tools,
		Temperature:          req.Temperature,
		MaxTokens:            req.MaxTokens,
		KeepAlive:            req.KeepAlive,
		TopP:                 req.TopP,
		TopK:                 req.TopK,
		Seed:                 req.Seed,
		StopSequences:        req.StopSequences,
		ResponseFormat:       req.ResponseFormat,
		RequestSchemaVersion: req.RequestSchemaVersion,
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
		Stream:               req.Stream,
		RetryCount:           req.RetryCount,
	}
	payload, err := proto.Marshal(infReq)
	if err != nil {
		return nil, fmt.Errorf("inference dispatch: marshal payload: %w", err)
	}

	attemptStartedAt := time.Now().UTC().UnixMilli()
	if s.observationNotifier != nil {
		if err := s.observationNotifier.NotifyAttemptBegin(ctx, req.RequestorUserID, req.ProviderAttemptID, attemptStartedAt, req.RetryCount); err != nil {
			return nil, fmt.Errorf("inference dispatch: %w", err)
		}
	}

	// Dispatch through the gateway's CommandDispatcher. The dispatcher
	// constructs the GovernanceEnvelope with the gateway's state root and
	// posture, publishes to the Inference Node's cmd channel, and
	// correlates the result by transaction ID. The timeout is the single
	// request deadline contract (RequestDeadline): it outlives an in-flight
	// provider call, and its expiry is an unknown remote outcome, not a
	// clean timeout.
	result, err := s.dispatcher.Dispatch(ctx, CommandDispatchRequest{
		TargetOperatorSessionID: operatorSessionID,
		ActionType:              string(constants.ActionTypeInference),
		Payload:                 payload,
		RequestorUserID:         req.RequestorUserID,
		ActingAppID:             req.ActingAppID,
		CaseID:                  req.CaseID,
		InvestigationID:         req.InvestigationID,
		TaskID:                  req.TaskID,
		WebSessionID:            req.WebSessionID,
		CliSessionID:            req.CliSessionID,
		Timeout:                 RequestDeadline,
		OnInferenceProgress:     req.OnProgress,
	})
	if err != nil {
		if s.observationNotifier != nil {
			_ = s.notifyObservationFinalize(ctx, req, "", attemptStartedAt, time.Now().UTC().UnixMilli(), true)
		}
		if errors.Is(err, constants.ErrDispatchResultTimeout) {
			// The dispatch deadline expired while the provider call may
			// still be running remotely. Record the unknown outcome and
			// never retry automatically: retrying could duplicate the
			// provider call and invalidate request-budget accounting.
			s.logger.Warn("inference dispatch: deadline exceeded; remote provider outcome unknown",
				"operator_session_id", operatorSessionID,
				"role", req.Role,
				"model", req.Model,
				"error", err)
			return nil, fmt.Errorf("inference dispatch: %w", constants.ErrInferenceOutcomeUnknown)
		}
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
	if err := validateInferenceResult(infResult, req); err != nil {
		if s.observationNotifier != nil {
			_ = s.notifyObservationFinalize(ctx, req, result.TransactionID, attemptStartedAt, time.Now().UTC().UnixMilli(), true)
		}
		return nil, fmt.Errorf("inference dispatch: %w", err)
	}

	failed := result.Receipt != nil && result.Receipt.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED
	if s.observationNotifier != nil {
		if err := s.notifyObservationFinalize(ctx, req, result.TransactionID, attemptStartedAt, time.Now().UTC().UnixMilli(), failed); err != nil {
			return nil, err
		}
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

func (s *DispatchService) notifyObservationFinalize(
	ctx context.Context,
	req DispatchInferenceRequest,
	inferenceTransactionID string,
	startedAtUnixMs int64,
	completedAtUnixMs int64,
	failed bool,
) error {
	if s == nil || s.observationNotifier == nil {
		return nil
	}
	err := s.observationNotifier.NotifyAttemptFinalize(
		ctx,
		req.RequestorUserID,
		req.ProviderAttemptID,
		inferenceTransactionID,
		startedAtUnixMs,
		completedAtUnixMs,
		failed,
		req.RetryCount,
	)
	if err == nil {
		return nil
	}
	if req.CampaignID != "" || req.ModelRegistryDigest != "" || len(req.ModelRegistry) != 0 {
		return fmt.Errorf("inference dispatch: %w", err)
	}
	s.logger.Warn("inference dispatch: provider-boundary observation finalize undelivered",
		"provider_attempt_id", req.ProviderAttemptID,
		"error", err)
	return nil
}

func validateCampaignModelRegistry(req DispatchInferenceRequest) error {
	hasCampaignAuthority := req.CampaignID != "" || req.ModelRegistryDigest != "" || len(req.ModelRegistry) != 0
	if !hasCampaignAuthority {
		return nil
	}
	if req.CampaignID == "" || req.RunID == "" || req.AssignmentID == "" || req.EvaluationAttemptID == "" || req.ScenarioID == "" ||
		req.Model == "" || !models.IsSHA256Hex(req.ModelDigest) || !models.IsSHA256Hex(req.ModelRegistryDigest) || len(req.ModelRegistry) == 0 {
		return constants.ErrInferenceCampaignBindingInvalid
	}
	seen := make(map[string]struct{}, len(req.ModelRegistry))
	matched := false
	for _, variant := range req.ModelRegistry {
		if variant == nil || variant.GetModel() == "" || !models.IsSHA256Hex(variant.GetDigest()) {
			return constants.ErrInferenceModelRegistryInvalid
		}
		if _, exists := seen[variant.GetModel()]; exists {
			return constants.ErrInferenceModelRegistryInvalid
		}
		seen[variant.GetModel()] = struct{}{}
		if variant.GetModel() == req.Model && variant.GetDigest() == req.ModelDigest {
			matched = true
		}
	}
	digest, err := models.ComputeInferenceModelRegistryDigest(req.CampaignID, req.ModelRegistry)
	if err != nil || digest != req.ModelRegistryDigest {
		return constants.ErrInferenceModelRegistryInvalid
	}
	if !matched {
		return constants.ErrInferenceModelOverrideDenied
	}
	return nil
}

// resolveInferenceOperator resolves the Inference Node's operator session
// from the requestor's enrolled operators. With an explicit
// TargetOperatorSessionID the target must appear in the requestor's
// operator list (ownership) and carry inference capability; without one,
// exactly one inference-capable session must exist. Terminated operators
// are never selectable. Returns ErrInferenceOperatorNotFound for no match,
// ErrInferenceOperatorNotCapable for an explicit target that lacks the
// capability, and ErrInferenceOperatorAmbiguous for multiple matches with
// no explicit target.
func validateInferenceResult(result *operatorv1.InferenceResult, req DispatchInferenceRequest) error {
	if result.GetModel() == "" || result.GetRequestedModel() == "" || len(result.GetParts()) == 0 {
		return constants.ErrInferenceProviderResponseInvalid
	}
	if result.GetProviderAttemptId() != req.ProviderAttemptID ||
		(req.Model != "" && result.GetRequestedModel() != req.Model) ||
		result.GetCampaignId() != req.CampaignID ||
		result.GetRunId() != req.RunID ||
		result.GetAssignmentId() != req.AssignmentID ||
		result.GetEvaluationAttemptId() != req.EvaluationAttemptID ||
		result.GetScenarioId() != req.ScenarioID ||
		result.GetRequestedModelDigest() != req.ModelDigest ||
		result.GetModelRegistryDigest() != req.ModelRegistryDigest ||
		result.GetRetryCount() != req.RetryCount {
		return constants.ErrInferenceIdentityMismatch
	}
	if result.GetRetryClassification() != models.ClassifyRetry(req.RetryCount) {
		return constants.ErrInferenceIdentityMismatch
	}
	if result.GetLoadState() != models.ClassifyLoadState(result.LoadDurationNs) {
		return constants.ErrInferenceIdentityMismatch
	}
	if !models.IsSHA256Hex(result.GetNormalizedRequestHash()) || !models.IsSHA256Hex(result.GetOutputHash()) {
		return constants.ErrInferenceEvidenceHashInvalid
	}
	if req.ModelDigest != "" && result.GetServedModelDigest() != req.ModelDigest {
		return constants.ErrInferenceIdentityMismatch
	}
	if result.GetServedModelDigest() != "" && !models.IsSHA256Hex(result.GetServedModelDigest()) {
		return constants.ErrInferenceEvidenceHashInvalid
	}
	if result.GetPromptTokens() < 0 || result.GetCompletionTokens() < 0 || result.GetTotalTokens() < 0 {
		return constants.ErrInferenceProviderResponseInvalid
	}
	if result.GetUsageReported() {
		if result.GetTotalTokens() != result.GetPromptTokens()+result.GetCompletionTokens() {
			return constants.ErrInferenceProviderResponseInvalid
		}
	} else if result.GetPromptTokens() != 0 || result.GetCompletionTokens() != 0 || result.GetTotalTokens() != 0 || result.ThinkingTokens != nil || result.CacheTokens != nil {
		return constants.ErrInferenceProviderResponseInvalid
	}
	if result.ThinkingTokens != nil && result.GetThinkingTokens() < 0 {
		return constants.ErrInferenceProviderResponseInvalid
	}
	if result.CacheTokens != nil && result.GetCacheTokens() < 0 {
		return constants.ErrInferenceProviderResponseInvalid
	}
	durations := []*int64{
		result.LoadDurationNs,
		result.PromptEvalDurationNs,
		result.GenerationDurationNs,
		result.TotalDurationNs,
		result.TimeToFirstTokenNs,
	}
	hasTiming := false
	for _, duration := range durations {
		if duration == nil {
			continue
		}
		hasTiming = true
		if *duration < 0 {
			return constants.ErrInferenceProviderResponseInvalid
		}
	}
	if hasTiming {
		if result.GetTimingSource() != operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_PROVIDER {
			return constants.ErrInferenceProviderResponseInvalid
		}
	} else if result.GetTimingSource() != operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_UNSPECIFIED {
		return constants.ErrInferenceProviderResponseInvalid
	}
	return nil
}

func (s *DispatchService) resolveInferenceOperator(req DispatchInferenceRequest) (string, error) {
	operators, err := s.operatorList.ListUserOperators(req.RequestorUserID)
	if err != nil {
		return "", err
	}

	capable := func(op *models.OperatorDocumentGo) bool {
		return op.RuntimeConfig != nil &&
			op.RuntimeConfig.InferenceEnabled &&
			op.OperatorSessionID != "" &&
			op.Status != constants.OperatorStatusTerminated
	}

	if req.TargetOperatorSessionID != "" {
		for i := range operators {
			if operators[i].OperatorSessionID == req.TargetOperatorSessionID {
				if !capable(&operators[i]) {
					return "", constants.ErrInferenceOperatorNotCapable
				}
				return operators[i].OperatorSessionID, nil
			}
		}
		// The target is not in the requestor's operator list. Report
		// not-found rather than a distinct ownership error so the response
		// does not disclose whether the session exists for another user.
		return "", constants.ErrInferenceOperatorNotFound
	}

	var matches []string
	for i := range operators {
		if capable(&operators[i]) {
			matches = append(matches, operators[i].OperatorSessionID)
		}
	}
	switch len(matches) {
	case 0:
		return "", constants.ErrInferenceOperatorNotFound
	case 1:
		return matches[0], nil
	default:
		return "", constants.ErrInferenceOperatorAmbiguous
	}
}
