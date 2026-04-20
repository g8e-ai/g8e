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
from typing import Any, Annotated
from uuid import uuid4

from pydantic import BaseModel, ConfigDict, Field, PlainSerializer, ValidationError, field_serializer, field_validator

__all__ = [
    "ConfigDict",
    "Field",
    "ValidationError",
    "G8eAuditableModel",
    "G8eBaseModel",
    "G8eIdentifiableModel",
    "G8eTimestampedModel",
    "UTCDatetime",
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


UTCDatetime = Annotated[
    datetime,
    PlainSerializer(lambda dt: _to_iso_z(dt) if dt else None, return_type=str | None, when_used="json"),
]


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


class G8eBaseModel(BaseModel):
    """Base model for all g8e domain objects.

    Enforces:
    - ``extra='ignore'``: unknown fields are silently dropped (safe deserialization from wire/DB)
    - ``exclude_none=True`` via model_config: ``model_dump()`` and ``model_dump_json()`` omit None
      fields by default, keeping payloads lean at every boundary
    - ``populate_by_name=True``: field aliases and field names both accepted on construction

    Enum fields remain enum instances inside the application boundary. They are
    only coerced to their string values when crossing a wire/DB/LLM boundary
    via the ``flatten_for_*`` methods (which call ``recursive_serialize`` with
    ``mode="json"``). This preserves typed-object invariants required by the
    developer guide: identity comparisons (``is``/``in``) and ``match``
    statements on enum-typed fields work uniformly whether the object was
    constructed in memory or parsed from a wire payload.

    Boundary methods — use these instead of calling model_dump() directly at boundaries:
    - ``flatten_for_llm()``  — before Part.from_tool_response in agent.py
    - ``flatten_for_db()``   — before db_client.create_document / update_document
    - ``flatten_for_wire()`` — before pubsub_client.publish / outbound HTTP POST to external services
    """

    model_config = ConfigDict(
        populate_by_name=True,
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


class G8eTimestampedModel(G8eBaseModel):
    """Adds UTC timestamps to any g8e model.

    Use for any object that needs creation/mutation time tracking but does not
    need a stable document identity (i.e. is not persisted as its own document).
    For persisted entities use ``G8eIdentifiableModel``.
    """

    created_at: UTCDatetime = Field(default_factory=now, description="When the entity was created (UTC)")
    updated_at: UTCDatetime | None = Field(default=None, description="When the entity was last updated (UTC)")

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

    def update_timestamp(self) -> None:
        self.updated_at = now()


class G8eIdentifiableModel(G8eTimestampedModel):
    """Adds a stable document identity to a timestamped g8e entity.

    Use for any object that is persisted as its own document in the database
    and needs a stable, addressable ID. The ``id`` field defaults to a UUID4
    string via ``generate_id()``.

    Do NOT use this for value objects, request DTOs, or config structs — those
    belong on ``G8eBaseModel`` directly. Misusing this class as a generic base
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


class G8eAuditableModel(G8eIdentifiableModel):
    """Adds actor-level audit fields to an identifiable g8e entity.

    Use for entities where you need to track which user or service created
    and last updated the record, in addition to the standard timestamps.
    """

    created_by: str | None = Field(default=None, description="User or service that created this entity")
    updated_by: str | None = Field(default=None, description="User or service that last updated this entity")

    def update_audit_info(self, user_or_service: str) -> None:
        self.updated_by = user_or_service
        self.update_timestamp()
