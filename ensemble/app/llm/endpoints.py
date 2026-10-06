# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Provider endpoint normalization without importing provider SDKs."""

from app.constants import OLLAMA_DEFAULT_PROTOCOL


def normalize_ollama_host(endpoint: str) -> str:
    """Normalize a user-supplied Ollama host into a base URL.

    Accepts:
      - "host:port"              -> "http://host:port"
      - "http://host:port"       -> "http://host:port"

    The Ollama native API lives at /api/chat. Endpoints containing a `/v1`
    path segment are rejected with a clear error so misconfigured settings
    fail fast instead of silently producing the wrong outbound URL.
    """
    cleaned = (endpoint or "").strip().rstrip("/")
    if "/v1" in cleaned:
        raise ValueError(
            f"Invalid Ollama endpoint {endpoint!r}: must not contain '/v1'. "
            "Ollama uses its native /api/chat surface; configure 'host:port' "
            "or 'http(s)://host:port' only."
        )
    if cleaned and not cleaned.startswith(("http://", "https://")):
        cleaned = OLLAMA_DEFAULT_PROTOCOL + cleaned
    return cleaned
