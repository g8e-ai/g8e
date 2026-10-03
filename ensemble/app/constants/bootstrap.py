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

from dataclasses import dataclass


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


_current = BootstrapSettings()


def get_bootstrap() -> BootstrapSettings:
    """Return the bootstrap settings for this process."""
    return _current


def configure_bootstrap(settings: BootstrapSettings) -> None:
    """Install the bootstrap settings and re-resolve the cached paths."""
    global _current
    _current = settings
    _reload_paths()


def reset_bootstrap() -> None:
    """Restore the in-code defaults. Used by tests for isolation."""
    configure_bootstrap(BootstrapSettings())


def _reload_paths() -> None:
    # Imported lazily: app.constants.paths reads these settings at resolution time.
    from app.constants.paths import reload_paths

    reload_paths()
