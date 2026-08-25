// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	storage "github.com/g8e-ai/g8e/v2/internal/services/storage"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
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

// recordMessage records a user or AI message to the audit store.
func (as *AuditService) recordMessage(ctx context.Context, msg *PubSubCommandMessage, eventType constants.EventType, unmarshalErr, recordErr error) error {
	var protoMsg operatorv1.AuditMsgRequested
	if err := proto.Unmarshal(msg.Payload, &protoMsg); err != nil {
		return fmt.Errorf("audit service: %w: %w", unmarshalErr, err)
	}
	content := protoMsg.Content
	if content == "" {
		as.logger.Warn("LFAA: Message has no content")
		return nil
	}

	event := &storage.Event{
		OperatorSessionID: as.config.OperatorSessionId,
		Timestamp:         time.Now().UTC(),
		Type:              eventType,
		ContentText:       content,
	}

	if _, err := as.auditStore.RecordEvent(event); err != nil {
		return fmt.Errorf("audit service: %w: %w", recordErr, err)
	}

	as.logger.Info("Message recorded in audit store (LFAA)",
		"operator_session_id", as.config.OperatorSessionId,
		"content_length", len(content))
	return nil
}

// HandleUserMsgRequest records an inbound user message to the audit store.
func (as *AuditService) HandleUserMsgRequest(ctx context.Context, msg *PubSubCommandMessage) error {
	as.logger.Info("LFAA: Recording user message (via Protobuf)")
	return as.recordMessage(ctx, msg, constants.Event.Operator.Audit.UserMsg, constants.ErrAuditUnmarshalUserMsg, constants.ErrAuditRecordUserMsg)
}

// HandleAIMsgRequest records an inbound AI message to the audit store.
func (as *AuditService) HandleAIMsgRequest(ctx context.Context, msg *PubSubCommandMessage) error {
	as.logger.Info("LFAA: Recording AI message (via Protobuf)")
	return as.recordMessage(ctx, msg, constants.Event.Operator.Audit.AIMsg, constants.ErrAuditUnmarshalAIMsg, constants.ErrAuditRecordAIMsg)
}

// recordDirectCommand records a direct terminal command to the audit store.
func (as *AuditService) recordDirectCommand(ctx context.Context, msg *PubSubCommandMessage, unmarshalErr, recordErr error, withResult bool) error {
	var event *storage.Event

	if withResult {
		var protoResult operatorv1.DirectCommandResultAuditRequested
		if err := proto.Unmarshal(msg.Payload, &protoResult); err != nil {
			return fmt.Errorf("audit service: %w: %w", unmarshalErr, err)
		}
		if protoResult.Command == "" {
			as.logger.Warn("LFAA: Direct command result audit has no command")
			return nil
		}

		event = &storage.Event{
			OperatorSessionID:   as.config.OperatorSessionId,
			Timestamp:           time.Now().UTC(),
			Type:                constants.Event.Operator.Audit.Command,
			ContentText:         marshaler.Status(constants.AISourceTerminalDirect),
			CommandRaw:          protoResult.Command,
			CommandExitCode:     int(protoResult.ExitCode),
			CommandStdout:       protoResult.Output,
			CommandStderr:       protoResult.Stderr,
			ExecutionDurationMs: int64(protoResult.ExecutionTimeSeconds * 1000),
		}

		as.logger.Info("Direct terminal command result recorded in audit store (LFAA)",
			"operator_session_id", as.config.OperatorSessionId,
			"execution_id", protoResult.ExecutionId)
	} else {
		var protoCmd operatorv1.DirectCommandAuditRequested
		if err := proto.Unmarshal(msg.Payload, &protoCmd); err != nil {
			return fmt.Errorf("audit service: %w: %w", unmarshalErr, err)
		}
		if protoCmd.Command == "" {
			as.logger.Warn("LFAA: Direct command audit has no command")
			return nil
		}

		event = &storage.Event{
			OperatorSessionID: as.config.OperatorSessionId,
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       marshaler.Status(constants.AISourceTerminalDirect),
			CommandRaw:        protoCmd.Command,
			CommandExitCode:   constants.ExitCodeNone,
		}

		as.logger.Info("Direct terminal command recorded in audit store (LFAA)",
			"operator_session_id", as.config.OperatorSessionId,
			"execution_id", protoCmd.ExecutionId)
	}

	if _, err := as.auditStore.RecordEvent(event); err != nil {
		return fmt.Errorf("audit service: %w: %w", recordErr, err)
	}

	return nil
}

// HandleDirectCmdRequest records an inbound direct terminal command to the audit store.
func (as *AuditService) HandleDirectCmdRequest(ctx context.Context, msg *PubSubCommandMessage) error {
	as.logger.Info("LFAA: Recording direct terminal command (via Protobuf)")
	return as.recordDirectCommand(ctx, msg, constants.ErrAuditUnmarshalDirectCmd, constants.ErrAuditRecordDirectCmd, false)
}

// HandleDirectCmdResultRequest records an inbound direct terminal command result to the audit store.
func (as *AuditService) HandleDirectCmdResultRequest(ctx context.Context, msg *PubSubCommandMessage) error {
	as.logger.Info("LFAA: Recording direct terminal command result (via Protobuf)")
	return as.recordDirectCommand(ctx, msg, constants.ErrAuditUnmarshalDirectResult, constants.ErrAuditRecordDirectResult, true)
}
