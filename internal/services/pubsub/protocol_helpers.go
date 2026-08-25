// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// mapProtoToPayloadType maps a protobuf message to its canonical g8e payload type string.
// This ensures synchronization with agent Pydantic models.
func mapProtoToPayloadType(msg proto.Message) string {
	switch m := msg.(type) {
	case *operatorv1.CommandResult:
		return "execution_result"
	case *operatorv1.ExecutionStatusUpdate:
		return "execution_status"
	case *operatorv1.FileEditResult:
		return "file_edit_result"
	case *operatorv1.FsListResult:
		return "fs_list_result"
	case *operatorv1.FsGrepResult:
		return "fs_grep_result"
	case *operatorv1.FsReadResult:
		return "fs_read_result"
	case *operatorv1.PortCheckResult:
		return "port_check_result"
	case *operatorv1.FetchLogsResult:
		if m.Error != "" {
			return "fetch_logs_error"
		}
		return "fetch_logs_result"
	case *operatorv1.FetchHistoryResult:
		if !m.Success {
			return "fetch_history_error"
		}
		return "fetch_history_success"
	case *operatorv1.FetchFileHistoryResult:
		if !m.Success {
			return "fetch_file_history_error"
		}
		return "fetch_file_history_success"
	case *operatorv1.RestoreFileResult:
		if !m.Success {
			return "restore_file_error"
		}
		return "restore_file_success"
	case *operatorv1.FetchFileDiffResult:
		if !m.Success {
			return "fetch_file_diff_error"
		}
		if m.Diff != nil {
			return "fetch_file_diff_by_id_success"
		}
		return "fetch_file_diff_by_session_success"
	case *operatorv1.HeartbeatResult:
		return "heartbeat"
	default:
		return string(constants.SystemHealthUnknown)
	}
}

// BuildUniversalResultEnvelope constructs a GovernanceEnvelope for result publishing.
// It preserves the original command's MessageID for correlation.
func BuildUniversalResultEnvelope(
	cfg *config.Config,
	eventType constants.EventType,
	payload proto.Message,
	originalMessageID string,
	senderID string,
	caseID string,
	investigationID string,
	taskID *string,
	webSessionID string,
	cliSessionID string,
) (*commonv1.GovernanceEnvelope, error) {
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Populate IntentData for JSON-first protocol (using JSON marshaling for simplicity during transition)
	var intentData map[string]interface{}
	jsonBytes, err := json.Marshal(payload)
	if err == nil {
		_ = json.Unmarshal(jsonBytes, &intentData)
	}

	// Inject canonical payload_type for consumer discriminator-based parsing (e.g., agent Pydantic)
	if intentData != nil {
		if _, ok := intentData["payload_type"]; !ok {
			intentData["payload_type"] = mapProtoToPayloadType(payload)
		}
	}

	// Convert map to structpb.Struct for GovernanceEnvelope
	var intentDataStruct *structpb.Struct
	if intentData != nil {
		if structBytes, err := json.Marshal(intentData); err == nil {
			intentDataStruct = &structpb.Struct{}
			_ = protojson.Unmarshal(structBytes, intentDataStruct)
		}
	}

	env := &commonv1.GovernanceEnvelope{
		Id:                originalMessageID, // Will be regenerated if empty
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
		SourceComponent:   commonv1.Component_COMPONENT_G8EO,
		OperatorId:        senderID,
		OperatorSessionId: cfg.OperatorSessionId,
		EventType:         string(eventType),
		ActionType:        string(constants.MapEventTypeToResultActionType(eventType)),
		Payload:           payloadBytes,
		IntentData:        intentDataStruct,
		CaseId:            caseID,
		InvestigationId:   investigationID,
		WebSessionId:      webSessionID,
		CliSessionId:      cliSessionID,
	}

	if taskID != nil {
		env.TaskId = *taskID
	}

	// Generate ID if not provided (for correlation with command)
	if env.Id == "" {
		// For now using simple UUID during transition, but eventually should be hash-based
		env.Id = fmt.Sprintf("res_%d", time.Now().UnixNano())
	}

	return env, nil
}

// MaxPayloadSize is the maximum allowed size for a protobuf payload (5MB).
const MaxPayloadSize = 5 * 1024 * 1024

// unmarshalPayload converts the raw bytes into a typed proto.Message for the given event type.
func unmarshalPayload(eventType constants.EventType, payload []byte) (proto.Message, error) {
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("payload exceeds maximum size limit (%d bytes): %w", MaxPayloadSize, constants.ErrPayloadExceedsLimit)
	}
	var m proto.Message
	switch eventType {
	case constants.Event.Operator.HeartbeatRequested:
		m = &operatorv1.HeartbeatRequested{}
	case constants.Event.Operator.Command.Requested:
		m = &operatorv1.CommandRequested{}
	case constants.Event.Operator.Command.CancelRequested:
		m = &operatorv1.CommandCancelRequested{}
	case constants.Event.Operator.FileEdit.Requested:
		m = &operatorv1.FileEditRequested{}
	case constants.Event.Operator.FsList.Requested:
		m = &operatorv1.FsListRequested{}
	case constants.Event.Operator.FsRead.Requested:
		m = &operatorv1.FsReadRequested{}
	case constants.Event.Operator.FsGrep.Requested:
		m = &operatorv1.FsGrepRequested{}
	case constants.Event.Operator.PortCheck.Requested:
		m = &operatorv1.CheckPortRequested{}
	case constants.Event.Operator.FetchLogs.Requested:
		m = &operatorv1.FetchLogsRequested{}
	case constants.Event.Operator.FetchHistory.Requested:
		m = &operatorv1.FetchHistoryRequested{}
	case constants.Event.Operator.FetchFileHistory.Requested:
		m = &operatorv1.FetchFileHistoryRequested{}
	case constants.Event.Operator.RestoreFile.Requested:
		m = &operatorv1.RestoreFileRequested{}
	case constants.Event.Operator.ShutdownRequested:
		m = &operatorv1.ShutdownRequested{}
	case constants.Event.Operator.Audit.UserMsg, constants.Event.Operator.Audit.AIMsg:
		m = &operatorv1.AuditMsgRequested{}
	case constants.Event.Operator.Audit.DirectCmd:
		m = &operatorv1.DirectCommandAuditRequested{}
	case constants.Event.Operator.Audit.DirectCmdResult:
		m = &operatorv1.DirectCommandResultAuditRequested{}
	case constants.Event.Operator.FetchFileDiff.Requested:
		m = &operatorv1.FetchFileDiffRequested{}
	case constants.Event.Operator.Mcp.CallRequested:
		m = &operatorv1.McpCallRequested{}
	case constants.Event.Operator.A2a.CallRequested:
		m = &operatorv1.A2ACallRequested{}
	case constants.Event.Operator.Eval.AnswerRequested:
		m = &operatorv1.EvalAnswerRequested{}
	case constants.EventPlatformEnrollmentCreateRequested,
		constants.EventPlatformEnrollmentDecideRequested,
		constants.EventPlatformEnrollmentIssueRequested,
		constants.EventPlatformEnrollmentPersistPolicyRequested,
		constants.EventPlatformEnrollmentCreateSessionRequested:
		m = &commonv1.PlatformEnrollmentGovernancePayload{}
	default:
		return nil, fmt.Errorf("unknown event type for unmarshaling: %s", eventType)
	}

	if err := proto.Unmarshal(payload, m); err != nil {
		return nil, err
	}
	return m, nil
}
