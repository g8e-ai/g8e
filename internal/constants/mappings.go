// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// eventToAction is the single source of truth for the EventType <-> ActionType
// relationship. The reverse map (actionToEvent) is derived from this in init().
// Add new pairs here only — never touch actionToEvent directly.
var eventToAction = map[EventType]ActionType{
	Event.Operator.Eval.AnswerRequested:           ActionTypeEvalAnswer,
	Event.Operator.HeartbeatRequested:             ActionTypeHeartbeat,
	Event.Operator.ShutdownRequested:              ActionTypeShutdown,
	Event.Operator.Command.Requested:              ActionTypeExecuteBash,
	Event.Operator.Command.CancelRequested:        ActionTypeCancel,
	Event.Operator.FileEdit.Requested:             ActionTypeFileEdit,
	Event.Operator.FetchFileHistory.Requested:     ActionTypeFetchFileHistory,
	Event.Operator.RestoreFile.Requested:          ActionTypeRestoreFile,
	Event.Operator.FsList.Requested:               ActionTypeFsList,
	Event.Operator.FsRead.Requested:               ActionTypeFsRead,
	Event.Operator.FsGrep.Requested:               ActionTypeFsGrep,
	Event.Operator.FetchLogs.Requested:            ActionTypeFetchLogs,
	Event.Operator.FetchHistory.Requested:         ActionTypeFetchHistory,
	Event.Operator.Mcp.CallRequested:              ActionTypeMcpCall,
	Event.Operator.A2a.CallRequested:              ActionTypeA2aCall,
	Event.Operator.PortCheck.Requested:            ActionTypePortCheck,
	EventAppCaseCreated:                           ActionTypeDocumentUpdate,
	EventAppCaseUpdated:                           ActionTypeDocumentUpdate,
	EventAppCaseDeleted:                           ActionTypeDocumentDelete,
	EventAppMemoryCreated:                         ActionTypeDocumentUpdate,
	EventAppMemoryUpdated:                         ActionTypeDocumentUpdate,
	EventAppInvestigationCreated:                  ActionTypeDocumentUpdate,
	EventAppInvestigationUpdated:                  ActionTypeDocumentUpdate,
	EventAppInvestigationDeleted:                  ActionTypeDocumentDelete,
	EventAppDocumentUpdateRequested:               ActionTypeDocumentUpdate,
	EventAppDocumentDeleteRequested:               ActionTypeDocumentDelete,
	EventPlatformEnrollmentCreateRequested:        ActionTypePlatformEnrollmentCreate,
	EventPlatformEnrollmentDecideRequested:        ActionTypePlatformEnrollmentDecide,
	EventPlatformEnrollmentIssueRequested:         ActionTypePlatformEnrollmentIssue,
	EventPlatformEnrollmentPersistPolicyRequested: ActionTypePlatformEnrollmentPersistPolicy,
	EventPlatformEnrollmentCreateSessionRequested: ActionTypePlatformEnrollmentCreateSession,
}

var actionToEvent map[ActionType]EventType

// canonicalActionEvent pins a deterministic EventType for action types whose
// reverse mapping in actionToEvent is ambiguous. Document mutations are emitted
// under several app-level event names (case/investigation/memory
// create/update/delete) that all collapse to DOCUMENT_UPDATE/DOCUMENT_DELETE, so
// the derived reverse map is many-to-one and its Go-map iteration would pick a
// nondeterministic EventType per process start. Pinning the canonical
// document-request events here keeps handler dispatch and the EventType stamped
// on the command message and signed receipt stable across repeated calls. This
// map takes precedence over the derived reverse map.
var canonicalActionEvent = map[ActionType]EventType{
	ActionTypeDocumentUpdate: EventAppDocumentUpdateRequested,
	ActionTypeDocumentDelete: EventAppDocumentDeleteRequested,
}

func init() {
	actionToEvent = make(map[ActionType]EventType, len(eventToAction))
	for e, a := range eventToAction {
		actionToEvent[a] = e
	}
	// Enforce the canonical pins so the derived reverse map cannot shadow them
	// with a nondeterministically chosen app-level event.
	for a, e := range canonicalActionEvent {
		actionToEvent[a] = e
	}
}

// MapActionTypeToEventType maps GovernanceEnvelope action types back to protobuf
// event types. Action types with a canonical pin (document mutations) resolve
// deterministically; all others use the reverse map derived from eventToAction.
// Unmapped action types pass through as-is.
func MapActionTypeToEventType(actionType ActionType) EventType {
	if e, ok := canonicalActionEvent[actionType]; ok {
		return e
	}
	if e, ok := actionToEvent[actionType]; ok {
		return e
	}
	return EventType(actionType)
}

func actionResult(a ActionType) ActionType {
	return ActionType(string(a) + "_RESULT")
}

func actionCancelled(a ActionType) ActionType {
	return ActionType(string(a) + "_CANCELLED")
}

var eventToResultAction = map[EventType]ActionType{
	Event.Operator.Heartbeat: actionResult(ActionTypeHeartbeat),

	Event.Operator.Command.Completed: actionResult(ActionTypeExecuteBash),
	Event.Operator.Command.Failed:    actionResult(ActionTypeExecuteBash),
	Event.Operator.Command.Cancelled: actionCancelled(ActionTypeExecuteBash),

	Event.Operator.Command.StatusUpdated.Queued:    "EXECUTE_STATUS_UPDATE",
	Event.Operator.Command.StatusUpdated.Running:   "EXECUTE_STATUS_UPDATE",
	Event.Operator.Command.StatusUpdated.Completed: "EXECUTE_STATUS_UPDATE",
	Event.Operator.Command.StatusUpdated.Failed:    "EXECUTE_STATUS_UPDATE",
	Event.Operator.Command.StatusUpdated.Cancelled: "EXECUTE_STATUS_UPDATE",

	Event.Operator.FileEdit.Completed: actionResult(ActionTypeFileEdit),
	Event.Operator.FileEdit.Failed:    actionResult(ActionTypeFileEdit),

	Event.Operator.FsList.Completed: actionResult(ActionTypeFsList),
	Event.Operator.FsList.Failed:    actionResult(ActionTypeFsList),

	Event.Operator.FsGrep.Completed: actionResult(ActionTypeFsGrep),
	Event.Operator.FsGrep.Failed:    actionResult(ActionTypeFsGrep),
}

// MapEventTypeToResultActionType maps protobuf event types to GovernanceEnvelope result action types.
func MapEventTypeToResultActionType(eventType EventType) ActionType {
	if a, ok := eventToResultAction[eventType]; ok {
		return a
	}
	return actionResult(ActionType(eventType))
}
