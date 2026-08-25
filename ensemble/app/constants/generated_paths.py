# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Path and port constants from g8e protocol.

This module provides PathConstants and PortConstants by loading from the
installed g8e package (from vendored submodule).
"""

import json
from pathlib import Path

from g8e.constants import _PROTOCOL_CONSTANTS_DIR


def _load_ports_json() -> dict:
    """Load ports.json from protocol constants."""
    ports_file = _PROTOCOL_CONSTANTS_DIR / "ports.json"
    if not ports_file.exists():
        return {}
    with ports_file.open() as f:
        return json.load(f)


_PORTS_DATA = _load_ports_json()


class PathConstants:
    """File system path constants."""

    PATH_DOCS_DIR = "docs"


class PortConstants:
    """Network port constants from g8e protocol.

    The ports.json format uses nested objects with ``value`` keys:
    ``{"OperatorHttp": {"value": 8080, ...}}``.
    """

    # Gateway/operator ports from protocol (v1.2.2 nested format)
    PORT_OPERATOR_HTTP = _PORTS_DATA.get("ports", {}).get("OperatorHttp", {}).get("value", 8080)
    PORT_OPERATOR_HTTPS = _PORTS_DATA.get("ports", {}).get("OperatorHttps", {}).get("value", 8443)

    # g8ee application uses operator HTTPS port
    G8E_PORT_G8EE_HTTPS = PORT_OPERATOR_HTTPS
