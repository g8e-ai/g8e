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

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	storage "github.com/g8e-ai/g8e/services/g8eo/internal/services/storage"
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

	rr.logger.Info("Publishing result via Universal", "original_message_id", originalMsg.ID)
	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, caseID, taskID, investigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish Universal result: %w", err)
	}

	rr.logger.Info("Result transmitted to g8e (Universal)",
		"operator_session_id", rr.config.OperatorSessionId,
		"event_type", eventType)
	return nil
}

// PublishCancellationResult publishes command cancellation result via operator pub/sub
func (rr *PubSubResultsService) PublishCancellationResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.Command.Cancelled

	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish Universal cancellation result: %w", err)
	}

	rr.logger.Info("Cancellation result transmitted to g8e (Universal)",
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

	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish Universal file edit result: %w", err)
	}

	rr.logger.Info("File operation result transmitted (Universal)", "operator_session_id", rr.config.OperatorSessionId)
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

	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish Universal fs list result: %w", err)
	}

	rr.logger.Info("FS list result transmitted (Universal)", "operator_session_id", rr.config.OperatorSessionId)
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

	if err := rr.publishResultEnvelopeUniversal(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish Universal fs grep result: %w", err)
	}

	rr.logger.Info("FS grep result transmitted (Universal)", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishExecutionStatus publishes periodic status updates during command execution.
func (rr *PubSubResultsService) PublishExecutionStatus(ctx context.Context, status proto.Message, originalMsg PubSubCommandMessage) error {
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
	env, err := BuildUniversalResultEnvelope(rr.config, eventType, status, originalMsg.ID, rr.config.OperatorID, originalMsg.CaseID, originalMsg.InvestigationID, originalMsg.TaskID, originalMsg.WebSessionID, originalMsg.CLISessionID)
	if err != nil {
		return fmt.Errorf("failed to build Universal status envelope: %w", err)
	}

	if err := rr.publishUniversal(ctx, env, originalMsg.OperatorSessionID); err != nil {
		return fmt.Errorf("failed to publish Universal status update: %w", err)
	}

	rr.logger.Info("Execution status update transmitted (UAP)", "event_type", eventType, "execution_id", executionID)
	return nil
}

// PublishHeartbeat publishes heartbeat to dedicated operator pub/sub heartbeat channel.
// It wraps the heartbeat in a UAPEnvelope for consistency with other results.
func (rr *PubSubResultsService) PublishHeartbeat(ctx context.Context, heartbeat proto.Message) error {
	rr.logger.Info("[HEARTBEAT] Publishing heartbeat to operator pub/sub (UAP)")

	// Build the UAP envelope
	operatorSessionID := rr.config.OperatorSessionId

	env, err := BuildUniversalResultEnvelope(rr.config, "HEARTBEAT_RESULT", heartbeat, "", rr.config.OperatorID, "", "", nil, "", "")
	if err != nil {
		return fmt.Errorf("failed to build Universal heartbeat envelope: %w", err)
	}

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal Universal heartbeat envelope: %w", err)
	}

	channelName := constants.HeartbeatChannel(rr.config.OperatorID, operatorSessionID)
	if err := rr.client.Publish(ctx, channelName, data); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	return nil
}

// publishUniversal marshals a GovernanceEnvelope as JSON and publishes it to the results channel.
func (rr *PubSubResultsService) publishUniversal(ctx context.Context, env *commonv1.GovernanceEnvelope, operatorSessionID string) error {
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal Governance Envelope: %w", err)
	}
	channel := rr.resultsChannel(operatorSessionID)
	rr.logger.Info("Publishing result (Universal)",
		"channel", channel,
		"event_type", env.EventType,
		"id", env.Id)
	return rr.client.Publish(ctx, channel, data)
}

// publishResultEnvelopeUniversal builds a GovernanceEnvelope for result publishing.
func (rr *PubSubResultsService) publishResultEnvelopeUniversal(
	ctx context.Context,
	eventType constants.EventType,
	caseID string,
	taskID *string,
	investigationID string,
	originalMsg PubSubCommandMessage,
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
		return fmt.Errorf("failed to build Governance Envelope: %w", err)
	}

	return rr.publishUniversal(ctx, env, originalMsg.OperatorSessionID)
}
