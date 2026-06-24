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

from pydantic import BaseModel, ConfigDict, Field, PlainSerializer, ValidationError, field_validator, model_validator

__all__ = [
    "ConfigDict",
    "Field",
    "G8eBaseModel",
    "UTCDatetime",
    "ValidationError",
    "field_validator",
    "model_validator",
]

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

class G8eBaseModel(BaseModel):
    """Base model for all g8e domain objects in the protocol package."""
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
