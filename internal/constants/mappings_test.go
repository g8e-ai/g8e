// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

	t.Run("maps document update deterministically to the canonical update event", func(t *testing.T) {
		// ActionTypeDocumentUpdate is the canonical action for all app-level
		// document create/update events. Its reverse mapping is pinned to the
		// canonical EventAppDocumentUpdateRequested event so dispatch is stable
		// across process restarts, not one of the ambiguous app-level events.
		result := MapActionTypeToEventType(ActionTypeDocumentUpdate)
		assert.Equal(t, EventAppDocumentUpdateRequested, result)
	})

	t.Run("maps document delete deterministically to the canonical delete event", func(t *testing.T) {
		// ActionTypeDocumentDelete is pinned to the canonical
		// EventAppDocumentDeleteRequested event for the same determinism reason.
		result := MapActionTypeToEventType(ActionTypeDocumentDelete)
		assert.Equal(t, EventAppDocumentDeleteRequested, result)
	})

	t.Run("document action mapping is stable across many calls", func(t *testing.T) {
		// Regression guard for the nondeterministic reverse-map defect: repeated
		// calls must always return the same canonical event for both document
		// action types.
		for i := 0; i < 1000; i++ {
			assert.Equal(t, EventAppDocumentUpdateRequested, MapActionTypeToEventType(ActionTypeDocumentUpdate))
			assert.Equal(t, EventAppDocumentDeleteRequested, MapActionTypeToEventType(ActionTypeDocumentDelete))
		}
	})

	t.Run("eventToAction maps all app-level document create/update events to ActionTypeDocumentUpdate", func(t *testing.T) {
		updateEvents := []EventType{
			EventAppCaseCreated,
			EventAppCaseUpdated,
			EventAppMemoryCreated,
			EventAppMemoryUpdated,
			EventAppInvestigationCreated,
			EventAppInvestigationUpdated,
		}
		for _, e := range updateEvents {
			e := e
			t.Run(string(e), func(t *testing.T) {
				assert.Equal(t, ActionTypeDocumentUpdate, eventToAction[e],
					"%s must map to ActionTypeDocumentUpdate", e)
			})
		}
	})

	t.Run("eventToAction maps all app-level document delete events to ActionTypeDocumentDelete", func(t *testing.T) {
		deleteEvents := []EventType{
			EventAppCaseDeleted,
			EventAppInvestigationDeleted,
		}
		for _, e := range deleteEvents {
			e := e
			t.Run(string(e), func(t *testing.T) {
				assert.Equal(t, ActionTypeDocumentDelete, eventToAction[e],
					"%s must map to ActionTypeDocumentDelete", e)
			})
		}
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
