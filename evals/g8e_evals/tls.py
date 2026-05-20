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

"""Trust bundle resolution for evals clients.

The harness contacts the Operator over mTLS. The trust anchor is the hub
trust bundle that every other g8e client uses. Disabling TLS verification
defeats the receipt-binding guarantee the harness claims to measure, so
resolution is strict: an explicit path that does not exist raises.
"""

from __future__ import annotations

import os
from pathlib import Path

from g8e_protocol.paths import get_trust_bundle


def resolve_trust_bundle() -> str:
    """Return the path to the hub trust bundle.

    Resolution order:
      1. ``G8E_TRUST_BUNDLE`` (explicit override)
      2. ``${G8E_PKI_DIR:-.g8e/pki}/trust/hub-bundle.pem``

    Raises ``FileNotFoundError`` if no bundle is available. Callers should
    pass the returned path to ``httpx`` via ``verify=...``.
    """
    return get_trust_bundle()
