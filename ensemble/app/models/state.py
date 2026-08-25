# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""State management for g8ee FastAPI application."""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol, runtime_checkable

if TYPE_CHECKING:
    from app.clients.blob_client import BlobClient
    from app.clients.db_client import DBClient
    from app.clients.kv_cache_client import KVCacheClient
    from app.clients.pubsub_client import PubSubClient
    from app.models.settings import G8eeAppSettings
    from app.services.service_factory import AllServices
    from app.services.infra.internal_http_client import InternalHttpClient


@runtime_checkable
class G8eeAppState(Protocol):
    """Protocol for g8ee FastAPI app.state to ensure type safety."""

    # Settings and bootstrap
    settings: G8eeAppSettings

    # Core transport clients
    db_client: DBClient
    kv_cache_client: KVCacheClient
    pubsub_client: PubSubClient
    blob_client: BlobClient
    internal_http_client: InternalHttpClient

    # Domain services container (The "typed state container")
    services: AllServices
