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
Unit tests for app/models/cases.py

Covers: HistoryEntry, CaseModel, CaseCreateRequest, CaseUpdateRequest
"""

import uuid
from datetime import UTC, datetime

import pytest
from app.models.base import ValidationError

from app.constants import (
    CaseStatus,
    ComponentName,
    EventType,
    Priority,
    Severity,
)
from app.models.cases import CaseCreateRequest, CaseModel, CaseUpdateRequest, HistoryEntry

pytestmark = [pytest.mark.unit]

_TS = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)


@pytest.mark.unit
class TestHistoryEntry:

    def test_instantiation_with_required_fields(self):
        entry = HistoryEntry(
            timestamp=_TS,
            event_type=EventType.CASE_CREATED,
            source_component=ComponentName.VSE,
            summary="Case was created",
        )
        assert entry.timestamp == _TS
        assert entry.event_type == EventType.CASE_CREATED
        assert entry.source_component == ComponentName.VSE
        assert entry.summary == "Case was created"

    def test_timestamp_is_datetime_not_str(self):
        entry = HistoryEntry(
            timestamp=_TS,
            event_type=EventType.CASE_CREATED,
            source_component=ComponentName.VSE,
            summary="test",
        )
        assert isinstance(entry.timestamp, datetime)

    def test_event_type_is_enum(self):
        entry = HistoryEntry(
            timestamp=_TS,
            event_type=EventType.CASE_CREATED,
            source_component=ComponentName.VSE,
            summary="test",
        )
        assert entry.event_type == EventType.CASE_CREATED

    def test_source_component_is_enum(self):
        entry = HistoryEntry(
            timestamp=_TS,
            event_type=EventType.CASE_CREATED,
            source_component=ComponentName.VSOD,
            summary="test",
        )
        assert entry.source_component == ComponentName.VSOD

    def test_optional_fields_default_to_none(self):
        entry = HistoryEntry(
            timestamp=_TS,
            event_type=EventType.CASE_CREATED,
            source_component=ComponentName.VSE,
            summary="test",
        )
        assert entry.actor_id is None
        assert entry.related_ids is None
        assert entry.details is None

    def test_optional_fields_can_be_set(self):
        entry = HistoryEntry(
            timestamp=_TS,
            event_type=EventType.CASE_CREATED,
            source_component=ComponentName.VSE,
            summary="test",
            actor_id="user-123",
            related_ids={"investigation_id": "inv-456"},
            details={"key": "value"},
        )
        assert entry.actor_id == "user-123"
        assert entry.related_ids == {"investigation_id": "inv-456"}
        assert entry.details == {"key": "value"}

    def test_model_dump_serializes_enum_values(self):
        entry = HistoryEntry(
            timestamp=_TS,
            event_type=EventType.CASE_CREATED,
            source_component=ComponentName.VSE,
            summary="test",
        )
        dumped = entry.model_dump()
        assert dumped["event_type"] == "g8e.v1.app.case.created"
        assert dumped["source_component"] == "vse"

    def test_model_dump_excludes_none_by_default(self):
        entry = HistoryEntry(
            timestamp=_TS,
            event_type=EventType.CASE_CREATED,
            source_component=ComponentName.VSE,
            summary="test",
        )
        dumped = entry.model_dump()
        assert "actor_id" not in dumped
        assert "related_ids" not in dumped
        assert "details" not in dumped

    def test_all_component_names_accepted(self):
        for component in ComponentName:
            entry = HistoryEntry(
                timestamp=_TS,
                event_type=EventType.CASE_CREATED,
                source_component=component,
                summary="test",
            )
            assert entry.source_component == component



@pytest.mark.unit
class TestCaseModel:

    def _make_case(self, **overrides):
        defaults = dict(
            title="Database connectivity issue",
            description="Cannot connect to the primary database",
        )
        defaults.update(overrides)
        return CaseModel(**defaults)

    def test_instantiation_with_required_fields(self):
        case = self._make_case()
        assert case.title == "Database connectivity issue"
        assert case.description == "Cannot connect to the primary database"

    def test_id_is_generated_as_uuid(self):
        case = self._make_case()
        assert isinstance(case.id, str)
        parsed = uuid.UUID(case.id)
        assert str(parsed) == case.id

    def test_each_instance_has_unique_id(self):
        ids = {self._make_case().id for _ in range(20)}
        assert len(ids) == 20

    def test_status_defaults_to_new(self):
        case = self._make_case()
        assert case.status == CaseStatus.NEW

    def test_priority_defaults_to_medium(self):
        case = self._make_case()
        assert case.priority == Priority.MEDIUM

    def test_severity_defaults_to_medium(self):
        case = self._make_case()
        assert case.severity == Severity.MEDIUM

    def test_optional_fields_default_to_none(self):
        case = self._make_case()
        assert case.user_id is None
        assert case.user_email is None
        assert case.web_session_id is None
        assert case.assignee is None
        assert case.investigation_id is None
        assert case.source is None
        assert case.last_processed_by is None

    def test_list_fields_default_to_empty(self):
        case = self._make_case()
        assert case.related_case_ids == []
        assert case.tags == []
        assert case.attachments == []
        assert case.history_trail == []

    def test_metadata_defaults_to_empty_dict(self):
        case = self._make_case()
        assert case.metadata == {}

    def test_history_trail_accepts_history_entries(self):
        entry = HistoryEntry(
            timestamp=_TS,
            event_type=EventType.CASE_CREATED,
            source_component=ComponentName.VSE,
            summary="Case opened",
        )
        case = self._make_case(history_trail=[entry])
        assert len(case.history_trail) == 1
        assert case.history_trail[0].event_type == EventType.CASE_CREATED

    def test_timestamps_are_set_on_instantiation(self):
        case = self._make_case()
        assert isinstance(case.created_at, datetime)
        assert case.created_at.tzinfo is not None

    def test_model_dump_serializes_status_as_string(self):
        case = self._make_case(status=CaseStatus.ESCALATED)
        dumped = case.model_dump()
        assert dumped["status"] == "Escalated"

    def test_model_dump_serializes_priority_as_int(self):
        case = self._make_case(priority=Priority.HIGH)
        dumped = case.model_dump()
        assert dumped["priority"] == Priority.HIGH.value

    def test_model_dump_excludes_none_by_default(self):
        case = self._make_case()
        dumped = case.model_dump()
        assert "user_id" not in dumped
        assert "user_email" not in dumped
        assert "investigation_id" not in dumped

    def test_extra_fields_ignored(self):
        case = self._make_case(injected_client_field="malicious")
        assert not hasattr(case, "injected_client_field")

    def test_inherits_extra_ignore_from_base(self):
        case = CaseModel(
            title="Test",
            description="Desc",
            unknown_field="should_be_dropped",
        )
        assert not hasattr(case, "unknown_field")

    def test_all_case_statuses_accepted(self):
        for status in CaseStatus:
            case = self._make_case(status=status)
            assert case.status == status

    def test_all_priorities_accepted(self):
        for priority in Priority:
            case = self._make_case(priority=priority)
            assert case.priority == priority

    def test_all_severities_accepted(self):
        for severity in Severity:
            case = self._make_case(severity=severity)
            assert case.severity == severity


@pytest.mark.unit
class TestCaseCreateRequest:

    def _make_request(self, **overrides):
        defaults = dict(
            initial_message="Nginx is returning 502 on all endpoints",
            user_id="user-abc",
            web_session_id="sess-xyz",
        )
        defaults.update(overrides)
        return CaseCreateRequest(**defaults)

    def test_instantiation_with_required_fields(self):
        req = self._make_request()
        assert req.initial_message == "Nginx is returning 502 on all endpoints"
        assert req.user_id == "user-abc"
        assert req.web_session_id == "sess-xyz"

    def test_initial_message_required(self):
        with pytest.raises(ValidationError):
            CaseCreateRequest(user_id="u", web_session_id="s")

    def test_initial_message_min_length_enforced(self):
        with pytest.raises(ValidationError):
            self._make_request(initial_message="")

    def test_user_id_required(self):
        with pytest.raises(ValidationError):
            CaseCreateRequest(
                initial_message="test",
                web_session_id="sess-xyz",
            )

    def test_user_id_min_length_enforced(self):
        with pytest.raises(ValidationError):
            self._make_request(user_id="")

    def test_web_session_id_required(self):
        with pytest.raises(ValidationError):
            CaseCreateRequest(
                initial_message="test",
                user_id="user-abc",
            )

    def test_web_session_id_min_length_enforced(self):
        with pytest.raises(ValidationError):
            self._make_request(web_session_id="")

    def test_sentinel_mode_defaults_to_true(self):
        req = self._make_request()
        assert req.sentinel_mode is True

    def test_sentinel_mode_can_be_disabled(self):
        req = self._make_request(sentinel_mode=False)
        assert req.sentinel_mode is False

    def test_attachments_default_to_empty(self):
        req = self._make_request()
        assert req.attachments == []

    def test_priority_defaults_to_medium(self):
        req = self._make_request()
        assert req.priority == Priority.MEDIUM

    def test_severity_defaults_to_medium(self):
        req = self._make_request()
        assert req.severity == Severity.MEDIUM

    def test_source_defaults_to_g8e_ai(self):
        req = self._make_request()
        assert req.source == "g8e.ai"

    def test_optional_fields_default_to_none(self):
        req = self._make_request()
        assert req.user_email is None
        assert req.organization_id is None

    def test_user_email_can_be_set(self):
        req = self._make_request(user_email="user@example.com")
        assert req.user_email == "user@example.com"

    def test_organization_id_can_be_set(self):
        req = self._make_request(organization_id="org-999")
        assert req.organization_id == "org-999"


    def test_priority_accepts_all_enum_values(self):
        for priority in Priority:
            req = self._make_request(priority=priority)
            assert req.priority == priority

    def test_severity_accepts_all_enum_values(self):
        for severity in Severity:
            req = self._make_request(severity=severity)
            assert req.severity == severity


@pytest.mark.unit
class TestCaseUpdateRequest:

    def test_empty_request_is_valid(self):
        req = CaseUpdateRequest()
        assert req.title is None
        assert req.description is None
        assert req.status is None
        assert req.priority is None
        assert req.severity is None
        assert req.assignee is None
        assert req.tags is None
        assert req.metadata is None

    def test_title_update(self):
        req = CaseUpdateRequest(title="New Title")
        assert req.title == "New Title"

    def test_title_min_length_enforced(self):
        with pytest.raises(ValidationError):
            CaseUpdateRequest(title="")

    def test_title_max_length_enforced(self):
        with pytest.raises(ValidationError):
            CaseUpdateRequest(title="x" * 501)

    def test_description_min_length_enforced(self):
        with pytest.raises(ValidationError):
            CaseUpdateRequest(description="")

    def test_status_update_with_enum(self):
        req = CaseUpdateRequest(status=CaseStatus.RESOLVED)
        assert req.status == CaseStatus.RESOLVED

    def test_priority_update_with_enum(self):
        req = CaseUpdateRequest(priority=Priority.CRITICAL)
        assert req.priority == Priority.CRITICAL

    def test_severity_update_with_enum(self):
        req = CaseUpdateRequest(severity=Severity.HIGH)
        assert req.severity == Severity.HIGH

    def test_assignee_update(self):
        req = CaseUpdateRequest(assignee="user-456")
        assert req.assignee == "user-456"

    def test_tags_update(self):
        req = CaseUpdateRequest(tags=["nginx", "production", "502"])
        assert req.tags == ["nginx", "production", "502"]

    def test_metadata_update(self):
        req = CaseUpdateRequest(metadata={"ticket_ref": "INC-9901"})
        assert req.metadata == {"ticket_ref": "INC-9901"}

    def test_partial_update_leaves_unset_fields_as_none(self):
        req = CaseUpdateRequest(title="Updated Title")
        assert req.status is None
        assert req.priority is None
        assert req.tags is None

    def test_all_case_statuses_accepted(self):
        for status in CaseStatus:
            req = CaseUpdateRequest(status=status)
            assert req.status == status

    def test_model_dump_excludes_none_by_default(self):
        req = CaseUpdateRequest(title="Only Title")
        dumped = req.model_dump()
        assert "title" in dumped
        assert "status" not in dumped
        assert "priority" not in dumped
