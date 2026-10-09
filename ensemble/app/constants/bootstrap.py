# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed bootstrap settings for the ensemble process.

Platform configuration (gateway URLs, runtime, PKI and secrets directories) is
not read from the environment (INV-ENV-04). Every field defaults to ``None``,
meaning "use the in-code default"; a deployment varies them only through the
explicit command arguments parsed by ``app.serve``, which calls
:func:`configure_bootstrap` once before the application is imported.
"""

import socket
from dataclasses import dataclass, replace
from urllib.parse import urlsplit, urlunsplit

from g8e.constants import NETWORK

GATEWAY_INTERNAL_HOSTNAME: str = NETWORK["network"]["GatewayInternalHostname"]["value"]
LOOPBACK_IP = "127.0.0.1"


@dataclass(frozen=True)
class BootstrapSettings:
    """Process-level platform configuration supplied by explicit launch arguments."""

    gateway_http_url: str | None = None
    gateway_url: str | None = None
    gateway_https_url: str | None = None
    gateway_pubsub_url: str | None = None
    runtime_dir: str | None = None
    pki_dir: str | None = None
    ca_cert_path: str | None = None


class _BootstrapState:
    """Mutable holder for the process bootstrap settings."""

    current: BootstrapSettings = BootstrapSettings()


def get_bootstrap() -> BootstrapSettings:
    """Return the bootstrap settings for this process."""
    return _BootstrapState.current


def gateway_dial_url(url: str | None) -> str | None:
    """Return ``url`` with an unresolvable internal Gateway hostname replaced by loopback.

    Mirrors Go ``config.GatewayDialHost``: a local Gateway binds loopback unless
    started with ``--listen-host``, and its certificate carries a ``127.0.0.1``
    IP SAN, so TLS verification still succeeds against the loopback address.
    """
    if not url:
        return url
    parts = urlsplit(url)
    if parts.hostname != GATEWAY_INTERNAL_HOSTNAME:
        return url
    try:
        socket.getaddrinfo(GATEWAY_INTERNAL_HOSTNAME, None)
        return url
    except socket.gaierror:
        netloc = LOOPBACK_IP if parts.port is None else f"{LOOPBACK_IP}:{parts.port}"
        return urlunsplit(parts._replace(netloc=netloc))


def resolve_gateway_dial_urls(settings: BootstrapSettings) -> BootstrapSettings:
    """Apply :func:`gateway_dial_url` to every Gateway URL in ``settings``."""
    return replace(
        settings,
        gateway_http_url=gateway_dial_url(settings.gateway_http_url),
        gateway_url=gateway_dial_url(settings.gateway_url),
        gateway_https_url=gateway_dial_url(settings.gateway_https_url),
        gateway_pubsub_url=gateway_dial_url(settings.gateway_pubsub_url),
    )


def configure_bootstrap(settings: BootstrapSettings) -> None:
    """Install the bootstrap settings.

    ``app.constants.paths`` caches resolved paths per bootstrap settings
    instance, so installing new settings re-resolves paths on next access.
    """
    _BootstrapState.current = settings


def reset_bootstrap() -> None:
    """Restore the in-code defaults. Used by tests for isolation."""
    configure_bootstrap(BootstrapSettings())
