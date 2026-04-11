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

from datetime import UTC, datetime
from typing import Any
from uuid import uuid4

from pydantic import BaseModel, ConfigDict, Field, ValidationError, field_serializer, field_validator

__all__ = [
    "ConfigDict",
    "Field",
    "ValidationError",
    "VSOAuditableModel",
    "VSOBaseModel",
    "VSOIdentifiableModel",
    "VSOTimestampedModel",
    "_to_iso_z",
    "field_validator",
    "recursive_serialize",
]

from app.utils.timestamp import now


def _to_iso_z(dt: datetime) -> str:
    """Serialize a datetime to ISO 8601 with Z suffix (UTC canonical form)."""
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=UTC)
    else:
        dt = dt.astimezone(UTC)
    return dt.strftime("%Y-%m-%dT%H:%M:%S") + (f".{dt.microsecond:06d}" if dt.microsecond else "") + "Z"


def recursive_serialize(value: Any) -> Any:
    """Recursively convert Pydantic models to plain dicts for boundary crossing.

    Used by flatten_for_db(), flatten_for_llm(), and flatten_for_wire(). Not for
    use inside the application boundary — models stay as models until a boundary
    is crossed.
    """
    if isinstance(value, BaseModel):
        return value.model_dump(mode="json", exclude_none=True)
    if isinstance(value, datetime):
        return _to_iso_z(value)
    if isinstance(value, list):
        return [recursive_serialize(item) for item in value]
    if isinstance(value, dict):
        return {k: recursive_serialize(v) for k, v in value.items()}
    return value


class VSOBaseModel(BaseModel):
    """Base model for all VSO domain objects.

    Enforces:
    - ``extra='ignore'``: unknown fields are silently dropped (safe deserialization from wire/DB)
    - ``exclude_none=True`` via model_config: ``model_dump()`` and ``model_dump_json()`` omit None
      fields by default, keeping payloads lean at every boundary
    - ``use_enum_values=True``: enums serialize as their primitive values, not as enum instances
    - ``populate_by_name=True``: field aliases and field names both accepted on construction

    Boundary methods — use these instead of calling model_dump() directly at boundaries:
    - ``flatten_for_llm()``  — before Part.from_tool_response in agent.py
    - ``flatten_for_db()``   — before db_client.create_document / update_document
    - ``flatten_for_wire()`` — before pubsub_client.publish / outbound HTTP POST to external services
    """

    model_config = ConfigDict(
        populate_by_name=True,
        use_enum_values=True,
        extra="ignore",
    )

    def model_dump(self, **kwargs: Any) -> dict[str, Any]:
        kwargs.setdefault("exclude_none", True)
        return super().model_dump(**kwargs)

    def model_dump_json(self, **kwargs: Any) -> str:
        kwargs.setdefault("exclude_none", True)
        return super().model_dump_json(**kwargs)

    def flatten_for_llm(self) -> dict[str, Any]:
        """Serialize for the LLM boundary (before Part.from_tool_response).

        Subclasses override to control exactly what the LLM sees.
        """
        return recursive_serialize(self)

    def flatten_for_db(self) -> dict[str, Any]:
        """Serialize for the DB write boundary (before create_document / update_document).

        Subclasses override to control exactly what is persisted.
        """
        return recursive_serialize(self)

    def flatten_for_wire(self) -> dict[str, Any]:
        """Serialize for the HTTP/pub-sub wire boundary (before any outbound payload).

        Subclasses override to control exactly what goes on the wire.
        """
        return recursive_serialize(self)


class VSOTimestampedModel(VSOBaseModel):
    """Adds UTC timestamps to any VSO model.

    Use for any object that needs creation/mutation time tracking but does not
    need a stable document identity (i.e. is not persisted as its own document).
    For persisted entities use ``VSOIdentifiableModel``.
    """

    created_at: datetime = Field(default_factory=now, description="When the entity was created (UTC)")
    updated_at: datetime | None = Field(default=None, description="When the entity was last updated (UTC)")

    @field_validator("created_at", "updated_at", mode="before")
    @classmethod
    def normalize_datetime_utc(cls, v):
        """Normalize all datetime fields to UTC with consistent timezone."""
        if v is None:
            return v
        if isinstance(v, datetime):
            if v.tzinfo is None:
                return v.replace(tzinfo=UTC)
            else:
                return v.astimezone(UTC)
        return v

    @field_serializer("created_at", "updated_at")
    def serialize_datetime(self, dt: datetime | None) -> str | None:
        return _to_iso_z(dt) if dt is not None else None

    def update_timestamp(self) -> None:
        self.updated_at = now()


class VSOIdentifiableModel(VSOTimestampedModel):
    """Adds a stable document identity to a timestamped VSO entity.

    Use for any object that is persisted as its own document in the database
    and needs a stable, addressable ID. The ``id`` field defaults to a UUID4
    string via ``generate_id()``.

    Do NOT use this for value objects, request DTOs, or config structs — those
    belong on ``VSOBaseModel`` directly. Misusing this class as a generic base
    pollutes every payload with ``id``, ``created_at``, and ``updated_at`` fields
    that have no meaning for ephemeral objects.
    """

    id: str = Field(default_factory=lambda: str(uuid4()), description="Stable document identifier (UUID4)")

    @classmethod
    def generate_id(cls, prefix: str | None = None) -> str:
        """Generate a new UUID4 ID, optionally prefixed (e.g. ``inv-<uuid>``).

        Useful at construction sites that need the ID before instantiation.
        The ``id`` field itself uses a plain UUID4 by default; call this when
        you need a prefixed variant and pass it explicitly:

            model = MyModel(id=MyModel.generate_id(prefix="inv"), ...)
        """
        base_id = str(uuid4())
        return f"{prefix}-{base_id}" if prefix else base_id


class VSOAuditableModel(VSOIdentifiableModel):
    """Adds actor-level audit fields to an identifiable VSO entity.

    Use for entities where you need to track which user or service created
    and last updated the record, in addition to the standard timestamps.
    """

    created_by: str | None = Field(default=None, description="User or service that created this entity")
    updated_by: str | None = Field(default=None, description="User or service that last updated this entity")

    def update_audit_info(self, user_or_service: str) -> None:
        self.updated_by = user_or_service
        self.update_timestamp()
