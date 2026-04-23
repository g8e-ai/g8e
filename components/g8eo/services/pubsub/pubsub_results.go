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
// applies MCP wrapping when the originating command was an MCP tools/call,
// stamps the envelope metadata copied from the originating command, and
// publishes it on the results channel. All result-publishing paths that carry
// an originalMsg (command-completed, command-cancelled, file-edit, fs-list)
// share this helper so none can drift out of sync on API key, task/investigation
// propagation, or MCP wrapping.
func (rr *PubSubResultsService) publishResultEnvelope(
	ctx context.Context,
	eventType, caseID string,
	taskID *string,
	investigationID string,
	originalMsg PubSubCommandMessage,
	payload interface{},
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
func (rr *PubSubResultsService) PublishExecutionResult(ctx context.Context, result *models.ExecutionResultsPayload, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.Command.Completed
	if result.Status == constants.ExecutionStatusFailed || result.Status == constants.ExecutionStatusTimeout {
		eventType = constants.Event.Operator.Command.Failed
	}

	payload := models.ExecutionResultsPayload{
		ExecutionID:       result.ExecutionID,
		Command:           result.Command,
		Status:            result.Status,
		DurationSeconds:   result.DurationSeconds,
		OperatorID:        rr.config.OperatorID,
		OperatorSessionID: rr.config.OperatorSessionId,
		Stdout:            result.Stdout,
		Stderr:            result.Stderr,
		StdoutSize:        len(result.Stdout),
		StderrSize:        len(result.Stderr),
		ReturnCode:        result.ReturnCode,
		ErrorMessage:      result.ErrorMessage,
		ErrorType:         result.ErrorType,
	}
	if rr.localStore != nil && rr.localStore.IsEnabled() {
		payload.StdoutHash = rr.localStore.HashString(result.Stdout)
		payload.StderrHash = rr.localStore.HashString(result.Stderr)
		payload.StoredLocally = true
		rr.logger.Info("Publishing sentinel.Sentinel-scrubbed output",
			"execution_id", result.ExecutionID,
			"stdout_size", len(result.Stdout),
			"stderr_size", len(result.Stderr))
	}

	if err := rr.publishResultEnvelope(ctx, eventType, result.CaseID, result.TaskID, result.InvestigationID, originalMsg, &payload); err != nil {
		return fmt.Errorf("failed to publish result: %w", err)
	}

	logArgs := []any{
		"operator_session_id", rr.config.OperatorSessionId,
		"status", result.Status,
	}
	if result.ReturnCode != nil {
		logArgs = append(logArgs, "return_code", *result.ReturnCode)
	}
	rr.logger.Info("Result transmitted to g8e", logArgs...)
	return nil
}

// PublishCancellationResult publishes command cancellation result via g8es pub/sub
func (rr *PubSubResultsService) PublishCancellationResult(ctx context.Context, result *models.ExecutionResultsPayload, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.Command.Cancelled
	payload := models.CancellationResultPayload{
		ExecutionID:       result.ExecutionID,
		Status:            result.Status,
		OperatorID:        rr.config.OperatorID,
		OperatorSessionID: rr.config.OperatorSessionId,
		ErrorMessage:      result.ErrorMessage,
		ErrorType:         result.ErrorType,
	}

	if err := rr.publishResultEnvelope(ctx, eventType, result.CaseID, result.TaskID, result.InvestigationID, originalMsg, &payload); err != nil {
		return fmt.Errorf("failed to publish cancellation result: %w", err)
	}

	rr.logger.Info("Cancellation result transmitted to g8e",
		"operator_session_id", rr.config.OperatorSessionId,
		"execution_id", result.ExecutionID,
		"status", result.Status)
	return nil
}

// PublishFileEditResult publishes file edit result via g8es pub/sub.
// Content has already been sentinel.Sentinel-scrubbed before this is called.
func (rr *PubSubResultsService) PublishFileEditResult(ctx context.Context, result *models.FileEditResult, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.FileEdit.Completed
	if result.Status == constants.ExecutionStatusFailed {
		eventType = constants.Event.Operator.FileEdit.Failed
	}

	var contentStr, errorStr string
	if result.Content != nil {
		contentStr = *result.Content
	}
	if result.ErrorMessage != nil {
		errorStr = *result.ErrorMessage
	}

	payload := models.FileEditResultPayload{
		ExecutionID:       result.ExecutionID,
		Operation:         result.Operation,
		FilePath:          result.FilePath,
		Status:            result.Status,
		DurationSeconds:   result.DurationSeconds,
		OperatorID:        rr.config.OperatorID,
		OperatorSessionID: rr.config.OperatorSessionId,
		Content:           result.Content,
		StdoutSize:        len(contentStr),
		StderrSize:        len(errorStr),
		ErrorMessage:      result.ErrorMessage,
		ErrorType:         result.ErrorType,
		BytesWritten:      result.BytesWritten,
		LinesChanged:      result.LinesChanged,
		BackupPath:        result.BackupPath,
	}
	if rr.localStore != nil && rr.localStore.IsEnabled() {
		payload.StdoutHash = rr.localStore.HashString(contentStr)
		payload.StderrHash = rr.localStore.HashString(errorStr)
		payload.StoredLocally = true
		rr.logger.Info("Publishing sentinel.Sentinel-scrubbed file edit result",
			"execution_id", result.ExecutionID,
			"operation", result.Operation,
			"content_size", len(contentStr))
	}

	if err := rr.publishResultEnvelope(ctx, eventType, result.CaseID, result.TaskID, result.InvestigationID, originalMsg, &payload); err != nil {
		return fmt.Errorf("failed to publish file edit result: %w", err)
	}

	rr.logger.Info("File operation result transmitted", "operator_session_id", rr.config.OperatorSessionId)
	return nil
}

// PublishFsListResult publishes file system list result via g8es pub/sub.
// Entries have already been sentinel.Sentinel-scrubbed before this is called.
func (rr *PubSubResultsService) PublishFsListResult(ctx context.Context, result *models.FsListResult, originalMsg PubSubCommandMessage) error {
	eventType := constants.Event.Operator.FsList.Completed
	if result.Status == constants.ExecutionStatusFailed {
		eventType = constants.Event.Operator.FsList.Failed
	}

	var entriesJSON, errorStr string
	if result.Entries != nil {
		if b, err := json.Marshal(result.Entries); err == nil {
			entriesJSON = string(b)
		}
	}
	if result.ErrorMessage != nil {
		errorStr = *result.ErrorMessage
	}

	payload := models.FsListResultPayload{
		ExecutionID:       result.ExecutionID,
		Path:              result.Path,
		Status:            result.Status,
		TotalCount:        result.TotalCount,
		Truncated:         result.Truncated,
		DurationSeconds:   result.DurationSeconds,
		OperatorID:        rr.config.OperatorID,
		OperatorSessionID: rr.config.OperatorSessionId,
		Entries:           result.Entries,
		StdoutSize:        len(entriesJSON),
		StderrSize:        len(errorStr),
		ErrorMessage:      result.ErrorMessage,
		ErrorType:         result.ErrorType,
	}
	if rr.localStore != nil && rr.localStore.IsEnabled() {
		payload.StdoutHash = rr.localStore.HashString(entriesJSON)
		payload.StderrHash = rr.localStore.HashString(errorStr)
		payload.StoredLocally = true
		rr.logger.Info("Publishing fs list result",
			"execution_id", result.ExecutionID,
			"path", result.Path,
			"entries_count", result.TotalCount)
	}

	if err := rr.publishResultEnvelope(ctx, eventType, result.CaseID, result.TaskID, result.InvestigationID, originalMsg, &payload); err != nil {
		return fmt.Errorf("failed to publish fs list result: %w", err)
	}

	rr.logger.Info("FS list result transmitted",
		"operator_session_id", rr.config.OperatorSessionId,
		"entries", result.TotalCount)
	return nil
}

// ExecutionStatusUpdate represents a periodic status update during command execution
type ExecutionStatusUpdate struct {
	ExecutionID       string
	CaseID            string
	TaskID            *string
	InvestigationID   string
	OperatorSessionID string
	Command           string
	Status            constants.ExecutionStatus
	ProcessAlive      bool
	NewOutput         string // New stdout since last update
	NewStderr         string // New stderr since last update
	ElapsedSeconds    float64
	Message           string // Human-readable status message
}

// PublishExecutionStatus publishes periodic status updates during command execution.
// Incremental output has already been sentinel.Sentinel-scrubbed before this is called.
func (rr *PubSubResultsService) PublishExecutionStatus(ctx context.Context, status *ExecutionStatusUpdate) error {
	payload := models.ExecutionStatusPayload{
		ExecutionID:       status.ExecutionID,
		Command:           status.Command,
		Status:            status.Status,
		ProcessAlive:      status.ProcessAlive,
		ElapsedSeconds:    status.ElapsedSeconds,
		OperatorID:        rr.config.OperatorID,
		OperatorSessionID: rr.config.OperatorSessionId,
		NewOutput:         status.NewOutput,
		NewStderr:         status.NewStderr,
		Message:           status.Message,
		StoredLocally:     rr.localStore != nil && rr.localStore.IsEnabled(),
	}

	eventType := constants.Event.Operator.Command.StatusUpdated.Running
	switch status.Status {
	case constants.ExecutionStatusPending:
		eventType = constants.Event.Operator.Command.StatusUpdated.Queued
	case constants.ExecutionStatusCompleted:
		eventType = constants.Event.Operator.Command.StatusUpdated.Completed
	case constants.ExecutionStatusFailed, constants.ExecutionStatusTimeout:
		eventType = constants.Event.Operator.Command.StatusUpdated.Failed
	case constants.ExecutionStatusCancelled:
		eventType = constants.Event.Operator.Command.StatusUpdated.Cancelled
	}

	msg, err := models.NewG8eMessage(
		eventType, status.CaseID,
		rr.config.OperatorID, rr.config.OperatorSessionId, rr.config.SystemFingerprint,
		payload,
	)
	if err != nil {
		return fmt.Errorf("failed to build status message: %w", err)
	}
	msg.APIKey = rr.config.APIKey
	msg.TaskID = status.TaskID
	msg.InvestigationID = status.InvestigationID
	msg.OperatorSessionID = status.OperatorSessionID

	if err := rr.publish(ctx, msg); err != nil {
		return fmt.Errorf("failed to publish status update: %w", err)
	}

	rr.logger.Info("Execution status update transmitted",
		"execution_id", status.ExecutionID,
		"elapsed", fmt.Sprintf("%.1fs", status.ElapsedSeconds),
		"process_alive", status.ProcessAlive)
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

// PublishHeartbeat publishes heartbeat to dedicated g8es pub/sub heartbeat channel
func (rr *PubSubResultsService) PublishHeartbeat(ctx context.Context, heartbeat *models.Heartbeat) error {
	data, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}
	channelName := constants.HeartbeatChannel(rr.config.OperatorID, heartbeat.OperatorSessionID)
	if err := rr.client.Publish(ctx, channelName, data); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	rr.logger.Info("Heartbeat transmitted", "operator_session_id", heartbeat.OperatorSessionID)
	return nil
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
