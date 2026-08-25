# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.


from app.constants import HealthStatus

from .base import G8eBaseModel, UTCDatetime


class DependencyStatus(G8eBaseModel):
    """Health status of a single dependency."""

    status: HealthStatus
    error: str | None = None


class HealthCheckResult(G8eBaseModel):
    """Result of a full dependency health check."""

    timestamp: UTCDatetime
    component: str
    dependencies: dict[str, DependencyStatus]
    overall_status: HealthStatus
    unhealthy_dependencies: list[str] | None = None


class WorkflowHealthResult(G8eBaseModel):
    """Health check result for a collection of workflows."""

    status: HealthStatus
    workflows: dict[str, DependencyStatus]


class ServiceHealthResult(G8eBaseModel):
    """Top-level health check result for a g8ee service."""

    service: HealthStatus
    timestamp: UTCDatetime
    checks: dict[str, DependencyStatus]
    error: str | None = None
