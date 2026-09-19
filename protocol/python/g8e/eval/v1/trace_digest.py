# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
import json
from typing import Any


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
    encoded = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    return (
        encoded.replace("<", "\\u003c")
        .replace(">", "\\u003e")
        .replace("&", "\\u0026")
        .encode()
    )


def compute_chat_probe_trace_digest(trace: dict[str, Any]) -> str:
    """Compute the SHA-256 digest for one chat-probe trace object."""
    payload = dict(trace)
    payload["trace_digest"] = ""
    return hashlib.sha256(marshal_canonical_json(payload)).hexdigest()
