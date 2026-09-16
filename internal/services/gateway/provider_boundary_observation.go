// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ProviderBoundaryObservationCoordinator fans out BEGIN/FINALIZE observation
// commands to the remote provider-boundary observer operator and ingests
// completed windows on the campaign Gateway host.
type ProviderBoundaryObservationCoordinator struct {
	dispatch *DispatchService
	reg      *RegistrationService
	pubsub   *GatewayWebSocketHandler
	windows  provider_observer.WindowStore
	logger   *slog.Logger

	mu         sync.Mutex
	unregister func()
	observer   *providerBoundaryObserverTarget
}

type providerBoundaryObserverTarget struct {
	OperatorID        string
	OperatorSessionID string
}

// NewProviderBoundaryObservationCoordinator constructs the gateway-side
// provider-boundary observation coordinator.
func NewProviderBoundaryObservationCoordinator(
	dispatch *DispatchService,
	reg *RegistrationService,
	pubsubHandler *GatewayWebSocketHandler,
	windows provider_observer.WindowStore,
	logger *slog.Logger,
) *ProviderBoundaryObservationCoordinator {
	return &ProviderBoundaryObservationCoordinator{
		dispatch: dispatch,
		reg:      reg,
		pubsub:   pubsubHandler,
		windows:  windows,
		logger:   logger,
	}
}

// ensureObserver lazily resolves the enrolled provider-boundary observer
// operator for one requestor and subscribes to its results channel once.
func (c *ProviderBoundaryObservationCoordinator) ensureObserver(ctx context.Context, requestorUserID string) bool {
	if c == nil || c.dispatch == nil || c.reg == nil || c.pubsub == nil || c.windows == nil || requestorUserID == "" {
		return false
	}
	c.mu.Lock()
	if c.observer != nil {
		c.mu.Unlock()
		return true
	}
	c.mu.Unlock()

	operators, err := c.reg.ListUserOperators(requestorUserID)
	if err != nil {
		c.logger.Warn("Provider-boundary observation: list operators failed", "error", err)
		return false
	}
	observer, err := selectProviderBoundaryObserver(operators)
	if err != nil {
		return false
	}

	resultsChannel := pubsub.ResultsChannel(observer.OperatorID, observer.OperatorSessionID)
	handler := func(_ string, data []byte) {
		c.ingestResult(ctx, data)
	}
	unregister := c.pubsub.RegisterHandler(resultsChannel, handler)

	c.mu.Lock()
	c.observer = observer
	c.unregister = unregister
	c.mu.Unlock()

	c.logger.Info("Provider-boundary observation coordinator subscribed",
		"operator_id", observer.OperatorID,
		"operator_session_id", observer.OperatorSessionID,
		"results_channel", resultsChannel)
	return true
}

// Stop unsubscribes from the observer results channel.
func (c *ProviderBoundaryObservationCoordinator) Stop() {
	c.mu.Lock()
	if c.unregister != nil {
		c.unregister()
		c.unregister = nil
	}
	c.observer = nil
	c.mu.Unlock()
}

// NotifyAttemptBegin sends a fire-and-forget BEGIN command to the observer.
func (c *ProviderBoundaryObservationCoordinator) NotifyAttemptBegin(ctx context.Context, requestorUserID, providerAttemptID string, startedAtUnixMs int64, retryCount uint32) {
	if providerAttemptID == "" {
		return
	}
	if !c.ensureObserver(ctx, requestorUserID) {
		return
	}
	command := &evalv1.ProviderBoundaryObservationCommand{
		ProviderAttemptId:      providerAttemptID,
		Phase:                  evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN,
		AttemptStartedAtUnixMs: startedAtUnixMs,
		RetryCount:             retryCount,
	}
	c.publishCommand(ctx, command)
}

// NotifyAttemptFinalize sends a fire-and-forget FINALIZE command to the observer.
func (c *ProviderBoundaryObservationCoordinator) NotifyAttemptFinalize(
	ctx context.Context,
	requestorUserID string,
	providerAttemptID string,
	inferenceTransactionID string,
	startedAtUnixMs int64,
	completedAtUnixMs int64,
	failed bool,
	retryCount uint32,
) {
	if providerAttemptID == "" {
		return
	}
	if !c.ensureObserver(ctx, requestorUserID) {
		return
	}
	status := evalv1.ProviderBoundaryObservationAttemptStatus_PROVIDER_BOUNDARY_OBSERVATION_ATTEMPT_STATUS_COMPLETED
	if failed {
		status = evalv1.ProviderBoundaryObservationAttemptStatus_PROVIDER_BOUNDARY_OBSERVATION_ATTEMPT_STATUS_FAILED
	}
	command := &evalv1.ProviderBoundaryObservationCommand{
		ProviderAttemptId:        providerAttemptID,
		InferenceTransactionId:   inferenceTransactionID,
		Phase:                    evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_FINALIZE,
		AttemptStartedAtUnixMs:   startedAtUnixMs,
		AttemptCompletedAtUnixMs: completedAtUnixMs,
		AttemptStatus:            status,
		RetryCount:               retryCount,
	}
	c.publishCommand(ctx, command)
}

func (c *ProviderBoundaryObservationCoordinator) publishCommand(ctx context.Context, command *evalv1.ProviderBoundaryObservationCommand) {
	c.mu.Lock()
	observer := c.observer
	c.mu.Unlock()
	if observer == nil || c.dispatch == nil {
		return
	}
	payload, err := proto.Marshal(command)
	if err != nil {
		c.logger.Warn("Provider-boundary observation command marshal failed", "error", err)
		return
	}
	if _, err := c.dispatch.PublishCommand(ctx, PublishCommandRequest{
		TargetOperatorSessionID: observer.OperatorSessionID,
		ActionType:              string(constants.ActionTypeProviderBoundaryObservation),
		Payload:                 payload,
	}); err != nil {
		c.logger.Warn("Provider-boundary observation command publish failed",
			"provider_attempt_id", command.GetProviderAttemptId(),
			"phase", command.GetPhase().String(),
			"error", err)
	}
}

func (c *ProviderBoundaryObservationCoordinator) ingestResult(ctx context.Context, data []byte) {
	env := &commonv1.GovernanceEnvelope{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, env); err != nil {
		c.logger.Warn("Provider-boundary observation ingest: unmarshal envelope failed", "error", err)
		return
	}
	if env.GetEventType() != string(constants.Event.Operator.ProviderBoundaryObservation.Completed) {
		return
	}
	completion := &evalv1.ProviderBoundaryObservationCompleted{}
	if err := proto.Unmarshal(env.GetPayload(), completion); err != nil {
		c.logger.Warn("Provider-boundary observation ingest: unmarshal completion failed", "error", err)
		return
	}
	window := completion.GetWindow()
	if window == nil {
		return
	}
	if err := c.windows.Save(ctx, window); err != nil {
		c.logger.Warn("Provider-boundary observation ingest: save window failed",
			"provider_attempt_id", window.GetProviderAttemptId(),
			"error", err)
		return
	}
	c.logger.Info("Provider-boundary observation window ingested",
		"provider_attempt_id", window.GetProviderAttemptId(),
		"sample_count", len(window.GetSamples()),
		"observation_digest", window.GetObservationDigest())
}

// PublishCommandRequest is a fire-and-forget governed command publish.
type PublishCommandRequest struct {
	TargetOperatorSessionID string
	ActionType              string
	Payload                 []byte
	RequestorUserID         string
	ActingAppID             string
}

// PublishCommand builds a governed envelope and publishes it to the target
// operator cmd channel without waiting for a correlated result.
func (d *DispatchService) PublishCommand(ctx context.Context, req PublishCommandRequest) (string, error) {
	op, err := d.auth.ValidateOperatorSession(req.TargetOperatorSessionID)
	if err != nil {
		return "", fmt.Errorf("dispatch: publish command: validate operator session: %w", err)
	}
	stateRoot, err := d.stateRootProvider.GetCurrentStateRoot()
	if err != nil {
		return "", fmt.Errorf("dispatch: publish command: get state root: %w", err)
	}
	env, err := BuildGovernanceEnvelope(BuildEnvelopeParams{
		OperatorID:        op.ID,
		OperatorSessionID: op.OperatorSessionID,
		ActionType:        req.ActionType,
		Payload:           req.Payload,
		RequestorUserID:   req.RequestorUserID,
		ActingAppID:       req.ActingAppID,
		StateMerkleRoot:   stateRoot,
		Posture:           d.posture,
		Doctrine:          d.doctrine,
	})
	if err != nil {
		return "", fmt.Errorf("dispatch: publish command: %w", err)
	}
	wire, err := protojson.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("dispatch: publish command: marshal envelope: %w", err)
	}
	posture, perr := governance.ParseGovernancePosture(d.posture)
	if perr != nil {
		return "", fmt.Errorf("dispatch: publish command: %w", perr)
	}
	if posture.RequiresL2Signature() && d.l2Deliberator != nil {
		deliberated, derr := d.l2Deliberator.Deliberate(ctx, wire)
		if derr != nil {
			return "", fmt.Errorf("dispatch: publish command: l2 deliberation: %w", derr)
		}
		wire = deliberated
	}
	cmdChannel := pubsub.CmdChannel(op.ID, op.OperatorSessionID)
	delivered := d.pubsub.Publish(cmdChannel, wire)
	if delivered == 0 {
		return "", fmt.Errorf("dispatch: publish command: %w", constants.ErrDispatchNoDelivery)
	}
	return env.Id, nil
}

func selectProviderBoundaryObserver(operators []models.OperatorDocumentGo) (*providerBoundaryObserverTarget, error) {
	matches := make([]providerBoundaryObserverTarget, 0)
	for _, op := range operators {
		if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
			continue
		}
		if op.RuntimeConfig == nil || !op.RuntimeConfig.ProviderBoundaryObserverEnabled {
			continue
		}
		if op.OperatorSessionID == "" {
			continue
		}
		matches = append(matches, providerBoundaryObserverTarget{
			OperatorID:        op.ID,
			OperatorSessionID: op.OperatorSessionID,
		})
	}
	switch len(matches) {
	case 0:
		return nil, constants.ErrProviderBoundaryObserverNotFound
	case 1:
		return &matches[0], nil
	default:
		return nil, constants.ErrProviderBoundaryObserverAmbiguous
	}
}
