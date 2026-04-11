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

import re
import time
from datetime import UTC, datetime
from enum import Enum

import pytest
from app.models.base import (
    Field,
    VSOAuditableModel,
    VSOBaseModel,
    VSOIdentifiableModel,
    VSOTimestampedModel,
    _to_iso_z,
)

pytestmark = [pytest.mark.unit]

_ISO_Z_RE = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$")


class _Status(str, Enum):
    ACTIVE = "active"
    INACTIVE = "inactive"


class _SampleModel(VSOBaseModel):
    name: str
    value: int | None = None
    status: _Status | None = None


class _AuditableChild(VSOAuditableModel):
    label: str = Field(default="")


class TestVSOBaseModel:

    def test_instantiation_with_required_fields(self):
        m = _SampleModel(name="test")
        assert m.name == "test"

    def test_extra_fields_are_ignored(self):
        m = _SampleModel(name="test", unknown_field="ignored")  # type: ignore[arg-type]
        assert not hasattr(m, "unknown_field")

    def test_populate_by_name(self):
        m = _SampleModel(name="test", value=42)
        assert m.value == 42

    def test_use_enum_values_serializes_enum_as_string(self):
        m = _SampleModel(name="test", status=_Status.ACTIVE)
        dumped = m.model_dump()
        assert dumped["status"] == "active"

    def test_model_dump_excludes_none_by_default(self):
        m = _SampleModel(name="test", value=None, status=None)
        dumped = m.model_dump()
        assert "value" not in dumped
        assert "status" not in dumped
        assert "name" in dumped

    def test_model_dump_includes_none_when_overridden(self):
        m = _SampleModel(name="test", value=None)
        dumped = m.model_dump(exclude_none=False)
        assert "value" in dumped
        assert dumped["value"] is None

    def test_model_dump_json_excludes_none_by_default(self):
        m = _SampleModel(name="test", value=None)
        json_str = m.model_dump_json()
        assert '"value"' not in json_str
        assert '"name"' in json_str

    def test_model_dump_json_includes_none_when_overridden(self):
        m = _SampleModel(name="test", value=None)
        json_str = m.model_dump_json(exclude_none=False)
        assert '"value":null' in json_str

    def test_model_dump_passes_through_extra_kwargs(self):
        m = _SampleModel(name="test", value=5)
        dumped = m.model_dump(include={"name"})
        assert "name" in dumped
        assert "value" not in dumped

    def test_model_dump_json_passes_through_extra_kwargs(self):
        m = _SampleModel(name="test", value=5)
        json_str = m.model_dump_json(include={"name"})
        assert '"name"' in json_str
        assert '"value"' not in json_str

    def test_flatten_for_llm_returns_dict(self):
        m = _SampleModel(name="test", value=42)
        result = m.flatten_for_llm()
        assert isinstance(result, dict)
        assert result["name"] == "test"
        assert result["value"] == 42

    def test_flatten_for_db_returns_dict(self):
        m = _SampleModel(name="test", value=42)
        result = m.flatten_for_db()
        assert isinstance(result, dict)
        assert result["name"] == "test"

    def test_flatten_for_wire_returns_dict(self):
        m = _SampleModel(name="test", value=42)
        result = m.flatten_for_wire()
        assert isinstance(result, dict)
        assert result["name"] == "test"

    def test_flatten_excludes_none_fields(self):
        m = _SampleModel(name="test", value=None)
        assert "value" not in m.flatten_for_llm()
        assert "value" not in m.flatten_for_db()
        assert "value" not in m.flatten_for_wire()

    def test_flatten_serializes_nested_model(self):
        class _Outer(VSOBaseModel):
            inner: _SampleModel

        m = _Outer(inner=_SampleModel(name="nested", value=7))
        result = m.flatten_for_wire()
        assert isinstance(result["inner"], dict)
        assert result["inner"]["name"] == "nested"


class TestToIsoZ:

    def test_utc_datetime_produces_z_suffix(self):
        dt = datetime(2026, 1, 15, 10, 30, 0, tzinfo=UTC)
        result = _to_iso_z(dt)
        assert result.endswith("Z")
        assert "+" not in result

    def test_naive_datetime_treated_as_utc(self):
        dt = datetime(2026, 1, 15, 10, 30, 0)
        result = _to_iso_z(dt)
        assert result.endswith("Z")

    def test_microseconds_included_when_nonzero(self):
        dt = datetime(2026, 1, 15, 10, 30, 0, 123456, tzinfo=UTC)
        result = _to_iso_z(dt)
        assert ".123456Z" in result

    def test_microseconds_omitted_when_zero(self):
        dt = datetime(2026, 1, 15, 10, 30, 0, 0, tzinfo=UTC)
        result = _to_iso_z(dt)
        assert "." not in result
        assert result.endswith("Z")

    def test_matches_iso_z_pattern(self):
        dt = datetime(2026, 3, 4, 23, 59, 59, tzinfo=UTC)
        assert _ISO_Z_RE.match(_to_iso_z(dt))


class TestVSOTimestampedModel:

    class _TimestampedChild(VSOTimestampedModel):
        label: str = ""

    def test_created_at_set_on_instantiation(self):
        m = self._TimestampedChild()
        assert isinstance(m.created_at, datetime)
        assert m.created_at.tzinfo is not None

    def test_created_at_is_utc(self):
        m = self._TimestampedChild()
        assert m.created_at.tzinfo == UTC

    def test_updated_at_is_none_by_default(self):
        m = self._TimestampedChild()
        assert m.updated_at is None

    def test_update_timestamp_sets_updated_at(self):
        m = self._TimestampedChild()
        assert m.updated_at is None
        m.update_timestamp()
        assert isinstance(m.updated_at, datetime)
        assert m.updated_at.tzinfo == UTC

    def test_update_timestamp_advances_time(self):
        m = self._TimestampedChild()
        m.update_timestamp()
        first = m.updated_at
        assert first is not None
        time.sleep(0.01)
        m.update_timestamp()
        assert m.updated_at is not None
        assert m.updated_at > first

    def test_serialize_datetime_uses_z_suffix(self):
        m = self._TimestampedChild()
        dumped = m.model_dump()
        assert isinstance(dumped["created_at"], str)
        assert dumped["created_at"].endswith("Z"), f"Expected Z suffix, got: {dumped['created_at']}"
        assert "+" not in dumped["created_at"]

    def test_serialize_datetime_updated_at_uses_z_suffix(self):
        m = self._TimestampedChild()
        m.update_timestamp()
        dumped = m.model_dump(exclude_none=False)
        assert isinstance(dumped["updated_at"], str)
        assert dumped["updated_at"].endswith("Z"), f"Expected Z suffix, got: {dumped['updated_at']}"

    def test_updated_at_excluded_from_dump_when_none(self):
        m = self._TimestampedChild()
        dumped = m.model_dump()
        assert "updated_at" not in dumped

    def test_serialize_datetime_accepts_string_passthrough(self):
        iso = "2026-01-15T10:30:00+00:00"
        m = self._TimestampedChild(created_at=iso)  # type: ignore[arg-type]
        dumped = m.model_dump()
        assert isinstance(dumped["created_at"], str)

    def test_created_at_can_be_provided_explicitly(self):
        ts = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)
        m = self._TimestampedChild(created_at=ts)
        assert m.created_at == ts

    def test_created_at_matches_iso_z_pattern(self):
        m = self._TimestampedChild()
        dumped = m.model_dump()
        assert _ISO_Z_RE.match(dumped["created_at"]), f"Pattern mismatch: {dumped['created_at']}"


class TestVSOIdentifiableModel:

    class _IdentifiableChild(VSOIdentifiableModel):
        label: str = ""

    def test_id_field_auto_generated_on_instantiation(self):
        m = self._IdentifiableChild()
        assert isinstance(m.id, str)
        assert len(m.id) == 36
        parts = m.id.split("-")
        assert len(parts) == 5

    def test_id_field_is_unique_per_instance(self):
        ids = {self._IdentifiableChild().id for _ in range(50)}
        assert len(ids) == 50

    def test_id_field_can_be_provided_explicitly(self):
        m = self._IdentifiableChild(id="custom-id-123")
        assert m.id == "custom-id-123"

    def test_id_included_in_model_dump(self):
        m = self._IdentifiableChild()
        dumped = m.model_dump()
        assert "id" in dumped
        assert isinstance(dumped["id"], str)

    def test_generate_id_returns_uuid_string(self):
        id_val = VSOIdentifiableModel.generate_id()
        assert isinstance(id_val, str)
        assert len(id_val) == 36
        parts = id_val.split("-")
        assert len(parts) == 5

    def test_generate_id_with_prefix(self):
        id_val = VSOIdentifiableModel.generate_id(prefix="inv")
        assert id_val.startswith("inv-")
        remainder = id_val[4:]
        assert len(remainder) == 36

    def test_generate_id_without_prefix_no_dash_prefix(self):
        id_val = VSOIdentifiableModel.generate_id()
        assert not id_val.startswith("-")

    def test_generate_id_uniqueness(self):
        ids = {VSOIdentifiableModel.generate_id() for _ in range(100)}
        assert len(ids) == 100

    def test_generate_id_with_prefix_uniqueness(self):
        ids = {VSOIdentifiableModel.generate_id(prefix="op") for _ in range(50)}
        assert len(ids) == 50

    def test_inherits_timestamps(self):
        m = self._IdentifiableChild()
        assert isinstance(m.created_at, datetime)
        assert m.updated_at is None

    def test_inherits_model_dump_excludes_none(self):
        m = self._IdentifiableChild()
        dumped = m.model_dump()
        assert "updated_at" not in dumped
        assert "created_at" in dumped

    def test_inherits_update_timestamp(self):
        m = self._IdentifiableChild()
        m.update_timestamp()
        assert isinstance(m.updated_at, datetime)

    def test_callable_from_subclass(self):
        id_val = self._IdentifiableChild.generate_id(prefix="test")
        assert id_val.startswith("test-")

    def test_flatten_for_db_includes_id(self):
        m = self._IdentifiableChild(label="x")
        result = m.flatten_for_db()
        assert "id" in result
        assert result["id"] == m.id


class TestVSOAuditableModel:

    def test_created_by_is_none_by_default(self):
        m = _AuditableChild()
        assert m.created_by is None

    def test_updated_by_is_none_by_default(self):
        m = _AuditableChild()
        assert m.updated_by is None

    def test_created_by_can_be_set(self):
        m = _AuditableChild(created_by="user-123")
        assert m.created_by == "user-123"

    def test_update_audit_info_sets_updated_by(self):
        m = _AuditableChild()
        m.update_audit_info("service-abc")
        assert m.updated_by == "service-abc"

    def test_update_audit_info_also_updates_timestamp(self):
        m = _AuditableChild()
        assert m.updated_at is None
        m.update_audit_info("svc")
        assert isinstance(m.updated_at, datetime)
        assert m.updated_at.tzinfo == UTC

    def test_update_audit_info_overwrites_previous_value(self):
        m = _AuditableChild(updated_by="old-svc")
        m.update_audit_info("new-svc")
        assert m.updated_by == "new-svc"

    def test_audit_fields_excluded_from_dump_when_none(self):
        m = _AuditableChild()
        dumped = m.model_dump()
        assert "created_by" not in dumped
        assert "updated_by" not in dumped

    def test_audit_fields_present_in_dump_when_set(self):
        m = _AuditableChild(created_by="creator", updated_by="updater")
        dumped = m.model_dump()
        assert dumped["created_by"] == "creator"
        assert dumped["updated_by"] == "updater"

    def test_inherits_id_field(self):
        m = _AuditableChild()
        assert isinstance(m.id, str)
        assert len(m.id) == 36

    def test_inherits_generate_id(self):
        id_val = _AuditableChild.generate_id(prefix="audit")
        assert id_val.startswith("audit-")

    def test_inherits_timestamps(self):
        m = _AuditableChild()
        assert isinstance(m.created_at, datetime)
        assert m.updated_at is None

    def test_full_lifecycle(self):
        m = _AuditableChild(created_by="creator-svc")
        assert m.created_by == "creator-svc"
        assert m.updated_by is None
        assert m.updated_at is None

        m.update_audit_info("updater-svc")
        assert m.updated_by == "updater-svc"
        assert m.updated_at is not None

        dumped = m.model_dump()
        assert dumped["created_by"] == "creator-svc"
        assert dumped["updated_by"] == "updater-svc"
        assert isinstance(dumped["created_at"], str)
        assert dumped["created_at"].endswith("Z")
        assert isinstance(dumped["updated_at"], str)
        assert dumped["updated_at"].endswith("Z")
        assert "id" in dumped


class TestHierarchyContracts:
    """Enforce structural contracts on the base model hierarchy."""

    def test_vso_timestamped_is_subclass_of_vso_base(self):
        assert issubclass(VSOTimestampedModel, VSOBaseModel)

    def test_vso_identifiable_is_subclass_of_vso_timestamped(self):
        assert issubclass(VSOIdentifiableModel, VSOTimestampedModel)

    def test_vso_auditable_is_subclass_of_vso_identifiable(self):
        assert issubclass(VSOAuditableModel, VSOIdentifiableModel)

    def test_vso_base_has_no_id_field(self):
        assert "id" not in VSOBaseModel.model_fields

    def test_vso_timestamped_has_no_id_field(self):
        assert "id" not in VSOTimestampedModel.model_fields

    def test_vso_identifiable_has_id_field(self):
        assert "id" in VSOIdentifiableModel.model_fields

    def test_vso_identifiable_has_created_at_field(self):
        assert "created_at" in VSOIdentifiableModel.model_fields

    def test_vso_auditable_has_created_by_field(self):
        assert "created_by" in VSOAuditableModel.model_fields

    def test_vso_auditable_has_updated_by_field(self):
        assert "updated_by" in VSOAuditableModel.model_fields

    def test_investigation_model_is_identifiable(self):
        from app.models.investigations import InvestigationModel
        assert issubclass(InvestigationModel, VSOIdentifiableModel)

    def test_investigation_model_has_no_standalone_id_override(self):
        from app.models.investigations import InvestigationModel
        assert "id" not in InvestigationModel.__annotations__, \
            "InvestigationModel must not redefine 'id' — it inherits from VSOIdentifiableModel"

    def test_case_model_is_identifiable(self):
        from app.models.cases import CaseModel
        assert issubclass(CaseModel, VSOIdentifiableModel)

    def test_investigation_create_request_is_not_identifiable(self):
        from app.models.investigations import InvestigationCreateRequest
        assert not issubclass(InvestigationCreateRequest, VSOIdentifiableModel)
        assert issubclass(InvestigationCreateRequest, VSOBaseModel)

    def test_investigation_update_request_is_not_identifiable(self):
        from app.models.investigations import InvestigationUpdateRequest
        assert not issubclass(InvestigationUpdateRequest, VSOIdentifiableModel)

    def test_investigation_query_request_is_not_identifiable(self):
        from app.models.investigations import InvestigationQueryRequest
        assert not issubclass(InvestigationQueryRequest, VSOIdentifiableModel)

    def test_investigation_customer_context_is_not_identifiable(self):
        from app.models.investigations import InvestigationCustomerContext
        assert not issubclass(InvestigationCustomerContext, VSOIdentifiableModel)

    def test_investigation_technical_context_is_not_identifiable(self):
        from app.models.investigations import InvestigationTechnicalContext
        assert not issubclass(InvestigationTechnicalContext, VSOIdentifiableModel)

    def test_investigation_current_state_is_not_identifiable(self):
        from app.models.investigations import InvestigationCurrentState
        assert not issubclass(InvestigationCurrentState, VSOIdentifiableModel)

    def test_investigation_history_entry_is_not_identifiable(self):
        from app.models.investigations import InvestigationHistoryEntry
        assert not issubclass(InvestigationHistoryEntry, VSOIdentifiableModel)

