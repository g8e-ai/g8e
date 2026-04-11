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

from datetime import datetime

from pydantic import Field

from app.constants import EventType, InvestigationStatus
from app.models.base import VSOBaseModel


class CaseHistoryQuery(VSOBaseModel):
    """Query parameters for case history operations."""
    case_id: str
    start_time: datetime | None = None
    end_time: datetime | None = None
    event_type: EventType | None = None
    limit: int = 100


class InvestigationQuery(VSOBaseModel):
    """Query parameters for investigation operations."""
    case_id: str
    investigation_id: str | None = None
    status: InvestigationStatus | None = None
    limit: int = 50


class AnalysisQuery(VSOBaseModel):
    """Query parameters for analysis searches."""
    case_id: str | None = Field(default=None, description="Filter by case ID")
    task_id: str | None = Field(default=None, description="Filter by task ID")
    investigation_id: str | None = Field(default=None, description="Filter by investigation ID")
    status: InvestigationStatus | None = Field(default=None, description="Filter by investigation status")
    confidence_min: float | None = Field(default=None, description="Minimum threat detection confidence score", ge=0.0, le=1.0)
    confidence_max: float | None = Field(default=None, description="Maximum threat detection confidence score", ge=0.0, le=1.0)
    limit: int = Field(default=50, description="Maximum number of results", gt=0, le=100)
