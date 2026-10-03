# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
import json
import math
from decimal import Decimal
from typing import Any

_GO_PLAIN_FLOAT_MIN = 1e-6
_GO_PLAIN_FLOAT_MAX = 1e21


def _marshal_go_float(value: float) -> str:
    """Encode a float exactly as Go's encoding/json encodes a float64.

    The Go side decodes every JSON number to float64 and re-encodes it, so an
    integral float is written without a fractional part (``0.0`` -> ``0``) and
    the plain/exponent switch sits at 1e-6 and 1e21, not at Python's repr limits.
    """
    if not math.isfinite(value):
        raise ValueError("canonical JSON cannot encode a non-finite float")
    if value == 0:
        return "-0" if math.copysign(1.0, value) < 0 else "0"
    magnitude = abs(value)
    if magnitude < _GO_PLAIN_FLOAT_MIN or magnitude >= _GO_PLAIN_FLOAT_MAX:
        mantissa, _, exponent = repr(value).partition("e")
        if exponent.startswith("-0"):
            exponent = "-" + exponent[2:]
        return f"{mantissa}e{exponent}"
    return format(Decimal(repr(value)).normalize(), "f")


def marshal_canonical_json(value: Any) -> bytes:
    """Return deterministic JSON bytes matching g8e evaluation canonical_json.go."""
    if isinstance(value, dict):
        keys = sorted(value.keys())
        body = (
            "{"
            + ",".join(
                marshal_canonical_json(key).decode()
                + ":"
                + marshal_canonical_json(value[key]).decode()
                for key in keys
            )
            + "}"
        )
        return body.encode()
    if isinstance(value, list):
        body = "[" + ",".join(marshal_canonical_json(item).decode() for item in value) + "]"
        return body.encode()
    if isinstance(value, float):
        return _marshal_go_float(value).encode()
    encoded = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    return (
        encoded.replace("<", "\\u003c")
        .replace(">", "\\u003e")
        .replace("&", "\\u0026")
        .replace(" ", "\\u2028")
        .replace(" ", "\\u2029")
        .encode()
    )


def compute_chat_probe_trace_digest(trace: dict[str, Any]) -> str:
    """Compute the SHA-256 digest for one chat-probe trace object."""
    payload = dict(trace)
    payload["trace_digest"] = ""
    return hashlib.sha256(marshal_canonical_json(payload)).hexdigest()
