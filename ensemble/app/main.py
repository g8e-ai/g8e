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
    2. Raw Gateway client connections (DB, Blob, HTTP)
    3. Handler services (sole users of each client): DocumentService, BlobService
    5. Platform settings load from operator
    6. Delegate ALL domain service construction to ServiceFactory
    7. Service lifecycle start / stop
    8. FastAPI app creation, CORS, router registration

    HTTP client is created and managed by ServiceFactory (HTTPService + InternalHttpClient).
"""

import logging
from contextlib import asynccontextmanager
from typing import cast

from dotenv import load_dotenv
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from .clients.blob_client import BlobClient
from .clients.db_client import DBClient
from .clients.governance_client import GovernanceClient
from .constants import (
    ACCEPT,
    ACCEPT_LANGUAGE,
    ACCESS_CONTROL_ALLOW_CREDENTIALS,
    ACCESS_CONTROL_ALLOW_ORIGIN,
    ACCESS_CONTROL_REQUEST_HEADERS,
    ACCESS_CONTROL_REQUEST_METHOD,
    AUTHORIZATION,
    CACHE_CONTROL,
    CONTENT_LANGUAGE,
    CONTENT_TYPE,
    COOKIE,
    CORS_ALLOWED_ORIGIN_CLIENT_HTTP,
    CORS_ALLOWED_ORIGIN_CLIENT_HTTPS,
    CORS_ALLOWED_ORIGIN_G8EE,
    CORS_ALLOWED_ORIGIN_LOCALHOST,
    EXECUTION_ID,
    G8EE_APP_CONTACT_EMAIL,
    G8EE_APP_CONTACT_NAME,
    G8EE_APP_CONTACT_URL,
    G8EE_APP_DESCRIPTION,
    G8EE_APP_LICENSE_NAME,
    G8EE_APP_LICENSE_URL,
    G8EE_APP_TITLE,
    G8EE_COMPONENT,
    HTTP_METHOD_DELETE,
    HTTP_METHOD_GET,
    HTTP_METHOD_OPTIONS,
    HTTP_METHOD_POST,
    HTTP_METHOD_PUT,
    LAST_EVENT_ID,
    PRAGMA,
    REQUESTED_WITH,
    SET_COOKIE,
)
from .constants.generated_paths import PortConstants
from .db.blob_service import BlobService
from .db.document_service import DocumentService
from .decision.validation import (
    log_jev_generative_lite_warning,
    validate_jev_lite_coexistence,
)
from .errors import ConfigurationError
from .llm import clear_provider_cache
from .llm.factory import set_internal_http_client, set_settings
from .logging import setup_logging
from .middleware.exception_handlers import setup_exception_handlers
from .middleware.http_context import G8eHttpContextMiddleware
from .models.settings import G8eeAppSettings, TLSConfig
from .models.state import G8eeAppState
from .routers import chat_router, health_router
from .routers.internal_router import router as internal_router
from .services.infra.app_enrollment_service import AppEnrollmentService, AppIdentity
from .services.infra.settings_service import SettingsService
from .services.service_factory import AllServices, ServiceFactory
from .utils.service_init import initialize_g8e_service
from .utils.version import get_version

load_dotenv(override=False)

logger = logging.getLogger(__name__)


async def _connect_clients(tls_config: TLSConfig):
    """Create and connect the core operator transport clients.

    Returns (db_client, blob_client).
    HTTP client is created by ServiceFactory (InternalHttpClient).
    """
    db_client = DBClient(tls_config=tls_config)
    await db_client.connect()

    blob_client = BlobClient(tls_config=tls_config)
    await blob_client.connect()

    return db_client, blob_client


async def _close_client(client, label: str) -> None:
    """Best-effort close of a single transport client."""
    if client is None:
        return
    try:
        await client.close()
        logger.info("%s disconnected", label)
    except Exception as exc:
        logger.error("Error disconnecting %s: %s", label, exc)


async def _resolve_app_identity(enrollment_service: AppEnrollmentService) -> AppIdentity:
    """Load the ensemble's app identity from disk, enrolling with the gateway if needed."""
    try:
        return enrollment_service.load_identity()
    except ConfigurationError as exc:
        # If cert and key exist but CA bundle is missing, pull bundle via HTTP like the operator does
        if "gateway CA bundle not found" in str(exc):
            try:
                await enrollment_service.fetch_ca_bundle()
                return enrollment_service.load_identity()
            except Exception:
                return await enrollment_service.enroll()
        return await enrollment_service.enroll()


def _validate_jev_lite(settings: G8eeAppSettings) -> None:
    log_jev_generative_lite_warning(logger, settings.llm)
    jev_startup_errors = validate_jev_lite_coexistence(settings.llm)
    if jev_startup_errors:
        raise ConfigurationError(
            "Jev lite provider configuration is incompatible with enabled features: "
            + " ".join(jev_startup_errors)
        )


async def _shutdown(state: G8eeAppState, all_services: AllServices | None) -> None:
    await clear_provider_cache()

    if all_services:
        await ServiceFactory.stop_services(all_services)

    await _close_client(getattr(state, "blob_client", None), "Blob client")
    await _close_client(
        getattr(state, "internal_http_client", None),
        "client HTTP client",
    )

    services = getattr(state, "services", None)
    document_service = getattr(services, "document_service", None) if services else None
    if document_service is not None:
        try:
            await document_service.close()
            logger.info("operator document service closed")
        except Exception as exc:
            logger.error("Error closing operator document service: %s", exc)


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
            document_service=None,
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
        app_identity = await _resolve_app_identity(AppEnrollmentService())
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

        # -- Phase 1: Core operator clients (db, blob) --
        (
            state.db_client,
            state.blob_client,
        ) = await _connect_clients(tls_config)
        logger.info("operator transport clients connected (db, blob)")

        # -- Phase 2: Handler services (sole users of each client) --
        document_service = DocumentService(state.db_client, component_name=G8EE_COMPONENT)
        blob_service = BlobService(state.blob_client)
        settings_service.attach_document_service(document_service)

        # -- Phase 4: Platform settings from operator --
        settings = await settings_service.get_app_settings()
        state.settings = settings
        set_settings(settings)
        logger.info("Platform settings merged: port=%s", settings.port)

        _validate_jev_lite(settings)

        # -- Phase 4.5: GovernanceClient for governed collection writes --
        governance_client = GovernanceClient(
            tls_config=tls_config,
            operator_session_id=settings.auth.operator_session_id,
            gateway_settings=settings.gateway,
        )

        # -- Phase 5: All domain services (single factory call) --
        all_services = ServiceFactory.create_all_services(
            settings,
            document_service,
            blob_service=blob_service,
            blob_service_client=state.blob_client,
            governance_client=governance_client,
        )
        ServiceFactory.bind_to_app_state(app, all_services)
        logger.info("All domain services created and bound to app state")

        # Inject the InternalHttpClient singleton into the LLM provider
        # factory so the G8E governed-dispatch provider can route inference
        # through the gateway's /api/v1/inference/dispatch endpoint. The
        # client is owned by the application lifecycle; the factory does
        # not own it.
        set_internal_http_client(all_services.internal_http_client)
        logger.info("InternalHttpClient injected into LLM provider factory")

        # -- Phase 6: Lifecycle start --
        await ServiceFactory.start_services(all_services)
        logger.info("g8ee startup completed successfully")

        yield

    except Exception as exc:
        logger.critical("g8ee startup failed: %s", exc)
        raise

    finally:
        logger.info("=== g8ee SHUTDOWN INITIATED ===")

        await _shutdown(state, all_services)

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
