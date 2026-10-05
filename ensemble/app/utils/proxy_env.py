# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Normalize ambient proxy environment variables for third-party HTTP clients.

Several HTTP client libraries read ``HTTP_PROXY`` / ``NO_PROXY``-style
variables from the environment at import or client-construction time. httpx
(used transitively by the ``ollama`` package, which builds a client at module
import time) cannot parse *bracketed* IPv6 literals in ``NO_PROXY`` /
``no_proxy`` (e.g. ``[::1]``): its ``is_ipv6_hostname`` check rejects the
brackets, the entry falls through to a wildcard pattern, and URL parsing
raises ``httpx.InvalidURL: Invalid port: ...`` — crashing the ensemble before
any g8e code runs. Bare (unbracketed) IPv6 literals parse fine.

This module rewrites only the broken form — bracketed IPv6 literals — back to
the bare form httpx understands. Matching semantics are preserved: httpx
re-brackets bare IPv6 itself when building its no-proxy patterns. Everything
else (hostnames, IPv4, CIDR ranges, entries with ports or schemes) is left
untouched.

This is intended to run at ``app`` package import time (see
``app/__init__.py``) so it takes effect before any third-party import can read
the raw values. These are ambient OS/network settings, not g8e platform
configuration.
"""

from __future__ import annotations

import ipaddress
import os
from collections.abc import MutableMapping

_NO_PROXY_KEYS = ("NO_PROXY", "no_proxy")


def _normalize_entry(entry: str) -> str:
    """Strip brackets from a bracketed IPv6 literal; leave anything else alone."""
    text = entry.strip()
    if text.startswith("[") and text.endswith("]") and ":" in text:
        inner = text[1:-1]
        try:
            ipaddress.IPv6Address(inner)
        except ValueError:
            return entry
        return inner
    return entry


def sanitize_proxy_env(env: MutableMapping[str, str] | None = None) -> bool:
    """Rewrite bracketed IPv6 literals in NO_PROXY/no_proxy to the bare form.

    Returns True when any variable was changed. Takes an optional mapping for
    testability; defaults to ``os.environ``. Idempotent.
    """
    if env is None:
        env = os.environ
    changed = False
    for key in _NO_PROXY_KEYS:
        value = env.get(key)
        if not value:
            continue
        normalized = ",".join(_normalize_entry(part) for part in value.split(","))
        if normalized != value:
            env[key] = normalized
            changed = True
    return changed
