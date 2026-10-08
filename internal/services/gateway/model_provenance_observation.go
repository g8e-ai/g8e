// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// modelProvenanceOperatorLister resolves operators capable of storage-side model
// provenance attestation. Provenance operators are enrolled under the gateway owner.
type modelProvenanceOperatorLister interface {
	ListOperatorsForProvenance(ctx context.Context) ([]*operatorv1.OperatorDocument, error)
}

// ModelProvenanceObservationCoordinator fans out BEGIN/FINALIZE provenance
// commands to the remote storage-side Provenance Operator and ingests
// completed attestation windows on the campaign Gateway host.
type ModelProvenanceObservationCoordinator struct {
	dispatch       *DispatchService
	operatorLister modelProvenanceOperatorLister
	pubsub         *GatewayWebSocketHandler
	windows        model_provenance.WindowStore
	logger         *slog.Logger

	mu         sync.Mutex
	unregister func()
	operator   *modelProvenanceOperatorTarget
	probes     map[string]chan modelProvenanceProbeResult
}

type modelProvenanceProbeResult struct {
	window *evalv1.ModelProvenanceAttestationWindow
	err    error
}

type modelProvenanceOperatorTarget struct {
	OperatorID        string
	OperatorSessionID string
}

// NewModelProvenanceObservationCoordinator constructs the gateway-side model
// provenance observation coordinator.
func NewModelProvenanceObservationCoordinator(
	dispatch *DispatchService,
	operatorLister modelProvenanceOperatorLister,
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

func (c *ModelProvenanceObservationCoordinator) synchronizeOperatorSubscription(ctx context.Context) error {
	if c == nil || c.dispatch == nil || c.operatorLister == nil || c.pubsub == nil || c.windows == nil {
		return constants.ErrServiceUnavailable
	}
	_ = ctx

	operators, err := c.operatorLister.ListOperatorsForProvenance(ctx)
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
	if err := c.synchronizeOperatorSubscription(ctx); err != nil {
		return fmt.Errorf("model provenance observation preflight: %w", err)
	}
	c.mu.Lock()
	operator := c.operator
	c.mu.Unlock()
	if operator == nil {
		return fmt.Errorf("model provenance observation preflight: %w", constants.ErrProvenanceOperatorNotFound)
	}
	cmdChannel := pubsub.CmdChannel(operator.OperatorID, operator.OperatorSessionID)
	if !c.dispatch.commandDeliverable(operator.OperatorID, cmdChannel, c.pubsub) {
		return fmt.Errorf("model provenance observation preflight: %w: cmd channel %s",
			constants.ErrEvaluationObservationUnavailable, cmdChannel)
	}
	return nil
}

// PreflightStorageAttestation verifies that the active provenance operator can
// read and attest the expected Ollama manifest for one served model tag before
// campaign execute burns scored assignments. The returned window is keyed to a
// preflight probe attempt id and must be rebound to inference provider_attempt_id
// before execute-time witness persistence.
func (c *ModelProvenanceObservationCoordinator) PreflightStorageAttestation(ctx context.Context, servedModelTag, expectedModelDigest string) (*evalv1.ModelProvenanceAttestationWindow, error) {
	return c.PreflightStorageAttestationWithProgress(ctx, servedModelTag, expectedModelDigest, nil)
}

// PreflightStorageAttestationWithProgress waits on pub/sub receipt and completion
// events. It never polls the window store. Progress is telemetry; only a persisted
// attestation window is returned as evidence.
func (c *ModelProvenanceObservationCoordinator) PreflightStorageAttestationWithProgress(ctx context.Context, servedModelTag, expectedModelDigest string, progress func(string)) (*evalv1.ModelProvenanceAttestationWindow, error) {
	if servedModelTag == "" || expectedModelDigest == "" {
		return nil, fmt.Errorf("model provenance attestation preflight: %w", constants.ErrMissingRequiredField)
	}
	if !models.IsSHA256Hex(expectedModelDigest) {
		return nil, fmt.Errorf("model provenance attestation preflight: invalid expected digest")
	}
	if err := c.PreflightCommandDelivery(ctx); err != nil {
		return nil, fmt.Errorf("model provenance attestation preflight: %w", err)
	}
	probeID, err := newModelProvenancePreflightAttemptID()
	if err != nil {
		return nil, err
	}
	completed := make(chan modelProvenanceProbeResult, 1)
	c.mu.Lock()
	operator := c.operator
	if c.probes == nil {
		c.probes = make(map[string]chan modelProvenanceProbeResult)
	}
	c.probes[probeID] = completed
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.probes, probeID); c.mu.Unlock() }()
	if operator == nil {
		return nil, constants.ErrProvenanceOperatorNotFound
	}
	emit := func(phase string) {
		if progress != nil {
			progress(phase)
		}
	}

	// Register before dispatch; a receipt may arrive before PublishCommand
	// returns its transaction ID. Remote receipts are verified by the broker.
	var receiptMu sync.Mutex
	receipts := make(map[string]*operatorv1.ActionReceipt)
	receiptReady := make(chan struct{}, 1)
	unregister := c.pubsub.RegisterHandler(pubsub.ReceiptsChannel(operator.OperatorID, operator.OperatorSessionID), func(_ string, data []byte) {
		env := &commonv1.GovernanceEnvelope{}
		if err := protojson.Unmarshal(data, env); err != nil ||
			env.GetEventType() != string(constants.Event.Operator.Receipt.Recorded) ||
			env.GetOperatorId() != operator.OperatorID || env.GetOperatorSessionId() != operator.OperatorSessionID {
			return
		}
		receipt := &operatorv1.ActionReceipt{}
		if err := proto.Unmarshal(env.GetPayload(), receipt); err != nil {
			return
		}
		receiptMu.Lock()
		receipts[receipt.GetTransactionId()] = receipt
		receiptMu.Unlock()
		select {
		case receiptReady <- struct{}{}:
		default:
		}
	})
	defer unregister()
	getReceipt := func(txID string) *operatorv1.ActionReceipt {
		receiptMu.Lock()
		defer receiptMu.Unlock()
		return receipts[txID]
	}
	failure := func(txID, phase string) error {
		receipt := getReceipt(txID)
		if receipt == nil || receipt.GetStatus() != operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED {
			return nil
		}
		return fmt.Errorf("model provenance attestation preflight: %w: operator %s failed %s for %q (transaction %s): %s (verify --model-storage-root and Ollama manifest layout)",
			constants.ErrEvaluationObservationUnavailable, operator.OperatorID, phase, servedModelTag, txID, receipt.GetResultSummary())
	}
	probeCtx, cancel := context.WithTimeout(ctx, constants.ModelProvenanceAttestationPreflightTimeout)
	defer cancel()
	now := time.Now().UTC().UnixMilli()
	emit("awaiting_operator")
	beginTxID, err := c.publishCommandTransaction(probeCtx, &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:      probeID,
		Phase:                  evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_BEGIN,
		AttemptStartedAtUnixMs: now, ServedModelTag: servedModelTag, ExpectedModelDigest: expectedModelDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("model provenance attestation preflight: begin: %w", err)
	}
	ack := time.NewTimer(constants.ModelProvenanceCommandAcknowledgementTimeout)
	defer ack.Stop()
	for {
		if err := failure(beginTxID, "BEGIN"); err != nil {
			return nil, err
		}
		if receipt := getReceipt(beginTxID); receipt != nil && receipt.GetStatus() == operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
			break
		}
		select {
		case <-probeCtx.Done():
			return nil, probeCtx.Err()
		case <-receiptReady:
		case <-ack.C:
			return nil, fmt.Errorf("model provenance attestation preflight: %w: operator %s did not acknowledge BEGIN for %q within %s (transaction %s; check operator connection and logs)",
				constants.ErrEvaluationObservationUnavailable, operator.OperatorID, servedModelTag, constants.ModelProvenanceCommandAcknowledgementTimeout, beginTxID)
		}
	}
	emit("attesting_storage")
	finalizeTxID, err := c.publishCommandTransaction(probeCtx, &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:      probeID,
		Phase:                  evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE,
		AttemptStartedAtUnixMs: now, AttemptCompletedAtUnixMs: time.Now().UTC().UnixMilli(),
		AttemptStatus:  evalv1.ModelProvenanceObservationAttemptStatus_MODEL_PROVENANCE_OBSERVATION_ATTEMPT_STATUS_COMPLETED,
		ServedModelTag: servedModelTag, ExpectedModelDigest: expectedModelDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("model provenance attestation preflight: finalize: %w", err)
	}
	for {
		if err := failure(beginTxID, "BEGIN"); err != nil {
			return nil, err
		}
		if err := failure(finalizeTxID, "FINALIZE"); err != nil {
			return nil, err
		}
		select {
		case result := <-completed:
			if result.err != nil {
				return nil, result.err
			}
			window := result.window
			if window.GetServedModelTag() != servedModelTag || window.GetExpectedModelDigest() != expectedModelDigest {
				return nil, fmt.Errorf("model provenance attestation preflight: probe model binding mismatch")
			}
			if !window.GetDigestMatch() {
				return nil, fmt.Errorf("model provenance attestation preflight: digest mismatch for %q (observed %s)", servedModelTag, window.GetObservedModelDigest())
			}
			emit("ready")
			return window, nil
		case <-receiptReady:
		case <-probeCtx.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("model provenance attestation preflight: %w: operator %s did not complete storage attestation for %q within %s (transaction %s; check operator logs and storage throughput)",
				constants.ErrEvaluationObservationUnavailable, operator.OperatorID, servedModelTag, constants.ModelProvenanceAttestationPreflightTimeout, finalizeTxID)
		}
	}
}

func newModelProvenancePreflightAttemptID() (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return "preflight-provenance-" + hex.EncodeToString(suffix[:]), nil
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
	if err := c.synchronizeOperatorSubscription(ctx); err != nil {
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
	if err := c.synchronizeOperatorSubscription(ctx); err != nil {
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
	_, err := c.publishCommandTransaction(ctx, command)
	return err
}

func (c *ModelProvenanceObservationCoordinator) publishCommandTransaction(ctx context.Context, command *evalv1.ModelProvenanceObservationCommand) (string, error) {
	c.mu.Lock()
	operator := c.operator
	c.mu.Unlock()
	if operator == nil || c.dispatch == nil {
		return "", constants.ErrMissingRequiredField
	}
	cmdChannel := pubsub.CmdChannel(operator.OperatorID, operator.OperatorSessionID)
	payload, err := proto.Marshal(command)
	if err != nil {
		c.logger.Warn("Model provenance observation command marshal failed", "error", err)
		return "", err
	}
	txID, err := c.dispatch.PublishCommand(ctx, PublishCommandRequest{
		TargetOperatorSessionID: operator.OperatorSessionID,
		EventType:               string(constants.Event.Operator.ModelProvenanceObservation.Requested),
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
		return "", err
	}
	c.logger.Info("Model provenance observation command published",
		"transaction_id", txID,
		"provider_attempt_id", command.GetProviderAttemptId(),
		"phase", command.GetPhase().String(),
		"operator_id", operator.OperatorID,
		"operator_session_id", operator.OperatorSessionID,
		"cmd_channel", cmdChannel)
	return txID, nil
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
	saveErr := c.windows.Save(ctx, window)
	c.mu.Lock()
	waiter := c.probes[window.GetProviderAttemptId()]
	c.mu.Unlock()
	if waiter != nil {
		result := modelProvenanceProbeResult{window: window}
		if saveErr != nil {
			result.err = fmt.Errorf("model provenance attestation preflight: persist window: %w", saveErr)
		}
		select {
		case waiter <- result:
		default:
		}
	}
	if err := saveErr; err != nil {
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
