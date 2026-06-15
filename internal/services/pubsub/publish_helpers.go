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

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// executionIDFromMessage resolves the execution_id for a command from the
// inbound payload's execution_id field using strict Protobuf extraction.
// If the payload does not carry one it falls back to the envelope id (msg.ID).
func executionIDFromMessage(msg *PubSubCommandMessage) string {
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

// AuditEventRecorder defines the interface for recording audit events.
type AuditEventRecorder interface {
	RecordEvent(event *storage.Event) (int64, error)
}

// publishLFAATypedResponseTo builds a GovernanceEnvelope from a typed payload and publishes it to the
// results channel. Used by services that hold a PubSubClient directly.
func publishLFAATypedResponseTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg *PubSubCommandMessage,
	eventType constants.EventType,
	payload proto.Message,
	auditStore AuditEventRecorder, // *storage.SQLAuditStore - optional for observed-state content evidence
	scrubbingService *scrubbing.ScrubbingService, // optional for scrubbing observed-state content
) {
	executionID := executionIDFromMessage(msg)
	setExecutionIDOnPayload(payload, executionID)

	env, err := BuildUniversalResultEnvelope(cfg, eventType, payload, msg.ID, cfg.OperatorID, msg.CaseID, msg.InvestigationID, msg.TaskID, msg.WebSessionID, msg.CLISessionID)
	if err != nil {
		logger.Error("Failed to build LFAA typed response Governance Envelope", string(constants.ConnectionStateError), err)
		return
	}

	data, err := protojson.Marshal(env)
	if err != nil {
		logger.Error("Failed to marshal LFAA typed response Governance Envelope", string(constants.ConnectionStateError), err)
		return
	}

	channelName := constants.ResultsChannel(cfg.OperatorID, msg.OperatorSessionID)
	if err := client.Publish(ctx, channelName, data); err != nil {
		logger.Error("Failed to publish LFAA typed response Universal", string(constants.ConnectionStateError), err)
		return
	}

	logger.Info("LFAA typed response published (Universal)", "event_type", eventType)

	// §3: observed-state content evidence (best-effort, non-fatal)
	publishObservedStateEvidence(ctx, logger, msg, eventType, payload, auditStore, scrubbingService)
}

// publishLFAAErrorTo builds an error GovernanceEnvelope and publishes it to the results channel.
func publishLFAAErrorTo(
	ctx context.Context,
	client PubSubClient,
	cfg *config.Config,
	logger *slog.Logger,
	msg *PubSubCommandMessage,
	eventType constants.EventType,
	errorMsg string,
	auditStore AuditEventRecorder, // *storage.SQLAuditStore - optional for observed-state content evidence
	scrubbingService *scrubbing.ScrubbingService, // optional for scrubbing observed-state content
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
		logger.Error("Failed to build LFAA error Governance Envelope", string(constants.ConnectionStateError), err)
		return
	}

	data, err := protojson.Marshal(env)
	if err != nil {
		logger.Error("Failed to marshal LFAA error Governance Envelope", string(constants.ConnectionStateError), err)
		return
	}

	channelName := constants.ResultsChannel(cfg.OperatorID, msg.OperatorSessionID)
	if err := client.Publish(ctx, channelName, data); err != nil {
		logger.Error("Failed to publish LFAA error Universal", string(constants.ConnectionStateError), err)
	}

	// §3: observed-state content evidence (best-effort, non-fatal)
	publishObservedStateEvidence(ctx, logger, msg, eventType, payload, auditStore, scrubbingService)
}

// publishObservedStateEvidence persists observed-state content evidence to the audit store.
// This is best-effort (non-fatal) and only applies to observe handlers (not commands).
// Commands already capture stdout/stderr via their own RecordEvent calls.
func publishObservedStateEvidence(
	ctx context.Context,
	logger *slog.Logger,
	msg *PubSubCommandMessage,
	eventType constants.EventType,
	payload proto.Message,
	auditStore AuditEventRecorder,
	scrubbingService *scrubbing.ScrubbingService,
) {
	if auditStore == nil {
		return
	}

	// Guard: only capture evidence for observe handlers, not commands
	// Commands already capture their own content via RecordEvent
	if isCommandEvent(eventType) {
		return
	}

	// Extract content text from the payload
	contentText := extractContentText(payload)
	if contentText == "" {
		return
	}

	// Scrub content before persisting (per position paper §10: raw data belongs only in raw vault)
	if scrubbingService != nil && scrubbingService.IsEnabled() {
		contentText = scrubbingService.ScrubText(contentText)
	}

	// Create event record
	event := &storage.Event{
		OperatorSessionID: msg.OperatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              eventType,
		ContentText:       contentText,
		StoredLocally:     true,
	}

	// Best-effort: log warning but don't fail the publish
	if _, err := auditStore.RecordEvent(event); err != nil {
		logger.Warn("Failed to record observed-state content evidence (non-fatal)",
			"event_type", eventType,
			"error", err)
	}
}

// isCommandEvent checks if the event type is a command event (which already captures its own content)
func isCommandEvent(eventType constants.EventType) bool {
	// Command events already capture stdout/stderr via their own RecordEvent calls
	// We should not double-record them here
	return eventType == constants.Event.Operator.Command.Completed ||
		eventType == constants.Event.Operator.Command.Failed ||
		eventType == constants.Event.Operator.FileEdit.Completed ||
		eventType == constants.Event.Operator.FileEdit.Failed
}

// extractContentText extracts human-readable content from observe handler payloads
func extractContentText(payload proto.Message) string {
	if payload == nil {
		return ""
	}

	// Handle different payload types
	switch p := payload.(type) {
	case *operatorv1.FsListResult:
		if len(p.Entries) > 0 {
			entriesJSON, _ := json.Marshal(p.Entries)
			return string(entriesJSON)
		}
		return ""
	case *operatorv1.FsReadResult:
		return p.Content
	case *operatorv1.FsGrepResult:
		if len(p.Matches) > 0 {
			matchesJSON, _ := json.Marshal(p.Matches)
			return string(matchesJSON)
		}
		return ""
	case *operatorv1.PortCheckResult:
		if len(p.Results) > 0 {
			resultsJSON, _ := json.Marshal(p.Results)
			return string(resultsJSON)
		}
		return ""
	case *operatorv1.FetchLogsResult:
		return fmt.Sprintf("command: %s, exit_code: %d, stdout_size: %d, stderr_size: %d",
			p.Command, p.ReturnCode, p.StdoutSize, p.StderrSize)
	case *operatorv1.FetchHistoryResult:
		return fmt.Sprintf("fetch_history: %d events", p.Total)
	case *operatorv1.FetchFileHistoryResult:
		return fmt.Sprintf("fetch_file_history: %s, %d entries", p.FilePath, len(p.History))
	case *operatorv1.FetchFileDiffResult:
		if p.Diff != nil {
			return fmt.Sprintf("fetch_file_diff: %s, operation: %s", p.Diff.FilePath, p.Diff.Operation)
		}
		return fmt.Sprintf("fetch_file_diff: %d diffs", p.Total)
	case *operatorv1.RestoreFileResult:
		return fmt.Sprintf("restore_file: %s, commit: %s", p.FilePath, p.CommitHash)
	case *operatorv1.HeartbeatResult:
		return fmt.Sprintf("heartbeat: %s, status: %s", p.OperatorId, p.Status)
	case *operatorv1.EvalAnswerRequested:
		return fmt.Sprintf("eval_answer: benchmark=%s, prompt_id=%s", p.Benchmark, p.PromptId)
	default:
		// Generic fallback: marshal to JSON
		if jsonBytes, err := json.Marshal(payload); err == nil {
			return string(jsonBytes)
		}
		return ""
	}
}
