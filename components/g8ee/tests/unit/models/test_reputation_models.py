# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Contract tests for the reputation Pydantic models.

Pins:
1. Round-trip stability — every field survives ``model_dump(mode="json")``.
2. Field-name parity with the canonical JSON schemas under
   `shared/models/`. The schemas group fields under organisational
   sub-objects (``core_identity``, ``reputation``, ``timestamps`` ...);
   the union of those groups is the flat field set on the Pydantic
   model. Drift in either direction fails the build.
"""

from __future__ import annotations

import json
from datetime import UTC, datetime
from pathlib import Path

import pytest

from app.models.reputation import (
    GENESIS_PREV_ROOT,
    ReputationCommitment,
    ReputationLeaf,
    ReputationState,
)

pytestmark = pytest.mark.unit


SHARED_MODELS_DIR = Path("/app/shared/models")


def _load_json(name: str) -> dict:
    with (SHARED_MODELS_DIR / name).open() as f:
        return json.load(f)


def _flatten_doc_fields(doc_root: dict) -> set[str]:
    """Walk the nested doc-style schema and return the union of leaf field names.

    Doc-style schemas (e.g. `reputation_state.json`) put real fields under
    organisational sub-objects. A "leaf" field is identified by the
    presence of a ``type`` key alongside the field's siblings under its
    parent group. Keys beginning with an underscore are documentation
    metadata and are ignored.
    """
    fields: set[str] = set()
    for group_name, group in doc_root.items():
        if group_name.startswith("_"):
            continue
        if not isinstance(group, dict):
            continue
        for field_name, field_def in group.items():
            if field_name.startswith("_"):
                continue
            if isinstance(field_def, dict) and "type" in field_def:
                fields.add(field_name)
    return fields


class TestReputationStateRoundTrip:
    def test_pydantic_roundtrip(self):
        s = ReputationState(
            agent_id="axiom",
            scalar=0.5,
            updated_at=datetime(2026, 4, 24, 12, 0, 0, tzinfo=UTC),
        )
        rebuilt = ReputationState.model_validate(s.model_dump(mode="json"))
        assert rebuilt == s

    def test_optional_fields_default_to_none(self):
        s = ReputationState(
            agent_id="axiom",
            scalar=0.5,
            updated_at=datetime(2026, 4, 24, 12, 0, 0, tzinfo=UTC),
        )
        # Pydantic exclude_none default keeps the wire payload lean.
        dumped = s.model_dump(mode="json")
        assert "unbonding_until" not in dumped
        assert "last_slash_tier" not in dumped

    def test_scalar_bounds_enforced(self):
        with pytest.raises(Exception):
            ReputationState(
                agent_id="axiom",
                scalar=1.5,
                updated_at=datetime(2026, 4, 24, 12, 0, 0, tzinfo=UTC),
            )
        with pytest.raises(Exception):
            ReputationState(
                agent_id="axiom",
                scalar=-0.1,
                updated_at=datetime(2026, 4, 24, 12, 0, 0, tzinfo=UTC),
            )


class TestReputationCommitmentRoundTrip:
    def test_pydantic_roundtrip(self):
        c = ReputationCommitment(
            investigation_id="inv-1",
            tribunal_command_id="tc-1",
            merkle_root="a" * 64,
            prev_root=GENESIS_PREV_ROOT,
            leaves_count=8,
            signature="b" * 64,
        )
        rebuilt = ReputationCommitment.model_validate(c.model_dump(mode="json"))
        assert rebuilt == c
        assert rebuilt.signed_by == "auditor"  # default

    def test_root_lengths_enforced(self):
        with pytest.raises(Exception):
            ReputationCommitment(
                investigation_id="inv-1",
                tribunal_command_id="tc-1",
                merkle_root="a" * 32,  # too short
                prev_root=GENESIS_PREV_ROOT,
                leaves_count=8,
                signature="b" * 64,
            )


class TestReputationLeaf:
    def test_pydantic_roundtrip(self):
        leaf = ReputationLeaf(agent_id="axiom", scalar=0.5)
        assert ReputationLeaf.model_validate(leaf.model_dump(mode="json")) == leaf


class TestSharedJsonAlignment:
    """Field-name parity between Pydantic models and shared/models/*.json."""

    def test_reputation_state_fields_match_shared_json(self):
        schema = _load_json("reputation_state.json")["reputation_state"]
        json_fields = _flatten_doc_fields(schema)
        py_fields = set(ReputationState.model_fields.keys())

        missing_in_py = json_fields - py_fields
        missing_in_json = py_fields - json_fields
        assert not missing_in_py, (
            f"shared/models/reputation_state.json declares fields not on "
            f"ReputationState: {missing_in_py}"
        )
        assert not missing_in_json, (
            f"ReputationState declares fields not in "
            f"shared/models/reputation_state.json: {missing_in_json}"
        )

    def test_reputation_commitment_fields_match_shared_json(self):
        schema = _load_json("reputation_commitment.json")["reputation_commitment"]
        json_fields = _flatten_doc_fields(schema)
        py_fields = set(ReputationCommitment.model_fields.keys())
        # `created_at` / `updated_at` come from G8eIdentifiableModel and
        # only `created_at` is documented in the JSON schema; `updated_at`
        # is intentionally never written for commitments. Drop it from the
        # comparison so the alignment test reflects the actual contract.
        py_fields.discard("updated_at")

        missing_in_py = json_fields - py_fields
        missing_in_json = py_fields - json_fields
        assert not missing_in_py, (
            f"shared/models/reputation_commitment.json declares fields not on "
            f"ReputationCommitment: {missing_in_py}"
        )
        assert not missing_in_json, (
            f"ReputationCommitment declares fields not in "
            f"shared/models/reputation_commitment.json: {missing_in_json}"
        )
