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

// executionIDFromMessage resolves the execution_id for a command from the
// inbound payload's execution_id field. If the payload does not carry one it
// falls back to the envelope id (msg.ID) for native g8e envelopes, where g8ee
// sets id == execution_id by convention.
//
// For MCP-translated envelopes (EventType == operator.mcp.tools.call) the
// envelope id has been rewritten to the JSON-RPC request id by
// handleMCPToolsCall and is NOT the execution_id; in that case the fallback
// is suppressed and "" is returned so callers surface the missing id rather
// than stamp results with the wrong value. Well-formed MCP tool arguments
// from g8ee always include execution_id, so an empty return here indicates
// a protocol violation upstream.
func executionIDFromMessage(msg PubSubCommandMessage) string {
	var probe struct {
		ExecutionID string `json:"execution_id"`
	}
	if err := json.Unmarshal(msg.Payload, &probe); err == nil && probe.ExecutionID != "" {
		return probe.ExecutionID
	}
	if msg.EventType == constants.Event.Operator.MCP.ToolsCall {
		return ""
	}
	return msg.ID
}

// warnIfMCPExecutionIDMissing logs a warning when an MCP-translated envelope
// resolves to an empty execution_id. Well-formed MCP tool arguments from g8ee
// always include execution_id, so an empty value here indicates an upstream
// protocol violation. Logged fields are kept to a minimum for signal density.
func warnIfMCPExecutionIDMissing(logger *slog.Logger, msg PubSubCommandMessage, resolvedID, eventType string) {
	if msg.EventType != constants.Event.Operator.MCP.ToolsCall || resolvedID != "" {
		return
	}
	logger.Warn("MCP envelope missing execution_id in tool arguments; result metadata will be empty",
		"envelope_event_type", msg.EventType,
		"jsonrpc_request_id", msg.ID,
		"case_id", msg.CaseID,
		"result_event_type", eventType)
}

// setExecutionIDOnPayload sets the ExecutionID field on typed payloads that support it.
// This is done before serialization to avoid manipulating JSON mid-stream.
func setExecutionIDOnPayload(payload interface{}, executionID string) {
	if executionID == "" {
		return
	}
	if setter, ok := payload.(models.ExecutionIDSetter); ok {
		setter.SetExecutionID(executionID)
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
	executionID := executionIDFromMessage(msg)
	setExecutionIDOnPayload(payload, executionID)

	resultMsg, err := models.NewG8eMessage(
		eventType, msg.CaseID,
		cfg.OperatorID, cfg.OperatorSessionId, cfg.SystemFingerprint,
		payload,
	)
	if err != nil {
		logger.Error("Failed to build LFAA typed response", "error", err)
		return
	}

	if msg.EventType == constants.Event.Operator.MCP.ToolsCall {
		warnIfMCPExecutionIDMissing(logger, msg, executionID, eventType)
		mcpRaw, err := mcp.WrapResult(msg.ID, executionID, eventType, payload)
		if err != nil {
			logger.Error("Failed to wrap result for MCP", "error", err)
		} else {
			resultMsg.EventType = constants.Event.Operator.MCP.ToolsResult
			resultMsg.Payload = mcpRaw
		}
	}

	resultMsg.APIKey = cfg.APIKey
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
	executionID := executionIDFromMessage(msg)
	payload := models.LFAAErrorPayload{
		Success:           false,
		Error:             errorMsg,
		ExecutionID:       executionID,
		OperatorID:        cfg.OperatorID,
		OperatorSessionID: cfg.OperatorSessionId,
	}

	resultMsg, err := models.NewG8eMessage(
		eventType, msg.CaseID,
		cfg.OperatorID, cfg.OperatorSessionId, cfg.SystemFingerprint,
		payload,
	)
	if err != nil {
		logger.Error("Failed to build LFAA error message", "error", err)
		return
	}

	if msg.EventType == constants.Event.Operator.MCP.ToolsCall {
		warnIfMCPExecutionIDMissing(logger, msg, executionID, eventType)
		mcpRaw, err := mcp.WrapResult(msg.ID, executionID, eventType, &payload)
		if err != nil {
			logger.Error("Failed to wrap error for MCP", "error", err)
		} else {
			resultMsg.EventType = constants.Event.Operator.MCP.ToolsResult
			resultMsg.Payload = mcpRaw
		}
	}

	resultMsg.APIKey = cfg.APIKey
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
		eventType, msg.CaseID,
		cfg.OperatorID, cfg.OperatorSessionId, cfg.SystemFingerprint,
		json.RawMessage(responseJSON),
	)
	if err != nil {
		logger.Error("Failed to build LFAA response", "error", err)
		return
	}

	if msg.EventType == constants.Event.Operator.MCP.ToolsCall {
		executionID := executionIDFromMessage(msg)
		warnIfMCPExecutionIDMissing(logger, msg, executionID, eventType)
		mcpRaw, err := mcp.WrapResult(msg.ID, executionID, eventType, responseJSON)
		if err != nil {
			logger.Error("Failed to wrap result for MCP", "error", err)
		} else {
			resultMsg.EventType = constants.Event.Operator.MCP.ToolsResult
			resultMsg.Payload = mcpRaw
		}
	}

	resultMsg.APIKey = cfg.APIKey
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
