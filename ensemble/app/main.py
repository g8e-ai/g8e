# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""g8ee FastAPI Application - Main Entry Point.

g8e-Compliant Agentic Ensemble (g8ee) - Reference AI reasoning system for g8e platform.
Agentic Ensemble with LLM provider abstraction providing Zero-Trust AI for infrastructure operations.

Bootstrap responsibilities (this file):
    1. SettingsService bootstrap + local settings
    2. Raw operator client connections (5 core clients: DB, KV, PubSub, Blob, HTTP)
    3. Handler services (sole users of each client): DBService, KVService, BlobService
    4. CacheAsideService (orchestrator over DB + KV handler services)
    5. Platform settings load from operator
    6. Delegate ALL domain service construction to ServiceFactory
    7. Service lifecycle start / stop
    8. FastAPI app creation, CORS, router registration

    HTTP client is created and managed by ServiceFactory (HTTPService + InternalHttpClient).
"""

import logging
import os
from typing import cast
from contextlib import asynccontextmanager

from dotenv import load_dotenv
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

load_dotenv(override=False)

from .clients.blob_client import BlobClient
from .clients.db_client import DBClient
from .clients.governance_client import GovernanceClient
from .clients.kv_cache_client import KVCacheClient
from .clients.pubsub_client import PubSubClient
from .constants import (
    AUTHORIZATION,
    CORS_ALLOWED_ORIGIN_G8EE,
    CORS_ALLOWED_ORIGIN_LOCALHOST,
    CORS_ALLOWED_ORIGIN_CLIENT_HTTP,
    CORS_ALLOWED_ORIGIN_CLIENT_HTTPS,
    ACCEPT,
    ACCEPT_LANGUAGE,
    ACCESS_CONTROL_ALLOW_CREDENTIALS,
    ACCESS_CONTROL_ALLOW_ORIGIN,
    ACCESS_CONTROL_REQUEST_HEADERS,
    ACCESS_CONTROL_REQUEST_METHOD,
    CACHE_CONTROL,
    CONTENT_LANGUAGE,
    CONTENT_TYPE,
    COOKIE,
    EXECUTION_ID,
    LAST_EVENT_ID,
    HTTP_METHOD_DELETE,
    HTTP_METHOD_GET,
    HTTP_METHOD_OPTIONS,
    HTTP_METHOD_POST,
    HTTP_METHOD_PUT,
    PRAGMA,
    REQUESTED_WITH,
    SET_COOKIE,
    G8EE_APP_CONTACT_EMAIL,
    G8EE_APP_CONTACT_NAME,
    G8EE_APP_CONTACT_URL,
    G8EE_APP_DESCRIPTION,
    G8EE_APP_LICENSE_NAME,
    G8EE_APP_LICENSE_URL,
    G8EE_APP_TITLE,
)
from .constants.generated_paths import PathConstants, PortConstants
from .constants.env_vars import EnvVar
from .models.state import G8eeAppState
from .models.settings import TLSConfig
from .db.blob_service import BlobService
from .db.db_service import DBService
from .db.kv_service import KVService
from .logging import setup_logging
from .routers import chat_router, health_router
from .routers.internal_router import router as internal_router
from .middleware.exception_handlers import setup_exception_handlers
from .middleware.http_context import G8eHttpContextMiddleware
from .services.cache.cache_aside import CacheAsideService
from .errors import ConfigurationError
from .services.infra.app_enrollment_service import AppEnrollmentService
from .services.infra.settings_service import SettingsService
from .services.service_factory import ServiceFactory
from .llm.factory import set_settings
from .utils.service_init import initialize_g8e_service
from .utils.version import get_version
from .llm import clear_provider_cache
from app.constants import G8EE_COMPONENT

logger = logging.getLogger(__name__)


async def _connect_clients(settings, tls_config):
    """Create and connect the 5 core operator transport clients.

    Returns (db_client, kv_cache_client, pubsub_client, blob_client).
    HTTP client is created by ServiceFactory (InternalHttpClient).
    """
    auditor_hmac_key = settings.auth.auditor_hmac_key

    db_client = DBClient(tls_config=tls_config)
    await db_client.connect()

    kv_cache_client = KVCacheClient(
        component_name=G8EE_COMPONENT,
        tls_config=tls_config,
    )
    await kv_cache_client.connect()

    pubsub_client = PubSubClient(
        component_name=G8EE_COMPONENT,
        tls_config=tls_config,
        auditor_hmac_key=auditor_hmac_key,
    )
    await pubsub_client.connect()

    blob_client = BlobClient(tls_config=tls_config)
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
    state = cast(G8eeAppState, app.state)
    all_services = None
    try:
        # -- Phase 0: Bootstrap settings --
        settings_service = SettingsService()
        initial_settings = settings_service.get_local_settings()
        settings = await initialize_g8e_service(
            "g8ee",
            settings=initial_settings,
            cache_aside_service=None,
            use_db_config=False,
        )
        state.settings = settings
        setup_logging(settings, component_name="g8ee")
        logger.info("Bootstrap settings loaded")

        # -- Phase 0.25: Resolve app identity with the gateway --
        # The ensemble authenticates to the gateway exclusively via its mTLS
        # app cert. Try to load an existing valid cert from disk first; if
        # none is available (missing, expired, or near-expiry), enroll with
        # the gateway to obtain a fresh one. This runs before the operator
        # clients connect so the TLS config below points at the ensemble's
        # own enrolled credentials.
        enrollment_service = AppEnrollmentService()
        try:
            app_identity = enrollment_service.load_identity()
        except ConfigurationError:
            app_identity = await enrollment_service.enroll()
        logger.info(
            "App identity ready (app_id=%s, cert=%s)",
            app_identity.app_id,
            app_identity.cert_path,
        )

        # -- Phase 0.5: Create TLS config for all clients --
        tls_config = TLSConfig(
            ca_cert_path=app_identity.ca_cert_path,
            client_cert_path=app_identity.cert_path,
            client_key_path=app_identity.key_path,
        )

        # -- Phase 1: Core operator clients (db, kv, pubsub, blob) --
        (
            state.db_client,
            state.kv_cache_client,
            state.pubsub_client,
            state.blob_client,
        ) = await _connect_clients(settings, tls_config)
        logger.info("operator transport clients connected (db, kv, pubsub, blob)")

        # -- Phase 2: Handler services (sole users of each client) --
        db_service = DBService(state.db_client)
        kv_service = KVService(state.kv_cache_client)
        blob_service = BlobService(state.blob_client)

        # -- Phase 3: CacheAsideService (orchestrator over DB + KV) --
        cache_aside_service = CacheAsideService(
            kv=kv_service,
            db=db_service,
            component_name=G8EE_COMPONENT,
            default_ttl=settings.gateway.default_ttl,
            read_enabled=settings.gateway.enable_cache_read,
        )
        settings_service._cache_aside = cache_aside_service

        # -- Phase 4: Platform settings from operator --
        settings = await settings_service.get_app_settings()
        state.settings = settings
        set_settings(settings)
        logger.info("Platform settings merged: port=%s", settings.port)

        # -- Phase 4.5: GovernanceClient for governed collection writes --
        # The gateway's PrivilegedRouteRegistry blocks app certificates from
        # the governance envelope endpoint. The ensemble must use the
        # operator's mTLS cert (whose SPIFFE URI carries the operator session
        # ID) to submit governance envelopes. The operator cert is shared via
        # a read-only volume mount (see docker-compose.yml). When the env vars
        # are not set, fall back to the app tls_config (which will fail-closed
        # at the gateway with ErrPrivilegedEndpointAccess).
        gov_operator_cert = os.environ.get(EnvVar.GOVERNANCE_OPERATOR_CERT)
        gov_operator_key = os.environ.get(EnvVar.GOVERNANCE_OPERATOR_KEY)
        if gov_operator_cert and gov_operator_key:
            governance_tls_config = TLSConfig(
                ca_cert_path=app_identity.ca_cert_path,
                client_cert_path=gov_operator_cert,
                client_key_path=gov_operator_key,
            )
            logger.info(
                "GovernanceClient using operator mTLS cert for governance "
                "submissions (cert=%s)",
                gov_operator_cert,
            )
        else:
            governance_tls_config = tls_config
            logger.warning(
                "GovernanceClient falling back to app cert for governance "
                "submissions — gateway will reject with ErrPrivilegedEndpointAccess "
                "unless G8E_GOVERNANCE_OPERATOR_CERT/G8E_GOVERNANCE_OPERATOR_KEY "
                "are set"
            )
        governance_client = GovernanceClient(
            tls_config=governance_tls_config,
            operator_session_id=settings.auth.operator_session_id,
            gateway_settings=settings.gateway,
        )

        # -- Phase 5: All domain services (single factory call) --
        all_services = ServiceFactory.create_all_services(
            settings,
            cache_aside_service,
            db_service=db_service,
            kv_service=kv_service,
            blob_service=blob_service,
            pubsub_client=state.pubsub_client,
            blob_service_client=state.blob_client,
            governance_client=governance_client,
        )
        ServiceFactory.bind_to_app_state(app, all_services)
        logger.info("All domain services created and bound to app state")

        # -- Phase 6: Lifecycle start --
        await ServiceFactory.start_services(all_services)
        logger.info("g8ee startup completed successfully")

        yield

    except Exception as exc:
        logger.critical("g8ee startup failed: %s", exc)
        raise

    finally:
        logger.info("=== g8ee SHUTDOWN INITIATED ===")

        await clear_provider_cache()

        if all_services:
            await ServiceFactory.stop_services(all_services)

        await _close_client(getattr(state, "pubsub_client", None), "PubSub client")
        await _close_client(getattr(state, "kv_cache_client", None), "KV cache client")
        await _close_client(getattr(state, "blob_client", None), "Blob client")
        await _close_client(
            getattr(state, "internal_http_client", None),
            "client HTTP client",
        )

        services = getattr(state, "services", None)
        db_service = getattr(services, "db_service", None) if services else None
        if db_service is not None:
            try:
                await db_service.close()
                logger.info("operator document service closed")
            except Exception as exc:
                logger.error("Error closing operator document service: %s", exc)

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
            {
                "name": "investigations",
                "description": "Investigation management with protocol models and troubleshooting framework",
            },
            {
                "name": "memories",
                "description": "Investigation memories for AI context and learning",
            },
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

    setup_exception_handlers(application)

    application.add_middleware(G8eHttpContextMiddleware)

    application.add_middleware(
        CORSMiddleware,
        allow_origins=[
            CORS_ALLOWED_ORIGIN_G8EE,
            CORS_ALLOWED_ORIGIN_CLIENT_HTTP,
            CORS_ALLOWED_ORIGIN_CLIENT_HTTPS,
            CORS_ALLOWED_ORIGIN_LOCALHOST,
        ],
        allow_credentials=True,
        allow_methods=[
            HTTP_METHOD_GET,
            HTTP_METHOD_POST,
            HTTP_METHOD_PUT,
            HTTP_METHOD_DELETE,
            HTTP_METHOD_OPTIONS,
        ],
        allow_headers=[
            ACCEPT,
            ACCEPT_LANGUAGE,
            CONTENT_LANGUAGE,
            CONTENT_TYPE,
            AUTHORIZATION,
            REQUESTED_WITH,
            EXECUTION_ID,
            CACHE_CONTROL,
            PRAGMA,
            COOKIE,
            SET_COOKIE,
            LAST_EVENT_ID,
            ACCESS_CONTROL_REQUEST_HEADERS,
            ACCESS_CONTROL_REQUEST_METHOD,
        ],
        expose_headers=[
            SET_COOKIE,
            CONTENT_TYPE,
            CACHE_CONTROL,
            ACCESS_CONTROL_ALLOW_ORIGIN,
            ACCESS_CONTROL_ALLOW_CREDENTIALS,
        ],
    )

    application.include_router(health_router)
    application.include_router(chat_router)
    application.include_router(internal_router)

    return application


app = _build_app()

if __name__ == "__main__":
    import uvicorn

    uvicorn.run("app.main:app", host="0.0.0.0", port=PortConstants.G8E_PORT_G8EE_HTTPS, reload=True)
