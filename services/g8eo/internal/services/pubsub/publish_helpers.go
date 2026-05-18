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

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
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

// publishLFAATypedResponseTo builds a UAPEnvelope from a typed payload and publishes it to the
// results channel. Used by services that hold a PubSubClient directly.
func publishLFAATypedResponseTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg PubSubCommandMessage,
	eventType constants.EventType,
	payload proto.Message,
) {
	executionID := executionIDFromMessage(msg)
	setExecutionIDOnPayload(payload, executionID)

	env, err := BuildUniversalResultEnvelope(cfg, eventType, payload, msg.ID, cfg.OperatorID, msg.CaseID, msg.InvestigationID, msg.TaskID, msg.WebSessionID, msg.CLISessionID)
	if err != nil {
		logger.Error("Failed to build LFAA typed response Governance Envelope", "error", err)
		return
	}

	data, err := json.Marshal(env)
	if err != nil {
		logger.Error("Failed to marshal LFAA typed response Governance Envelope", "error", err)
		return
	}

	channelName := constants.ResultsChannel(cfg.OperatorID, msg.OperatorSessionID)
	if err := client.Publish(ctx, channelName, data); err != nil {
		logger.Error("Failed to publish LFAA typed response Universal", "error", err)
		return
	}

	logger.Info("LFAA typed response published (Universal)", "event_type", eventType)
}

// publishLFAAErrorTo builds an error UAPEnvelope and publishes it to the results channel.
func publishLFAAErrorTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg PubSubCommandMessage,
	eventType constants.EventType,
	errorMsg string,
) {
	executionID := executionIDFromMessage(msg)

	// Use CommandResult as a generic error container
	payload := &operatorv1.CommandResult{
		ExecutionId: executionID,
		Status:      operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
		Error:       errorMsg,
	}

	env, err := BuildUniversalResultEnvelope(cfg, eventType, payload, msg.ID, cfg.OperatorID, msg.CaseID, msg.InvestigationID, msg.TaskID, msg.WebSessionID, msg.CLISessionID)
	if err != nil {
		logger.Error("Failed to build LFAA error Governance Envelope", "error", err)
		return
	}

	data, err := json.Marshal(env)
	if err != nil {
		logger.Error("Failed to marshal LFAA error Governance Envelope", "error", err)
		return
	}

	channelName := constants.ResultsChannel(cfg.OperatorID, msg.OperatorSessionID)
	if err := client.Publish(ctx, channelName, data); err != nil {
		logger.Error("Failed to publish LFAA error Universal", "error", err)
	}
}
