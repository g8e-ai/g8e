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

	"github.com/g8e-ai/g8e/components/vsa/config"
	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	"github.com/g8e-ai/g8e/components/vsa/services/mcp"
)

// publishLFAATypedResponseTo builds a VSOMessage from a typed payload and publishes it to the
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
	resultMsg, err := models.NewVSOMessage(
		msg.ID, eventType, msg.CaseID,
		cfg.OperatorID, cfg.OperatorSessionId, cfg.SystemFingerprint,
		payload,
	)
	if err != nil {
		logger.Error("Failed to build LFAA typed response", "error", err)
		return
	}

	isMCP := msg.EventType == constants.Event.Operator.MCP.ToolsCall ||
		msg.EventType == constants.Event.Operator.MCP.ResourcesRead ||
		msg.EventType == constants.Event.Operator.MCP.ResourcesList

	if isMCP {
		mcpRaw, err := mcp.WrapResult(msg.ID, msg.ID, eventType, payload)
		if err != nil {
			logger.Error("Failed to wrap result for MCP", "error", err)
		} else {
			resultMsg.EventType = constants.Event.Operator.MCP.ToolsResult
			if msg.EventType == constants.Event.Operator.MCP.ResourcesRead || msg.EventType == constants.Event.Operator.MCP.ResourcesList {
				resultMsg.EventType = constants.Event.Operator.MCP.ResourcesResult
			}
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

// publishLFAAErrorTo builds an error VSOMessage and publishes it to the results channel.
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
		OperatorID:        cfg.OperatorID,
		OperatorSessionID: cfg.OperatorSessionId,
	}

	resultMsg, err := models.NewVSOMessage(
		msg.ID, eventType, msg.CaseID,
		cfg.OperatorID, cfg.OperatorSessionId, cfg.SystemFingerprint,
		payload,
	)
	if err != nil {
		logger.Error("Failed to build LFAA error message", "error", err)
		return
	}

	isMCP := msg.EventType == constants.Event.Operator.MCP.ToolsCall ||
		msg.EventType == constants.Event.Operator.MCP.ResourcesRead ||
		msg.EventType == constants.Event.Operator.MCP.ResourcesList

	if isMCP {
		mcpRaw, err := mcp.WrapResult(msg.ID, msg.ID, eventType, &payload)
		if err != nil {
			logger.Error("Failed to wrap error for MCP", "error", err)
		} else {
			resultMsg.EventType = constants.Event.Operator.MCP.ToolsResult
			if msg.EventType == constants.Event.Operator.MCP.ResourcesRead || msg.EventType == constants.Event.Operator.MCP.ResourcesList {
				resultMsg.EventType = constants.Event.Operator.MCP.ResourcesResult
			}
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
func publishLFAAResponseTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg PubSubCommandMessage,
	eventType string,
	responseJSON []byte,
) {
	resultMsg, err := models.NewVSOMessage(
		msg.ID, eventType, msg.CaseID,
		cfg.OperatorID, cfg.OperatorSessionId, cfg.SystemFingerprint,
		json.RawMessage(responseJSON),
	)
	if err != nil {
		logger.Error("Failed to build LFAA response", "error", err)
		return
	}

	isMCP := msg.EventType == constants.Event.Operator.MCP.ToolsCall ||
		msg.EventType == constants.Event.Operator.MCP.ResourcesRead ||
		msg.EventType == constants.Event.Operator.MCP.ResourcesList

	if isMCP {
		mcpRaw, err := mcp.WrapResult(msg.ID, msg.ID, eventType, responseJSON)
		if err != nil {
			logger.Error("Failed to wrap result for MCP", "error", err)
		} else {
			resultMsg.EventType = constants.Event.Operator.MCP.ToolsResult
			if msg.EventType == constants.Event.Operator.MCP.ResourcesRead || msg.EventType == constants.Event.Operator.MCP.ResourcesList {
				resultMsg.EventType = constants.Event.Operator.MCP.ResourcesResult
			}
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
