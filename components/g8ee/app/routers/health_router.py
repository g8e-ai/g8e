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

"""
Health Router for g8ee

Provides health check endpoints following the standard g8e pattern.
"""

import logging
from typing import cast

from fastapi import APIRouter, Depends, Request

from app.utils.timestamp import now_iso
from app.dependencies import require_internal_origin
from app.models.state import G8eeAppState

router = APIRouter(tags=["health"])
logger = logging.getLogger(__name__)


@router.get("/health")
async def health_check():
    """
    Basic health check endpoint - publicly accessible.
    
    Returns simple 'OK' response for load balancer health checks.
    No authentication required for this endpoint only.
    """
    return {"status": "ok"}


@router.get("/health/live")
async def liveness_check(_: bool = Depends(require_internal_origin)):
    """
    Liveness probe.
    
    Checks if the service process is alive.
    This should be a fast, simple check that only verifies the process is responsive.
    
    SECURITY: Internal only - for health probes.
    """
    return {
        "status": "alive",
        "service": "g8ee"
    }

@router.get("/health/details")
async def detailed_health_check(
    request: Request,
    _: bool = Depends(require_internal_origin),
):
    """Detailed health check endpoint that verifies all services are available."""
    state = cast(G8eeAppState, request.app.state)
    services = getattr(state, "services", None)
    clients_status = {
        "cache_aside_service": "up" if services and getattr(services, "cache_aside_service", None) else "down",
        "g8es_kv": "up" if hasattr(state, "pubsub_client") and state.pubsub_client else "down",
        "internal_http_client": "up" if hasattr(state, "internal_http_client") and state.internal_http_client else "down",
        "operator_command_service": "up" if services and getattr(services, "operator_command_service", None) else "down",
        "chat_pipeline": "up" if services and getattr(services, "chat_pipeline", None) else "down"
    }

    return {
        "status": "ok",
        "service": "g8ee",
        "timestamp": now_iso(),
        "clients": clients_status,
    }
