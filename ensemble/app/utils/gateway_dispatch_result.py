# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Decode Gateway-owned operator dispatch HTTP responses for g8ee orchestration."""

from __future__ import annotations

import base64
from typing import Any

from google.protobuf.json_format import MessageToDict
from google.protobuf.message import Message

from app.constants import EventType
from app.constants import ExecutionStatus
from app.constants.proto_mappings import protobuf_execution_status_to_python
from app.models.http_context import G8eHttpContext
from app.models.pubsub_messages import G8eoResultEnvelope
from app.utils.result_decoder import parse_inbound_g8eo_payload
from g8e.operator.v1 import operator_pb2

_EVENT_TYPE_TO_RESULT_PROTO: dict[str, type[Message]] = {
    EventType.OPERATOR_COMMAND_COMPLETED: operator_pb2.CommandResult,
    EventType.OPERATOR_COMMAND_FAILED: operator_pb2.CommandResult,
    EventType.OPERATOR_COMMAND_CANCELLED: operator_pb2.CommandResult,
    EventType.OPERATOR_FILE_EDIT_COMPLETED: operator_pb2.FileEditResult,
    EventType.OPERATOR_FILE_EDIT_FAILED: operator_pb2.FileEditResult,
    EventType.OPERATOR_FILESYSTEM_LIST_COMPLETED: operator_pb2.FsListResult,
    EventType.OPERATOR_FILESYSTEM_LIST_FAILED: operator_pb2.FsListResult,
    EventType.OPERATOR_FILESYSTEM_READ_COMPLETED: operator_pb2.FsReadResult,
    EventType.OPERATOR_FILESYSTEM_READ_FAILED: operator_pb2.FsReadResult,
    EventType.OPERATOR_FILESYSTEM_GREP_COMPLETED: operator_pb2.FsGrepResult,
    EventType.OPERATOR_FILESYSTEM_GREP_FAILED: operator_pb2.FsGrepResult,
    EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED: operator_pb2.PortCheckResult,
    EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED: operator_pb2.PortCheckResult,
}

_PROTO_TO_PAYLOAD_TYPE: dict[str, str] = {
    "CommandResult": "execution_result",
    "FileEditResult": "file_edit_result",
    "FsListResult": "fs_list_result",
    "FsReadResult": "fs_read_result",
    "FsGrepResult": "fs_grep_result",
    "PortCheckResult": "port_check_result",
}


def decode_gateway_result_payload_bytes(payload_raw: Any) -> bytes | None:
    if payload_raw is None:
        return None
    if isinstance(payload_raw, str):
        return base64.b64decode(payload_raw)
    if isinstance(payload_raw, (bytes, bytearray)):
        return bytes(payload_raw)
    return None


def gateway_result_payload_dict(
    *,
    event_type: str,
    payload_raw: Any,
    execution_id: str,
) -> dict[str, Any] | None:
    payload_bytes = decode_gateway_result_payload_bytes(payload_raw)
    if not payload_bytes:
        return None

    proto_cls = _EVENT_TYPE_TO_RESULT_PROTO.get(event_type, operator_pb2.CommandResult)
    message = proto_cls()
    message.ParseFromString(payload_bytes)

    payload_dict = MessageToDict(message, preserving_proto_field_name=True)
    payload_type = _PROTO_TO_PAYLOAD_TYPE.get(message.DESCRIPTOR.name)
    if payload_type:
        payload_dict["payload_type"] = payload_type
    payload_dict.setdefault("execution_id", execution_id)

    if "status" in payload_dict:
        status_val = payload_dict["status"]
        if isinstance(status_val, (int, float)):
            payload_dict["status"] = protobuf_execution_status_to_python(
                int(status_val)
            ).value
        elif isinstance(status_val, str):
            if status_val.startswith("EXECUTION_STATUS_"):
                enum_val = getattr(operator_pb2, status_val, None)
                if enum_val is not None:
                    payload_dict["status"] = protobuf_execution_status_to_python(
                        int(enum_val)
                    ).value
            elif status_val in ExecutionStatus._value2member_map_:
                payload_dict["status"] = status_val

    return payload_dict


def envelope_from_gateway_dispatch(
    dispatch_result: dict[str, Any],
    *,
    execution_id: str,
    operator_id: str,
    operator_session_id: str,
    g8e_context: G8eHttpContext,
) -> G8eoResultEnvelope | None:
    event_type = str(dispatch_result.get("event_type") or "")
    payload_dict = gateway_result_payload_dict(
        event_type=event_type,
        payload_raw=dispatch_result.get("result_payload"),
        execution_id=execution_id,
    )
    if payload_dict is None:
        return None

    typed_payload = parse_inbound_g8eo_payload(payload_dict)
    return G8eoResultEnvelope.model_validate(
        {
            "id": dispatch_result.get("transaction_id") or execution_id,
            "event_type": event_type,
            "operator_id": operator_id,
            "operator_session_id": operator_session_id,
            "case_id": g8e_context.case_id,
            "investigation_id": g8e_context.investigation_id,
            "task_id": g8e_context.task_id,
            "web_session_id": g8e_context.web_session_id,
            "cli_session_id": g8e_context.cli_session_id,
            "payload": typed_payload,
        }
    )
