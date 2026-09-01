# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import json
from pathlib import Path

import pytest
from google.protobuf import json_format

from g8e.compliance.v1.canonical import parse_canonical, serialize_canonical
from g8e.compliance.v1.compliance_pb2 import ControlAssertionDefinition

VECTORS_DIRECTORY_NAME = "vectors"
COMPLIANCE_DIRECTORY_NAME = "compliance"
VECTOR_FILENAME = "control_assertion_definition.json"
VECTOR_PATH = Path(__file__).resolve().parents[2] / VECTORS_DIRECTORY_NAME / COMPLIANCE_DIRECTORY_NAME / VECTOR_FILENAME


@pytest.fixture
def vector() -> dict[str, object]:
    return json.loads(VECTOR_PATH.read_text())


def test_compliance_canonicalization_matches_cross_language_vector(vector):
    message = ControlAssertionDefinition()
    json_format.ParseDict(vector["message"], message, ignore_unknown_fields=False)
    encoded = serialize_canonical(message)
    assert encoded.decode() == vector["canonical_json"]
    assert parse_canonical(encoded, ControlAssertionDefinition()) == message


def test_compliance_canonical_parser_rejects_noncanonical_json(vector):
    encoded = json.dumps(vector["message"], indent=2).encode()
    with pytest.raises(ValueError, match="not canonical"):
        parse_canonical(encoded, ControlAssertionDefinition())
