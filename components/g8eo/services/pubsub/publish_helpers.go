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
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
)

// executionIDFromMessage resolves the execution_id for a command from the
// inbound payload's execution_id field using strict Protobuf extraction.
// If the payload does not carry one it falls back to the envelope id (msg.ID).
func executionIDFromMessage(msg PubSubCommandMessage) string {
	payloadMsg, err := unmarshalPayload(msg.EventType, msg.Payload)
	if err != nil {
		return msg.ID
	}

	reflectMsg := payloadMsg.ProtoReflect()
	descriptor := reflectMsg.Descriptor()

	// Try "execution_id" field by name first (Protocol-First reflection)
	fd := descriptor.Fields().ByName("execution_id")
	if fd != nil && fd.Kind() == protoreflect.StringKind {
		val := reflectMsg.Get(fd).String()
		if val != "" {
			return val
		}
	}

	return msg.ID
}

// setExecutionIDOnPayload sets the execution_id field on Protobuf payloads that support it via reflection.
func setExecutionIDOnPayload(payload proto.Message, executionID string) {
	if executionID == "" {
		return
	}
	reflectMsg := payload.ProtoReflect()
	fd := reflectMsg.Descriptor().Fields().ByName("execution_id")
	if fd != nil && fd.Kind() == protoreflect.StringKind {
		reflectMsg.Set(fd, protoreflect.ValueOfString(executionID))
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
	payload proto.Message,
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
func publishLFAAErrorTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg PubSubCommandMessage,
	eventType, errorMsg string,
) {
	executionID := executionIDFromMessage(msg)

	// Use CommandResult as a generic error container for Protocol-First
	payload := &operatorv1.CommandResult{
		ExecutionId: executionID,
		Status:      string(constants.ExecutionStatusFailed),
		Error:       errorMsg,
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
