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
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
)

// AuditService owns LFAA audit event recording for user messages, AI messages,
// and direct terminal command capture.
type AuditService struct {
	config     *config.Config
	logger     *slog.Logger
	auditVault *storage.AuditVaultService
}

// NewAuditService creates a new AuditService.
func NewAuditService(cfg *config.Config, logger *slog.Logger) *AuditService {
	return &AuditService{
		config: cfg,
		logger: logger,
	}
}

// HandleUserMsgRequest records an inbound user message to the audit vault.
func (as *AuditService) HandleUserMsgRequest(_ context.Context, msg PubSubCommandMessage) {
	as.logger.Info("LFAA: Recording user message")

	if as.auditVault == nil || !as.auditVault.IsEnabled() {
		as.logger.Info("Audit vault not enabled, skipping user message recording")
		return
	}

	var ump models.AuditMsgRequestPayload
	if err := json.Unmarshal(msg.Payload, &ump); err != nil {
		as.logger.Error("Failed to decode audit user msg payload", "error", err)
		return
	}
	content := ump.Content
	if content == "" {
		as.logger.Warn("LFAA: User message has no content")
		return
	}

	event := &storage.Event{
		OperatorSessionID: as.config.OperatorSessionId,
		Timestamp:         time.Now().UTC(),
		Type:              storage.EventTypeUserMsg,
		ContentText:       content,
	}

	if _, err := as.auditVault.RecordEvent(event); err != nil {
		as.logger.Warn("Failed to record user message in audit vault", "error", err)
	} else {
		as.logger.Info("User message recorded in audit vault (LFAA)",
			"operator_session_id", as.config.OperatorSessionId,
			"content_length", len(content))
	}
}

// HandleAIMsgRequest records an inbound AI message to the audit vault.
func (as *AuditService) HandleAIMsgRequest(_ context.Context, msg PubSubCommandMessage) {
	as.logger.Info("LFAA: Recording AI message")

	if as.auditVault == nil || !as.auditVault.IsEnabled() {
		as.logger.Info("Audit vault not enabled, skipping AI message recording")
		return
	}

	var ap models.AuditMsgRequestPayload
	if err := json.Unmarshal(msg.Payload, &ap); err != nil {
		as.logger.Error("Failed to decode audit AI msg payload", "error", err)
		return
	}
	content := ap.Content
	if content == "" {
		as.logger.Warn("LFAA: AI message has no content")
		return
	}

	event := &storage.Event{
		OperatorSessionID: as.config.OperatorSessionId,
		Timestamp:         time.Now().UTC(),
		Type:              storage.EventTypeAIMsg,
		ContentText:       content,
	}

	if _, err := as.auditVault.RecordEvent(event); err != nil {
		as.logger.Warn("Failed to record AI message in audit vault", "error", err)
	} else {
		as.logger.Info("AI message recorded in audit vault (LFAA)",
			"operator_session_id", as.config.OperatorSessionId,
			"content_length", len(content))
	}
}

// HandleDirectCmdRequest records an inbound direct terminal command to the audit vault.
func (as *AuditService) HandleDirectCmdRequest(_ context.Context, msg PubSubCommandMessage) {
	as.logger.Info("LFAA: Recording direct terminal command")

	if as.auditVault == nil || !as.auditVault.IsEnabled() {
		as.logger.Info("Audit vault not enabled, skipping direct command recording")
		return
	}

	var p models.AuditDirectCmdRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		as.logger.Error("Failed to decode audit direct cmd payload", "error", err)
		return
	}
	if p.Command == "" {
		as.logger.Warn("LFAA: Direct command audit has no command")
		return
	}

	event := &storage.Event{
		OperatorSessionID: as.config.OperatorSessionId,
		Timestamp:         time.Now().UTC(),
		Type:              storage.EventTypeCmdExec,
		ContentText:       constants.Status.AISource.TerminalDirect,
		CommandRaw:        p.Command,
	}

	if _, err := as.auditVault.RecordEvent(event); err != nil {
		as.logger.Warn("Failed to record direct command in audit vault", "error", err)
	} else {
		as.logger.Info("Direct terminal command recorded in audit vault (LFAA)",
			"operator_session_id", as.config.OperatorSessionId,
			"execution_id", p.ExecutionID)
	}
}

// HandleDirectCmdResultRequest records an inbound direct terminal command result to the audit vault.
func (as *AuditService) HandleDirectCmdResultRequest(_ context.Context, msg PubSubCommandMessage) {
	as.logger.Info("LFAA: Recording direct terminal command result")

	if as.auditVault == nil || !as.auditVault.IsEnabled() {
		as.logger.Info("Audit vault not enabled, skipping direct command result recording")
		return
	}

	var p models.AuditDirectCmdResultPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		as.logger.Error("Failed to decode audit direct cmd result payload", "error", err)
		return
	}
	if p.Command == "" {
		as.logger.Warn("LFAA: Direct command result audit has no command")
		return
	}

	event := &storage.Event{
		OperatorSessionID:   as.config.OperatorSessionId,
		Timestamp:           time.Now().UTC(),
		Type:                storage.EventTypeCmdExec,
		ContentText:         constants.Status.AISource.TerminalDirect,
		CommandRaw:          p.Command,
		CommandExitCode:     p.ExitCode,
		CommandStdout:       p.Output,
		CommandStderr:       p.Stderr,
		ExecutionDurationMs: int64(p.ExecutionTimeSeconds * 1000),
	}

	if _, err := as.auditVault.RecordEvent(event); err != nil {
		as.logger.Warn("Failed to record direct command result in audit vault", "error", err)
	} else {
		as.logger.Info("Direct terminal command result recorded in audit vault (LFAA)",
			"operator_session_id", as.config.OperatorSessionId,
			"execution_id", p.ExecutionID)
	}
}
