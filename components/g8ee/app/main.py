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

"""g8ee FastAPI Application - Main Entry Point.

g8e Engine (g8ee) - AI engine for g8e platform. Agentic AI system with
LLM provider abstraction providing Zero-Trust AI for infrastructure operations.

Bootstrap responsibilities (this file):
    1. SettingsService bootstrap + local settings
    2. Raw g8es client connections (5 core clients: DB, KV, PubSub, Blob, HTTP)
    3. Handler services (sole users of each client): DBService, KVService, BlobService
    4. CacheAsideService (orchestrator over DB + KV handler services)
    5. Platform settings load from g8es
    6. Delegate ALL domain service construction to ServiceFactory
    7. Service lifecycle start / stop
    8. FastAPI app creation, CORS, router registration

    HTTP client is created and managed by ServiceFactory (HTTPService + InternalHttpClient).
"""
import logging

from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from .clients.blob_client import BlobClient
from .clients.db_client import DBClient
from .clients.kv_cache_client import KVCacheClient
from .clients.pubsub_client import PubSubClient
from .constants import (
    CORS_ALLOWED_ORIGIN_G8E,
    CORS_ALLOWED_ORIGIN_LOCALHOST,
    CORS_ALLOWED_ORIGIN_G8EE,
    CORS_ALLOWED_ORIGIN_G8ED_HTTP,
    CORS_ALLOWED_ORIGIN_G8ED_HTTPS,
    HTTP_ACCEPT_HEADER,
    HTTP_ACCEPT_LANGUAGE_HEADER,
    HTTP_ACCESS_CONTROL_ALLOW_CREDENTIALS,
    HTTP_ACCESS_CONTROL_ALLOW_ORIGIN,
    HTTP_ACCESS_CONTROL_REQUEST_HEADERS,
    HTTP_ACCESS_CONTROL_REQUEST_METHOD,
    HTTP_API_KEY_HEADER,
    HTTP_AUTHORIZATION_HEADER,
    HTTP_CACHE_CONTROL_HEADER,
    HTTP_CONTENT_LANGUAGE_HEADER,
    HTTP_CONTENT_TYPE_HEADER,
    HTTP_COOKIE_HEADER,
    HTTP_LAST_EVENT_ID_HEADER,
    HTTP_METHOD_DELETE,
    HTTP_METHOD_GET,
    HTTP_METHOD_OPTIONS,
    HTTP_METHOD_POST,
    HTTP_METHOD_PUT,
    HTTP_PRAGMA_HEADER,
    HTTP_REQUESTED_WITH_HEADER,
    HTTP_SET_COOKIE_HEADER,
    G8EE_APP_CONTACT_EMAIL,
    G8EE_APP_CONTACT_NAME,
    G8EE_APP_CONTACT_URL,
    G8EE_APP_DESCRIPTION,
    G8EE_APP_LICENSE_NAME,
    G8EE_APP_LICENSE_URL,
    G8EE_APP_TITLE,
    ComponentName,
    G8eHeaders,
)
from .db.blob_service import BlobService
from .db.db_service import DBService
from .db.kv_service import KVService
from .logging import setup_logging
from .routers import chat_router, health_router
from .routers.internal_router import router as internal_router
from .services.cache.cache_aside import CacheAsideService
from .services.infra.settings_service import SettingsService
from .services.service_factory import ServiceFactory
from .llm.factory import set_settings
from .utils.service_init import initialize_g8e_service
from .utils.version import get_version

logger = logging.getLogger(__name__)


async def _connect_clients(settings):
    """Create and connect the 5 core g8es transport clients.

    Returns (db_client, kv_cache_client, pubsub_client, blob_client).
    HTTP client is created by ServiceFactory (InternalHttpClient).
    """
    ca = settings.ca_cert_path
    token = settings.auth.internal_auth_token

    db_client = DBClient(ca_cert_path=ca, internal_auth_token=token)
    await db_client.connect()

    kv_cache_client = KVCacheClient(
        component_name=ComponentName.G8EE, ca_cert_path=ca, internal_auth_token=token,
    )
    await kv_cache_client.connect()

    pubsub_client = PubSubClient(
        component_name=ComponentName.G8EE, ca_cert_path=ca, internal_auth_token=token,
    )
    await pubsub_client.connect()

    blob_client = BlobClient(ca_cert_path=ca, internal_auth_token=token)
    await blob_client.connect()

    return db_client, kv_cache_client, pubsub_client, blob_client


async def _close_client(client, label: str) -> None:
    """Best-effort close of a single transport client."""
    if client is None:
        return
    try:
        await client.close()
        logger.info("%s disconnected", label)
    except Exception as exc:
        logger.error("Error disconnecting %s: %s", label, exc)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Initialize application resources on startup and clean up on shutdown."""
    services: dict[str, object] = {}
    try:
        # -- Phase 0: Bootstrap settings --
        app.state.settings_service = SettingsService()
        initial_settings = app.state.settings_service.get_local_settings()
        settings = await initialize_g8e_service(
            "g8ee", settings=initial_settings,
            cache_aside_service=None, use_db_config=False,
        )
        app.state.settings = settings
        setup_logging(settings, component_name="g8ee")
        logger.info("Bootstrap settings loaded")

        # -- Phase 1: Core g8es clients (db, kv, pubsub, blob) --
        (
            app.state.db_client,
            app.state.kv_cache_client,
            app.state.pubsub_client,
            app.state.blob_client,
        ) = await _connect_clients(settings)
        logger.info("g8es transport clients connected (db, kv, pubsub, blob)")

        # -- Phase 2: Handler services (sole users of each client) --
        app.state.db_service = DBService(app.state.db_client)
        app.state.kv_service = KVService(app.state.kv_cache_client)
        app.state.blob_service = BlobService(app.state.blob_client)

        # -- Phase 3: CacheAsideService (orchestrator over DB + KV) --
        app.state.cache_aside_service = CacheAsideService(
            kv=app.state.kv_service,
            db=app.state.db_service,
            component_name=ComponentName.G8EE,
            default_ttl=settings.listen.default_ttl,
        )
        app.state.settings_service._cache_aside = app.state.cache_aside_service

        # -- Phase 4: Platform settings from g8es --
        settings = await app.state.settings_service.get_platform_settings()
        app.state.settings = settings
        set_settings(settings)
        logger.info("Platform settings merged: port=%s", settings.port)

        # -- Phase 5: All domain services (single factory call) --
        services = ServiceFactory.create_all_services(
            settings,
            app.state.cache_aside_service,
            pubsub_client=app.state.pubsub_client,
            blob_service=app.state.blob_service,
        )
        ServiceFactory.bind_to_app_state(app, services)
        logger.info("All domain services created and bound to app state")

        # -- Phase 5b: Reconcile g8ep operator API key mirror (split-brain guard) --
        from .services.auth.operator_key_reconciler import reconcile_g8ep_operator_key
        await reconcile_g8ep_operator_key(
            api_key_service=services.api_key_service,
            settings_service=app.state.settings_service,
        )

        # -- Phase 6: Lifecycle start --
        await ServiceFactory.start_services(services)
        logger.info("g8ee startup completed successfully")

        yield

    except Exception as exc:
        logger.critical("g8ee startup failed: %s", exc)
        raise

    finally:
        logger.info("=== g8ee SHUTDOWN INITIATED ===")

        from app.llm import clear_provider_cache
        await clear_provider_cache()

        await ServiceFactory.stop_services(services)

        await _close_client(getattr(app.state, "pubsub_client", None), "PubSub client")
        await _close_client(getattr(app.state, "kv_cache_client", None), "KV cache client")
        await _close_client(getattr(app.state, "blob_client", None), "Blob client")
        await _close_client(
            getattr(app.state, "internal_http_client", None), "g8ed HTTP client",
        )

        db_service = getattr(app.state, "db_service", None)
        if db_service is not None:
            try:
                await db_service.close()
                logger.info("g8es document service closed")
            except Exception as exc:
                logger.error("Error closing g8es document service: %s", exc)

        logger.info("g8ee shutdown complete")


def _build_app() -> FastAPI:
    """Construct the FastAPI application with CORS and routers."""
    application = FastAPI(
        title=G8EE_APP_TITLE,
        description=G8EE_APP_DESCRIPTION,
        version=get_version(),
        lifespan=lifespan,
        openapi_tags=[
            {"name": "health", "description": "Health checks and monitoring endpoints"},
            {"name": "investigations", "description": "Investigation management with shared models and troubleshooting framework"},
            {"name": "memories", "description": "Investigation memories for AI context and learning"},
        ],
        contact={
            "name": G8EE_APP_CONTACT_NAME,
            "url": G8EE_APP_CONTACT_URL,
            "email": G8EE_APP_CONTACT_EMAIL,
        },
        license_info={
            "name": G8EE_APP_LICENSE_NAME,
            "url": G8EE_APP_LICENSE_URL,
        },
    )

    application.add_middleware(
        CORSMiddleware,
        allow_origins=[
            CORS_ALLOWED_ORIGIN_G8EE,
            CORS_ALLOWED_ORIGIN_G8ED_HTTP,
            CORS_ALLOWED_ORIGIN_G8ED_HTTPS,
            CORS_ALLOWED_ORIGIN_LOCALHOST,
            CORS_ALLOWED_ORIGIN_G8E,
        ],
        allow_credentials=True,
        allow_methods=[
            HTTP_METHOD_GET, HTTP_METHOD_POST, HTTP_METHOD_PUT,
            HTTP_METHOD_DELETE, HTTP_METHOD_OPTIONS,
        ],
        allow_headers=[
            HTTP_ACCEPT_HEADER, HTTP_ACCEPT_LANGUAGE_HEADER,
            HTTP_CONTENT_LANGUAGE_HEADER, HTTP_CONTENT_TYPE_HEADER,
            HTTP_AUTHORIZATION_HEADER, HTTP_API_KEY_HEADER,
            HTTP_REQUESTED_WITH_HEADER,
            G8eHeaders.SERVICE, G8eHeaders.CLIENT, G8eHeaders.WEB_SESSION_ID,
            G8eHeaders.USER_ID, G8eHeaders.ORGANIZATION_ID, G8eHeaders.CASE_ID,
            G8eHeaders.INVESTIGATION_ID, G8eHeaders.TASK_ID,
            G8eHeaders.BOUND_OPERATORS, G8eHeaders.OPERATOR_STATUS,
            G8eHeaders.EXECUTION_ID, G8eHeaders.SOURCE_COMPONENT,
            HTTP_CACHE_CONTROL_HEADER, HTTP_PRAGMA_HEADER,
            HTTP_COOKIE_HEADER, HTTP_SET_COOKIE_HEADER,
            HTTP_LAST_EVENT_ID_HEADER,
            HTTP_ACCESS_CONTROL_REQUEST_HEADERS, HTTP_ACCESS_CONTROL_REQUEST_METHOD,
        ],
        expose_headers=[
            HTTP_SET_COOKIE_HEADER, HTTP_CONTENT_TYPE_HEADER,
            HTTP_CACHE_CONTROL_HEADER, HTTP_ACCESS_CONTROL_ALLOW_ORIGIN,
            HTTP_ACCESS_CONTROL_ALLOW_CREDENTIALS,
        ],
    )

    application.include_router(health_router)
    application.include_router(chat_router)
    application.include_router(internal_router)

    return application


app = _build_app()

if __name__ == "__main__":
    import uvicorn
    uvicorn.run("components.g8ee.app.main:app", host="0.0.0.0", port=443, reload=True)
