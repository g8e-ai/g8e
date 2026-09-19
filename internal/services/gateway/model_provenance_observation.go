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
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ModelProvenanceObservationCoordinator fans out BEGIN/FINALIZE provenance
// commands to the remote storage-side Provenance Operator and ingests
// completed attestation windows on the campaign Gateway host.
type ModelProvenanceObservationCoordinator struct {
	dispatch       *DispatchService
	operatorLister providerBoundaryOperatorLister
	pubsub         *GatewayWebSocketHandler
	windows        model_provenance.WindowStore
	logger         *slog.Logger

	mu         sync.Mutex
	unregister func()
	operator   *modelProvenanceOperatorTarget
}

type modelProvenanceOperatorTarget struct {
	OperatorID        string
	OperatorSessionID string
}

// NewModelProvenanceObservationCoordinator constructs the gateway-side model
// provenance observation coordinator.
func NewModelProvenanceObservationCoordinator(
	dispatch *DispatchService,
	operatorLister providerBoundaryOperatorLister,
	pubsubHandler *GatewayWebSocketHandler,
	windows model_provenance.WindowStore,
	logger *slog.Logger,
) *ModelProvenanceObservationCoordinator {
	return &ModelProvenanceObservationCoordinator{
		dispatch:       dispatch,
		operatorLister: operatorLister,
		pubsub:         pubsubHandler,
		windows:        windows,
		logger:         logger,
	}
}

func (c *ModelProvenanceObservationCoordinator) ensureOperator(ctx context.Context) error {
	if c == nil || c.dispatch == nil || c.operatorLister == nil || c.pubsub == nil || c.windows == nil {
		return constants.ErrServiceUnavailable
	}
	_ = ctx

	operators, err := c.operatorLister.ListOperatorsForObservation()
	if err != nil {
		c.logger.Warn("Model provenance observation: list operators failed", "error", err)
		return err
	}
	selected, err := operatorcapability.SelectProvenanceOperator(operators, "")
	if err != nil {
		c.resetOperator()
		switch {
		case errors.Is(err, constants.ErrProvenanceOperatorNotFound):
			c.logger.Warn("Model provenance observation: operator not found", "reason", "not found")
		case errors.Is(err, constants.ErrProvenanceOperatorAmbiguous):
			c.logger.Warn("Model provenance observation: operator ambiguous", "reason", "ambiguous")
		default:
			c.logger.Warn("Model provenance observation: operator selection failed", "error", err)
		}
		return err
	}

	c.mu.Lock()
	cached := c.operator
	c.mu.Unlock()

	if cached != nil && cached.OperatorID == selected.OperatorID && cached.OperatorSessionID == selected.OperatorSessionID {
		return nil
	}

	if cached != nil {
		c.logger.Info("Model provenance observation: operator session changed; re-subscribing",
			"prior_operator_session_id", cached.OperatorSessionID,
			"operator_session_id", selected.OperatorSessionID)
	}
	c.resetOperator()

	operator := &modelProvenanceOperatorTarget{
		OperatorID:        selected.OperatorID,
		OperatorSessionID: selected.OperatorSessionID,
	}

	resultsChannel := pubsub.ResultsChannel(operator.OperatorID, operator.OperatorSessionID)
	handler := func(_ string, data []byte) {
		c.ingestResult(context.Background(), data)
	}
	unregister := c.pubsub.RegisterHandler(resultsChannel, handler)

	c.mu.Lock()
	c.operator = operator
	c.unregister = unregister
	c.mu.Unlock()

	c.logger.Info("Model provenance observation coordinator subscribed",
		"operator_id", operator.OperatorID,
		"operator_session_id", operator.OperatorSessionID,
		"results_channel", resultsChannel)
	return nil
}

func (c *ModelProvenanceObservationCoordinator) resetOperator() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.unregister != nil {
		c.unregister()
		c.unregister = nil
	}
	c.operator = nil
	c.mu.Unlock()
}

// Stop unsubscribes from the provenance operator results channel.
func (c *ModelProvenanceObservationCoordinator) Stop() {
	c.resetOperator()
}

// PreflightCommandDelivery verifies that the active provenance operator is
// enrolled and has at least one cmd-channel WebSocket subscriber.
func (c *ModelProvenanceObservationCoordinator) PreflightCommandDelivery(ctx context.Context) error {
	if err := c.ensureOperator(ctx); err != nil {
		return fmt.Errorf("model provenance observation preflight: %w", err)
	}
	c.mu.Lock()
	operator := c.operator
	c.mu.Unlock()
	if operator == nil {
		return fmt.Errorf("model provenance observation preflight: %w", constants.ErrProvenanceOperatorNotFound)
	}
	cmdChannel := pubsub.CmdChannel(operator.OperatorID, operator.OperatorSessionID)
	if c.pubsub == nil || c.pubsub.ChannelSubscriberCount(cmdChannel) == 0 {
		return fmt.Errorf("model provenance observation preflight: %w: cmd channel %s",
			constants.ErrEvaluationObservationUnavailable, cmdChannel)
	}
	return nil
}

// NotifyAttemptBegin delivers a BEGIN command with the expected model binding.
func (c *ModelProvenanceObservationCoordinator) NotifyAttemptBegin(
	ctx context.Context,
	_ string,
	providerAttemptID string,
	servedModelTag string,
	expectedModelDigest string,
	modelRegistryDigest string,
	campaignID string,
	startedAtUnixMs int64,
	retryCount uint32,
) error {
	if providerAttemptID == "" || servedModelTag == "" || expectedModelDigest == "" {
		return nil
	}
	if err := c.ensureOperator(ctx); err != nil {
		return fmt.Errorf("model provenance observation begin: %w", err)
	}
	command := &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:      providerAttemptID,
		Phase:                  evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_BEGIN,
		AttemptStartedAtUnixMs: startedAtUnixMs,
		RetryCount:             retryCount,
		ServedModelTag:         servedModelTag,
		ExpectedModelDigest:    expectedModelDigest,
		ModelRegistryDigest:    modelRegistryDigest,
		CampaignId:             campaignID,
	}
	return c.publishCommand(ctx, command)
}

// NotifyAttemptFinalize delivers a FINALIZE command to the provenance operator.
func (c *ModelProvenanceObservationCoordinator) NotifyAttemptFinalize(
	ctx context.Context,
	_ string,
	providerAttemptID string,
	inferenceTransactionID string,
	servedModelTag string,
	expectedModelDigest string,
	modelRegistryDigest string,
	campaignID string,
	startedAtUnixMs int64,
	completedAtUnixMs int64,
	failed bool,
	retryCount uint32,
) error {
	if providerAttemptID == "" {
		return nil
	}
	if err := c.ensureOperator(ctx); err != nil {
		return fmt.Errorf("model provenance observation finalize: %w", err)
	}
	status := evalv1.ModelProvenanceObservationAttemptStatus_MODEL_PROVENANCE_OBSERVATION_ATTEMPT_STATUS_COMPLETED
	if failed {
		status = evalv1.ModelProvenanceObservationAttemptStatus_MODEL_PROVENANCE_OBSERVATION_ATTEMPT_STATUS_FAILED
	}
	command := &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:        providerAttemptID,
		InferenceTransactionId:   inferenceTransactionID,
		Phase:                    evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE,
		AttemptStartedAtUnixMs:   startedAtUnixMs,
		AttemptCompletedAtUnixMs: completedAtUnixMs,
		AttemptStatus:            status,
		RetryCount:               retryCount,
		ServedModelTag:           servedModelTag,
		ExpectedModelDigest:      expectedModelDigest,
		ModelRegistryDigest:      modelRegistryDigest,
		CampaignId:               campaignID,
	}
	return c.publishCommand(ctx, command)
}

func (c *ModelProvenanceObservationCoordinator) publishCommand(ctx context.Context, command *evalv1.ModelProvenanceObservationCommand) error {
	c.mu.Lock()
	operator := c.operator
	c.mu.Unlock()
	if operator == nil || c.dispatch == nil {
		return constants.ErrMissingRequiredField
	}
	cmdChannel := pubsub.CmdChannel(operator.OperatorID, operator.OperatorSessionID)
	payload, err := proto.Marshal(command)
	if err != nil {
		c.logger.Warn("Model provenance observation command marshal failed", "error", err)
		return err
	}
	txID, err := c.dispatch.PublishCommand(ctx, PublishCommandRequest{
		TargetOperatorSessionID: operator.OperatorSessionID,
		ActionType:              string(constants.ActionTypeModelProvenanceObservation),
		Payload:                 payload,
	})
	if err != nil {
		c.logger.Warn("Model provenance observation command publish failed",
			"provider_attempt_id", command.GetProviderAttemptId(),
			"phase", command.GetPhase().String(),
			"operator_id", operator.OperatorID,
			"operator_session_id", operator.OperatorSessionID,
			"cmd_channel", cmdChannel,
			"error", err)
		return err
	}
	c.logger.Info("Model provenance observation command published",
		"transaction_id", txID,
		"provider_attempt_id", command.GetProviderAttemptId(),
		"phase", command.GetPhase().String(),
		"operator_id", operator.OperatorID,
		"operator_session_id", operator.OperatorSessionID,
		"cmd_channel", cmdChannel)
	return nil
}

func (c *ModelProvenanceObservationCoordinator) ingestResult(ctx context.Context, data []byte) {
	env := &commonv1.GovernanceEnvelope{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, env); err != nil {
		c.logger.Warn("Model provenance observation ingest: unmarshal envelope failed", "error", err)
		return
	}
	if env.GetEventType() != string(constants.Event.Operator.ModelProvenanceObservation.Completed) {
		return
	}
	completion := &evalv1.ModelProvenanceObservationCompleted{}
	if err := proto.Unmarshal(env.GetPayload(), completion); err != nil {
		c.logger.Warn("Model provenance observation ingest: unmarshal completion failed", "error", err)
		return
	}
	window := completion.GetWindow()
	if window == nil {
		return
	}
	if err := c.windows.Save(ctx, window); err != nil {
		c.logger.Warn("Model provenance observation ingest: save window failed",
			"provider_attempt_id", window.GetProviderAttemptId(),
			"error", err)
		return
	}
	c.logger.Info("Model provenance attestation window ingested",
		"provider_attempt_id", window.GetProviderAttemptId(),
		"served_model_tag", window.GetServedModelTag(),
		"observed_model_digest", window.GetObservedModelDigest(),
		"attestation_digest", window.GetAttestationDigest())
}
