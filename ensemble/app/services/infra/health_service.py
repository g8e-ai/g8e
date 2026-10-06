# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging
from collections.abc import Coroutine, Mapping
from typing import Any

from app.constants import G8EE_COMPONENT, HealthStatus
from app.models.health import DependencyStatus, HealthCheckResult
from app.utils.time_ids.timestamp import now

logger = logging.getLogger(__name__)


class HealthService:
    """Service for checking the health of g8ee and its dependencies."""

    @staticmethod
    async def check_dependencies(
        checks: Mapping[str, Coroutine[Any, Any, Any]],
    ) -> HealthCheckResult:
        """
        Check the health of all registered g8ee dependencies.

        ``checks`` maps each dependency name to an un-awaited dependency getter
        coroutine; a getter that raises marks that dependency unhealthy.
        """
        dependencies: dict[str, DependencyStatus] = {}

        for name, coro in checks.items():
            try:
                await coro
                dependencies[name] = DependencyStatus(status=HealthStatus.HEALTHY)
            except Exception as e:
                dependencies[name] = DependencyStatus(status=HealthStatus.UNHEALTHY, error=str(e))

        unhealthy_deps = [
            name for name, dep in dependencies.items() if dep.status != HealthStatus.HEALTHY
        ]
        overall_status = HealthStatus.HEALTHY if not unhealthy_deps else HealthStatus.UNHEALTHY

        logger.info("g8ee dependency health check completed: %s", overall_status)
        return HealthCheckResult(
            timestamp=now(),
            component=G8EE_COMPONENT,
            dependencies=dependencies,
            overall_status=overall_status,
            unhealthy_dependencies=unhealthy_deps if unhealthy_deps else None,
        )
