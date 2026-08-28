# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Trust bundle resolution for eval clients with explicit runtime identities."""

from __future__ import annotations

import os
from enum import StrEnum
from pathlib import Path


APP_TRUST_BUNDLE_NAME = "hub-bundle.pem"
GATEWAY_TRUST_BUNDLE_NAME = "g8eg-ca-bundle.pem"
TRUST_DIRECTORY_NAME = "trust"
DEFAULT_PKI_DIRECTORY = ".g8e/pki"


class RuntimeIdentity(StrEnum):
    APP = "app"
    GATEWAY = "gateway"


def resolve_trust_bundle(identity: RuntimeIdentity) -> str:
    """Resolve and validate the trust bundle for an app or gateway client."""
    if identity is RuntimeIdentity.APP:
        explicit = os.environ.get("G8E_APP_TRUST_BUNDLE", "").strip()
        pki_dir = os.environ.get("G8E_APP_PKI_DIR", DEFAULT_PKI_DIRECTORY)
        filename = APP_TRUST_BUNDLE_NAME
    else:
        explicit = os.environ.get("G8E_GATEWAY_TRUST_BUNDLE", "").strip()
        pki_dir = os.environ.get("G8E_GATEWAY_PKI_DIR", DEFAULT_PKI_DIRECTORY)
        filename = GATEWAY_TRUST_BUNDLE_NAME
    bundle = Path(explicit) if explicit else Path(pki_dir) / TRUST_DIRECTORY_NAME / filename

    if not bundle.is_file():
        raise FileNotFoundError(f"{identity.value} trust bundle not found: {bundle}")
    return str(bundle)
