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

package constants

import (
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
)

func TestMapEventTypeToActionType(t *testing.T) {
	t.Run("maps eval answer requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.Eval.AnswerRequested)
		assert.Equal(t, ActionTypeEvalAnswer, result)
	})

	t.Run("maps heartbeat requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.HeartbeatRequested)
		assert.Equal(t, ActionTypeHeartbeat, result)
	})

	t.Run("maps shutdown requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.ShutdownRequested)
		assert.Equal(t, ActionTypeShutdown, result)
	})

	t.Run("maps command requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.Command.Requested)
		assert.Equal(t, ActionTypeExecuteBash, result)
	})

	t.Run("maps file edit requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.FileEdit.Requested)
		assert.Equal(t, ActionTypeFileEdit, result)
	})

	t.Run("maps fetch file history requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.FetchFileHistory.Requested)
		assert.Equal(t, ActionTypeFetchFileHistory, result)
	})

	t.Run("maps restore file requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.RestoreFile.Requested)
		assert.Equal(t, ActionTypeRestoreFile, result)
	})

	t.Run("maps fs list requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.FsList.Requested)
		assert.Equal(t, ActionTypeFsList, result)
	})

	t.Run("maps fs read requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.FsRead.Requested)
		assert.Equal(t, ActionTypeFsRead, result)
	})

	t.Run("maps fs grep requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.FsGrep.Requested)
		assert.Equal(t, ActionTypeFsGrep, result)
	})

	t.Run("maps fetch logs requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.FetchLogs.Requested)
		assert.Equal(t, ActionTypeFetchLogs, result)
	})

	t.Run("maps fetch history requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.FetchHistory.Requested)
		assert.Equal(t, ActionTypeFetchHistory, result)
	})

	t.Run("maps intent requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.Intent.Requested)
		assert.Equal(t, ActionTypeGrantIntent, result)
	})

	t.Run("maps intent revoke requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.Intent.RevokeRequested)
		assert.Equal(t, ActionTypeRevokeIntent, result)
	})

	t.Run("maps mcp call requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.Mcp.CallRequested)
		assert.Equal(t, ActionTypeMcpCall, result)
	})

	t.Run("maps a2a call requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.A2a.CallRequested)
		assert.Equal(t, ActionTypeA2aCall, result)
	})

	t.Run("maps port check requested", func(t *testing.T) {
		result := MapEventTypeToActionType(Event.Operator.PortCheck.Requested)
		assert.Equal(t, ActionTypePortCheck, result)
	})

	t.Run("maps investigation created", func(t *testing.T) {
		result := MapEventTypeToActionType(EventAppInvestigationCreated)
		assert.Equal(t, ActionTypeInvestigationCreate, result)
	})

	t.Run("passes through unknown event types as string", func(t *testing.T) {
		unknownEvent := EventType("g8e.v1.unknown.event")
		result := MapEventTypeToActionType(unknownEvent)
		assert.Equal(t, ActionType(unknownEvent), result)
	})
}

func TestMapActionTypeToEventType(t *testing.T) {
	t.Run("maps eval answer", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeEvalAnswer)
		assert.Equal(t, Event.Operator.Eval.AnswerRequested, result)
	})

	t.Run("maps heartbeat", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeHeartbeat)
		assert.Equal(t, Event.Operator.HeartbeatRequested, result)
	})

	t.Run("maps shutdown", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeShutdown)
		assert.Equal(t, Event.Operator.ShutdownRequested, result)
	})

	t.Run("maps execute bash", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeExecuteBash)
		assert.Equal(t, Event.Operator.Command.Requested, result)
	})

	t.Run("maps file edit", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeFileEdit)
		assert.Equal(t, Event.Operator.FileEdit.Requested, result)
	})

	t.Run("maps fetch file history", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeFetchFileHistory)
		assert.Equal(t, Event.Operator.FetchFileHistory.Requested, result)
	})

	t.Run("maps restore file", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeRestoreFile)
		assert.Equal(t, Event.Operator.RestoreFile.Requested, result)
	})

	t.Run("maps fs list", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeFsList)
		assert.Equal(t, Event.Operator.FsList.Requested, result)
	})

	t.Run("maps fs read", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeFsRead)
		assert.Equal(t, Event.Operator.FsRead.Requested, result)
	})

	t.Run("maps fs grep", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeFsGrep)
		assert.Equal(t, Event.Operator.FsGrep.Requested, result)
	})

	t.Run("maps fetch logs", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeFetchLogs)
		assert.Equal(t, Event.Operator.FetchLogs.Requested, result)
	})

	t.Run("maps fetch history", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeFetchHistory)
		assert.Equal(t, Event.Operator.FetchHistory.Requested, result)
	})

	t.Run("maps grant intent", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeGrantIntent)
		assert.Equal(t, Event.Operator.Intent.Requested, result)
	})

	t.Run("maps revoke intent", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeRevokeIntent)
		assert.Equal(t, Event.Operator.Intent.RevokeRequested, result)
	})

	t.Run("maps mcp call", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeMcpCall)
		assert.Equal(t, Event.Operator.Mcp.CallRequested, result)
	})

	t.Run("maps a2a call", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeA2aCall)
		assert.Equal(t, Event.Operator.A2a.CallRequested, result)
	})

	t.Run("maps port check", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypePortCheck)
		assert.Equal(t, Event.Operator.PortCheck.Requested, result)
	})

	t.Run("maps investigation create", func(t *testing.T) {
		result := MapActionTypeToEventType(ActionTypeInvestigationCreate)
		assert.Equal(t, EventAppInvestigationCreated, result)
	})

	t.Run("passes through unknown action types as string", func(t *testing.T) {
		unknownAction := ActionType("UNKNOWN_ACTION")
		result := MapActionTypeToEventType(unknownAction)
		assert.Equal(t, EventType(unknownAction), result)
	})
}

func TestMapEventTypeToResultActionType(t *testing.T) {
	t.Run("maps heartbeat to result", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.Heartbeat)
		assert.Equal(t, ActionType("HEARTBEAT_RESULT"), result)
	})

	t.Run("maps command completed to result", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.Command.Completed)
		assert.Equal(t, ActionType("EXECUTE_BASH_RESULT"), result)
	})

	t.Run("maps command failed to result", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.Command.Failed)
		assert.Equal(t, ActionType("EXECUTE_BASH_RESULT"), result)
	})

	t.Run("maps command cancelled to cancelled", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.Command.Cancelled)
		assert.Equal(t, ActionType("EXECUTE_BASH_CANCELLED"), result)
	})

	t.Run("maps command status updated queued to status update", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.Command.StatusUpdated.Queued)
		assert.Equal(t, ActionType("EXECUTE_STATUS_UPDATE"), result)
	})

	t.Run("maps command status updated running to status update", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.Command.StatusUpdated.Running)
		assert.Equal(t, ActionType("EXECUTE_STATUS_UPDATE"), result)
	})

	t.Run("maps command status updated completed to status update", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.Command.StatusUpdated.Completed)
		assert.Equal(t, ActionType("EXECUTE_STATUS_UPDATE"), result)
	})

	t.Run("maps command status updated failed to status update", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.Command.StatusUpdated.Failed)
		assert.Equal(t, ActionType("EXECUTE_STATUS_UPDATE"), result)
	})

	t.Run("maps command status updated cancelled to status update", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.Command.StatusUpdated.Cancelled)
		assert.Equal(t, ActionType("EXECUTE_STATUS_UPDATE"), result)
	})

	t.Run("maps file edit completed to result", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.FileEdit.Completed)
		assert.Equal(t, ActionType("FILE_EDIT_RESULT"), result)
	})

	t.Run("maps file edit failed to result", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.FileEdit.Failed)
		assert.Equal(t, ActionType("FILE_EDIT_RESULT"), result)
	})

	t.Run("maps fs list completed to result", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.FsList.Completed)
		assert.Equal(t, ActionType("FS_LIST_RESULT"), result)
	})

	t.Run("maps fs list failed to result", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.FsList.Failed)
		assert.Equal(t, ActionType("FS_LIST_RESULT"), result)
	})

	t.Run("maps fs grep completed to result", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.FsGrep.Completed)
		assert.Equal(t, ActionType("FS_GREP_RESULT"), result)
	})

	t.Run("maps fs grep failed to result", func(t *testing.T) {
		result := MapEventTypeToResultActionType(Event.Operator.FsGrep.Failed)
		assert.Equal(t, ActionType("FS_GREP_RESULT"), result)
	})

	t.Run("appends _RESULT suffix to unknown event types", func(t *testing.T) {
		unknownEvent := EventType("g8e.v1.unknown.event")
		result := MapEventTypeToResultActionType(unknownEvent)
		assert.Equal(t, ActionType("g8e.v1.unknown.event_RESULT"), result)
	})
}

func TestProtoToExecutionStatus(t *testing.T) {
	t.Run("maps unspecified to pending", func(t *testing.T) {
		result := ProtoToExecutionStatus(operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED)
		assert.Equal(t, ExecutionStatusPending, result)
	})

	t.Run("maps executing to executing", func(t *testing.T) {
		result := ProtoToExecutionStatus(operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING)
		assert.Equal(t, ExecutionStatusExecuting, result)
	})

	t.Run("maps completed to completed", func(t *testing.T) {
		result := ProtoToExecutionStatus(operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED)
		assert.Equal(t, ExecutionStatusCompleted, result)
	})

	t.Run("maps failed to failed", func(t *testing.T) {
		result := ProtoToExecutionStatus(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED)
		assert.Equal(t, ExecutionStatusFailed, result)
	})

	t.Run("maps timeout to timeout", func(t *testing.T) {
		result := ProtoToExecutionStatus(operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT)
		assert.Equal(t, ExecutionStatusTimeout, result)
	})

	t.Run("maps cancelled to cancelled", func(t *testing.T) {
		result := ProtoToExecutionStatus(operatorv1.ExecutionStatus_EXECUTION_STATUS_CANCELLED)
		assert.Equal(t, ExecutionStatusCancelled, result)
	})

	t.Run("maps unknown status to pending", func(t *testing.T) {
		result := ProtoToExecutionStatus(operatorv1.ExecutionStatus(999))
		assert.Equal(t, ExecutionStatusPending, result)
	})
}

func TestMappingRoundTrip(t *testing.T) {
	t.Run("event type to action type and back", func(t *testing.T) {
		originalEvent := Event.Operator.Command.Requested
		actionType := MapEventTypeToActionType(originalEvent)
		resultEvent := MapActionTypeToEventType(actionType)
		assert.Equal(t, originalEvent, resultEvent)
	})

	t.Run("action type to event type and back", func(t *testing.T) {
		originalAction := ActionTypeFileEdit
		eventType := MapActionTypeToEventType(originalAction)
		resultAction := MapEventTypeToActionType(eventType)
		assert.Equal(t, originalAction, resultAction)
	})
}
