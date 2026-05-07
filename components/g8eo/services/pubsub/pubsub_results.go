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

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
)

func (rr *PubSubResultsService) resultsChannel(operatorSessionID string) string {
	return constants.ResultsChannel(rr.config.OperatorID, operatorSessionID)
}

// PubSubResultsService handles publishing results back to AI Agent Services via g8es pub/sub
type PubSubResultsService struct {
	client     PubSubClient
	config     *config.Config
	logger     *slog.Logger
	localStore *storage.LocalStoreService
}

// NewPubSubResultsService creates a new g8es pub/sub results service
func NewPubSubResultsService(cfg *config.Config, logger *slog.Logger, client PubSubClient, localStore *storage.LocalStoreService) (*PubSubResultsService, error) {
	return &PubSubResultsService{
		client:     client,
		config:     cfg,
		logger:     logger,
		localStore: localStore,
	}, nil
}

// publishResultEnvelope builds a G8eMessage for a typed result payload,
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
	msg, err := models.NewG8eMessage(
		eventType, caseID,
		rr.config.OperatorID, rr.config.OperatorSessionId, rr.config.SystemFingerprint,
		payload,
	)
	if err != nil {
		return fmt.Errorf("failed to build %s message: %w", eventType, err)
	}

	msg.APIKey = rr.config.APIKey
	msg.TaskID = taskID
	msg.InvestigationID = investigationID
	msg.OperatorSessionID = originalMsg.OperatorSessionID

	return rr.publish(ctx, msg)
}

// PublishExecutionResult publishes command execution result via g8es pub/sub
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
		status := reflectMsg.Get(statusFd).String()
		if status == string(constants.ExecutionStatusFailed) || status == string(constants.ExecutionStatusTimeout) {
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

// PublishCancellationResult publishes command cancellation result via g8es pub/sub
func (rr *PubSubResultsService) PublishCancellationResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.Command.Cancelled

	if err := rr.publishResultEnvelope(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish cancellation result: %w", err)
	}

	rr.logger.Info("Cancellation result transmitted to g8e",
		"operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFileEditResult publishes file edit result via g8es pub/sub.
func (rr *PubSubResultsService) PublishFileEditResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.FileEdit.Completed

	reflectMsg := result.ProtoReflect()
	statusFd := reflectMsg.Descriptor().Fields().ByName("status")
	if statusFd != nil {
		status := reflectMsg.Get(statusFd).String()
		if status == string(constants.ExecutionStatusFailed) {
			eventType = constants.Event.Operator.FileEdit.Failed
		}
	}

	if err := rr.publishResultEnvelope(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish file edit result: %w", err)
	}

	rr.logger.Info("File operation result transmitted", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFsListResult publishes file system list result via g8es pub/sub.
func (rr *PubSubResultsService) PublishFsListResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.FsList.Completed

	reflectMsg := result.ProtoReflect()
	statusFd := reflectMsg.Descriptor().Fields().ByName("status")
	if statusFd != nil {
		status := reflectMsg.Get(statusFd).String()
		if status == string(constants.ExecutionStatusFailed) {
			eventType = constants.Event.Operator.FsList.Failed
		}
	}

	if err := rr.publishResultEnvelope(ctx, eventType, originalMsg.CaseID, originalMsg.TaskID, originalMsg.InvestigationID, originalMsg, result); err != nil {
		return fmt.Errorf("failed to publish fs list result: %w", err)
	}

	rr.logger.Info("FS list result transmitted", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFsGrepResult publishes file system grep result via g8es pub/sub.
func (rr *PubSubResultsService) PublishFsGrepResult(ctx context.Context, result proto.Message, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.FsGrep.Completed

	reflectMsg := result.ProtoReflect()
	statusFd := reflectMsg.Descriptor().Fields().ByName("status")
	if statusFd != nil {
		status := reflectMsg.Get(statusFd).String()
		if status == string(constants.ExecutionStatusFailed) {
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
	var executionStatus string

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
		executionStatus = reflectMsg.Get(fd).String()
	}

	eventType := constants.Event.Operator.Command.StatusUpdated.Running
	switch executionStatus {
	case string(constants.ExecutionStatusPending):
		eventType = constants.Event.Operator.Command.StatusUpdated.Queued
	case string(constants.ExecutionStatusCompleted):
		eventType = constants.Event.Operator.Command.StatusUpdated.Completed
	case string(constants.ExecutionStatusFailed), string(constants.ExecutionStatusTimeout):
		eventType = constants.Event.Operator.Command.StatusUpdated.Failed
	case string(constants.ExecutionStatusCancelled):
		eventType = constants.Event.Operator.Command.StatusUpdated.Cancelled
	}

	msg, err := models.NewG8eMessage(
		eventType, caseID,
		rr.config.OperatorID, rr.config.OperatorSessionId, rr.config.SystemFingerprint,
		status,
	)
	if err != nil {
		return fmt.Errorf("failed to build status message: %w", err)
	}
	msg.APIKey = rr.config.APIKey
	msg.TaskID = taskID
	msg.InvestigationID = investigationID
	msg.OperatorSessionID = operatorSessionID

	if err := rr.publish(ctx, msg); err != nil {
		return fmt.Errorf("failed to publish status update: %w", err)
	}

	rr.logger.Info("Execution status update transmitted (Protocol-First)",
		"event_type", eventType)
	return nil
}

// PublishHeartbeat publishes heartbeat to dedicated g8es pub/sub heartbeat channel
func (rr *PubSubResultsService) PublishHeartbeat(ctx context.Context, heartbeat proto.Message) error {
	rr.logger.Info("[HEARTBEAT] Publishing heartbeat to g8es pub/sub (Protocol-First)")

	data, err := json.Marshal(heartbeat) // models.NewG8eMessage handles protojson, but this is direct
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	// Extract operatorSessionID via reflection
	operatorSessionID := rr.config.OperatorSessionId
	reflectMsg := heartbeat.ProtoReflect()
	if fd := reflectMsg.Descriptor().Fields().ByName("operator_session_id"); fd != nil {
		operatorSessionID = reflectMsg.Get(fd).String()
	}

	channelName := constants.HeartbeatChannel(rr.config.OperatorID, operatorSessionID)
	if err := rr.client.Publish(ctx, channelName, data); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	return nil
}

// PublishResult publishes a pre-built ResultMessage to the g8es pub/sub results channel.
func (rr *PubSubResultsService) PublishResult(ctx context.Context, result *models.G8eMessage) error {
	if result.OperatorSessionID == "" {
		result.OperatorSessionID = rr.config.OperatorSessionId
	}
	if result.OperatorID == "" {
		result.OperatorID = rr.config.OperatorID
	}
	return rr.publish(ctx, result)
}

// publish marshals a ResultMessage and publishes it to the results channel.
func (rr *PubSubResultsService) publish(ctx context.Context, msg *models.G8eMessage) error {
	data, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal result message: %w", err)
	}
	channel := rr.resultsChannel(msg.OperatorSessionID)
	rr.logger.Info("Publishing result",
		"channel", channel,
		"event_type", msg.EventType,
		"message_id", msg.ID)
	return rr.client.Publish(ctx, channel, data)
}
