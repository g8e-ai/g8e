# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from dataclasses import dataclass
from enum import StrEnum

import pytest
from pydantic import BaseModel

from app.llm.llm_dataclasses import Content, Part, Role, Schema, ToolDeclaration, ToolGroup, Type
from app.llm.llm_types import PrimaryLLMSettings
from app.llm.model_evidence import model_boundary_hash, model_boundary_privacy_attestation

pytestmark = [pytest.mark.unit]


class EvidenceKind(StrEnum):
    REQUEST = "request"


@dataclass
class EvidenceEnvelope:
    kind: EvidenceKind
    payload: bytes


class EvidenceModel(BaseModel):
    kind: EvidenceKind
    value: str


def test_model_boundary_hash_is_stable_across_dictionary_ordering():
    first = {"model": "test-model", "settings": {"temperature": 0, "top_p": 1}}
    second = {"settings": {"top_p": 1, "temperature": 0}, "model": "test-model"}

    assert model_boundary_hash(first) == model_boundary_hash(second)


def test_model_boundary_hash_canonicalizes_dataclasses_enums_and_bytes():
    typed = EvidenceEnvelope(kind=EvidenceKind.REQUEST, payload=b"\x00\xff")
    equivalent = {"kind": "request", "payload": {"base64": "AP8="}}

    assert model_boundary_hash(typed) == model_boundary_hash(equivalent)


def test_model_boundary_hash_canonicalizes_pydantic_models():
    typed = EvidenceModel(kind=EvidenceKind.REQUEST, value="payload")
    equivalent = {"kind": "request", "value": "payload"}

    assert model_boundary_hash(typed) == model_boundary_hash(equivalent)


def test_model_boundary_hash_canonicalizes_tool_schemas():
    first_schema = Schema(
        type=Type.OBJECT,
        properties={
            "path": Schema(type=Type.STRING, description="Path to inspect"),
            "depth": Schema(type=Type.INTEGER),
        },
        required=["path"],
    )
    second_schema = Schema(
        type=Type.OBJECT,
        properties={
            "depth": Schema(type=Type.INTEGER),
            "path": Schema(type=Type.STRING, description="Path to inspect"),
        },
        required=["path"],
    )
    first = PrimaryLLMSettings(
        system_instructions="Inspect the target",
        tools=[ToolGroup(tools=[ToolDeclaration(name="inspect_path", description="Inspect a path", parameters=first_schema)])],
    )
    second = PrimaryLLMSettings(
        system_instructions="Inspect the target",
        tools=[ToolGroup(tools=[ToolDeclaration(name="inspect_path", description="Inspect a path", parameters=second_schema)])],
    )

    assert model_boundary_hash(first) == model_boundary_hash(second)


def test_model_boundary_hash_matches_equivalent_typed_requests():
    first = {
        "model": "test-model",
        "contents": [Content(role=Role.USER, parts=[Part.from_text("same prompt")])],
        "settings": PrimaryLLMSettings(max_output_tokens=128),
    }
    second = {
        "settings": PrimaryLLMSettings(max_output_tokens=128),
        "contents": [Content(role="user", parts=[Part(text="same prompt")])],
        "model": "test-model",
    }

    assert model_boundary_hash(first) == model_boundary_hash(second)


def test_model_boundary_hash_returns_only_digest_not_plaintext():
    plaintext = "sensitive prompt value"
    digest = model_boundary_hash({"prompt": plaintext})

    assert len(digest) == 64
    assert digest != plaintext
    assert plaintext not in digest


def test_model_boundary_privacy_attestation_counts_sensitive_values_without_retaining_them():
    canary = "canary@example.com"
    payload = {"messages": [{"role": "user", "content": f"Contact {canary}"}]}

    attestation = model_boundary_privacy_attestation(payload)

    assert attestation.scanner_version == "sentinel-regex@1.0.0"
    assert attestation.input_artifact_hash == model_boundary_hash(payload)
    assert attestation.raw_sensitive_occurrences == 1
    assert attestation.raw_sensitive_types == ["email"]
    assert canary not in attestation.model_dump_json()
