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
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/services/mcp"
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

func (rr *PubSubResultsService) wrapMCPIfNecessary(msg *models.G8eMessage, originalMsg PubSubCommandMessage, eventType string, payload interface{}) error {
	rr.logger.Debug("Checking if MCP wrapping is necessary",
		"original_event_type", originalMsg.EventType,
		"target_event_type", eventType,
		"mcp_tools_call", constants.Event.Operator.MCP.ToolsCall)

	if originalMsg.EventType != constants.Event.Operator.MCP.ToolsCall {
		rr.logger.Debug("Skipping MCP wrapping - not an MCP tools call")
		return nil
	}

	rr.logger.Info("MCP wrapping triggered - wrapping result as MCP JSON-RPC response",
		"original_event_type", originalMsg.EventType,
		"target_event_type", eventType,
		"original_msg_id", originalMsg.ID,
		"original_msg_operator_session_id", originalMsg.OperatorSessionID)

	// If it was an MCP request, we wrap the entire result payload as an MCP JSON-RPC response
	mcpResp, err := mcp.TranslateResultToMCP(originalMsg.ID, originalMsg.ID, eventType, payload)
	if err != nil {
		return fmt.Errorf("failed to wrap result for MCP: %w", err)
	}
	mcpRaw, err := json.Marshal(mcpResp)
	if err != nil {
		return fmt.Errorf("failed to marshal MCP response: %w", err)
	}
	msg.EventType = constants.Event.Operator.MCP.ToolsResult
	msg.Payload = mcpRaw

	rr.logger.Info("MCP wrapping completed",
		"new_event_type", msg.EventType,
		"payload_size", len(mcpRaw))

	return nil
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

	msg, err := models.NewG8eMessage(
		result.ExecutionID, eventType, result.CaseID,
		rr.config.OperatorID, rr.config.OperatorSessionId, rr.config.SystemFingerprint,
		payload,
	)
	if err != nil {
		return fmt.Errorf("failed to build result message: %w", err)
	}

	if err := rr.wrapMCPIfNecessary(msg, originalMsg, eventType, &payload); err != nil {
		return err
	}

	msg.APIKey = rr.config.APIKey
	msg.TaskID = result.TaskID
	msg.InvestigationID = result.InvestigationID
	msg.OperatorSessionID = originalMsg.OperatorSessionID

	if err := rr.publish(ctx, msg); err != nil {
		return fmt.Errorf("failed to publish result: %w", err)
	}

	logArgs := []any{
		"operator_session_id", rr.config.OperatorSessionId,
		"status", result.Status,
	}
	if result.ReturnCode != nil {
		logArgs = append(logArgs, "return_code", *result.ReturnCode)
	}
	rr.logger.Debug("Result transmitted to g8e", logArgs...)
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

	msg, err := models.NewG8eMessage(
		result.ExecutionID, eventType, result.CaseID,
		rr.config.OperatorID, rr.config.OperatorSessionId, rr.config.SystemFingerprint,
		payload,
	)
	if err != nil {
		return fmt.Errorf("failed to build cancellation message: %w", err)
	}

	if err := rr.wrapMCPIfNecessary(msg, originalMsg, eventType, &payload); err != nil {
		return err
	}

	msg.APIKey = rr.config.APIKey
	msg.TaskID = result.TaskID
	msg.InvestigationID = result.InvestigationID
	msg.OperatorSessionID = originalMsg.OperatorSessionID

	if err := rr.publish(ctx, msg); err != nil {
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

	msg, err := models.NewG8eMessage(
		result.ExecutionID, eventType, result.CaseID,
		rr.config.OperatorID, rr.config.OperatorSessionId, rr.config.SystemFingerprint,
		payload,
	)
	if err != nil {
		return fmt.Errorf("failed to build file edit message: %w", err)
	}

	if err := rr.wrapMCPIfNecessary(msg, originalMsg, eventType, &payload); err != nil {
		return err
	}

	msg.APIKey = rr.config.APIKey
	msg.TaskID = result.TaskID
	msg.InvestigationID = result.InvestigationID
	msg.OperatorSessionID = originalMsg.OperatorSessionID

	if err := rr.publish(ctx, msg); err != nil {
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

	msg, err := models.NewG8eMessage(
		result.ExecutionID, eventType, result.CaseID,
		rr.config.OperatorID, rr.config.OperatorSessionId, rr.config.SystemFingerprint,
		payload,
	)
	if err != nil {
		return fmt.Errorf("failed to build fs list message: %w", err)
	}

	if err := rr.wrapMCPIfNecessary(msg, originalMsg, eventType, &payload); err != nil {
		return err
	}

	msg.APIKey = rr.config.APIKey
	msg.TaskID = result.TaskID
	msg.InvestigationID = result.InvestigationID
	msg.OperatorSessionID = originalMsg.OperatorSessionID

	if err := rr.publish(ctx, msg); err != nil {
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

	msgID := fmt.Sprintf("%s_status_%d", status.ExecutionID, timeNowNano())
	msg, err := models.NewG8eMessage(
		msgID, eventType, status.CaseID,
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
	if result.ID == "" {
		result.ID = fmt.Sprintf("result_%d", timeNowNano())
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
	rr.logger.Debug("Publishing result",
		"channel", channel,
		"event_type", msg.EventType,
		"message_id", msg.ID)
	return rr.client.Publish(ctx, channel, data)
}

// timeNowNano returns the current time as a Unix nanosecond timestamp.
func timeNowNano() int64 {
	return time.Now().UnixNano()
}
