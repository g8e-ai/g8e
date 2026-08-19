# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import logging
from typing import TYPE_CHECKING
from fastapi import Request

from app.constants import (
    AUTHORIZATION,
    X_PROXY_ORGANIZATION_ID,
    X_PROXY_USER_EMAIL,
    X_PROXY_USER_ID,
    AuthMethod,
)
from app.errors import AuthenticationError
from app.models.auth import AuthenticatedUser
from app.models.settings import G8eeAppSettings

if TYPE_CHECKING:
    from app.services.operator.operator_session_service import OperatorSessionService

logger = logging.getLogger(__name__)


def is_infrastructure_health_check_ip(ip: str) -> bool:
    """Check if the given IP belongs to known infrastructure health checkers."""
    if not ip:
        return False

    normalized_ip = ip.replace("::ffff:", "") if ip.startswith("::ffff:") else ip

    if normalized_ip.startswith("35.191."):
        return True

    if normalized_ip.startswith("130.211."):
        parts = normalized_ip.split(".")
        if len(parts) == 4:
            try:
                third_octet = int(parts[2])
                if 0 <= third_octet <= 3:
                    return True
            except ValueError:
                pass

    return bool(normalized_ip.startswith("10."))
