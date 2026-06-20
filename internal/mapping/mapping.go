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

// MapEventTypeToActionType maps protobuf event types to GovernanceEnvelope action types.
func MapEventTypeToActionType(eventType constants.EventType) constants.ActionType {
	switch eventType {
	case constants.Event.Operator.Eval.AnswerRequested:
		return constants.ActionTypeEvalAnswer
	case constants.Event.Operator.HeartbeatRequested:
		return constants.ActionTypeHeartbeat
	case constants.Event.Operator.ShutdownRequested:
		return constants.ActionTypeShutdown
	case constants.Event.Operator.Command.Requested:
		return constants.ActionTypeExecuteBash
	case constants.Event.Operator.Command.CancelRequested:
		return constants.ActionTypeCancel
	case constants.Event.Operator.FileEdit.Requested:
		return constants.ActionTypeFileEdit
	case constants.Event.Operator.FetchFileHistory.Requested:
		return constants.ActionTypeFetchFileHistory
	case constants.Event.Operator.RestoreFile.Requested:
		return constants.ActionTypeRestoreFile
	case constants.Event.Operator.FsList.Requested:
		return constants.ActionTypeFsList
	case constants.Event.Operator.FsRead.Requested:
		return constants.ActionTypeFsRead
	case constants.Event.Operator.FsGrep.Requested:
		return constants.ActionTypeFsGrep
	case constants.Event.Operator.FetchLogs.Requested:
		return constants.ActionTypeFetchLogs
	case constants.Event.Operator.FetchHistory.Requested:
		return constants.ActionTypeFetchHistory
	case constants.Event.Operator.Intent.Requested:
		return constants.ActionTypeGrantIntent
	case constants.Event.Operator.Intent.RevokeRequested:
		return constants.ActionTypeRevokeIntent
	case constants.Event.Operator.Mcp.CallRequested:
		return constants.ActionTypeMcpCall
	case constants.Event.Operator.A2a.CallRequested:
		return constants.ActionTypeA2aCall
	case constants.Event.Operator.PortCheck.Requested:
		return constants.ActionTypePortCheck
	case constants.EventAppInvestigationCreated:
		return constants.ActionTypeInvestigationCreate
	default:
		return constants.ActionType(eventType)
	}
}

// MapActionTypeToEventType maps GovernanceEnvelope action types back to protobuf event types.
func MapActionTypeToEventType(actionType constants.ActionType) constants.EventType {
	switch actionType {
	case constants.ActionTypeEvalAnswer:
		return constants.Event.Operator.Eval.AnswerRequested
	case constants.ActionTypeHeartbeat:
		return constants.Event.Operator.HeartbeatRequested
	case constants.ActionTypeShutdown:
		return constants.Event.Operator.ShutdownRequested
	case constants.ActionTypeExecuteBash:
		return constants.Event.Operator.Command.Requested
	case constants.ActionTypeCancel:
		return constants.Event.Operator.Command.CancelRequested
	case constants.ActionTypeFileEdit:
		return constants.Event.Operator.FileEdit.Requested
	case constants.ActionTypeFetchFileHistory:
		return constants.Event.Operator.FetchFileHistory.Requested
	case constants.ActionTypeRestoreFile:
		return constants.Event.Operator.RestoreFile.Requested
	case constants.ActionTypeFsList:
		return constants.Event.Operator.FsList.Requested
	case constants.ActionTypeFsRead:
		return constants.Event.Operator.FsRead.Requested
	case constants.ActionTypeFsGrep:
		return constants.Event.Operator.FsGrep.Requested
	case constants.ActionTypeFetchLogs:
		return constants.Event.Operator.FetchLogs.Requested
	case constants.ActionTypeFetchHistory:
		return constants.Event.Operator.FetchHistory.Requested
	case constants.ActionTypeGrantIntent:
		return constants.Event.Operator.Intent.Requested
	case constants.ActionTypeRevokeIntent:
		return constants.Event.Operator.Intent.RevokeRequested
	case constants.ActionTypeMcpCall:
		return constants.Event.Operator.Mcp.CallRequested
	case constants.ActionTypeA2aCall:
		return constants.Event.Operator.A2a.CallRequested
	case constants.ActionTypePortCheck:
		return constants.Event.Operator.PortCheck.Requested
	case constants.ActionTypeInvestigationCreate:
		return constants.EventAppInvestigationCreated
	default:
		return constants.EventType(actionType)
	}
}

// MapEventTypeToResultActionType maps protobuf event types to GovernanceEnvelope result action types.
func MapEventTypeToResultActionType(eventType constants.EventType) constants.ActionType {
	switch eventType {
	case constants.Event.Operator.Heartbeat:
		return constants.ActionType(string(constants.ActionTypeHeartbeat) + "_RESULT")
	case constants.Event.Operator.Command.Completed,
		constants.Event.Operator.Command.Failed:
		return constants.ActionType(string(constants.ActionTypeExecuteBash) + "_RESULT")
	case constants.Event.Operator.Command.Cancelled:
		return constants.ActionType(string(constants.ActionTypeExecuteBash) + "_CANCELLED")
	case constants.Event.Operator.Command.StatusUpdated.Queued,
		constants.Event.Operator.Command.StatusUpdated.Running,
		constants.Event.Operator.Command.StatusUpdated.Completed,
		constants.Event.Operator.Command.StatusUpdated.Failed,
		constants.Event.Operator.Command.StatusUpdated.Cancelled:
		return constants.ActionType("EXECUTE_STATUS_UPDATE")
	case constants.Event.Operator.FileEdit.Completed,
		constants.Event.Operator.FileEdit.Failed:
		return constants.ActionType(string(constants.ActionTypeFileEdit) + "_RESULT")
	case constants.Event.Operator.FsList.Completed,
		constants.Event.Operator.FsList.Failed:
		return constants.ActionType(string(constants.ActionTypeFsList) + "_RESULT")
	case constants.Event.Operator.FsGrep.Completed,
		constants.Event.Operator.FsGrep.Failed:
		return constants.ActionType(string(constants.ActionTypeFsGrep) + "_RESULT")
	default:
		return constants.ActionType(string(eventType) + "_RESULT")
	}
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
