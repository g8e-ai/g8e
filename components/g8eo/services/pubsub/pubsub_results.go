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

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
)

func (rr *PubSubResultsService) resultsChannel(operatorSessionID string) string {
	return constants.ResultsChannel(rr.config.OperatorID, operatorSessionID)
}

// isUAPMessageID checks if a message ID is a UAP MessageID (64-character hex SHA-256 hash).
func isUAPMessageID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// PubSubResultsService handles publishing results back to AI Agent Services via operator pub/sub
type PubSubResultsService struct {
	client     PubSubClient
	config     *config.Config
	logger     *slog.Logger
	localStore *storage.LocalStoreService
}

// NewPubSubResultsService creates a new operator pub/sub results service
func NewPubSubResultsService(cfg *config.Config, logger *slog.Logger, client PubSubClient, localStore *storage.LocalStoreService) (*PubSubResultsService, error) {
	return &PubSubResultsService{
		client:     client,
		config:     cfg,
		logger:     logger,
		localStore: localStore,
	}, nil
}

// publishResultEnvelope is DEPRECATED and now REJECTS legacy envelopes.
func (rr *PubSubResultsService) publishResultEnvelope(
	ctx context.Context,
	eventType, caseID string,
	taskID *string,
	investigationID string,
	originalMsg PubSubCommandMessage,
	payload proto.Message,
) error {
	return fmt.Errorf("publishResultEnvelope(GovernanceEnvelope) is deprecated and no longer supported")
}

// PublishExecutionResult publishes command execution result via operator pub/sub
// Stdout/stderr have already been sentinel.Sentinel-scrubbed by pubsub_commands.go before this is called.
func (rr *PubSubResultsService) PublishExecutionResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	// Determine event type based on status field via reflection
	eventType := constants.Event.Operator.Command.Completed

	reflectMsg := result.ProtoReflect()
	statusFd := reflectMsg.Descriptor().Fields().ByName("status")
	if statusFd != nil {
		status := reflectMsg.Get(statusFd).Enum()
		if status == protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED) || status == protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT) {
			eventType = constants.Event.Operator.Command.Failed
		}
	}

	caseID := originalMsg.CaseID
	taskID := originalMsg.TaskID
	investigationID := originalMsg.InvestigationID

	rr.logger.Info("Publishing result via UAP", "original_message_id", originalMsg.ID)
	if err := rr.publishResultEnvelopeUAP(ctx, eventType, caseID, taskID, investigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish UAP result: %w", err)
	}

	rr.logger.Info("Result transmitted to g8e (UAP)",
		"operator_session_id", rr.config.OperatorSessionId,
		"event_type", eventType)
	return nil
}

// PublishCancellationResult publishes command cancellation result via operator pub/sub
func (rr *PubSubResultsService) PublishCancellationResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.Command.Cancelled

	if err := rr.publishResultEnvelopeUAP(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish UAP cancellation result: %w", err)
	}

	rr.logger.Info("Cancellation result transmitted to g8e (UAP)",
		"operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFileEditResult publishes file edit result via operator pub/sub.
func (rr *PubSubResultsService) PublishFileEditResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.FileEdit.Completed

	reflectMsg := result.ProtoReflect()
	statusFd := reflectMsg.Descriptor().Fields().ByName("status")
	if statusFd != nil {
		status := reflectMsg.Get(statusFd).Enum()
		if status == protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED) {
			eventType = constants.Event.Operator.FileEdit.Failed
		}
	}

	if err := rr.publishResultEnvelopeUAP(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish UAP file edit result: %w", err)
	}

	rr.logger.Info("File operation result transmitted (UAP)", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFsListResult publishes file system list result via operator pub/sub.
func (rr *PubSubResultsService) PublishFsListResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.FsList.Completed

	reflectMsg := result.ProtoReflect()
	statusFd := reflectMsg.Descriptor().Fields().ByName("status")
	if statusFd != nil {
		status := reflectMsg.Get(statusFd).Enum()
		if status == protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED) {
			eventType = constants.Event.Operator.FsList.Failed
		}
	}

	if err := rr.publishResultEnvelopeUAP(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish UAP fs list result: %w", err)
	}

	rr.logger.Info("FS list result transmitted (UAP)", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFsGrepResult publishes file system grep result via operator pub/sub.
func (rr *PubSubResultsService) PublishFsGrepResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.FsGrep.Completed

	reflectMsg := result.ProtoReflect()
	statusFd := reflectMsg.Descriptor().Fields().ByName("status")
	if statusFd != nil {
		status := reflectMsg.Get(statusFd).Enum()
		if status == protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED) {
			eventType = constants.Event.Operator.FsGrep.Failed
		}
	}

	if err := rr.publishResultEnvelopeUAP(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish UAP fs grep result: %w", err)
	}

	rr.logger.Info("FS grep result transmitted (UAP)", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishExecutionStatus publishes periodic status updates during command execution.
func (rr *PubSubResultsService) PublishExecutionStatus(ctx context.Context, status proto.Message) error {
	reflectMsg := status.ProtoReflect()

	// Extract fields via reflection for envelope routing
	var caseID string
	var taskID *string
	var investigationID string
	var operatorSessionID string
	var executionStatus protoreflect.EnumNumber
	var executionID string

	if fd := reflectMsg.Descriptor().Fields().ByName("case_id"); fd != nil {
		caseID = reflectMsg.Get(fd).String()
	}
	if fd := reflectMsg.Descriptor().Fields().ByName("task_id"); fd != nil {
		val := reflectMsg.Get(fd).String()
		taskID = &val
	}
	if fd := reflectMsg.Descriptor().Fields().ByName("investigation_id"); fd != nil {
		investigationID = reflectMsg.Get(fd).String()
	}
	if fd := reflectMsg.Descriptor().Fields().ByName("operator_session_id"); fd != nil {
		operatorSessionID = reflectMsg.Get(fd).String()
	}
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

	// Use executionID for UAP MessageID correlation if available
	actionType := mapEventTypeToResultActionType(eventType)
	env, err := BuildUAPResultEnvelope(rr.config, actionType, status, executionID, rr.config.OperatorID, caseID, investigationID, taskID)
	if err != nil {
		return fmt.Errorf("failed to build UAP status envelope: %w", err)
	}

	if err := rr.publishUAP(ctx, env, operatorSessionID); err != nil {
		return fmt.Errorf("failed to publish UAP status update: %w", err)
	}

	rr.logger.Info("Execution status update transmitted (UAP)", "event_type", eventType)
	return nil
}

// PublishHeartbeat publishes heartbeat to dedicated operator pub/sub heartbeat channel.
// It wraps the heartbeat in a UAPEnvelope for consistency with other results.
func (rr *PubSubResultsService) PublishHeartbeat(ctx context.Context, heartbeat proto.Message) error {
	rr.logger.Info("[HEARTBEAT] Publishing heartbeat to operator pub/sub (UAP)")

	// Build the UAP envelope
	reflectMsg := heartbeat.ProtoReflect()
	operatorSessionID := rr.config.OperatorSessionId
	if fd := reflectMsg.Descriptor().Fields().ByName("operator_session_id"); fd != nil {
		val := reflectMsg.Get(fd).String()
		if val != "" {
			operatorSessionID = val
		}
	}

	env, err := BuildUAPResultEnvelope(rr.config, "HEARTBEAT_RESULT", heartbeat, "", rr.config.OperatorID, "", "", nil)
	if err != nil {
		return fmt.Errorf("failed to build UAP heartbeat envelope: %w", err)
	}

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal UAP heartbeat envelope: %w", err)
	}

	channelName := constants.HeartbeatChannel(rr.config.OperatorID, operatorSessionID)
	if err := rr.client.Publish(ctx, channelName, data); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	return nil
}

// PublishResult publishes a pre-built GovernanceEnvelope to the operator pub/sub results channel.
// This is DEPRECATED and now REJECTS legacy envelopes.
func (rr *PubSubResultsService) PublishResult(ctx context.Context, env *commonv1.GovernanceEnvelope) error {
	return fmt.Errorf("PublishResult(GovernanceEnvelope) is deprecated and no longer supported")
}

// publish marshals a GovernanceEnvelope and publishes it to the results channel.
// This is DEPRECATED and now REJECTS legacy envelopes.
func (rr *PubSubResultsService) publish(ctx context.Context, env *commonv1.GovernanceEnvelope) error {
	return fmt.Errorf("publish(GovernanceEnvelope) is deprecated and no longer supported")
}

// publishUAP marshals a UAPEnvelope as JSON and publishes it to the results channel.
func (rr *PubSubResultsService) publishUAP(ctx context.Context, env *uap.UAPEnvelope, operatorSessionID string) error {
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal UAP envelope: %w", err)
	}
	channel := rr.resultsChannel(operatorSessionID)
	rr.logger.Info("Publishing result (UAP)",
		"channel", channel,
		"action_type", env.Intent.ActionType,
		"message_id", env.MessageID)
	return rr.client.Publish(ctx, channel, data)
}

// publishResultEnvelopeUAP builds a UAPEnvelope for result publishing.
// This is the Phase 3.2 UAP migration path for result publishing.
func (rr *PubSubResultsService) publishResultEnvelopeUAP(
	ctx context.Context,
	eventType, caseID string,
	taskID *string,
	investigationID string,
	originalMsg PubSubCommandMessage,
	payload proto.Message,
) error {
	// Map event type to UAP action type
	actionType := mapEventTypeToResultActionType(eventType)

	// Use original message ID for correlation (if it looks like a UAP MessageID)
	originalMessageID := originalMsg.ID
	senderID := rr.config.OperatorID
	if originalMsg.OperatorID != nil {
		senderID = *originalMsg.OperatorID
	}

	env, err := BuildUAPResultEnvelope(rr.config, actionType, payload, originalMessageID, senderID, caseID, investigationID, taskID)
	if err != nil {
		return fmt.Errorf("failed to build UAP envelope: %w", err)
	}

	return rr.publishUAP(ctx, env, originalMsg.OperatorSessionID)
}
