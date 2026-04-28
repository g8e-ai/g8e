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

"""Startup reconciler for ``platform_settings.g8ep_operator_api_key``.

`api_keys` is the single source of truth for credentials; the platform
settings field is a bootstrap-distribution mirror consumed by g8ep's
``fetch-key-and-run.sh``. This reconciler verifies the mirror points at an
ACTIVE row in ``api_keys`` and clears it otherwise, so g8ep's wait-loop
re-engages instead of authenticating with a stale or revoked key.
"""

from __future__ import annotations

import logging

from app.services.auth.api_key_service import ApiKeyService
from app.services.infra.settings_service import SettingsServiceProtocol

logger = logging.getLogger(__name__)


async def reconcile_g8ep_operator_key(
    api_key_service: ApiKeyService,
    settings_service: SettingsServiceProtocol,
) -> None:
    """Ensure ``platform_settings.g8ep_operator_api_key`` matches an ACTIVE
    record in ``api_keys``; clear it if not.

    Safe to call repeatedly. Failures are logged and swallowed so a
    transient g8es hiccup at boot does not block g8ee startup.
    """
    try:
        stored = await settings_service.get_stored_g8ep_operator_api_key()
    except Exception as e:
        logger.warning(
            "[OPERATOR-KEY-RECONCILER] Failed to read platform_settings; skipping",
            extra={"error": str(e)},
        )
        return

    if not stored:
        logger.info("[OPERATOR-KEY-RECONCILER] No mirrored g8ep operator API key; nothing to reconcile")
        return

    try:
        valid, _doc, reason = await api_key_service.validate_key(stored)
    except Exception as e:
        logger.warning(
            "[OPERATOR-KEY-RECONCILER] Validation lookup failed; skipping clear to avoid false-negative",
            extra={"error": str(e)},
        )
        return

    if valid:
        logger.info("[OPERATOR-KEY-RECONCILER] Mirrored g8ep operator API key is valid")
        return

    logger.warning(
        "[OPERATOR-KEY-RECONCILER] Mirrored g8ep operator API key is invalid; clearing platform_settings",
        extra={"reason": reason},
    )
    try:
        await settings_service.clear_g8ep_operator_api_key(expected=stored)
    except Exception as e:
        logger.error(
            "[OPERATOR-KEY-RECONCILER] Failed to clear stale g8ep operator API key",
            extra={"error": str(e)},
        )
