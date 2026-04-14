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

import pytest

from app.constants import ExecutionStatus, CommandErrorType
from app.models.operators import (
    CommandCancelledBroadcastEvent,
    CommandResultBroadcastEvent,
    CommandStatusBroadcastEvent,
)

pytestmark = pytest.mark.unit


class TestCommandStatusBroadcastEventModel:
    """Sync model construction tests for CommandStatusBroadcastEvent.

    G8eBaseModel uses `use_enum_values=True` — Pydantic stores the enum's
    string value internally.  Equality against str,Enum members works because
    str,Enum members ARE their string values.
    """

    def test_default_status_is_running(self):
        """Default status equals ExecutionStatus.EXECUTING."""
        event = CommandStatusBroadcastEvent(execution_id="exec-s-1")
        assert event.status == ExecutionStatus.EXECUTING

    def test_status_field_accepts_running_enum(self):
        """Explicit ExecutionStatus.EXECUTING roundtrips correctly."""
        event = CommandStatusBroadcastEvent(
            execution_id="exec-s-2",
            status=ExecutionStatus.EXECUTING,
        )
        assert event.status == ExecutionStatus.EXECUTING

    def test_status_string_value_matches_enum(self):
        """Stored value equals the string behind the enum member."""
        event = CommandStatusBroadcastEvent(execution_id="exec-s-3")
        assert event.status == "executing"


class TestCommandResultBroadcastEventModel:
    """Sync model construction tests for CommandResultBroadcastEvent."""

    def test_status_accepts_completed(self):
        """ExecutionStatus.COMPLETED roundtrips correctly."""
        event = CommandResultBroadcastEvent(
            execution_id="exec-r-1",
            status=ExecutionStatus.COMPLETED,
        )
        assert event.status == ExecutionStatus.COMPLETED

    def test_status_accepts_failed(self):
        """ExecutionStatus.FAILED roundtrips correctly."""
        event = CommandResultBroadcastEvent(
            execution_id="exec-r-2",
            status=ExecutionStatus.FAILED,
        )
        assert event.status == ExecutionStatus.FAILED

    def test_completed_status_string_value(self):
        """Stored value equals the string behind ExecutionStatus.COMPLETED."""
        event = CommandResultBroadcastEvent(
            execution_id="exec-r-3",
            status=ExecutionStatus.COMPLETED,
        )
        assert event.status == "completed"

    def test_failed_status_string_value(self):
        """Stored value equals the string behind ExecutionStatus.FAILED."""
        event = CommandResultBroadcastEvent(
            execution_id="exec-r-4",
            status=ExecutionStatus.FAILED,
        )
        assert event.status == "failed"


class TestCommandCancelledBroadcastEventModel:
    """Sync model construction tests for CommandCancelledBroadcastEvent."""

    def test_default_status_is_cancelled(self):
        """Default status equals ExecutionStatus.CANCELLED."""
        event = CommandCancelledBroadcastEvent(execution_id="exec-c-1")
        assert event.status == ExecutionStatus.CANCELLED

    def test_status_string_value_matches_enum(self):
        """Stored value equals the string behind ExecutionStatus.CANCELLED."""
        event = CommandCancelledBroadcastEvent(execution_id="exec-c-2")
        assert event.status == "cancelled"

    def test_error_type_accepts_user_cancelled(self):
        """error_type=CommandErrorType.USER_CANCELLED roundtrips correctly."""
        event = CommandCancelledBroadcastEvent(
            execution_id="exec-c-3",
            error_type=CommandErrorType.USER_CANCELLED,
        )
        assert event.error_type == CommandErrorType.USER_CANCELLED

    def test_error_type_string_value_matches_enum(self):
        """Stored error_type value equals the string behind CommandErrorType.USER_CANCELLED."""
        event = CommandCancelledBroadcastEvent(
            execution_id="exec-c-4",
            error_type=CommandErrorType.USER_CANCELLED,
        )
        assert event.error_type == "user.cancelled"

    def test_error_type_defaults_to_none(self):
        """error_type is None when not provided."""
        event = CommandCancelledBroadcastEvent(execution_id="exec-c-5")
        assert event.error_type is None
