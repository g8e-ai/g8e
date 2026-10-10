# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

"""Provider HTTP client for external LLM providers.

This module defines the explicit HTTP client for talking to external LLM
providers (OpenAI, Anthropic, Ollama, Gemini, etc.). It uses httpx.

For internal g8e service-to-service communication, use
``app.clients.http_client.HTTPClient`` (aiohttp-based) instead.

The explicit naming distinguishes provider-bound traffic (external,
untrusted, billed per call) from internal traffic (internal, trusted,
no per-call cost).
"""

import httpx

from app.constants import DEFAULT_HTTP_CLIENT_TIMEOUT

# Explicit alias: the HTTP client for external LLM providers.
# Use this instead of importing httpx directly in provider code.
ProviderHTTPClient = httpx.AsyncClient

# Re-exported httpx types for provider error handling.
# Import these from here, not from httpx directly.
ProviderHTTPError = httpx.HTTPError
ProviderTimeoutError = httpx.TimeoutException
ProviderResponse = httpx.Response


def create_provider_http_client(
    timeout: float = DEFAULT_HTTP_CLIENT_TIMEOUT,
    follow_redirects: bool = False,
) -> ProviderHTTPClient:
    """Create an HTTP client for external LLM provider calls.

    Args:
        timeout: Request timeout in seconds.
        follow_redirects: Whether to follow HTTP redirects.

    Returns:
        A configured async HTTP client for provider traffic.
    """
    return httpx.AsyncClient(timeout=timeout, follow_redirects=follow_redirects)
