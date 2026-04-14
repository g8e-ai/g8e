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
Request Timestamp Validation for Replay Protection

Provides replay attack protection by validating request timestamps and nonces.

Security Features:
1. Timestamp validation: Request timestamp must be within ±5 minutes of server time
2. Nonce tracking: Optional nonce prevents exact request replay within time window

Headers (for HTTP requests):
- X-Request-Timestamp: ISO 8601 timestamp of when request was created
- X-Request-Nonce: Unique request identifier for replay prevention

Message Fields (for g8es pub/sub messages):
- request_timestamp: ISO 8601 timestamp in message metadata
- request_nonce: Unique identifier in message metadata
"""

import logging
from datetime import datetime, timedelta
from typing import Any, TYPE_CHECKING

from app.services.cache.cache_aside import CacheAsideService

from app.constants import (
    NONCE_TTL_SECONDS,
    TIMESTAMP_WINDOW_SECONDS,
    KVKey,
    NonceErrorCode,
    TimestampErrorCode,
)
from app.models.base import G8eBaseModel
from app.utils.timestamp import ensure_utc, now, parse_iso

logger = logging.getLogger(__name__)

_nonce_cache: dict[str, datetime] = {}


class TimestampValidationResult(G8eBaseModel):
    """Result of timestamp validation."""
    is_valid: bool
    error: str | None = None
    error_code: TimestampErrorCode | None = None
    skew_seconds: float = 0.0


class NonceCheckResult(G8eBaseModel):
    """Result of nonce check operation."""
    success: bool
    is_replay: bool = False
    error: str = ""
    error_code: NonceErrorCode = NonceErrorCode.CHECK_FAILED


class RequestValidationResult(G8eBaseModel):
    """Result of full request validation (timestamp + optional nonce)."""
    is_valid: bool
    error: str | None = None
    error_code: TimestampErrorCode | NonceErrorCode | None = None


def _cleanup_expired_nonces() -> None:
    """Remove expired nonces from in-memory cache."""
    current = now()
    expired_threshold = current - timedelta(seconds=NONCE_TTL_SECONDS)

    expired_nonces = [
        nonce for nonce, ts in _nonce_cache.items()
        if ts < expired_threshold
    ]

    for nonce in expired_nonces:
        del _nonce_cache[nonce]


def validate_timestamp(
    timestamp_str: str | datetime | None,
    allow_missing: bool = False
) -> TimestampValidationResult:
    """
    Validate request timestamp is within acceptable window.

    Args:
        timestamp_str: ISO 8601 timestamp string or datetime object
        allow_missing: If True, missing timestamp returns valid (for optional validation)

    Returns:
        TimestampValidationResult with validation outcome
    """
    if not timestamp_str:
        if allow_missing:
            return TimestampValidationResult(is_valid=True)
        return TimestampValidationResult(
            is_valid=False,
            error="Missing request timestamp",
            error_code=TimestampErrorCode.MISSING_TIMESTAMP
        )

    try:
        if isinstance(timestamp_str, datetime):
            request_time = ensure_utc(timestamp_str)
        else:
            request_time = parse_iso(timestamp_str)

    except (ValueError, TypeError) as e:
        return TimestampValidationResult(
            is_valid=False,
            error=f"Invalid timestamp format (use ISO 8601): {e}",
            error_code=TimestampErrorCode.INVALID_FORMAT
        )

    server_time = now()
    skew = abs((server_time - request_time).total_seconds())

    if skew > TIMESTAMP_WINDOW_SECONDS:
        return TimestampValidationResult(
            is_valid=False,
            error=f"Request timestamp outside acceptable window ({skew:.1f}s skew, max {TIMESTAMP_WINDOW_SECONDS}s)",
            error_code=TimestampErrorCode.OUTSIDE_WINDOW,
            skew_seconds=skew
        )

    return TimestampValidationResult(is_valid=True, skew_seconds=skew)


async def check_nonce_kv(
    nonce: str,
    cache_aside_service: "CacheAsideService"
) -> NonceCheckResult:
    """
    Check and mark nonce using g8es KV store.

    Args:
        nonce: Unique request nonce
        cache_aside_service: CacheAsideService instance

    Returns:
        NonceCheckResult with operation outcome
    """
    try:
        nonce_key = KVKey.nonce(nonce)

        was_set = await cache_aside_service.kv.set(
            nonce_key,
            "1",
            ex=NONCE_TTL_SECONDS,
            #nx=True
        )

        is_replay = not was_set
        return NonceCheckResult(success=True, is_replay=is_replay)

    except Exception as e:
        logger.warning("g8es nonce check failed: %s", e)
        return NonceCheckResult(
            success=False,
            error=str(e),
            error_code=NonceErrorCode.CHECK_FAILED
        )


def check_nonce_memory(nonce: str) -> NonceCheckResult:
    """
    Check and mark nonce using in-memory cache.

    Args:
        nonce: Unique request nonce

    Returns:
        NonceCheckResult with replay detection outcome
    """
    if len(_nonce_cache) > 10000:
        _cleanup_expired_nonces()

    if nonce in _nonce_cache:
        return NonceCheckResult(success=True, is_replay=True)

    _nonce_cache[nonce] = now()
    return NonceCheckResult(success=True, is_replay=False)


async def validate_request_timestamp(
    timestamp_str: str | None,
    nonce: str,
    require_nonce: bool,
    cache_aside_service: CacheAsideService,
    context: dict[str, object]
) -> RequestValidationResult:
    """
    Validate request timestamp and optional nonce for replay protection.

    Args:
        timestamp_str: ISO 8601 timestamp string
        nonce: Optional unique request nonce
        require_nonce: Whether nonce is required
        cache_aside_service: Optional CacheAsideService for distributed nonce tracking
        context: Optional context dict for logging (operator_id, etc.)

    Returns:
        RequestValidationResult with validation outcome
    """
    ctx = context or {}

    timestamp_result = validate_timestamp(timestamp_str)

    if not timestamp_result.is_valid:
        logger.warning(
            "[REQUEST-TIMESTAMP] Request rejected - invalid timestamp",
            extra={
                "error": timestamp_result.error,
                "provided_timestamp": timestamp_str,
                "security_event": "replay_protection_timestamp_rejected",
                **ctx
            }
        )
        return RequestValidationResult(
            is_valid=False,
            error=timestamp_result.error,
            error_code=timestamp_result.error_code
        )

    if nonce:
        is_replay = False

        if cache_aside_service:
            nonce_result = await check_nonce_kv(nonce, cache_aside_service)
            if not nonce_result.success:
                is_replay = check_nonce_memory(nonce).is_replay
            else:
                is_replay = nonce_result.is_replay
        else:
            is_replay = check_nonce_memory(nonce).is_replay

        if is_replay:
            logger.error(
                "[REQUEST-TIMESTAMP] Request rejected - nonce replay detected",
                extra={
                    "nonce_prefix": nonce[:16] + "..." if len(nonce) > 16 else nonce,
                    "security_event": "replay_attack_detected",
                    **ctx
                }
            )
            return RequestValidationResult(
                is_valid=False,
                error="Request replay detected (duplicate nonce)",
                error_code=NonceErrorCode.REPLAY_DETECTED
            )

    elif require_nonce:
        logger.warning(
            "[REQUEST-TIMESTAMP] Request rejected - missing required nonce",
            extra={
                "security_event": "missing_nonce",
                **ctx
            }
        )
        return RequestValidationResult(
            is_valid=False,
            error="Request nonce is required",
            error_code=NonceErrorCode.MISSING_REQUIRED
        )

    logger.info(
        "[REQUEST-TIMESTAMP] Request timestamp validated",
        extra={
            "skew_seconds": timestamp_result.skew_seconds,
            "has_nonce": bool(nonce),
            **ctx
        }
    )

    return RequestValidationResult(is_valid=True)


def validate_message_timestamp(
    message: dict[str, object],
    timestamp_field: str = "timestamp",
    nonce_field: str = "request_nonce",
    require_nonce: bool = False
) -> RequestValidationResult:
    """
    Validate timestamp in a message/payload (synchronous version for PubSub messages).

    Args:
        message: Message dict containing timestamp
        timestamp_field: Field name for timestamp (default: "timestamp")
        nonce_field: Field name for nonce (default: "request_nonce")
        require_nonce: Whether nonce is required

    Returns:
        RequestValidationResult with validation outcome
    """
    timestamp_str = message.get(timestamp_field) or message.get("metadata", {}).get(timestamp_field)
    nonce = message.get(nonce_field) or message.get("metadata", {}).get(nonce_field)

    timestamp_result = validate_timestamp(timestamp_str)

    if not timestamp_result.is_valid:
        return RequestValidationResult(
            is_valid=False,
            error=timestamp_result.error,
            error_code=timestamp_result.error_code
        )

    if nonce:
        nonce_result = check_nonce_memory(nonce)
        if nonce_result.is_replay:
            return RequestValidationResult(
                is_valid=False,
                error="Request replay detected (duplicate nonce)",
                error_code=NonceErrorCode.REPLAY_DETECTED
            )
    elif require_nonce:
        return RequestValidationResult(
            is_valid=False,
            error="Request nonce is required",
            error_code=NonceErrorCode.MISSING_REQUIRED
        )

    return RequestValidationResult(is_valid=True)


class RequestTimestampValidator:
    """
    Request timestamp validator with optional g8es KV support.

    Provides both sync and async validation methods.
    """

    def __init__(self, cache_aside_service: "CacheAsideService | None", require_nonce: bool = False):
        """
        Initialize validator.

        Args:
            cache_aside_service: Optional CacheAsideService for distributed nonce tracking
            require_nonce: Whether nonce is required by default
        """
        self.cache_aside_service = cache_aside_service
        self.require_nonce = require_nonce

    async def validate_async(
        self,
        timestamp_str: str | None,
        nonce: str,
        context: dict[str, object]
    ) -> RequestValidationResult:
        """Async validation with g8es KV support."""
        return await validate_request_timestamp(
            timestamp_str=timestamp_str,
            nonce=nonce,
            require_nonce=self.require_nonce,
            cache_aside_service=self.cache_aside_service,
            context=context
        )

    def validate_sync(
        self,
        timestamp_str: str | None,
        nonce: str
    ) -> RequestValidationResult:
        """Synchronous validation (in-memory nonce cache only)."""
        timestamp_result = validate_timestamp(timestamp_str)
        if not timestamp_result.is_valid:
            return RequestValidationResult(
                is_valid=False,
                error=timestamp_result.error,
                error_code=timestamp_result.error_code
            )

        if nonce:
            nonce_result = check_nonce_memory(nonce)
            if nonce_result.is_replay:
                return RequestValidationResult(
                    is_valid=False,
                    error="Request replay detected",
                    error_code=NonceErrorCode.REPLAY_DETECTED
                )
        elif self.require_nonce:
            return RequestValidationResult(
                is_valid=False,
                error="Request nonce is required",
                error_code=NonceErrorCode.MISSING_REQUIRED
            )

        return RequestValidationResult(is_valid=True)

    def validate_message(
        self,
        message: dict[str, object],
        timestamp_field: str = "timestamp",
        nonce_field: str = "request_nonce"
    ) -> RequestValidationResult:
        """Validate timestamp in a message payload."""
        return validate_message_timestamp(
            message=message,
            timestamp_field=timestamp_field,
            nonce_field=nonce_field,
            require_nonce=self.require_nonce
        )
