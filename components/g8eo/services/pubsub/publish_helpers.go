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
	"log/slog"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/services/mcp"
)

// setExecutionIDOnPayload sets the ExecutionID field on typed payloads that support it.
// This is done before serialization to avoid manipulating JSON mid-stream.
func setExecutionIDOnPayload(payload interface{}, executionID string) {
	if executionID == "" {
		return
	}
	switch p := payload.(type) {
	case *models.CancellationResultPayload:
		p.ExecutionID = executionID
	case *models.FileEditResultPayload:
		p.ExecutionID = executionID
	case *models.FsListResultPayload:
		p.ExecutionID = executionID
	case *models.ExecutionStatusPayload:
		p.ExecutionID = executionID
	case *models.PortCheckResultPayload:
		p.ExecutionID = executionID
	case *models.FetchLogsResultPayload:
		p.ExecutionID = executionID
	case *models.FsReadResultPayload:
		p.ExecutionID = executionID
	case *models.LFAAErrorPayload:
		p.ExecutionID = executionID
	case *models.FetchFileDiffResultPayload:
		p.ExecutionID = executionID
	case *models.FetchHistoryResultPayload:
		p.ExecutionID = executionID
	case *models.FetchFileHistoryResultPayload:
		p.ExecutionID = executionID
	case *models.RestoreFileResultPayload:
		p.ExecutionID = executionID
	}
}

// publishLFAATypedResponseTo builds a G8eMessage from a typed payload and publishes it to the
// results channel. Used by services that hold a PubSubClient directly.
func publishLFAATypedResponseTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg PubSubCommandMessage,
	eventType string,
	payload interface{},
) {
	setExecutionIDOnPayload(payload, msg.ID)

	resultMsg, err := models.NewG8eMessage(
		msg.ID, eventType, msg.CaseID,
		cfg.OperatorID, cfg.OperatorSessionId, cfg.SystemFingerprint,
		payload,
	)
	if err != nil {
		logger.Error("Failed to build LFAA typed response", "error", err)
		return
	}

	if msg.EventType == constants.Event.Operator.MCP.ToolsCall {
		mcpRaw, err := mcp.WrapResult(msg.ID, msg.ID, eventType, payload)
		if err != nil {
			logger.Error("Failed to wrap result for MCP", "error", err)
		} else {
			resultMsg.EventType = constants.Event.Operator.MCP.ToolsResult
			resultMsg.Payload = mcpRaw
		}
	}

	resultMsg.TaskID = msg.TaskID
	resultMsg.InvestigationID = msg.InvestigationID
	resultMsg.OperatorSessionID = msg.OperatorSessionID

	data, err := resultMsg.Marshal()
	if err != nil {
		logger.Error("Failed to marshal LFAA typed response", "error", err)
		return
	}

	channelName := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
	if err := client.Publish(ctx, channelName, data); err != nil {
		logger.Error("Failed to publish LFAA typed response", "error", err)
		return
	}

	logger.Info("LFAA typed response published", "event_type", eventType)
}

// publishLFAAErrorTo builds an error G8eMessage and publishes it to the results channel.
func publishLFAAErrorTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg PubSubCommandMessage,
	eventType, errorMsg string,
) {
	payload := models.LFAAErrorPayload{
		Success:           false,
		Error:             errorMsg,
		ExecutionID:       msg.ID,
		OperatorID:        cfg.OperatorID,
		OperatorSessionID: cfg.OperatorSessionId,
	}

	resultMsg, err := models.NewG8eMessage(
		msg.ID, eventType, msg.CaseID,
		cfg.OperatorID, cfg.OperatorSessionId, cfg.SystemFingerprint,
		payload,
	)
	if err != nil {
		logger.Error("Failed to build LFAA error message", "error", err)
		return
	}

	if msg.EventType == constants.Event.Operator.MCP.ToolsCall {
		mcpRaw, err := mcp.WrapResult(msg.ID, msg.ID, eventType, &payload)
		if err != nil {
			logger.Error("Failed to wrap error for MCP", "error", err)
		} else {
			resultMsg.EventType = constants.Event.Operator.MCP.ToolsResult
			resultMsg.Payload = mcpRaw
		}
	}

	resultMsg.TaskID = msg.TaskID
	resultMsg.InvestigationID = msg.InvestigationID
	resultMsg.OperatorSessionID = msg.OperatorSessionID

	data, err := resultMsg.Marshal()
	if err != nil {
		logger.Error("Failed to marshal LFAA error", "error", err)
		return
	}

	channelName := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
	if err := client.Publish(ctx, channelName, data); err != nil {
		logger.Error("Failed to publish LFAA error", "error", err)
	}
}

// publishLFAAResponseTo publishes a pre-marshalled JSON response to the results channel.
// The responseJSON must already include execution_id if needed for correlation.
func publishLFAAResponseTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg PubSubCommandMessage,
	eventType string,
	responseJSON []byte,
) {
	resultMsg, err := models.NewG8eMessage(
		msg.ID, eventType, msg.CaseID,
		cfg.OperatorID, cfg.OperatorSessionId, cfg.SystemFingerprint,
		json.RawMessage(responseJSON),
	)
	if err != nil {
		logger.Error("Failed to build LFAA response", "error", err)
		return
	}

	if msg.EventType == constants.Event.Operator.MCP.ToolsCall {
		mcpRaw, err := mcp.WrapResult(msg.ID, msg.ID, eventType, responseJSON)
		if err != nil {
			logger.Error("Failed to wrap result for MCP", "error", err)
		} else {
			resultMsg.EventType = constants.Event.Operator.MCP.ToolsResult
			resultMsg.Payload = mcpRaw
		}
	}

	resultMsg.TaskID = msg.TaskID
	resultMsg.InvestigationID = msg.InvestigationID
	resultMsg.OperatorSessionID = msg.OperatorSessionID

	data, err := resultMsg.Marshal()
	if err != nil {
		logger.Error("Failed to marshal LFAA response", "error", err)
		return
	}

	channelName := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
	if err := client.Publish(ctx, channelName, data); err != nil {
		logger.Error("Failed to publish LFAA response", "error", err)
		return
	}

	logger.Info("LFAA response published", "event_type", eventType)
}
