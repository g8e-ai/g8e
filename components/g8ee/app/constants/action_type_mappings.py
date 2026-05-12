# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Canonical ActionType/EventType mappings for g8e protocol.

This module provides the centralized mapping functions between protobuf event types
and UAP action types. These mappings must stay in sync with the Go implementation in
components/g8eo/internal/mappings/action_types.go.
"""

from app.constants.events import EventType


def map_event_type_to_action_type(event_type: str) -> str:
    """Map protobuf event types to UAP action types.

    This is the canonical mapping used across g8ee. Must match the Go implementation
    in components/g8eo/internal/mappings/action_types.go.
    """
    mapping = {
        EventType.OPERATOR_COMMAND_REQUESTED: "EXECUTE_BASH",
        EventType.OPERATOR_FILE_EDIT_REQUESTED: "FILE_EDIT",
        EventType.OPERATOR_FS_LIST_REQUESTED: "FS_LIST",
        EventType.OPERATOR_FS_READ_REQUESTED: "FS_READ",
        EventType.OPERATOR_FS_GREP_REQUESTED: "FS_GREP",
        EventType.OPERATOR_PORT_CHECK_REQUESTED: "PORT_CHECK",
        EventType.OPERATOR_FETCH_LOGS_REQUESTED: "FETCH_LOGS",
        EventType.OPERATOR_FETCH_HISTORY_REQUESTED: "FETCH_HISTORY",
        EventType.OPERATOR_FETCH_FILE_HISTORY_REQUESTED: "FETCH_FILE_HISTORY",
        EventType.OPERATOR_RESTORE_FILE_REQUESTED: "RESTORE_FILE",
        EventType.OPERATOR_SHUTDOWN_REQUESTED: "SHUTDOWN",
        EventType.OPERATOR_HEARTBEAT_REQUESTED: "HEARTBEAT",
    }
    return mapping.get(event_type, event_type)


def map_action_type_to_event_type(action_type: str) -> str:
    """Map UAP action types back to protobuf event types for handler dispatch.

    This is the canonical mapping used across g8ee. Must match the Go implementation
    in components/g8eo/internal/mappings/action_types.go.
    """
    mapping = {
        "EXECUTE_BASH": EventType.OPERATOR_COMMAND_REQUESTED,
        "FILE_EDIT": EventType.OPERATOR_FILE_EDIT_REQUESTED,
        "FS_LIST": EventType.OPERATOR_FS_LIST_REQUESTED,
        "FS_READ": EventType.OPERATOR_FS_READ_REQUESTED,
        "FS_GREP": EventType.OPERATOR_FS_GREP_REQUESTED,
        "PORT_CHECK": EventType.OPERATOR_PORT_CHECK_REQUESTED,
        "FETCH_LOGS": EventType.OPERATOR_FETCH_LOGS_REQUESTED,
        "FETCH_HISTORY": EventType.OPERATOR_FETCH_HISTORY_REQUESTED,
        "FETCH_FILE_HISTORY": EventType.OPERATOR_FETCH_FILE_HISTORY_REQUESTED,
        "RESTORE_FILE": EventType.OPERATOR_RESTORE_FILE_REQUESTED,
        "SHUTDOWN": EventType.OPERATOR_SHUTDOWN_REQUESTED,
        "HEARTBEAT": EventType.OPERATOR_HEARTBEAT_REQUESTED,
    }
    return mapping.get(action_type, action_type)
