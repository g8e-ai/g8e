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

import logging
from collections.abc import Coroutine
from fastapi import Request

from app.constants import ComponentName, HealthStatus
from app.models.health import DependencyStatus, HealthCheckResult
from app.utils.timestamp import now

logger = logging.getLogger(__name__)


class HealthService:
    """Service for checking the health of g8ee and its dependencies."""

    @staticmethod
    async def check_dependencies(request_context: Request) -> HealthCheckResult:
        """
        Check the health of all registered g8ee dependencies.
        
        request_context should be an object (like FastAPI Request) that provides access
        to the dependency getters.
        """
        from app.dependencies import (
            get_g8ee_platform_settings,
            get_g8ee_cache_aside_service,
            get_g8ee_investigation_data_service,
            get_g8ee_investigation_service,
            get_g8ee_memory_service,
            get_g8ee_chat_pipeline,
            get_g8ee_attachment_service,
        )

        dependencies: dict[str, DependencyStatus] = {}

        async def _check(name: str, coro: Coroutine) -> None:
            try:
                await coro
                dependencies[name] = DependencyStatus(status=HealthStatus.HEALTHY)
            except Exception as e:
                dependencies[name] = DependencyStatus(status=HealthStatus.UNHEALTHY, error=str(e))

        await _check("settings", get_g8ee_platform_settings(request_context))
        dependencies["llm_provider"] = DependencyStatus(status=HealthStatus.HEALTHY)
        await _check("cache_aside_service", get_g8ee_cache_aside_service(request_context))
        await _check("investigation_data_service", get_g8ee_investigation_data_service(request_context))
        await _check("investigation_service", get_g8ee_investigation_service(request_context))
        await _check("memory_service", get_g8ee_memory_service(request_context))
        await _check("chat_pipeline", get_g8ee_chat_pipeline(request_context))
        await _check("attachment_service", get_g8ee_attachment_service(request_context))

        unhealthy_deps = [name for name, dep in dependencies.items() if dep.status != HealthStatus.HEALTHY]
        overall_status = HealthStatus.HEALTHY if not unhealthy_deps else HealthStatus.UNHEALTHY

        logger.info("g8ee dependency health check completed: %s", overall_status)
        return HealthCheckResult(
            timestamp=now(),
            component=ComponentName.G8EE,
            dependencies=dependencies,
            overall_status=overall_status,
            unhealthy_dependencies=unhealthy_deps if unhealthy_deps else None,
        )
