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

package mappings

import (
	"github.com/g8e-ai/g8e/components/g8eo/internal/constants"
)

// MapEventTypeToActionType maps protobuf event types to UAP action types.
// This is the canonical mapping used across g8eo.
func MapEventTypeToActionType(eventType string) string {
	switch eventType {
	case constants.Event.Operator.Command.Requested:
		return "EXECUTE_BASH"
	case constants.Event.Operator.FileEdit.Requested:
		return "FILE_EDIT"
	case constants.Event.Operator.FsList.Requested:
		return "FS_LIST"
	case constants.Event.Operator.FsRead.Requested:
		return "FS_READ"
	case constants.Event.Operator.FsGrep.Requested:
		return "FS_GREP"
	case constants.Event.Operator.PortCheck.Requested:
		return "PORT_CHECK"
	case constants.Event.Operator.FetchLogs.Requested:
		return "FETCH_LOGS"
	case constants.Event.Operator.FetchHistory.Requested:
		return "FETCH_HISTORY"
	case constants.Event.Operator.FetchFileHistory.Requested:
		return "FETCH_FILE_HISTORY"
	case constants.Event.Operator.RestoreFile.Requested:
		return "RESTORE_FILE"
	case constants.Event.Operator.ShutdownRequested:
		return "SHUTDOWN"
	case constants.Event.Operator.HeartbeatRequested:
		return "HEARTBEAT"
	default:
		// For unknown event types, use the event type itself as action type
		return eventType
	}
}

// MapActionTypeToEventType maps UAP action types back to protobuf event types for handler dispatch.
// This is the canonical mapping used across g8eo.
func MapActionTypeToEventType(actionType string) string {
	switch actionType {
	case "EXECUTE_BASH":
		return constants.Event.Operator.Command.Requested
	case "FILE_EDIT":
		return constants.Event.Operator.FileEdit.Requested
	case "FS_LIST":
		return constants.Event.Operator.FsList.Requested
	case "FS_READ":
		return constants.Event.Operator.FsRead.Requested
	case "FS_GREP":
		return constants.Event.Operator.FsGrep.Requested
	case "PORT_CHECK":
		return constants.Event.Operator.PortCheck.Requested
	case "FETCH_LOGS":
		return constants.Event.Operator.FetchLogs.Requested
	case "FETCH_HISTORY":
		return constants.Event.Operator.FetchHistory.Requested
	case "FETCH_FILE_HISTORY":
		return constants.Event.Operator.FetchFileHistory.Requested
	case "RESTORE_FILE":
		return constants.Event.Operator.RestoreFile.Requested
	case "SHUTDOWN":
		return constants.Event.Operator.ShutdownRequested
	case "HEARTBEAT":
		return constants.Event.Operator.HeartbeatRequested
	default:
		// For unknown action types, use the action type itself as event type
		return actionType
	}
}

// MapEventTypeToResultActionType maps protobuf event types to UAP result action types.
// This is the canonical mapping used across g8eo.
func MapEventTypeToResultActionType(eventType string) string {
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
	case constants.Event.Operator.Command.StatusUpdated.Queued,
		constants.Event.Operator.Command.StatusUpdated.Running,
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
