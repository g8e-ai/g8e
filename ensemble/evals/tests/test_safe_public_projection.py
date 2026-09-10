# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for safe public projection.

Verifies that public projections contain no raw prompts, outputs, keys,
credentials, private endpoints, machine-specific paths, private download
locations, encrypted artifact locations, or fields outside the
explicit schema allowlist. The projection function fails closed on any
prohibited field.
"""

# pyright: reportCallIssue=false

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.projection import (
    PublicProjection,
    PublicProjectionError,
    project_to_public,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _make_private_record(
    *,
    campaign_id: str = "campaign-1",
    campaign_revision: str = "1",
    variant_id: str = "qwen3-8b-q4_0",
    task_id: str = "task-1",
    metric_id: str = "ifeval_subset_verifier",
    numerator: int = 8,
    denominator: int = 10,
    rate: float = 0.8,
    unit: str = "boolean",
    verification_status: str = "verified",
    evidence_link: str = "proofs/run-1/analysis.json",
    raw_prompt: str | None = None,
    raw_output: str | None = None,
    api_key: str | None = None,
    private_endpoint: str | None = None,
    machine_path: str | None = None,
    evidence_key: str | None = None,
    extra_field: str | None = None,
) -> dict:
    record = {
        "campaign_id": campaign_id,
        "campaign_revision": campaign_revision,
        "variant_id": variant_id,
        "task_id": task_id,
        "metric_id": metric_id,
        "numerator": numerator,
        "denominator": denominator,
        "rate": rate,
        "unit": unit,
        "verification_status": verification_status,
        "evidence_link": evidence_link,
    }
    if raw_prompt is not None:
        record["raw_prompt"] = raw_prompt
    if raw_output is not None:
        record["raw_output"] = raw_output
    if api_key is not None:
        record["api_key"] = api_key
    if private_endpoint is not None:
        record["private_endpoint"] = private_endpoint
    if machine_path is not None:
        record["machine_path"] = machine_path
    if evidence_key is not None:
        record["evidence_key"] = evidence_key
    if extra_field is not None:
        record["extra_field"] = extra_field
    return record


class TestSafePublicProjection:
    def test_clean_record_projects_successfully(self):
        """A record with only allowlisted fields projects to a public projection."""
        record = _make_private_record()
        projection = project_to_public(record)
        assert projection.campaign_id == "campaign-1"
        assert projection.variant_id == "qwen3-8b-q4_0"
        assert projection.numerator == 8
        assert projection.denominator == 10
        assert projection.rate == pytest.approx(0.8)

    def test_raw_prompt_rejected(self):
        """A record containing raw_prompt is rejected."""
        record = _make_private_record(raw_prompt="Write a poem about cats.")
        with pytest.raises(PublicProjectionError, match="raw_prompt"):
            project_to_public(record)

    def test_raw_output_rejected(self):
        """A record containing raw_output is rejected."""
        record = _make_private_record(raw_output="Here is a poem about cats...")
        with pytest.raises(PublicProjectionError, match="raw_output"):
            project_to_public(record)

    def test_api_key_rejected(self):
        """A record containing an api_key is rejected."""
        record = _make_private_record(api_key="sk-abc123secret")
        with pytest.raises(PublicProjectionError, match="api_key"):
            project_to_public(record)

    def test_private_endpoint_rejected(self):
        """A record containing a private_endpoint is rejected."""
        record = _make_private_record(private_endpoint="http://192.168.1.2:11434")
        with pytest.raises(PublicProjectionError, match="private_endpoint"):
            project_to_public(record)

    def test_machine_path_rejected(self):
        """A record containing a machine-specific path is rejected."""
        record = _make_private_record(machine_path="/home/bob/.g8e/evidence/keys")
        with pytest.raises(PublicProjectionError, match="machine_path"):
            project_to_public(record)

    def test_evidence_key_rejected(self):
        """A record containing an evidence_key is rejected."""
        record = _make_private_record(evidence_key="enc-key-uuid-123")
        with pytest.raises(PublicProjectionError, match="evidence_key"):
            project_to_public(record)

    def test_unknown_field_rejected(self):
        """A record containing a field outside the allowlist is rejected."""
        record = _make_private_record(extra_field="something")
        with pytest.raises(PublicProjectionError, match="extra_field"):
            project_to_public(record)


class TestPublicProjectionModel:
    def test_round_trip_preserves_all_fields(self):
        record = _make_private_record()
        projection = project_to_public(record)
        restored = PublicProjection.model_validate_json(projection.model_dump_json())
        assert restored == projection

    def test_rejects_unknown_fields(self):
        data = project_to_public(_make_private_record()).model_dump()
        data["extra_field"] = "bad"
        with pytest.raises(ValidationError):
            PublicProjection(**data)

    def test_frozen_model(self):
        projection = project_to_public(_make_private_record())
        with pytest.raises(ValidationError):
            projection.campaign_id = "changed"  # type: ignore[misc]

    def test_rejects_non_finite_rate(self):
        """Non-finite rate values are rejected."""
        record = _make_private_record(rate=float("nan"))
        with pytest.raises(PublicProjectionError, match="non-finite"):
            project_to_public(record)

    def test_evidence_link_must_not_contain_traversal(self):
        """Evidence links with path traversal are rejected."""
        record = _make_private_record(evidence_link="../../etc/passwd")
        with pytest.raises(PublicProjectionError, match="traversal"):
            project_to_public(record)


class TestProjectionAllowlist:
    def test_allowed_fields_are_preserved(self):
        """Every field in the allowlist is preserved in the projection."""
        record = _make_private_record()
        projection = project_to_public(record)
        assert projection.campaign_id == "campaign-1"
        assert projection.campaign_revision == "1"
        assert projection.variant_id == "qwen3-8b-q4_0"
        assert projection.task_id == "task-1"
        assert projection.metric_id == "ifeval_subset_verifier"
        assert projection.numerator == 8
        assert projection.denominator == 10
        assert projection.rate == pytest.approx(0.8)
        assert projection.unit == "boolean"
        assert projection.verification_status == "verified"
        assert projection.evidence_link == "proofs/run-1/analysis.json"

    def test_no_secret_fields_in_projection(self):
        """The PublicProjection model has no fields for secrets or private data."""
        field_names = set(PublicProjection.model_fields.keys())
        prohibited = {"raw_prompt", "raw_output", "api_key", "private_endpoint",
                      "machine_path", "evidence_key", "credentials", "private_key"}
        assert not (field_names & prohibited)
