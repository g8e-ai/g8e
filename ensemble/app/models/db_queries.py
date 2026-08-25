# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from datetime import datetime

from app.constants import EventType, InvestigationStatus
from app.models.base import G8eBaseModel, Field


class CaseHistoryQuery(G8eBaseModel):
    """Query parameters for case history operations."""

    case_id: str
    start_time: datetime | None = None
    end_time: datetime | None = None
    event_type: EventType | None = None
    limit: int = 100


class InvestigationQuery(G8eBaseModel):
    """Query parameters for investigation operations."""

    case_id: str
    investigation_id: str | None = None
    status: InvestigationStatus | None = None
    limit: int = 50


class AnalysisQuery(G8eBaseModel):
    """Query parameters for analysis searches."""

    case_id: str | None = Field(default=None, description="Filter by case ID")
    task_id: str | None = Field(default=None, description="Filter by task ID")
    investigation_id: str | None = Field(default=None, description="Filter by investigation ID")
    status: InvestigationStatus | None = Field(
        default=None, description="Filter by investigation status"
    )
    confidence_min: float | None = Field(
        default=None, description="Minimum threat detection confidence score", ge=0.0, le=1.0
    )
    confidence_max: float | None = Field(
        default=None, description="Maximum threat detection confidence score", ge=0.0, le=1.0
    )
    limit: int = Field(default=50, description="Maximum number of results", gt=0, le=100)
