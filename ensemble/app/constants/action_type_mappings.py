# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Hand-authored mapping functions for event/action type conversion

from app.constants import EventType


def map_event_type_to_action_type(event_type: str) -> str:
    """Map protobuf event types to UAP action types."""
    mapping = {
        EventType.OPERATOR_EVAL_ANSWER_REQUESTED: "EVAL_ANSWER",
        EventType.OPERATOR_HEARTBEAT_REQUESTED: "HEARTBEAT",
        EventType.OPERATOR_SHUTDOWN_REQUESTED: "SHUTDOWN",
        EventType.OPERATOR_COMMAND_REQUESTED: "EXECUTE_BASH",
        EventType.OPERATOR_FILE_EDIT_REQUESTED: "FILE_EDIT",
        EventType.OPERATOR_FILE_HISTORY_FETCH_REQUESTED: "FETCH_FILE_HISTORY",
        EventType.OPERATOR_FILE_RESTORE_REQUESTED: "RESTORE_FILE",
        EventType.OPERATOR_FILESYSTEM_LIST_REQUESTED: "FS_LIST",
        EventType.OPERATOR_FILESYSTEM_READ_REQUESTED: "FS_READ",
        EventType.OPERATOR_FILESYSTEM_GREP_REQUESTED: "FS_GREP",
        EventType.OPERATOR_LOGS_FETCH_REQUESTED: "FETCH_LOGS",
        EventType.OPERATOR_HISTORY_FETCH_REQUESTED: "FETCH_HISTORY",
        EventType.OPERATOR_INTENT_REQUESTED: "GRANT_INTENT",
        EventType.OPERATOR_INTENT_REVOKE_REQUESTED: "REVOKE_INTENT",
        EventType.OPERATOR_MCP_CALL_REQUESTED: "MCP_CALL",
        EventType.OPERATOR_A2A_CALL_REQUESTED: "A2A_CALL",
        EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED: "PORT_CHECK",
        # App-level document mutations route through DOCUMENT_UPDATE/DOCUMENT_DELETE,
        # matching the Go eventToAction map in internal/constants/mappings.go.
        # All case/investigation/memory create/update events map to DOCUMENT_UPDATE;
        # all delete events map to DOCUMENT_DELETE. This unifies governed document
        # mutations through one action type pair with typed protobuf payloads.
        EventType.APP_CASE_CREATED: "DOCUMENT_UPDATE",
        EventType.APP_CASE_UPDATED: "DOCUMENT_UPDATE",
        EventType.APP_CASE_DELETED: "DOCUMENT_DELETE",
        EventType.APP_MEMORY_CREATED: "DOCUMENT_UPDATE",
        EventType.APP_MEMORY_UPDATED: "DOCUMENT_UPDATE",
        EventType.APP_INVESTIGATION_CREATED: "DOCUMENT_UPDATE",
        EventType.APP_INVESTIGATION_UPDATED: "DOCUMENT_UPDATE",
        EventType.APP_INVESTIGATION_DELETED: "DOCUMENT_DELETE",
        EventType.OPERATOR_REPUTATION_COMMITMENT_CREATED: "DOCUMENT_UPDATE",
    }
    return mapping.get(event_type, event_type)


def map_action_type_to_event_type(action_type: str) -> str:
    """Map UAP action types back to protobuf event types."""
    mapping = {
        "EVAL_ANSWER": EventType.OPERATOR_EVAL_ANSWER_REQUESTED,
        "HEARTBEAT": EventType.OPERATOR_HEARTBEAT_REQUESTED,
        "SHUTDOWN": EventType.OPERATOR_SHUTDOWN_REQUESTED,
        "EXECUTE_BASH": EventType.OPERATOR_COMMAND_REQUESTED,
        "FILE_EDIT": EventType.OPERATOR_FILE_EDIT_REQUESTED,
        "FETCH_FILE_HISTORY": EventType.OPERATOR_FILE_HISTORY_FETCH_REQUESTED,
        "RESTORE_FILE": EventType.OPERATOR_FILE_RESTORE_REQUESTED,
        "FS_LIST": EventType.OPERATOR_FILESYSTEM_LIST_REQUESTED,
        "FS_READ": EventType.OPERATOR_FILESYSTEM_READ_REQUESTED,
        "FS_GREP": EventType.OPERATOR_FILESYSTEM_GREP_REQUESTED,
        "FETCH_LOGS": EventType.OPERATOR_LOGS_FETCH_REQUESTED,
        "FETCH_HISTORY": EventType.OPERATOR_HISTORY_FETCH_REQUESTED,
        "GRANT_INTENT": EventType.OPERATOR_INTENT_REQUESTED,
        "REVOKE_INTENT": EventType.OPERATOR_INTENT_REVOKE_REQUESTED,
        "MCP_CALL": EventType.OPERATOR_MCP_CALL_REQUESTED,
        "A2A_CALL": EventType.OPERATOR_A2A_CALL_REQUESTED,
        "PORT_CHECK": EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED,
    }
    return mapping.get(action_type, action_type)


def map_event_type_to_result_action_type(event_type: str) -> str:
    """Map protobuf event types to UAP result action types."""
    mapping = {
        EventType.OPERATOR_HEARTBEAT_SENT: "HEARTBEAT_RESULT",
        EventType.OPERATOR_COMMAND_COMPLETED: "EXECUTE_BASH_RESULT",
        EventType.OPERATOR_COMMAND_FAILED: "EXECUTE_BASH_RESULT",
        EventType.OPERATOR_COMMAND_CANCELLED: "EXECUTE_BASH_CANCELLED",
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_QUEUED: "EXECUTE_STATUS_UPDATE",
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_RUNNING: "EXECUTE_STATUS_UPDATE",
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_COMPLETED: "EXECUTE_STATUS_UPDATE",
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_FAILED: "EXECUTE_STATUS_UPDATE",
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_CANCELLED: "EXECUTE_STATUS_UPDATE",
        EventType.OPERATOR_FILE_EDIT_COMPLETED: "FILE_EDIT_RESULT",
        EventType.OPERATOR_FILE_EDIT_FAILED: "FILE_EDIT_RESULT",
        EventType.OPERATOR_FILESYSTEM_LIST_COMPLETED: "FS_LIST_RESULT",
        EventType.OPERATOR_FILESYSTEM_LIST_FAILED: "FS_LIST_RESULT",
        EventType.OPERATOR_FILESYSTEM_GREP_COMPLETED: "FS_GREP_RESULT",
        EventType.OPERATOR_FILESYSTEM_GREP_FAILED: "FS_GREP_RESULT",
    }
    return mapping.get(event_type, event_type + "_RESULT")
