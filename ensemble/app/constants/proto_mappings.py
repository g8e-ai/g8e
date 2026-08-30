# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Hand-authored mapping functions for protobuf/Python enum conversion

from app.constants import ExecutionStatus
from g8e.operator.v1 import operator_pb2

_PROTOBUF_EXECUTIONSTATUS_TO_PYTHON: dict[int, ExecutionStatus] = {
    operator_pb2.EXECUTION_STATUS_UNSPECIFIED: ExecutionStatus.PENDING,
    operator_pb2.EXECUTION_STATUS_EXECUTING: ExecutionStatus.EXECUTING,
    operator_pb2.EXECUTION_STATUS_COMPLETED: ExecutionStatus.COMPLETED,
    operator_pb2.EXECUTION_STATUS_FAILED: ExecutionStatus.FAILED,
    operator_pb2.EXECUTION_STATUS_TIMEOUT: ExecutionStatus.TIMEOUT,
    operator_pb2.EXECUTION_STATUS_CANCELLED: ExecutionStatus.CANCELLED,
}


def protobuf_execution_status_to_python(proto_val: int) -> ExecutionStatus:
    """Convert protobuf ExecutionStatus to Python ExecutionStatus."""
    if proto_val not in _PROTOBUF_EXECUTIONSTATUS_TO_PYTHON:
        raise ValueError(f"Unknown protobuf ExecutionStatus value: {proto_val}")
    return _PROTOBUF_EXECUTIONSTATUS_TO_PYTHON[proto_val]
