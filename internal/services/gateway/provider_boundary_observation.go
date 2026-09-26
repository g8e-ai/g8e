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
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// providerBoundaryOperatorLister resolves operators for campaign gateway
// infrastructure. Observer operators are enrolled under the gateway owner,
// not the per-request app identity on inference dispatch.
type providerBoundaryOperatorLister interface {
	ListOperatorsForObservation() ([]models.OperatorDocumentGo, error)
}

type registrationOwnerOperatorLister struct {
	reg     *RegistrationService
	userSvc *UserService
}

func (l *registrationOwnerOperatorLister) ListOperatorsForObservation() ([]models.OperatorDocumentGo, error) {
	if l.reg == nil || l.userSvc == nil {
		return nil, constants.ErrServiceUnavailable
	}
	ownerID, err := l.userSvc.FirstUserID()
	if err != nil {
		return nil, err
	}
	if ownerID == "" {
		return nil, constants.ErrNotFound
	}
	return l.reg.ListUserOperators(ownerID)
}

// ProviderBoundaryObservationCoordinator fans out BEGIN/FINALIZE observation
// commands to the remote provider-boundary observer operator and ingests
// completed windows on the campaign Gateway host.
type ProviderBoundaryObservationCoordinator struct {
	dispatch       *DispatchService
	operatorLister providerBoundaryOperatorLister
	pubsub         *GatewayWebSocketHandler
	windows        provider_observer.WindowStore
	logger         *slog.Logger

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
	operatorLister providerBoundaryOperatorLister,
	pubsubHandler *GatewayWebSocketHandler,
	windows provider_observer.WindowStore,
	logger *slog.Logger,
) *ProviderBoundaryObservationCoordinator {
	return &ProviderBoundaryObservationCoordinator{
		dispatch:       dispatch,
		operatorLister: operatorLister,
		pubsub:         pubsubHandler,
		windows:        windows,
		logger:         logger,
	}
}

// synchronizeObserverSubscription resolves the enrolled provider-boundary
// observer for the gateway owner and subscribes to its results channel. When
// the active observer session changes, any cached target is dropped and the
// coordinator re-subscribes to the newly selected session.
func (c *ProviderBoundaryObservationCoordinator) synchronizeObserverSubscription(ctx context.Context) error {
	if c == nil || c.dispatch == nil || c.operatorLister == nil || c.pubsub == nil || c.windows == nil {
		return constants.ErrServiceUnavailable
	}
	_ = ctx

	operators, err := c.operatorLister.ListOperatorsForObservation()
	if err != nil {
		c.logger.Warn("Provider-boundary observation: list operators failed", "error", err)
		return err
	}
	selected, err := selectProviderBoundaryObserverForGateway(operators)
	if err != nil {
		c.resetObserver()
		switch {
		case errors.Is(err, constants.ErrProviderBoundaryObserverNotFound):
			c.logger.Warn("Provider-boundary observation: observer not found", "reason", "not found")
		case errors.Is(err, constants.ErrProviderBoundaryObserverAmbiguous):
			c.logger.Warn("Provider-boundary observation: observer ambiguous", "reason", "ambiguous")
		default:
			c.logger.Warn("Provider-boundary observation: observer selection failed", "error", err)
		}
		return err
	}

	c.mu.Lock()
	cached := c.observer
	c.mu.Unlock()

	if cached != nil && cached.OperatorID == selected.OperatorID && cached.OperatorSessionID == selected.OperatorSessionID {
		return nil
	}

	if cached != nil {
		c.logger.Info("Provider-boundary observation: observer session changed; re-subscribing",
			"prior_operator_session_id", cached.OperatorSessionID,
			"operator_session_id", selected.OperatorSessionID)
	}
	c.resetObserver()

	observer := &providerBoundaryObserverTarget{
		OperatorID:        selected.OperatorID,
		OperatorSessionID: selected.OperatorSessionID,
	}

	resultsChannel := pubsub.ResultsChannel(observer.OperatorID, observer.OperatorSessionID)
	handler := func(_ string, data []byte) {
		// Observer completions arrive asynchronously after inference dispatch
		// returns; never tie durable ingest to the ephemeral dispatch context.
		c.ingestResult(context.Background(), data)
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
	return nil
}

func selectProviderBoundaryObserverForGateway(operators []models.OperatorDocumentGo) (*operatorcapability.ProviderBoundaryObserverStatus, error) {
	selected, err := operatorcapability.SelectProviderBoundaryObserver(operators, "")
	if err == nil || !errors.Is(err, constants.ErrProviderBoundaryObserverAmbiguous) {
		return selected, err
	}

	// The observer and inference node are hardware-bound Operator sessions.
	// When there is one inference node, use the canonical system fingerprint
	// to disambiguate multiple observer sessions on the owner account. Never
	// choose by list order or recency.
	inference := make([]models.OperatorDocumentGo, 0, len(operators))
	for _, op := range operators {
		if op.Status == constants.OperatorStatusActive &&
			op.OperatorType == constants.OperatorTypeRemote &&
			op.RuntimeConfig != nil &&
			op.RuntimeConfig.InferenceEnabled &&
			op.SystemFingerprint != "" {
			inference = append(inference, op)
		}
	}
	if len(inference) != 1 {
		return nil, err
	}
	return operatorcapability.SelectProviderBoundaryObserverForHardware(
		operators,
		"",
		inference[0].SystemFingerprint,
	)
}

func (c *ProviderBoundaryObservationCoordinator) resetObserver() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.unregister != nil {
		c.unregister()
		c.unregister = nil
	}
	c.observer = nil
	c.mu.Unlock()
}

// Stop unsubscribes from the observer results channel.
func (c *ProviderBoundaryObservationCoordinator) Stop() {
	c.resetObserver()
}

// PreflightCommandDelivery verifies that the active observer is enrolled and
// has at least one cmd-channel WebSocket subscriber on this gateway broker.
func (c *ProviderBoundaryObservationCoordinator) PreflightCommandDelivery(ctx context.Context) error {
	if err := c.synchronizeObserverSubscription(ctx); err != nil {
		return fmt.Errorf("provider-boundary observation preflight: %w", err)
	}
	c.mu.Lock()
	observer := c.observer
	c.mu.Unlock()
	if observer == nil {
		return fmt.Errorf("provider-boundary observation preflight: %w", constants.ErrProviderBoundaryObserverNotFound)
	}
	cmdChannel := pubsub.CmdChannel(observer.OperatorID, observer.OperatorSessionID)
	if c.pubsub == nil || c.pubsub.ChannelSubscriberCount(cmdChannel) == 0 {
		return fmt.Errorf("provider-boundary observation preflight: %w: cmd channel %s",
			constants.ErrEvaluationObservationUnavailable, cmdChannel)
	}
	return nil
}

// NotifyAttemptBegin delivers a BEGIN command to the observer. Delivery uses
// the same governed PublishCommand transport as other gateway dispatches and
// fails closed when the observer cmd channel has no subscribers.
func (c *ProviderBoundaryObservationCoordinator) NotifyAttemptBegin(ctx context.Context, _ string, providerAttemptID string, startedAtUnixMs int64, retryCount uint32) error {
	if providerAttemptID == "" {
		return nil
	}
	if err := c.synchronizeObserverSubscription(ctx); err != nil {
		return fmt.Errorf("provider-boundary observation begin: %w", err)
	}
	command := &evalv1.ProviderBoundaryObservationCommand{
		ProviderAttemptId:      providerAttemptID,
		Phase:                  evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN,
		AttemptStartedAtUnixMs: startedAtUnixMs,
		RetryCount:             retryCount,
	}
	return c.publishCommand(ctx, command)
}

// NotifyAttemptFinalize delivers a FINALIZE command to the observer.
func (c *ProviderBoundaryObservationCoordinator) NotifyAttemptFinalize(
	ctx context.Context,
	_ string,
	providerAttemptID string,
	inferenceTransactionID string,
	startedAtUnixMs int64,
	completedAtUnixMs int64,
	failed bool,
	retryCount uint32,
) error {
	if providerAttemptID == "" {
		return nil
	}
	if err := c.synchronizeObserverSubscription(ctx); err != nil {
		return fmt.Errorf("provider-boundary observation finalize: %w", err)
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
	return c.publishCommand(ctx, command)
}

func (c *ProviderBoundaryObservationCoordinator) publishCommand(ctx context.Context, command *evalv1.ProviderBoundaryObservationCommand) error {
	if err := c.tryPublishCommand(ctx, command); err != nil {
		return err
	}
	return nil
}

func (c *ProviderBoundaryObservationCoordinator) tryPublishCommand(ctx context.Context, command *evalv1.ProviderBoundaryObservationCommand) error {
	c.mu.Lock()
	observer := c.observer
	c.mu.Unlock()
	if observer == nil || c.dispatch == nil {
		return constants.ErrMissingRequiredField
	}
	cmdChannel := pubsub.CmdChannel(observer.OperatorID, observer.OperatorSessionID)
	payload, err := proto.Marshal(command)
	if err != nil {
		c.logger.Warn("Provider-boundary observation command marshal failed", "error", err)
		return err
	}
	txID, err := c.dispatch.PublishCommand(ctx, PublishCommandRequest{
		TargetOperatorSessionID: observer.OperatorSessionID,
		EventType:               string(constants.Event.Operator.ProviderBoundaryObservation.Requested),
		Payload:                 payload,
	})
	if err != nil {
		c.logger.Warn("Provider-boundary observation command publish failed",
			"provider_attempt_id", command.GetProviderAttemptId(),
			"phase", command.GetPhase().String(),
			"operator_id", observer.OperatorID,
			"operator_session_id", observer.OperatorSessionID,
			"cmd_channel", cmdChannel,
			"error", err)
		return err
	}
	c.logger.Info("Provider-boundary observation command published",
		"transaction_id", txID,
		"provider_attempt_id", command.GetProviderAttemptId(),
		"phase", command.GetPhase().String(),
		"operator_id", observer.OperatorID,
		"operator_session_id", observer.OperatorSessionID,
		"cmd_channel", cmdChannel)
	return nil
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
	EventType               string
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
		EventType:         req.EventType,
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
