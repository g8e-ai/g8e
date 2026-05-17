"""Trust bundle resolution for evals clients.

The harness contacts the Operator over mTLS. The trust anchor is the hub
trust bundle that every other g8e client uses. Disabling TLS verification
defeats the receipt-binding guarantee the harness claims to measure, so
resolution is strict: an explicit path that does not exist raises.
"""

from __future__ import annotations

import os
from pathlib import Path


def resolve_trust_bundle() -> str:
    """Return the path to the hub trust bundle.

    Resolution order:
      1. ``G8E_TRUST_BUNDLE`` (explicit override)
      2. ``${G8E_PKI_DIR:-.g8e/pki}/trust/hub-bundle.pem``

    Raises ``FileNotFoundError`` if no bundle is available. Callers should
    pass the returned path to ``httpx`` via ``verify=...``.
    """
    explicit = os.environ.get("G8E_TRUST_BUNDLE", "").strip()
    if explicit:
        path = Path(explicit)
        if not path.exists():
            raise FileNotFoundError(
                f"G8E_TRUST_BUNDLE points to a missing file: {path}"
            )
        return str(path)

    pki_dir = Path(os.environ.get("G8E_PKI_DIR", ".g8e/pki"))
    path = pki_dir / "trust" / "hub-bundle.pem"
    if not path.exists():
        raise FileNotFoundError(
            "Hub trust bundle not found. Run `./g8e platform start` to "
            f"provision PKI, set G8E_TRUST_BUNDLE, or G8E_PKI_DIR. Looked at: {path}"
        )
    return str(path)
