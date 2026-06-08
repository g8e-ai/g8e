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
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/system"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// AuditService owns LFAA audit event recording for user messages, AI messages,
// and direct terminal command capture.
type AuditService struct {
	config     *config.Config
	logger     *slog.Logger
	auditStore *storage.SQLAuditStore
}

// NewAuditService creates a new AuditService.
func NewAuditService(cfg *config.Config, logger *slog.Logger, auditStore *storage.SQLAuditStore) *AuditService {
	return &AuditService{
		config:     cfg,
		logger:     logger,
		auditStore: auditStore,
	}
}

// HandleUserMsgRequest records an inbound user message to the audit store.
func (as *AuditService) HandleUserMsgRequest(ctx context.Context, msg *PubSubCommandMessage) error {
	as.logger.Info("LFAA: Recording user message (via Protobuf)")

	if !as.auditStore.IsEnabled() {
		as.logger.Info("Audit store not enabled, skipping user message recording")
		return nil
	}

	var protoMsg operatorv1.AuditMsgRequested
	if err := proto.Unmarshal(msg.Payload, &protoMsg); err != nil {
		return fmt.Errorf("audit service: failed to decode user msg payload as protobuf AuditMsgRequested: %w", err)
	}
	content := protoMsg.Content
	if content == "" {
		as.logger.Warn("LFAA: User message has no content")
		return nil
	}

	event := &storage.Event{
		OperatorSessionID: as.config.OperatorSessionId,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.UserMsg,
		ContentText:       content,
	}

	if _, err := as.auditStore.RecordEvent(event); err != nil {
		return fmt.Errorf("audit service: failed to record user message in audit store: %w", err)
	}

	as.logger.Info("User message recorded in audit store (LFAA)",
		"operator_session_id", as.config.OperatorSessionId,
		"content_length", len(content))
	return nil
}

// HandleAIMsgRequest records an inbound AI message to the audit store.
func (as *AuditService) HandleAIMsgRequest(ctx context.Context, msg *PubSubCommandMessage) error {
	as.logger.Info("LFAA: Recording AI message (via Protobuf)")

	if !as.auditStore.IsEnabled() {
		as.logger.Info("Audit store not enabled, skipping AI message recording")
		return nil
	}

	var protoMsg operatorv1.AuditMsgRequested
	if err := proto.Unmarshal(msg.Payload, &protoMsg); err != nil {
		return fmt.Errorf("audit service: failed to decode AI msg payload as protobuf AuditMsgRequested: %w", err)
	}
	content := protoMsg.Content
	if content == "" {
		as.logger.Warn("LFAA: AI message has no content")
		return nil
	}

	event := &storage.Event{
		OperatorSessionID: as.config.OperatorSessionId,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.AIMsg,
		ContentText:       content,
	}

	if _, err := as.auditStore.RecordEvent(event); err != nil {
		return fmt.Errorf("audit service: failed to record AI message in audit store: %w", err)
	}

	as.logger.Info("AI message recorded in audit store (LFAA)",
		"operator_session_id", as.config.OperatorSessionId,
		"content_length", len(content))
	return nil
}

// HandleDirectCmdRequest records an inbound direct terminal command to the audit store.
func (as *AuditService) HandleDirectCmdRequest(ctx context.Context, msg *PubSubCommandMessage) error {
	as.logger.Info("LFAA: Recording direct terminal command (via Protobuf)")

	if !as.auditStore.IsEnabled() {
		as.logger.Info("Audit store not enabled, skipping direct command recording")
		return nil
	}

	var protoCmd operatorv1.DirectCommandAuditRequested
	if err := proto.Unmarshal(msg.Payload, &protoCmd); err != nil {
		return fmt.Errorf("audit service: failed to decode direct cmd payload as protobuf DirectCommandAuditRequested: %w", err)
	}
	if protoCmd.Command == "" {
		as.logger.Warn("LFAA: Direct command audit has no command")
		return nil
	}

	event := &storage.Event{
		OperatorSessionID: as.config.OperatorSessionId,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       marshaler.Status(constants.AISourceTerminalDirect),
		CommandRaw:        protoCmd.Command,
	}

	if _, err := as.auditStore.RecordEvent(event); err != nil {
		return fmt.Errorf("audit service: failed to record direct command in audit store: %w", err)
	}

	as.logger.Info("Direct terminal command recorded in audit store (LFAA)",
		"operator_session_id", as.config.OperatorSessionId,
		"execution_id", protoCmd.ExecutionId)
	return nil
}

// HandleDirectCmdResultRequest records an inbound direct terminal command result to the audit store.
func (as *AuditService) HandleDirectCmdResultRequest(ctx context.Context, msg *PubSubCommandMessage) error {
	as.logger.Info("LFAA: Recording direct terminal command result (via Protobuf)")

	if !as.auditStore.IsEnabled() {
		as.logger.Info("Audit store not enabled, skipping direct command result recording")
		return nil
	}

	var protoResult operatorv1.DirectCommandResultAuditRequested
	if err := proto.Unmarshal(msg.Payload, &protoResult); err != nil {
		return fmt.Errorf("audit service: failed to decode direct cmd result payload as protobuf DirectCommandResultAuditRequested: %w", err)
	}
	if protoResult.Command == "" {
		as.logger.Warn("LFAA: Direct command result audit has no command")
		return nil
	}

	event := &storage.Event{
		OperatorSessionID:   as.config.OperatorSessionId,
		Timestamp:           time.Now().UTC(),
		Type:                constants.Event.Operator.Audit.Command,
		ContentText:         marshaler.Status(constants.AISourceTerminalDirect),
		CommandRaw:          protoResult.Command,
		CommandExitCode:     system.IntPtr(int(protoResult.ExitCode)),
		CommandStdout:       protoResult.Output,
		CommandStderr:       protoResult.Stderr,
		ExecutionDurationMs: int64(protoResult.ExecutionTimeSeconds * 1000),
	}

	if _, err := as.auditStore.RecordEvent(event); err != nil {
		return fmt.Errorf("audit service: failed to record direct command result in audit store: %w", err)
	}

	as.logger.Info("Direct terminal command result recorded in audit store (LFAA)",
		"operator_session_id", as.config.OperatorSessionId,
		"execution_id", protoResult.ExecutionId)
	return nil
}
