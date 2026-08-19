# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
g8e Security utilities.

Security modules:
- auth.py: Authentication utilities
- operator_command_validator.py: Operator command validation
- output_sanitizer.py: Output sanitization for security
- request_timestamp.py: Request timestamp validation
- sentinel_scrubber.py: Sentinel scrubbing for sensitive data
"""

from .operator_command_validator import OperatorCommandValidator
from .output_sanitizer import (
    SanitizationResult,
    sanitize_file_content,
    sanitize_g8eo_output,
)
from .request_timestamp import (
    NONCE_TTL_SECONDS,
    TIMESTAMP_WINDOW_SECONDS,
    NonceCheckResult,
    NonceErrorCode,
    RequestTimestampValidator,
    RequestValidationResult,
    TimestampErrorCode,
    TimestampValidationResult,
    validate_message_timestamp,
    validate_request_timestamp,
    validate_timestamp,
)
from .sentinel_scrubber import (
    ScrubResult,
    SentinelConfig,
    SentinelScrubber,
    get_sentinel_scrubber,
    scrub_user_message,
)

__all__ = [
    "NONCE_TTL_SECONDS",
    "TIMESTAMP_WINDOW_SECONDS",
    "NonceCheckResult",
    "NonceErrorCode",
    "OperatorCommandValidator",
    "RequestTimestampValidator",
    "RequestValidationResult",
    "SanitizationResult",
    "ScrubResult",
    "SentinelConfig",
    "SentinelScrubber",
    "TimestampErrorCode",
    "TimestampValidationResult",
    "get_sentinel_scrubber",
    "sanitize_file_content",
    "sanitize_g8eo_output",
    "scrub_user_message",
    "validate_message_timestamp",
    "validate_request_timestamp",
    "validate_timestamp",
]
