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
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"github.com/google/uuid"
)

// BuildUniversalEnvelope constructs a canonical v0.2.0 BFT envelope for cross-component communication.
func BuildUniversalEnvelope(
	cfg *config.Config,
	eventType string,
	payload proto.Message,
	originalID string, // Optional: for correlation if needed, but normally use payload.execution_id
) (*commonv1.GovernanceEnvelope, error) {
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Use originalID if provided, otherwise generate new UUID
	id := originalID
	if id == "" {
		id = uuid.NewString()
	}

	env := &commonv1.GovernanceEnvelope{
		Id:                id,
		Timestamp:         timestamppb.Now(),
		SourceComponent:   commonv1.Component_COMPONENT_G8EO,
		EventType:         eventType,
		OperatorId:        cfg.OperatorID,
		OperatorSessionId: cfg.OperatorSessionId,
		SystemFingerprint: cfg.SystemFingerprint,
		Payload:           payloadBytes,
	}

	// Attempt to extract metadata from payload via reflection if available
	reflectMsg := payload.ProtoReflect()
	md := reflectMsg.Descriptor()

	if fd := md.Fields().ByName("case_id"); fd != nil {
		env.CaseId = reflectMsg.Get(fd).String()
	}
	if fd := md.Fields().ByName("investigation_id"); fd != nil {
		env.InvestigationId = reflectMsg.Get(fd).String()
	}
	if fd := md.Fields().ByName("task_id"); fd != nil {
		env.TaskId = reflectMsg.Get(fd).String()
	}

	return env, nil
}

// BuildUAPResultEnvelope constructs a UAPEnvelope for result publishing.
// It preserves the original command's MessageID for correlation.
func BuildUAPResultEnvelope(
	cfg *config.Config,
	actionType string,
	payload proto.Message,
	originalMessageID string,
	senderID string,
	caseID string,
	investigationID string,
	taskID *string,
) (*uap.UAPEnvelope, error) {
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	env := &uap.UAPEnvelope{
		ProtocolVersion: "1.0",
		MessageID:       originalMessageID, // Will be regenerated if empty
		Metadata: uap.Metadata{
			SenderID:  senderID,
			Timestamp: time.Now(),
			Signature: "", // Operator signature would be added here in production
		},
		Intent: uap.Intent{
			ActionType:     actionType,
			TargetResource: "localhost", // Results are typically local
		},
		Context: uap.Context{
			DataFormat: "protobuf",
			DataBlob:   string(payloadBytes),
		},
		Consensus: uap.ConsensusState{
			RequiredVotes: 0, // Results don't require consensus
			CurrentVotes:  []uap.Vote{},
			Status:        "COMPLETED",
		},
		CaseID:          caseID,
		InvestigationID: investigationID,
		TaskID:          taskID,
		Payload:         payloadBytes,
	}

	// Generate MessageID if not provided (for correlation with command)
	if originalMessageID == "" {
		if id, err := env.GenerateMessageID(); err != nil {
			return nil, fmt.Errorf("failed to generate MessageID: %w", err)
		} else {
			env.MessageID = id
		}
	}

	return env, nil
}

// mapEventTypeToResultActionType maps protobuf event types to UAP result action types.
func mapEventTypeToResultActionType(eventType string) string {
	switch eventType {
	case constants.Event.Operator.Command.Completed, constants.Event.Operator.Command.Failed:
		return "EXECUTE_BASH_RESULT"
	case constants.Event.Operator.Command.Cancelled:
		return "EXECUTE_BASH_CANCELLED"
	case constants.Event.Operator.FileEdit.Completed, constants.Event.Operator.FileEdit.Failed:
		return "FILE_EDIT_RESULT"
	case constants.Event.Operator.FsList.Completed, constants.Event.Operator.FsList.Failed:
		return "FS_LIST_RESULT"
	case constants.Event.Operator.FsGrep.Completed, constants.Event.Operator.FsGrep.Failed:
		return "FS_GREP_RESULT"
	case constants.Event.Operator.Command.StatusUpdated.Running,
		constants.Event.Operator.Command.StatusUpdated.Queued,
		constants.Event.Operator.Command.StatusUpdated.Completed,
		constants.Event.Operator.Command.StatusUpdated.Failed,
		constants.Event.Operator.Command.StatusUpdated.Cancelled:
		return "EXECUTE_STATUS_UPDATE"
	case constants.Event.Operator.Heartbeat:
		return "HEARTBEAT_RESULT"
	default:
		// For unknown event types, use the event type itself as action type
		return eventType + "_RESULT"
	}
}

// validateL1Governance uses Protobuf reflection to find and enforce fields with the
// forbidden_patterns custom option.
func validateL1Governance(msg proto.Message) []string {
	var violations []string
	md := msg.ProtoReflect().Descriptor()
	fields := md.Fields()

	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		opts := fd.Options()
		if opts == nil {
			continue
		}

		if proto.HasExtension(opts, commonv1.E_ForbiddenPatterns) {
			patternsStr := proto.GetExtension(opts, commonv1.E_ForbiddenPatterns).(string)
			if patternsStr == "" {
				continue
			}

			val := msg.ProtoReflect().Get(fd)
			// Only strings are supported for forbidden_patterns currently
			if fd.Kind() == protoreflect.StringKind {
				strVal := val.String()
				patterns := strings.Split(patternsStr, ",")
				for _, p := range patterns {
					p = strings.TrimSpace(p)
					if p == "" {
						continue
					}
					matched, err := regexp.MatchString(p, strVal)
					if err == nil && matched {
						violations = append(violations, fmt.Sprintf("Field '%s' violates pattern '%s'", fd.Name(), p))
					}
				}
			}
		}
	}
	return violations
}

// MaxPayloadSize is the maximum allowed size for a protobuf payload (5MB).
const MaxPayloadSize = 5 * 1024 * 1024

// unmarshalPayload converts the raw bytes into a typed proto.Message for the given event type.
func unmarshalPayload(eventType string, payload []byte) (proto.Message, error) {
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("payload exceeds maximum size limit (%d bytes)", MaxPayloadSize)
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
	default:
		return nil, fmt.Errorf("unknown event type for unmarshaling: %s", eventType)
	}

	if err := proto.Unmarshal(payload, m); err != nil {
		return nil, err
	}
	return m, nil
}
