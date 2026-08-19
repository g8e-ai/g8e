# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Models for HTTP client and service state."""

from app.models.base import G8eBaseModel


class HTTPClientStatus(G8eBaseModel):
    """Status information for an individual HTTP client."""

    service_name: str
    base_url: str
    is_session_closed: bool
    circuit_breaker_count: int


class HTTPServiceStatus(G8eBaseModel):
    """Complete status information for the HTTP service."""

    is_ready: bool
    active_clients: dict[str, HTTPClientStatus]
