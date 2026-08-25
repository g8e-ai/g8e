# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Trust bundle resolution for evals clients.

The harness contacts the Operator over mTLS. The trust anchor is the hub
trust bundle that every other g8e client uses. Disabling TLS verification
defeats the receipt-binding guarantee the harness claims to measure, so
resolution is strict: an explicit path that does not exist raises.
"""

from __future__ import annotations

import os
from pathlib import Path

from g8e.paths import get_trust_bundle


def resolve_trust_bundle() -> str:
    """Return the path to the hub trust bundle.

    Resolution order:
      1. ``G8E_TRUST_BUNDLE`` (explicit override)
      2. ``${G8E_PKI_DIR:-.g8e/pki}/trust/hub-bundle.pem``

    Raises ``FileNotFoundError`` if no bundle is available. Callers should
    pass the returned path to ``httpx`` via ``verify=...``.
    """
    return get_trust_bundle()
