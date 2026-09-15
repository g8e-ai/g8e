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
from google.protobuf.json_format import ParseError

from g8e.compliance.v1.canonical import parse_canonical, serialize_canonical
from g8e.eval.v1.eval_pb2 import EvaluationReport

VECTORS_DIRECTORY_NAME = "vectors"
EVALUATION_DIRECTORY_NAME = "eval"
PHASE1_REPORT_VECTOR_FILENAME = "phase1_report.json"
PROTOCOL_ROOT = Path(__file__).resolve().parents[2]
PHASE1_REPORT_VECTOR_PATH = (
    PROTOCOL_ROOT / VECTORS_DIRECTORY_NAME / EVALUATION_DIRECTORY_NAME / PHASE1_REPORT_VECTOR_FILENAME
)


def test_evaluation_report_canonicalization_matches_cross_language_vector():
    vector = json.loads(PHASE1_REPORT_VECTOR_PATH.read_text())
    encoded = vector["canonical_json"].encode()
    report = parse_canonical(encoded, EvaluationReport())

    assert vector["message_type"] == "EvaluationReport"
    assert serialize_canonical(report) == encoded
    assert report.run.suite_ref.id == "core-execution-boundary"
    assert report.run.deployment.topology_ref.id == "unified-compose-remote-operator"
    assert report.run.target_operator_session_id == "session-1"


def test_evaluation_report_canonical_parser_rejects_unknown_fields():
    with pytest.raises(ParseError):
        parse_canonical(b'{"schema_version":"1.0.0","unknown":true}', EvaluationReport())
