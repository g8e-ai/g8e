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

package mapping

import (
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/internal/constants"
)

// eventToAction is the single source of truth for the EventType <-> ActionType
// relationship. The reverse map (actionToEvent) is derived from this in init().
// Add new pairs here only — never touch actionToEvent directly.
var eventToAction = map[constants.EventType]constants.ActionType{
	constants.Event.Operator.Eval.AnswerRequested:       constants.ActionTypeEvalAnswer,
	constants.Event.Operator.HeartbeatRequested:         constants.ActionTypeHeartbeat,
	constants.Event.Operator.ShutdownRequested:          constants.ActionTypeShutdown,
	constants.Event.Operator.Command.Requested:          constants.ActionTypeExecuteBash,
	constants.Event.Operator.Command.CancelRequested:    constants.ActionTypeCancel,
	constants.Event.Operator.FileEdit.Requested:         constants.ActionTypeFileEdit,
	constants.Event.Operator.FetchFileHistory.Requested: constants.ActionTypeFetchFileHistory,
	constants.Event.Operator.RestoreFile.Requested:      constants.ActionTypeRestoreFile,
	constants.Event.Operator.FsList.Requested:           constants.ActionTypeFsList,
	constants.Event.Operator.FsRead.Requested:           constants.ActionTypeFsRead,
	constants.Event.Operator.FsGrep.Requested:           constants.ActionTypeFsGrep,
	constants.Event.Operator.FetchLogs.Requested:        constants.ActionTypeFetchLogs,
	constants.Event.Operator.FetchHistory.Requested:     constants.ActionTypeFetchHistory,
	constants.Event.Operator.Mcp.CallRequested:          constants.ActionTypeMcpCall,
	constants.Event.Operator.A2a.CallRequested:          constants.ActionTypeA2aCall,
	constants.Event.Operator.PortCheck.Requested:        constants.ActionTypePortCheck,
	constants.EventAppInvestigationCreated:              constants.ActionTypeInvestigationCreate,
}

var actionToEvent map[constants.ActionType]constants.EventType

func init() {
	actionToEvent = make(map[constants.ActionType]constants.EventType, len(eventToAction))
	for e, a := range eventToAction {
		actionToEvent[a] = e
	}
}

// MapEventTypeToActionType maps protobuf event types to GovernanceEnvelope action types.
func MapEventTypeToActionType(eventType constants.EventType) constants.ActionType {
	if a, ok := eventToAction[eventType]; ok {
		return a
	}
	return constants.ActionType(eventType)
}

// MapActionTypeToEventType maps GovernanceEnvelope action types back to protobuf event types.
func MapActionTypeToEventType(actionType constants.ActionType) constants.EventType {
	if e, ok := actionToEvent[actionType]; ok {
		return e
	}
	return constants.EventType(actionType)
}

func actionResult(a constants.ActionType) constants.ActionType {
	return constants.ActionType(string(a) + "_RESULT")
}

func actionCancelled(a constants.ActionType) constants.ActionType {
	return constants.ActionType(string(a) + "_CANCELLED")
}

var eventToResultAction = map[constants.EventType]constants.ActionType{
	constants.Event.Operator.Heartbeat: actionResult(constants.ActionTypeHeartbeat),

	constants.Event.Operator.Command.Completed: actionResult(constants.ActionTypeExecuteBash),
	constants.Event.Operator.Command.Failed:    actionResult(constants.ActionTypeExecuteBash),
	constants.Event.Operator.Command.Cancelled: actionCancelled(constants.ActionTypeExecuteBash),

	constants.Event.Operator.Command.StatusUpdated.Queued:    "EXECUTE_STATUS_UPDATE",
	constants.Event.Operator.Command.StatusUpdated.Running:   "EXECUTE_STATUS_UPDATE",
	constants.Event.Operator.Command.StatusUpdated.Completed: "EXECUTE_STATUS_UPDATE",
	constants.Event.Operator.Command.StatusUpdated.Failed:    "EXECUTE_STATUS_UPDATE",
	constants.Event.Operator.Command.StatusUpdated.Cancelled: "EXECUTE_STATUS_UPDATE",

	constants.Event.Operator.FileEdit.Completed: actionResult(constants.ActionTypeFileEdit),
	constants.Event.Operator.FileEdit.Failed:    actionResult(constants.ActionTypeFileEdit),

	constants.Event.Operator.FsList.Completed: actionResult(constants.ActionTypeFsList),
	constants.Event.Operator.FsList.Failed:    actionResult(constants.ActionTypeFsList),

	constants.Event.Operator.FsGrep.Completed: actionResult(constants.ActionTypeFsGrep),
	constants.Event.Operator.FsGrep.Failed:    actionResult(constants.ActionTypeFsGrep),
}

// MapEventTypeToResultActionType maps protobuf event types to GovernanceEnvelope result action types.
func MapEventTypeToResultActionType(eventType constants.EventType) constants.ActionType {
	if a, ok := eventToResultAction[eventType]; ok {
		return a
	}
	return actionResult(constants.ActionType(eventType))
}

// ProtoToExecutionStatus maps protobuf ExecutionStatus enum to internal ExecutionStatus constants.
func ProtoToExecutionStatus(status operatorv1.ExecutionStatus) constants.ExecutionStatus {
	switch status {
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED:
		return constants.ExecutionStatusPending
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING:
		return constants.ExecutionStatusExecuting
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED:
		return constants.ExecutionStatusCompleted
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED:
		return constants.ExecutionStatusFailed
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT:
		return constants.ExecutionStatusTimeout
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_CANCELLED:
		return constants.ExecutionStatusCancelled
	default:
		return constants.ExecutionStatusPending
	}
}
