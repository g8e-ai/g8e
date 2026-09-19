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
from g8e.eval.v1.trace_digest import compute_chat_probe_trace_digest
from g8e.eval.v1 import eval_pb2
from g8e.eval.v1.eval_pb2 import (
    EvaluationAssignmentResult,
    EvaluationCampaignSpec,
    EvaluationReport,
    PublicAssignmentResultProjection,
)

VECTORS_DIRECTORY_NAME = "vectors"
EVALUATION_DIRECTORY_NAME = "eval"
PHASE1_REPORT_VECTOR_FILENAME = "phase1_report.json"
MODEL_CAMPAIGN_SPEC_VECTOR_FILENAME = "model_campaign_spec.json"
MODEL_ASSIGNMENT_RESULT_VECTOR_FILENAME = "model_assignment_result.json"
PUBLIC_ASSIGNMENT_RESULT_VECTOR_FILENAME = "public_assignment_result.json"
CHAT_PROBE_TRACE_VECTOR_FILENAME = "chat_probe_trace.json"
PROTOCOL_ROOT = Path(__file__).resolve().parents[2]
EVAL_VECTOR_DIR = PROTOCOL_ROOT / VECTORS_DIRECTORY_NAME / EVALUATION_DIRECTORY_NAME
PHASE1_REPORT_VECTOR_PATH = EVAL_VECTOR_DIR / PHASE1_REPORT_VECTOR_FILENAME
MODEL_CAMPAIGN_SPEC_VECTOR_PATH = EVAL_VECTOR_DIR / MODEL_CAMPAIGN_SPEC_VECTOR_FILENAME
MODEL_ASSIGNMENT_RESULT_VECTOR_PATH = EVAL_VECTOR_DIR / MODEL_ASSIGNMENT_RESULT_VECTOR_FILENAME
PUBLIC_ASSIGNMENT_RESULT_VECTOR_PATH = EVAL_VECTOR_DIR / PUBLIC_ASSIGNMENT_RESULT_VECTOR_FILENAME
CHAT_PROBE_TRACE_VECTOR_PATH = EVAL_VECTOR_DIR / CHAT_PROBE_TRACE_VECTOR_FILENAME
MODEL_CAMPAIGN_DESCRIPTOR_PATH = (
    PROTOCOL_ROOT / "descriptors" / EVALUATION_DIRECTORY_NAME / "v1" / "model_campaign.json"
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


def _assert_vector_round_trip(vector_path: Path, message):
    vector = json.loads(vector_path.read_text())
    encoded = vector["canonical_json"].encode()
    parsed = parse_canonical(encoded, message())
    assert serialize_canonical(parsed) == encoded
    return vector, parsed


def test_evaluation_campaign_spec_canonicalization_matches_cross_language_vector():
    vector, spec = _assert_vector_round_trip(MODEL_CAMPAIGN_SPEC_VECTOR_PATH, EvaluationCampaignSpec)
    assert vector["message_type"] == "EvaluationCampaignSpec"
    assert spec.campaign_id == "phase1a-smoke"
    assert spec.scenario_count == 25
    assert spec.model_registry[0].served_model_tag == "qwen3:4b"


def test_evaluation_assignment_result_canonicalization_matches_cross_language_vector():
    vector, result = _assert_vector_round_trip(
        MODEL_ASSIGNMENT_RESULT_VECTOR_PATH, EvaluationAssignmentResult
    )
    assert vector["message_type"] == "EvaluationAssignmentResult"
    assert result.lane == 2  # EVALUATION_LANE_MODEL_ROLE
    assert len(result.model_inferences) == 1
    assert result.model_inferences[0].agent_persona == "sage"


def test_chat_probe_trace_digest_matches_cross_language_vector():
    vector = json.loads(CHAT_PROBE_TRACE_VECTOR_PATH.read_text())
    assert vector["message_type"] == "ChatProbeTrace"
    got = compute_chat_probe_trace_digest(vector["trace"])
    assert got == vector["trace_digest"]


def test_public_assignment_result_projection_canonicalization_matches_cross_language_vector():
    vector, projection = _assert_vector_round_trip(
        PUBLIC_ASSIGNMENT_RESULT_VECTOR_PATH, PublicAssignmentResultProjection
    )
    assert vector["message_type"] == "PublicAssignmentResultProjection"
    assert projection.assignment_id == "assign-1"
    assert projection.verification_status == "verified"


def _live_message_field_names(message_descriptor):
    field_names = []
    oneof_groups = set()
    for oneof in message_descriptor.oneofs:
        if len(oneof.fields) == 1:
            field_names.append(oneof.fields[0].name)
        else:
            oneof_groups.add(oneof.name)
    for field in message_descriptor.fields:
        if field.containing_oneof is None:
            field_names.append(field.name)
    return sorted(field_names), sorted(oneof_groups)


def test_evaluation_model_campaign_descriptor_matches_protobuf():
    descriptor = json.loads(MODEL_CAMPAIGN_DESCRIPTOR_PATH.read_text())
    assert descriptor["schema_version"] == "1.0.0"
    assert descriptor["protobuf_package"] == "g8e.eval.v1"
    assert len(descriptor["canonical_vectors"]) == 4

    for enum_name, values in descriptor["enums"].items():
        live_enum = eval_pb2.DESCRIPTOR.enum_types_by_name[enum_name]
        live_values = sorted(value.name for value in live_enum.values)
        assert live_values == sorted(values)

    for message_name, message_descriptor in descriptor["messages"].items():
        live_message = eval_pb2.DESCRIPTOR.message_types_by_name[message_name]
        live_fields, live_oneofs = _live_message_field_names(live_message)
        assert sorted(message_descriptor["fields"]) == live_fields
        assert sorted(message_descriptor.get("oneof_groups", [])) == live_oneofs

        for prohibited in message_descriptor.get("prohibited_fields", []):
            assert prohibited not in {field.name for field in live_message.fields}


def test_public_assignment_projection_omits_private_evidence_fields():
    descriptor = PublicAssignmentResultProjection.DESCRIPTOR
    forbidden = {
        "prompt",
        "output",
        "thinking",
        "receipt",
        "transaction_id",
        "operator_session_id",
        "operator_id",
        "governed_receipt_ref",
        "model_inferences",
        "tool_calls",
        "governed_actions",
    }
    field_names = {field.name for field in descriptor.fields}
    assert forbidden.isdisjoint(field_names)
