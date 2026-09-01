# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
import json
from importlib import resources
from pathlib import Path

import pytest
from google.protobuf import json_format

from g8e.compliance.v1 import compliance_pb2
from g8e.compliance.v1.canonical import parse_canonical, serialize_canonical
from g8e.compliance.v1.compliance_pb2 import ControlAssertionDefinition

VECTORS_DIRECTORY_NAME = "vectors"
COMPLIANCE_DIRECTORY_NAME = "compliance"
VECTOR_FILENAME = "control_assertion_definition.json"
PHASE1_VECTOR_FILENAME = "phase1_records.json"
PROTOCOL_ROOT = Path(__file__).resolve().parents[2]
VECTOR_PATH = PROTOCOL_ROOT / VECTORS_DIRECTORY_NAME / COMPLIANCE_DIRECTORY_NAME / VECTOR_FILENAME
PHASE1_VECTOR_PATH = PROTOCOL_ROOT / VECTORS_DIRECTORY_NAME / COMPLIANCE_DIRECTORY_NAME / PHASE1_VECTOR_FILENAME
FRAMEWORK_CATALOG_FILENAME = "framework-catalog.json"
CATALOG_FILENAMES = ("assertion-catalog.json", FRAMEWORK_CATALOG_FILENAME, "fedramp-nist-crosswalk.json")
COMPLIANCE_PATHS_FILENAME = "compliance_paths.json"
PHASE1_MESSAGE_COUNT = 20


def catalog_digest(record: dict[str, object], digest_field: str) -> tuple[str, str]:
    candidate = dict(record)
    expected = str(candidate.pop(digest_field))
    encoded = json.dumps(candidate, ensure_ascii=True, separators=(",", ":")).encode()
    return expected, hashlib.sha256(encoded).hexdigest()


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


def test_compliance_phase1_records_match_cross_language_vectors():
    vector_set = json.loads(PHASE1_VECTOR_PATH.read_text())
    assert len(vector_set["vectors"]) == PHASE1_MESSAGE_COUNT
    assert len({vector["message_type"] for vector in vector_set["vectors"]}) == PHASE1_MESSAGE_COUNT
    for vector in vector_set["vectors"]:
        message = getattr(compliance_pb2, vector["message_type"])()
        encoded = vector["canonical_json"].encode()
        assert parse_canonical(encoded, message) == message
        assert serialize_canonical(message) == encoded


def test_packaged_compliance_catalogs_match_go_source_bytes_and_digests():
    source_directory = PROTOCOL_ROOT / "constants" / COMPLIANCE_DIRECTORY_NAME
    packaged_directory = resources.files("g8e").joinpath("_data", COMPLIANCE_DIRECTORY_NAME)
    for filename in CATALOG_FILENAMES:
        source = (source_directory / filename).read_bytes()
        packaged = packaged_directory.joinpath(filename).read_bytes()
        assert packaged == source
        source_record = json.loads(source)
        expected, actual = catalog_digest(source_record, "sha256")
        assert actual == expected
        if filename == FRAMEWORK_CATALOG_FILENAME:
            for framework in source_record["frameworks"]:
                expected, actual = catalog_digest(framework, "catalog_sha256")
                assert actual == expected

    source_paths = (PROTOCOL_ROOT / "constants" / COMPLIANCE_PATHS_FILENAME).read_bytes()
    packaged_paths = resources.files("g8e").joinpath("_data", COMPLIANCE_PATHS_FILENAME).read_bytes()
    assert packaged_paths == source_paths
