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

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	operatorv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
)

// executionIDFromMessage resolves the execution_id for a command from the
// inbound payload's execution_id field. It handles both JSON and Protobuf payloads.
// If the payload does not carry one it falls back to the envelope id (msg.ID).
func executionIDFromMessage(msg PubSubCommandMessage) string {
	// 1. Try Protobuf first (protocol-first architecture)
	// Many requested types have an execution_id at field 2 (common convention in our .proto)
	// We'll try unmarshaling into a generic struct that matches the most common ones.
	var protoCmd operatorv1.CommandRequested
	if err := proto.Unmarshal(msg.Payload, &protoCmd); err == nil && protoCmd.ExecutionId != "" {
		return protoCmd.ExecutionId
	}

	// 2. Fall back to JSON for components not yet fully migrated
	var probe struct {
		ExecutionID string `json:"execution_id"`
	}
	if err := json.Unmarshal(msg.Payload, &probe); err == nil && probe.ExecutionID != "" {
		return probe.ExecutionID
	}
	return msg.ID
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
// payloadType must be set to the discriminator value expected by the Python consumer
// (e.g. "fetch_logs_error", "fetch_history_error"). It is required — callers must provide
// the correct value matching the G8eoResultPayload union member on the Python side.
func publishLFAAErrorTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg PubSubCommandMessage,
	eventType, errorMsg string,
	payloadType string,
) {
	executionID := executionIDFromMessage(msg)
	payload := models.LFAAErrorPayload{
		PayloadType:       payloadType,
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
