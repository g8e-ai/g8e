# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from datetime import UTC, datetime
from typing import Any
from uuid import uuid4

from pydantic import BaseModel, PrivateAttr, TypeAdapter, ValidationInfo, computed_field

from g8e.models.base import (
    ConfigDict,
    Field,
    G8eBaseModel,
    UTCDatetime,
    ValidationError,
    _to_iso_z,
    field_validator,
    model_validator,
)

__all__ = [
    "BaseModel",
    "ConfigDict",
    "Field",
    "G8eAuditableModel",
    "G8eBaseModel",
    "G8eIdentifiableModel",
    "G8eTimestampedModel",
    "PrivateAttr",
    "TypeAdapter",
    "UTCDatetime",
    "ValidationError",
    "ValidationInfo",
    "_to_iso_z",
    "computed_field",
    "field_validator",
    "model_validator",
    "recursive_serialize",
]

from app.utils.timestamp import now


def recursive_serialize(value: Any) -> Any:
    """Recursively convert Pydantic models to plain dicts for boundary crossing.

    Intended for dict/list inputs only (e.g., raw DB responses). For models,
    use ``model.model_dump(mode="json")`` directly. Used by CacheAsideService
    to flatten datetime-bearing dicts returned from the DB (cache_aside.py:281).
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


class G8eTimestampedModel(G8eBaseModel):
    """Adds UTC timestamps to any g8e model.

    Use for any object that needs creation/mutation time tracking but does not
    need a stable document identity (i.e. is not persisted as its own document).
    For persisted entities use ``G8eIdentifiableModel``.
    """

    created_at: UTCDatetime = Field(
        default_factory=now, description="When the entity was created (UTC)"
    )
    updated_at: UTCDatetime | None = Field(
        default=None, description="When the entity was last updated (UTC)"
    )

    @field_validator("created_at", "updated_at", mode="before")
    @classmethod
    def normalize_datetime_utc(cls, v):
        """Normalize all datetime fields to UTC with consistent timezone."""
        if v is None:
            return v
        if isinstance(v, datetime):
            if v.tzinfo is None:
                return v.replace(tzinfo=UTC)
            return v.astimezone(UTC)
        return v

    def update_timestamp(self) -> None:
        self.updated_at = now()


class G8eIdentifiableModel(G8eTimestampedModel):
    """Adds a stable document identity to a timestamped g8e entity.

    Use for any object that is persisted as its own document in the database
    and needs a stable, addressable ID. The ``id`` field defaults to a UUID4
    string via ``generate_id()``.

    Do NOT use this for value objects, request DTOs, or config structs - those
    belong on ``G8eBaseModel`` directly. Misusing this class as a generic base
    pollutes every payload with ``id``, ``created_at``, and ``updated_at`` fields
    that have no meaning for ephemeral objects.
    """

    id: str = Field(
        default_factory=lambda: str(uuid4()), description="Stable document identifier (UUID4)"
    )

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

    created_by: str | None = Field(
        default=None, description="User or service that created this entity"
    )
    updated_by: str | None = Field(
        default=None, description="User or service that last updated this entity"
    )

    def update_audit_info(self, user_or_service: str) -> None:
        self.updated_by = user_or_service
        self.update_timestamp()
