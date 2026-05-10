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
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
)

func (rr *PubSubResultsService) resultsChannel(operatorSessionID string) string {
	return constants.ResultsChannel(rr.config.OperatorID, operatorSessionID)
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

// publishResultEnvelope builds a UniversalEnvelope for a typed result payload,
// stamps the envelope metadata copied from the originating command, and
// publishes it on the results channel. All result-publishing paths that carry
// an originalMsg (command-completed, command-cancelled, file-edit, fs-list)
// share this helper so none can drift out of sync on API key, task/investigation
// propagation.
func (rr *PubSubResultsService) publishResultEnvelope(
	ctx context.Context,
	eventType, caseID string,
	taskID *string,
	investigationID string,
	originalMsg PubSubCommandMessage,
	payload proto.Message,
) error {
	env, err := BuildUniversalEnvelope(rr.config, eventType, payload, "")
	if err != nil {
		return fmt.Errorf("failed to build %s envelope: %w", eventType, err)
	}

	// Override envelope metadata from original message if not present in payload
	if env.CaseId == "" {
		env.CaseId = caseID
	}
	if env.TaskId == "" && taskID != nil {
		env.TaskId = *taskID
	}
	if env.InvestigationId == "" {
		env.InvestigationId = investigationID
	}
	env.OperatorSessionId = originalMsg.OperatorSessionID

	return rr.publish(ctx, env)
}

// PublishExecutionResult publishes command execution result via operator pub/sub
// Stdout/stderr have already been sentinel.Sentinel-scrubbed by pubsub_commands.go before this is called.
func (rr *PubSubResultsService) PublishExecutionResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	// If it's already a proto.Message, we just publish it.
	// We assume the caller has already populated it correctly.

	// Determine event type based on status field via reflection if needed,
	// or assume the caller knows. But ResultsPublisher methods are specific.
	eventType := constants.Event.Operator.Command.Completed

	reflectMsg := result.ProtoReflect()
	statusFd := reflectMsg.Descriptor().Fields().ByName("status")
	if statusFd != nil {
		status := reflectMsg.Get(statusFd).Enum()
		if status == protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED) || status == protoreflect.EnumNumber(operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT) {
			eventType = constants.Event.Operator.Command.Failed
		}
	}

	caseID := ""
	taskID := originalMsg.TaskID
	investigationID := originalMsg.InvestigationID

	// Try to get CaseID from original message if not in result
	caseID = originalMsg.CaseID

	if err := rr.publishResultEnvelope(ctx, eventType, caseID, taskID, investigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish result: %w", err)
	}

	rr.logger.Info("Result transmitted to g8e (Protocol-First)",
		"operator_session_id", rr.config.OperatorSessionId,
		"event_type", eventType)
	return nil
}

// PublishCancellationResult publishes command cancellation result via operator pub/sub
func (rr *PubSubResultsService) PublishCancellationResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.Command.Cancelled

	if err := rr.publishResultEnvelope(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish cancellation result: %w", err)
	}

	rr.logger.Info("Cancellation result transmitted to g8e",
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

	if err := rr.publishResultEnvelope(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish file edit result: %w", err)
	}

	rr.logger.Info("File operation result transmitted", "operator_session_id", rr.config.OperatorSessionId)
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

	if err := rr.publishResultEnvelope(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish fs list result: %w", err)
	}

	rr.logger.Info("FS list result transmitted", "operator_session_id", rr.config.OperatorSessionId)
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

	if err := rr.publishResultEnvelope(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish fs grep result: %w", err)
	}

	rr.logger.Info("FS grep result transmitted", "operator_session_id", rr.config.OperatorSessionId)
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

	env, err := BuildUniversalEnvelope(rr.config, eventType, status, "")
	if err != nil {
		return fmt.Errorf("failed to build status envelope: %w", err)
	}

	// Override envelope metadata from payload if present
	if caseID != "" {
		env.CaseId = caseID
	}
	if taskID != nil {
		env.TaskId = *taskID
	}
	if investigationID != "" {
		env.InvestigationId = investigationID
	}
	if operatorSessionID != "" {
		env.OperatorSessionId = operatorSessionID
	}

	if err := rr.publish(ctx, env); err != nil {
		return fmt.Errorf("failed to publish status update: %w", err)
	}

	rr.logger.Info("Execution status update transmitted (Protocol-First)",
		"event_type", eventType)
	return nil
}

// PublishHeartbeat publishes heartbeat to dedicated operator pub/sub heartbeat channel.
// It wraps the heartbeat in a UniversalEnvelope for consistency with other results.
func (rr *PubSubResultsService) PublishHeartbeat(ctx context.Context, heartbeat proto.Message) error {
	rr.logger.Info("[HEARTBEAT] Publishing heartbeat to operator pub/sub (Protocol-First)")

	// Build the envelope
	env, err := BuildUniversalEnvelope(rr.config, constants.Event.Operator.Heartbeat, heartbeat, "")
	if err != nil {
		return fmt.Errorf("failed to build heartbeat envelope: %w", err)
	}

	// Ensure operatorSessionID is correct in the envelope
	reflectMsg := heartbeat.ProtoReflect()
	if fd := reflectMsg.Descriptor().Fields().ByName("operator_session_id"); fd != nil {
		val := reflectMsg.Get(fd).String()
		if val != "" {
			env.OperatorSessionId = val
		}
	}

	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat envelope: %w", err)
	}

	channelName := constants.HeartbeatChannel(rr.config.OperatorID, env.OperatorSessionId)
	if err := rr.client.Publish(ctx, channelName, data); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	return nil
}

// PublishResult publishes a pre-built UniversalEnvelope to the operator pub/sub results channel.
func (rr *PubSubResultsService) PublishResult(ctx context.Context, env *commonv1.UniversalEnvelope) error {
	if env.OperatorSessionId == "" {
		env.OperatorSessionId = rr.config.OperatorSessionId
	}
	if env.OperatorId == "" {
		env.OperatorId = rr.config.OperatorID
	}
	return rr.publish(ctx, env)
}

// publish marshals a UniversalEnvelope and publishes it to the results channel.
func (rr *PubSubResultsService) publish(ctx context.Context, env *commonv1.UniversalEnvelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal result envelope: %w", err)
	}
	channel := rr.resultsChannel(env.OperatorSessionId)
	rr.logger.Info("Publishing result (Protocol-First)",
		"channel", channel,
		"event_type", env.EventType,
		"message_id", env.Id)
	return rr.client.Publish(ctx, channel, data)
}
