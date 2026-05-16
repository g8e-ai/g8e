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

"""State management for g8ee FastAPI application."""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol, runtime_checkable

if TYPE_CHECKING:
    from app.clients.blob_client import BlobClient
    from app.clients.db_client import DBClient
    from app.clients.kv_cache_client import KVCacheClient
    from app.clients.pubsub_client import PubSubClient
    from app.models.settings import G8eePlatformSettings
    from app.services.service_factory import AllServices
    from app.services.infra.internal_http_client import InternalHttpClient


@runtime_checkable
class G8eeAppState(Protocol):
    """Protocol for g8ee FastAPI app.state to ensure type safety."""

    # Settings and bootstrap
    settings: G8eePlatformSettings

    # Core transport clients
    db_client: DBClient
    kv_cache_client: KVCacheClient
    pubsub_client: PubSubClient
    blob_client: BlobClient
    internal_http_client: InternalHttpClient

    # Domain services container (The "typed state container")
    services: AllServices
