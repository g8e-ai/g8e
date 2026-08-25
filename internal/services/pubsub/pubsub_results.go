// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// PubSubResultsService handles publishing results back to g8e-Compliant Agentic Ensemble via Operator pub/sub
type PubSubResultsService struct {
	client PubSubClient
	config *config.Config
	logger *slog.Logger
}

// NewPubSubResultsService creates a new Operator pub/sub results service
func NewPubSubResultsService(cfg *config.Config, logger *slog.Logger, client PubSubClient) (*PubSubResultsService, error) {
	return &PubSubResultsService{
		client: client,
		config: cfg,
		logger: logger,
	}, nil
}

// PublishExecutionResult publishes command execution result via Operator pub/sub
// Stdout/stderr have already been sentinel.Sentinel-scrubbed by pubsub_commands.go before this is called.
func (rr *PubSubResultsService) PublishExecutionResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	eventType := rr.determineEventStatus(result, constants.Event.Operator.Command.Completed, constants.Event.Operator.Command.Failed)

	caseID := originalMsg.CaseID
	taskID := originalMsg.TaskID
	investigationID := originalMsg.InvestigationID

	rr.logger.Info("Publishing execution result", "original_message_id", originalMsg.ID)
	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, caseID, taskID, investigationID, originalMsg, result); err != nil {
		return fmt.Errorf("pubsub: publish execution result: %w", err)
	}

	rr.logger.Info("Execution result transmitted to g8e",
		"operator_session_id", rr.config.OperatorSessionId,
		"event_type", eventType)
	return nil
}

// PublishCancellationResult publishes command cancellation result via Operator pub/sub
func (rr *PubSubResultsService) PublishCancellationResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	eventType := constants.Event.Operator.Command.Cancelled

	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("pubsub: publish cancellation result: %w", err)
	}

	rr.logger.Info("Cancellation result transmitted to g8e",
		"operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFileEditResult publishes file edit result via Operator pub/sub.
func (rr *PubSubResultsService) PublishFileEditResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	eventType := rr.determineEventStatus(result, constants.Event.Operator.FileEdit.Completed, constants.Event.Operator.FileEdit.Failed)

	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("pubsub: publish file edit result: %w", err)
	}

	rr.logger.Info("File operation result transmitted to g8e", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFsListResult publishes file system list result via Operator pub/sub.
func (rr *PubSubResultsService) PublishFsListResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	eventType := rr.determineEventStatus(result, constants.Event.Operator.FsList.Completed, constants.Event.Operator.FsList.Failed)

	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("pubsub: publish fs list result: %w", err)
	}

	rr.logger.Info("FS list result transmitted to g8e", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFsGrepResult publishes file system grep result via Operator pub/sub.
func (rr *PubSubResultsService) PublishFsGrepResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	eventType := rr.determineEventStatus(result, constants.Event.Operator.FsGrep.Completed, constants.Event.Operator.FsGrep.Failed)

	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("pubsub: publish fs grep result: %w", err)
	}

	rr.logger.Info("FS grep result transmitted to g8e", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishExecutionStatus publishes periodic status updates during command execution.
func (rr *PubSubResultsService) PublishExecutionStatus(ctx context.Context, status proto.Message, originalMsg *PubSubCommandMessage) error {
	reflectMsg := status.ProtoReflect()

	// Extract execution status and execution ID via reflection (payload-specific)
	var executionStatus protoreflect.EnumNumber
	var executionID string

	if fd := reflectMsg.Descriptor().Fields().ByName("status"); fd != nil {
		executionStatus = reflectMsg.Get(fd).Enum()
	}
	if fd := reflectMsg.Descriptor().Fields().ByName("execution_id"); fd != nil {
		executionID = reflectMsg.Get(fd).String()
	}

	eventType := constants.Event.Operator.Command.StatusUpdated.Running
	switch executionStatus {
	case protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED):
		eventType = constants.Event.Operator.Command.StatusUpdated.Queued
	case protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED):
		eventType = constants.Event.Operator.Command.StatusUpdated.Completed
	case protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED), protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT):
		eventType = constants.Event.Operator.Command.StatusUpdated.Failed
	case protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_CANCELLED):
		eventType = constants.Event.Operator.Command.StatusUpdated.Cancelled
	}

	// Use original message ID for correlation and context from originalMsg
	operatorID := rr.config.OperatorID
	if originalMsg.OperatorID != nil && *originalMsg.OperatorID != "" {
		operatorID = *originalMsg.OperatorID
	}
	env, err := BuildUniversalResultEnvelope(rr.config, eventType, status, originalMsg.ID, operatorID, originalMsg.CaseID, originalMsg.InvestigationID, originalMsg.TaskID, originalMsg.WebSessionID, originalMsg.CLISessionID)
	if err != nil {
		return fmt.Errorf("pubsub: build status envelope: %w", err)
	}

	if err := rr.publishUniversal(ctx, env, operatorID, originalMsg.OperatorSessionID); err != nil {
		return fmt.Errorf("pubsub: publish status update: %w", err)
	}

	rr.logger.Info("Execution status update transmitted", "event_type", eventType, "execution_id", executionID)
	return nil
}

// PublishHeartbeat publishes heartbeat to dedicated Operator pub/sub heartbeat channel.
// It wraps the heartbeat in a GovernanceEnvelope for consistency with other results.
func (rr *PubSubResultsService) PublishHeartbeat(ctx context.Context, heartbeat proto.Message) error {
	rr.logger.Info("Publishing heartbeat to Operator pub/sub")

	// Build the GovernanceEnvelope
	operatorSessionID := rr.config.OperatorSessionId

	env, err := BuildUniversalResultEnvelope(rr.config, constants.Event.Operator.Heartbeat, heartbeat, "", rr.config.OperatorID, "", "", nil, "", "")
	if err != nil {
		return fmt.Errorf("pubsub: build heartbeat envelope: %w", err)
	}

	data, err := protojson.Marshal(env)
	if err != nil {
		return fmt.Errorf("pubsub: marshal heartbeat envelope: %w", err)
	}

	channelName := HeartbeatChannel(rr.config.OperatorID, operatorSessionID)
	if err := rr.client.Publish(ctx, channelName, data); err != nil {
		return fmt.Errorf("pubsub: publish heartbeat: %w", err)
	}
	return nil
}

// PublishActionReceipt publishes a signed ActionReceipt to the gateway's
// receipts:<operator_id>:<operator_session_id> channel. The receipt is wrapped
// as the payload of a GovernanceEnvelope that copies the identity fields
// (operator_id, operator_session_id, requestor_user_id, acting_app_id,
// action_type, target_resource, case_id, investigation_id, task_id) from the
// original command envelope env, so the gateway can build a complete
// ActionReceiptRecord without the ActionReceipt proto carrying those fields.
// The gateway intercepts the publish, verifies the receipt signature against
// the operator's actuator public key, records the receipt in its
// SQLAuditStore, and fans out the envelope to subscribers.
func (rr *PubSubResultsService) PublishActionReceipt(ctx context.Context, env *commonv1.GovernanceEnvelope, receipt *operatorv1.ActionReceipt) error {
	if env == nil || receipt == nil {
		return fmt.Errorf("pubsub: publish action receipt: %w", constants.ErrMissingRequiredField)
	}

	receiptBytes, err := proto.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("pubsub: publish action receipt: marshal receipt: %w", err)
	}

	receiptEnv := &commonv1.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         env.Timestamp,
		SourceComponent:   commonv1.Component_COMPONENT_G8EO,
		OperatorId:        env.OperatorId,
		OperatorSessionId: env.OperatorSessionId,
		ActionType:        env.ActionType,
		TargetResource:    env.TargetResource,
		EventType:         string(constants.Event.Operator.Receipt.Recorded),
		Payload:           receiptBytes,
		RequestorUserId:   env.RequestorUserId,
		ActingAppId:       env.ActingAppId,
		CaseId:            env.CaseId,
		InvestigationId:   env.InvestigationId,
		TaskId:            env.TaskId,
		WebSessionId:      env.WebSessionId,
		CliSessionId:      env.CliSessionId,
		Posture:           env.Posture,
	}

	wire, err := protojson.Marshal(receiptEnv)
	if err != nil {
		return fmt.Errorf("pubsub: publish action receipt: marshal envelope: %w", err)
	}

	channel := ReceiptsChannel(env.OperatorId, env.OperatorSessionId)
	rr.logger.Info("Publishing action receipt to gateway receipts channel",
		"channel", channel,
		"transaction_id", receipt.TransactionId,
		"event_type", receiptEnv.EventType)
	if err := rr.client.Publish(ctx, channel, wire); err != nil {
		return fmt.Errorf("pubsub: publish action receipt: %w", err)
	}
	return nil
}

// publishUniversal marshals a GovernanceEnvelope as protojson and publishes it to the results channel.
// operatorID overrides rr.config.OperatorID for channel routing (e.g. gateway mode where config has no operator ID).
func (rr *PubSubResultsService) publishUniversal(ctx context.Context, env *commonv1.GovernanceEnvelope, operatorID, operatorSessionID string) error {
	data, err := protojson.Marshal(env)
	if err != nil {
		return fmt.Errorf("pubsub: marshal envelope: %w", err)
	}
	if operatorID == "" {
		operatorID = rr.config.OperatorID
	}
	channel := ResultsChannel(operatorID, operatorSessionID)
	rr.logger.Info("Publishing result",
		"channel", channel,
		"event_type", env.EventType,
		"id", env.Id)
	return rr.client.Publish(ctx, channel, data)
}

// determineEventStatus determines the event type based on the status field of a proto message via reflection.
// Returns failedEventType if the status is FAILED or TIMEOUT, otherwise returns completedEventType.
func (rr *PubSubResultsService) determineEventStatus(result proto.Message, completedEventType, failedEventType constants.EventType) constants.EventType {
	reflectMsg := result.ProtoReflect()
	statusFd := reflectMsg.Descriptor().Fields().ByName("status")
	if statusFd != nil {
		status := reflectMsg.Get(statusFd).Enum()
		if status == protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED) || status == protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT) {
			return failedEventType
		}
	}
	return completedEventType
}

// publishResultEnvelopeUniversal builds a GovernanceEnvelope for result publishing.
func (rr *PubSubResultsService) publishResultEnvelopeUniversal(
	ctx context.Context,
	eventType constants.EventType,
	caseID string,
	taskID *string,
	investigationID string,
	originalMsg *PubSubCommandMessage,
	payload proto.Message,
) error {
	// Use original message ID for correlation
	originalMessageID := originalMsg.ID
	senderID := rr.config.OperatorID
	if originalMsg.OperatorID != nil {
		senderID = *originalMsg.OperatorID
	}

	env, err := BuildUniversalResultEnvelope(rr.config, eventType, payload, originalMessageID, senderID, caseID, investigationID, taskID, originalMsg.WebSessionID, originalMsg.CLISessionID)
	if err != nil {
		return fmt.Errorf("pubsub: build result envelope: %w", err)
	}

	return rr.publishUniversal(ctx, env, senderID, originalMsg.OperatorSessionID)
}
