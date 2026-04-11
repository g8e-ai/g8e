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

from app.constants import HealthStatus

from .base import Field, VSOBaseModel


class DependencyStatus(VSOBaseModel):
    """Health status of a single dependency."""
    status: HealthStatus
    error: str | None = None


class HealthCheckResult(VSOBaseModel):
    """Result of a full dependency health check."""
    timestamp: datetime
    component: str
    dependencies: dict[str, DependencyStatus]
    overall_status: HealthStatus
    unhealthy_dependencies: list[str] | None = None


class WorkflowHealthResult(VSOBaseModel):
    """Health check result for a collection of workflows."""
    status: HealthStatus
    workflows: dict[str, DependencyStatus]


class ServiceHealthResult(VSOBaseModel):
    """Top-level health check result for a g8ee service."""
    service: HealthStatus
    timestamp: datetime
    checks: dict[str, DependencyStatus]
    error: str | None = None
