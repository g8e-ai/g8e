# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import json

from google.protobuf import json_format
from google.protobuf.message import Message


def serialize_canonical(message: Message) -> bytes:
    encoded = json_format.MessageToJson(
        message,
        preserving_proto_field_name=True,
        indent=None,
        sort_keys=False,
        ensure_ascii=True,
    )
    value = json.loads(encoded)
    return json.dumps(value, ensure_ascii=True, separators=(",", ":")).encode()


def parse_canonical(encoded: bytes, message: Message) -> Message:
    json_format.Parse(encoded.decode(), message, ignore_unknown_fields=False)
    if serialize_canonical(message) != encoded:
        raise ValueError("compliance protocol: input is not canonical protojson")
    return message
