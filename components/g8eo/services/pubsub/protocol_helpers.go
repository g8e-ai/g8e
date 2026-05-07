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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
)

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

// unmarshalPayload converts the raw bytes into a typed proto.Message for the given event type.
func unmarshalPayload(eventType string, payload []byte) (proto.Message, error) {
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
