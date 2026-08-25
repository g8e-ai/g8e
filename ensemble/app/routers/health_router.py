# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Health Router for g8ee

Provides health check endpoints following the standard g8e pattern.
"""

import logging
from typing import cast

from fastapi import APIRouter, Request

from app.utils.timestamp import now_iso
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
async def liveness_check():
    """
    Liveness probe.

    Checks if the service process is alive.
    This should be a fast, simple check that only verifies the process is responsive.

    SECURITY: Internal only - for health probes.
    """
    return {"status": "alive", "service": "g8ee"}


@router.get("/health/details")
async def detailed_health_check(
    request: Request,
):
    """Detailed health check endpoint that verifies all services are available."""
    state = cast(G8eeAppState, request.app.state)
    services = getattr(state, "services", None)
    clients_status = {
        "cache_aside_service": "up"
        if services and getattr(services, "cache_aside_service", None)
        else "down",
        "operator_kv": "up" if hasattr(state, "pubsub_client") and state.pubsub_client else "down",
        "internal_http_client": "up"
        if services and getattr(services, "internal_http_client", None)
        else "down",
        "operator_command_service": "up"
        if services and getattr(services, "operator_command_service", None)
        else "down",
        "chat_pipeline": "up" if services and getattr(services, "chat_pipeline", None) else "down",
    }

    return {
        "status": "ok",
        "service": "g8ee",
        "timestamp": now_iso(),
        "clients": clients_status,
    }
